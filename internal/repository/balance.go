package repository

import (
	"context"

	"github.com/selis18/gomart_dipl/internal/model"
)

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
