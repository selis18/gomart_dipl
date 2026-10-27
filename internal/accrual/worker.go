package accrual

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

	batchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	const poolSize = 4
	jobs := make(chan string)
	type result struct {
		order Order
		err   error
	}
	results := make(chan result)
	var wg sync.WaitGroup
	for range poolSize {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for number := range jobs {
				if batchCtx.Err() != nil {
					return
				}
				order, err := w.client.GetOrder(batchCtx, number)
				var limited *RateLimitError
				if errors.As(err, &limited) {
					cancel()
				}
				results <- result{order, err}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, number := range orders {
			select {
			case <-batchCtx.Done():
				return
			case jobs <- number:
			}
		}
	}()

	go func() { wg.Wait(); close(results) }()
	delay := time.Second
	for result := range results {
		var limited *RateLimitError
		switch {
		case errors.As(result.err, &limited):
			delay = max(delay, limited.RetryAfter)
			if limited.ParseError != nil {
				w.log(fmt.Errorf("Retry-After %q: %w", limited.Header, limited.ParseError))
			}
		case errors.Is(result.err, ErrNotRegistered), errors.Is(result.err, context.Canceled):
			continue
		case result.err != nil:
			w.log(result.err)
		default:
			if err := w.storage.UpdateAccrual(ctx, result.order); err != nil {
				w.log(err)
			}
		}
	}
	return delay
}

func (w *Worker) log(err error) {
	if w.report != nil {
		w.report(err)
	}
}
