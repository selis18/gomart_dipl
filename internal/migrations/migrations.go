package migrations

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)

var createTablesSQL string

func Apply(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, createTablesSQL); err != nil {
		return fmt.Errorf("create database tables: %w", err)
	}
	return nil
}
