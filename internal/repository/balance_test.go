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
