package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/selis18/gomart_dipl/internal/model"
)

var ErrInsufficientFunds = errors.New("insufficient funds")
var ErrWithdrawalConflict = errors.New("withdrawal already exists with different details")

func (s *StorageRepo) Withdraw(ctx context.Context, userID, order, sum string) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&id); err != nil {
		return err
	}
	var matches bool
	err = tx.QueryRowContext(ctx, `SELECT user_id = $2 AND sum = $3::numeric FROM withdrawals WHERE order_number = $1`, order, userID, sum).Scan(&matches)
	if err == nil {
		if !matches {
			return ErrWithdrawalConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var enough bool
	err = tx.QueryRowContext(ctx, `SELECT
		(SELECT COALESCE(SUM(accrual), 0) FROM orders WHERE user_id = $1 AND status = 'PROCESSED') -
		(SELECT COALESCE(SUM(sum), 0) FROM withdrawals WHERE user_id = $1) >= $2::numeric`, userID, sum).Scan(&enough)
	if err != nil {
		return err
	}
	if !enough {
		return ErrInsufficientFunds
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO withdrawals (order_number, user_id, sum)
		VALUES ($1, $2, $3::numeric) ON CONFLICT (order_number) DO NOTHING`, order, userID, sum)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrWithdrawalConflict
	}
	return tx.Commit()
}

func (s *StorageRepo) GetBalance(ctx context.Context, userID string) (model.Balance, error) {
	var balance model.Balance
	err := s.db.QueryRowContext(ctx, `SELECT earned.total - spent.total, spent.total
 FROM (SELECT COALESCE(SUM(accrual), 0) AS total FROM orders
       WHERE user_id = $1 AND status = 'PROCESSED') AS earned
 CROSS JOIN (SELECT COALESCE(SUM(sum), 0) AS total FROM withdrawals
             WHERE user_id = $1) AS spent`, userID).Scan(&balance.Current, &balance.Withdrawn)
	if err != nil {
		return model.Balance{}, err
	}
	return balance, nil
}
