package accrual

import (
	"context"
	"errors"
	"time"
)

type OrderStorage interface {
	PendingOrders(context.Context) ([]string, error)
	UpdateAccrual(context.Context, Order) error
}

type OrderClient interface {
	GetOrder(context.Context, string) (Order, error)
}

type Worker struct {
	storage OrderStorage
	client  OrderClient
	report  func(error)
}

func NewWorker(storage OrderStorage, client OrderClient, report func(error)) *Worker {
	return &Worker{storage: storage, client: client, report: report}
}

func (w *Worker) Run(ctx context.Context) {
	for ctx.Err() == nil {
		delay := w.poll(ctx)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (w *Worker) poll(ctx context.Context) time.Duration {
	orders, err := w.storage.PendingOrders(ctx)
	if err != nil {
		w.log(err)
		return time.Second
	}
	for _, number := range orders {
		if ctx.Err() != nil {
			break
		}
		order, err := w.client.GetOrder(ctx, number)
		var limited *RateLimitError
		switch {
		case errors.As(err, &limited):
			return max(limited.RetryAfter, time.Second)
		case errors.Is(err, ErrNotRegistered):
			continue
		case err != nil:
			w.log(err)
			continue
		}
		if err := w.storage.UpdateAccrual(ctx, order); err != nil {
			w.log(err)
		}
	}
	return time.Second
}

func (w *Worker) log(err error) {
	if w.report != nil {
		w.report(err)
	}
}
