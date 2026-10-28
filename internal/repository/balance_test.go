package repository

import (
	"errors"
	"testing"

	"github.com/selis18/gomart_dipl/internal/accrual"
	"github.com/selis18/gomart_dipl/internal/model"
)

func TestBalanceAndWithdrawalsPostgreSQL(t *testing.T) {
	s, ctx, alice, bob := integrationStorage(t)
	balance, err := s.GetBalance(ctx, alice)
	if err != nil || balance != (model.Balance{}) {
		t.Fatalf("empty balance=%+v err=%v", balance, err)
	}
	if err := s.AddOrder(ctx, alice, "18"); err != nil {
		t.Fatal(err)
	}
	amount := 500.5
	if err := s.UpdateAccrual(ctx, accrual.Order{Number: "18", Status: "PROCESSED", Accrual: &amount}); err != nil {
		t.Fatal(err)
	}
	if err := s.Withdraw(ctx, alice, "0018", "0.10"); err != nil {
		t.Fatal(err)
	}
	if err := s.Withdraw(ctx, alice, "0018", "0.10"); err != nil {
		t.Fatalf("repeat withdrawal: %v", err)
	}
	if err := s.Withdraw(ctx, alice, "0018", "0.20"); !errors.Is(err, ErrWithdrawalConflict) {
		t.Fatalf("different amount: %v", err)
	}
	if err := s.Withdraw(ctx, bob, "0018", "0.10"); !errors.Is(err, ErrWithdrawalConflict) {
		t.Fatalf("different owner: %v", err)
	}
	if err := s.Withdraw(ctx, alice, "123", "500.41"); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("insufficient balance: %v", err)
	}
	balance, err = s.GetBalance(ctx, alice)
	if err != nil || balance != (model.Balance{Current: 500.4, Withdrawn: 0.1}) {
		t.Fatalf("balance=%+v err=%v", balance, err)
	}
	balance, err = s.GetBalance(ctx, bob)
	if err != nil || balance != (model.Balance{}) {
		t.Fatalf("other user balance=%+v err=%v", balance, err)
	}
	withdrawals, err := s.GetWithdrawals(ctx, alice)
	if err != nil || len(withdrawals) != 1 {
		t.Fatalf("withdrawals=%+v err=%v", withdrawals, err)
	}
	if withdrawals[0].Order != "0018" || withdrawals[0].Sum != 0.1 || withdrawals[0].ProcessedAt.IsZero() {
		t.Fatalf("withdrawal=%+v", withdrawals[0])
	}
	withdrawals, err = s.GetWithdrawals(ctx, bob)
	if err != nil || len(withdrawals) != 0 {
		t.Fatalf("other user withdrawals=%+v err=%v", withdrawals, err)
	}
}
