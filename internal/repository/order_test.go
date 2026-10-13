package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
)

type orderConnector struct{ conn *orderConn }

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
