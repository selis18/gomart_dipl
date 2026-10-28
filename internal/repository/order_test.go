package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selis18/gomart_dipl/internal/accrual"
	"github.com/selis18/gomart_dipl/internal/migrations"
)

func integrationStorage(t *testing.T) (*StorageRepo, context.Context, string, string) {
	t.Helper()
	uri := os.Getenv("TEST_DATABASE_URI")
	if uri == "" {
		t.Skip("set TEST_DATABASE_URI to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := pgxpool.New(ctx, uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("repository_test_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("clean test schema: %v", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(uri)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	storage := NewStorageRepo(pool)
	alice, err := storage.RegisterUser(ctx, "alice", "hash")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := storage.RegisterUser(ctx, "bob", "hash")
	if err != nil {
		t.Fatal(err)
	}
	return storage, ctx, alice, bob
}

func TestOrdersPostgreSQL(t *testing.T) {
	s, ctx, alice, bob := integrationStorage(t)
	if err := s.AddOrder(ctx, alice, "0018"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddOrder(ctx, alice, "0018"); !errors.Is(err, ErrOrderExists) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := s.AddOrder(ctx, bob, "0018"); !errors.Is(err, ErrOrderConflict) {
		t.Fatalf("other owner: %v", err)
	}
	orders, err := s.GetOrdersByUserID(ctx, alice)
	if err != nil || len(orders) != 1 {
		t.Fatalf("orders=%+v err=%v", orders, err)
	}
	if orders[0].Number != "0018" || orders[0].Status != "NEW" || orders[0].Accrual != nil {
		t.Fatalf("new order=%+v", orders[0])
	}
	otherOrders, err := s.GetOrdersByUserID(ctx, bob)
	if err != nil || len(otherOrders) != 0 {
		t.Fatalf("other user orders=%+v err=%v", otherOrders, err)
	}
	pending, err := s.PendingOrders(ctx)
	if err != nil || len(pending) != 1 || pending[0] != "0018" {
		t.Fatalf("pending=%v err=%v", pending, err)
	}
	amount := 500.5
	if err := s.UpdateAccrual(ctx, accrual.Order{Number: "0018", Status: "PROCESSED", Accrual: &amount}); err != nil {
		t.Fatal(err)
	}
	orders, err = s.GetOrdersByUserID(ctx, alice)
	if err != nil || len(orders) != 1 || orders[0].Accrual == nil || *orders[0].Accrual != amount || orders[0].Status != "PROCESSED" {
		t.Fatalf("processed orders=%+v err=%v", orders, err)
	}
	pending, err = s.PendingOrders(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("processed order still pending: %v err=%v", pending, err)
	}
}
