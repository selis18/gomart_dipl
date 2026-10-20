package repository

import (
	"context"
	"github.com/selis18/gomart_dipl/internal/accrual"
	"github.com/selis18/gomart_dipl/internal/model"
)

func (s *StorageRepo) PendingOrders(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT number FROM orders WHERE status IN ('NEW', 'PROCESSING') ORDER BY uploaded_at, number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var numbers []string
	for rows.Next() {
		var number string
		if err := rows.Scan(&number); err != nil {
			return nil, err
		}
		numbers = append(numbers, number)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return numbers, nil
}

func (s *StorageRepo) UpdateAccrual(ctx context.Context, order accrual.Order) error {
	status := order.Status
	if status == "REGISTERED" {
		status = "NEW"
	}
	var amount *float64
	if status == "PROCESSED" {
		amount = order.Accrual
	}
	_, err := s.db.ExecContext(ctx, `UPDATE orders SET status = $2, accrual = $3 WHERE number = $1 AND status IN ('NEW', 'PROCESSING')`, order.Number, status, amount)
	return err
}

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
