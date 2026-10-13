package repository

import (
	"context"
	"github.com/selis18/gomart_dipl/internal/model"
)

func (s *StorageRepo) GetOrdersByUserID(ctx context.Context, userID string) ([]model.Order, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT number, status, accrual, uploaded_at
 FROM orders WHERE user_id = $1 ORDER BY uploaded_at DESC, number DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var orders []model.Order
	for rows.Next() {
		var order model.Order
		if err := rows.Scan(&order.Number, &order.Status, &order.Accrual, &order.UploadedAt); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return orders, nil
}
