package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/selis18/gomart_dipl/internal/model"
)

type balanceConn struct {
	orderConn
	values []driver.Value
	err    error
}

func (c *balanceConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if ctx.Value(orderContextKey{}) != "request" {
		c.t.Error("missing request context")
	}
	if len(args) != 1 || args[0].Value != "alice" {
		c.t.Errorf("wrong user: %v", args)
	}
	for _, clause := range []string{"earned.total - spent.total", "spent.total", "COALESCE(SUM(accrual), 0)", "status = 'PROCESSED'", "COALESCE(SUM(sum), 0)", "FROM withdrawals"} {
		if !strings.Contains(query, clause) {
			c.t.Errorf("missing balance clause %q", clause)
		}
	}
	if strings.Count(query, "user_id = $1") != 2 {
		c.t.Error("both totals must be scoped to user")
	}
	if c.err != nil {
		return nil, c.err
	}
	return &balanceRows{values: c.values}, nil
}

type balanceRows struct {
	values []driver.Value
	done   bool
}

func (*balanceRows) Columns() []string { return []string{"current", "withdrawn"} }
func (*balanceRows) Close() error      { return nil }
func (r *balanceRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	copy(dest, r.values)
	return nil
}
func TestGetBalance(t *testing.T) {
	dbErr := errors.New("database failure")
	for _, tt := range []struct {
		name    string
		values  []driver.Value
		err     error
		want    model.Balance
		wantErr bool
	}{
		{"decimal totals", []driver.Value{"500.5", "42"}, nil, model.Balance{Current: 500.5, Withdrawn: 42}, false},
		{"empty totals", []driver.Value{"0", "0"}, nil, model.Balance{}, false},
		{"all spent", []driver.Value{"0", "42.5"}, nil, model.Balance{Withdrawn: 42.5}, false},
		{"query failure", nil, dbErr, model.Balance{}, true},
		{"scan failure", []driver.Value{"500.5", "bad"}, nil, model.Balance{}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			conn := &balanceConn{orderConn: orderConn{t: t}, values: tt.values, err: tt.err}
			db := sql.OpenDB(orderConnector{conn: conn})
			defer db.Close()
			got, err := NewStorageRepo(db).GetBalance(context.WithValue(context.Background(), orderContextKey{}, "request"), "alice")
			if (err != nil) != tt.wantErr {
				t.Fatalf("error=%v", err)
			}
			if got != tt.want {
				t.Errorf("balance=%+v, want %+v", got, tt.want)
			}
			if tt.err != nil && !errors.Is(err, tt.err) {
				t.Errorf("error not propagated: %v", err)
			}
		})
	}
}

type withdrawalConn struct {
	orderConn
	step                  int
	failAt                int
	existing              string
	enough                bool
	inserted              int64
	committed, rolledBack bool
	commitErr             error
}

func (c *withdrawalConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if opts.Isolation != driver.IsolationLevel(sql.LevelReadCommitted) {
		c.t.Error("expected read committed isolation")
	}
	if c.failAt == -1 {
		return nil, errors.New("begin failure")
	}
	return c, nil
}
func (c *withdrawalConn) Commit() error   { c.committed = true; return c.commitErr }
func (c *withdrawalConn) Rollback() error { c.rolledBack = true; return nil }
func (c *withdrawalConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	c.step++
	if ctx.Value(orderContextKey{}) != "request" {
		c.t.Error("context missing")
	}
	if c.step == c.failAt {
		return nil, errors.New("query failure")
	}
	switch c.step {
	case 1:
		if !strings.Contains(q, "FOR UPDATE") || args[0].Value != "alice" {
			c.t.Error("user must be locked before reading balance")
		}
		return &orderRows{value: "alice"}, nil
	case 2:
		if args[0].Value != "0018" || args[1].Value != "alice" || args[2].Value != "0.10" {
			c.t.Error("wrong duplicate lookup")
		}
		return &orderRows{value: c.existing}, nil
	case 3:
		if !strings.Contains(q, "status = 'PROCESSED'") || strings.Count(q, "user_id = $1") != 2 || !strings.Contains(q, ">= $2::numeric") {
			c.t.Error("balance must use exact arithmetic and user scope")
		}
		if c.enough {
			return &orderRows{value: "true"}, nil
		}
		return &orderRows{value: "false"}, nil
	}
	return nil, errors.New("unexpected query")
}
func (c *withdrawalConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	c.step++
	if c.step == c.failAt {
		return nil, errors.New("insert failure")
	}
	if c.step != 4 || !strings.Contains(q, "ON CONFLICT (order_number) DO NOTHING") || args[0].Value != "0018" || args[1].Value != "alice" || args[2].Value != "0.10" {
		c.t.Error("unexpected insert")
	}
	return driver.RowsAffected(c.inserted), nil
}
func TestWithdraw(t *testing.T) {
	for _, tt := range []struct {
		name, existing   string
		enough           bool
		inserted         int64
		failAt           int
		want             error
		commit, rollback bool
	}{
		{"success", "", true, 1, 0, nil, true, false},
		{"insufficient", "", false, 0, 0, ErrInsufficientFunds, false, true},
		{"repeat", "true", false, 0, 0, nil, true, false},
		{"different details", "false", true, 0, 0, ErrWithdrawalConflict, false, true},
		{"concurrent order conflict", "", true, 0, 0, ErrWithdrawalConflict, false, true},
		{"begin failure", "", true, 1, -1, nil, false, false},
		{"lock failure", "", true, 1, 1, nil, false, true},
		{"lookup failure", "", true, 1, 2, nil, false, true},
		{"balance failure", "", true, 1, 3, nil, false, true},
		{"insert failure", "", true, 1, 4, nil, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			conn := &withdrawalConn{orderConn: orderConn{t: t}, existing: tt.existing, enough: tt.enough, inserted: tt.inserted, failAt: tt.failAt}
			db := sql.OpenDB(orderConnector{conn: conn})
			defer db.Close()
			err := NewStorageRepo(db).Withdraw(context.WithValue(context.Background(), orderContextKey{}, "request"), "alice", "0018", "0.10")
			if tt.failAt != 0 {
				if err == nil {
					t.Fatal("expected database failure")
				}
			} else if !errors.Is(err, tt.want) {
				t.Errorf("error=%v want %v", err, tt.want)
			}
			if conn.committed != tt.commit || conn.rolledBack != tt.rollback {
				t.Errorf("commit=%v rollback=%v", conn.committed, conn.rolledBack)
			}
		})
	}
	t.Run("commit failure", func(t *testing.T) {
		failure := errors.New("commit failure")
		conn := &withdrawalConn{orderConn: orderConn{t: t}, enough: true, inserted: 1, commitErr: failure}
		db := sql.OpenDB(orderConnector{conn: conn})
		defer db.Close()
		err := NewStorageRepo(db).Withdraw(context.WithValue(context.Background(), orderContextKey{}, "request"), "alice", "0018", "0.10")
		if !errors.Is(err, failure) {
			t.Errorf("error=%v", err)
		}
	})
}
