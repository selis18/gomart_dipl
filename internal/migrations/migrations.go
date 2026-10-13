package migrations

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed 000001_create_tables.up.sql
var createTablesSQL string

//go:embed 000002_order_accrual.up.sql
var orderAccrualSQL string

//go:embed 000003_withdrawals.up.sql
var withdrawalsSQL string

func Apply(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, createTablesSQL); err != nil {
		return fmt.Errorf("create database tables: %w", err)
	}
	if _, err := db.ExecContext(ctx, orderAccrualSQL); err != nil {
		return fmt.Errorf("add order accrual: %w", err)
	}
	if _, err := db.ExecContext(ctx, withdrawalsSQL); err != nil {
		return fmt.Errorf("create withdrawals: %w", err)
	}
	return nil
}
