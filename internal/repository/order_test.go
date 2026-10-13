package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type orderConnector struct{ conn driver.Conn }

func (c orderConnector) Connect(context.Context) (driver.Conn, error) { return c.conn, nil }
func (c orderConnector) Driver() driver.Driver                        { return orderDriver{} }

type orderDriver struct{}

func (orderDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }

type orderConn struct {
	t                    *testing.T
	owner                string
	insertErr, lookupErr error
	calls                int
}

func (c *orderConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *orderConn) Close() error              { return nil }
func (c *orderConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }
func (c *orderConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.calls++
	if ctx.Value(orderContextKey{}) != "request" {
		c.t.Error("context not propagated")
	}
	if len(args) == 0 || args[0].Value != "0018" {
		c.t.Errorf("unexpected number arguments: %v", args)
	}
	if c.calls == 1 {
		if !strings.Contains(query, "ON CONFLICT (number) DO NOTHING") || !strings.Contains(query, "RETURNING number") {
			c.t.Errorf("insert must preserve the existing owner: %s", query)
		}
		if len(args) != 2 || args[1].Value != "alice" {
			c.t.Errorf("unexpected owner arguments: %v", args)
		}
		if c.insertErr != nil {
			return nil, c.insertErr
		}
		if c.owner != "" {
			return &orderRows{}, nil
		}
		return &orderRows{value: "0018"}, nil
	}
	if !strings.Contains(query, "SELECT user_id FROM orders WHERE number = $1") || len(args) != 1 {
		c.t.Errorf("unexpected lookup: %s %v", query, args)
	}
	if c.lookupErr != nil {
		return nil, c.lookupErr
	}
	return &orderRows{value: c.owner}, nil
}

type orderRows struct {
	value string
	done  bool
}

func (*orderRows) Columns() []string { return []string{"value"} }
func (*orderRows) Close() error      { return nil }
func (r *orderRows) Next(dest []driver.Value) error {
	if r.done || r.value == "" {
		return io.EOF
	}
	r.done = true
	dest[0] = r.value
	return nil
}

type orderContextKey struct{}

func TestAddOrder(t *testing.T) {
	dbErr := errors.New("database failure")
	for _, tt := range []struct {
		name, owner                string
		insertErr, lookupErr, want error
		calls                      int
	}{
		{"new", "", nil, nil, nil, 1},
		{"same owner", "alice", nil, nil, ErrOrderExists, 2},
		{"different owner", "bob", nil, nil, ErrOrderConflict, 2},
		{"insert failure", "", dbErr, nil, dbErr, 1},
		{"lookup failure", "alice", nil, dbErr, dbErr, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			conn := &orderConn{t: t, owner: tt.owner, insertErr: tt.insertErr, lookupErr: tt.lookupErr}
			db := sql.OpenDB(orderConnector{conn})
			defer db.Close()
			ctx := context.WithValue(context.Background(), orderContextKey{}, "request")
			err := NewStorageRepo(db).AddOrder(ctx, "alice", "0018")
			if !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
			if conn.calls != tt.calls {
				t.Errorf("queries = %d, want %d", conn.calls, tt.calls)
			}
		})
	}
}

type listConn struct {
	orderConn
	rows *listRows
	err  error
}

func (c *listConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if ctx.Value(orderContextKey{}) != "request" {
		c.t.Error("missing request context")
	}
	if len(args) != 1 || args[0].Value != "alice" {
		c.t.Errorf("wrong user filter: %v", args)
	}
	if !strings.Contains(query, "WHERE user_id = $1") || !strings.Contains(query, "ORDER BY uploaded_at DESC") {
		c.t.Errorf("missing user filter or newest-first sorting: %s", query)
	}
	return c.rows, c.err
}

type listRows struct {
	values [][]driver.Value
	index  int
	err    error
	closed bool
}

func (*listRows) Columns() []string { return []string{"number", "status", "accrual", "uploaded_at"} }
func (r *listRows) Close() error    { r.closed = true; return nil }
func (r *listRows) Next(dest []driver.Value) error {
	if r.index == len(r.values) {
		if r.err != nil {
			return r.err
		}
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
func TestGetOrdersByUserID(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	dbErr := errors.New("database failure")
	values := [][]driver.Value{{"0018", "PROCESSED", "500.5", now}, {"12345678903", "NEW", nil, now.Add(-time.Hour)}, {"9278923470", "PROCESSED", "0", now.Add(-2 * time.Hour)}}
	for _, tt := range []struct {
		name              string
		values            [][]driver.Value
		queryErr, rowsErr error
		wantErr           bool
	}{
		{"orders", values, nil, nil, false},
		{"empty", nil, nil, nil, false},
		{"query failure", nil, dbErr, nil, true},
		{"iteration failure", values, nil, dbErr, true},
		{"scan failure", [][]driver.Value{{"18", "NEW", "invalid amount", now}}, nil, nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rows := &listRows{values: tt.values, err: tt.rowsErr}
			conn := &listConn{orderConn: orderConn{t: t}, rows: rows, err: tt.queryErr}
			db := sql.OpenDB(orderConnector{conn: conn})
			defer db.Close()
			ctx := context.WithValue(context.Background(), orderContextKey{}, "request")
			got, err := NewStorageRepo(db).GetOrdersByUserID(ctx, "alice")
			if (err != nil) != tt.wantErr {
				t.Fatalf("error=%v", err)
			}
			if tt.queryErr == nil && !rows.closed {
				t.Error("rows were not closed")
			}
			if tt.wantErr {
				if got != nil {
					t.Error("partial results returned on error")
				}
				return
			}
			if len(got) != len(tt.values) {
				t.Fatalf("got %d orders", len(got))
			}
			if len(got) == 0 {
				return
			}
			if got[0].Number != "0018" || got[0].Status != "PROCESSED" || !got[0].UploadedAt.Equal(now) || got[0].Accrual == nil || *got[0].Accrual != 500.5 {
				t.Errorf("unexpected order: %+v", got[0])
			}
			if got[1].Accrual != nil {
				t.Error("NULL accrual must remain absent")
			}
			if got[2].Accrual == nil || *got[2].Accrual != 0 {
				t.Error("zero accrual lost")
			}
		})
	}
}
