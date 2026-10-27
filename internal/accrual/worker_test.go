package accrual

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type workerStore struct {
	numbers []string
	updates []Order
	err     error
}

func (s *workerStore) PendingOrders(context.Context) ([]string, error) { return s.numbers, s.err }
func (s *workerStore) UpdateAccrual(_ context.Context, order Order) error {
	s.updates = append(s.updates, order)
	return s.err
}

type orderClientFunc func(context.Context, string) (Order, error)

func (f orderClientFunc) GetOrder(ctx context.Context, number string) (Order, error) {
	return f(ctx, number)
}

func TestWorkerRateLimitPausesAllOrders(t *testing.T) {
	store := &workerStore{numbers: []string{"18", "26"}}
	var calls atomic.Int32
	worker := NewWorker(store, orderClientFunc(func(context.Context, string) (Order, error) {
		calls.Add(1)
		return Order{}, &RateLimitError{RetryAfter: time.Minute}
	}), nil)
	if delay := worker.poll(context.Background()); delay != time.Minute || (calls.Load() < 1 || calls.Load() > 2) || len(store.updates) != 0 {
		t.Fatalf("delay=%s calls=%d updates=%v", delay, calls.Load(), store.updates)
	}
}

func TestWorkerRetriesAndProcessesOrder(t *testing.T) {
	responses := []string{"", `{"order":"0018","status":"REGISTERED"}`, `{"order":"0018","status":"PROCESSING"}`, `{"order":"0018","status":"PROCESSED","accrual":729.98}`}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := responses[calls]
		calls++
		if body == "" {
			w.WriteHeader(204)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	store := &workerStore{numbers: []string{"0018"}}
	worker := NewWorker(store, client, func(err error) { t.Error(err) })
	for range responses {
		worker.poll(context.Background())
	}
	var statuses []string
	for _, order := range store.updates {
		statuses = append(statuses, order.Status)
	}
	if !reflect.DeepEqual(statuses, []string{"REGISTERED", "PROCESSING", "PROCESSED"}) {
		t.Fatal(statuses)
	}
	if amount := store.updates[2].Accrual; amount == nil || *amount != 729.98 {
		t.Fatalf("amount=%v", amount)
	}
}

func TestWorkerContinuesAfterFailedOrder(t *testing.T) {
	store := &workerStore{numbers: []string{"18", "26"}}
	reported := 0
	worker := NewWorker(store, orderClientFunc(func(_ context.Context, number string) (Order, error) {
		if number == "18" {
			return Order{}, errors.New("upstream unavailable")
		}
		return Order{Number: number, Status: "INVALID"}, nil
	}), func(error) { reported++ })
	worker.poll(context.Background())
	if reported != 1 || len(store.updates) != 1 || store.updates[0].Number != "26" {
		t.Fatalf("reported=%d updates=%v", reported, store.updates)
	}
}

func TestWorkerStorageFailure(t *testing.T) {
	store := &workerStore{err: errors.New("database unavailable")}
	reported := false
	worker := NewWorker(store, nil, func(error) { reported = true })
	if delay := worker.poll(context.Background()); delay != time.Second || !reported {
		t.Fatal("storage failure not retried")
	}
}

func TestWorkerStopsDuringRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	worker := NewWorker(&workerStore{numbers: []string{"18"}}, orderClientFunc(func(ctx context.Context, _ string) (Order, error) {
		close(started)
		<-ctx.Done()
		return Order{}, ctx.Err()
	}), nil)
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(ctx) }()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
}

func TestWorkerPoolParallelAndBounded(t *testing.T) {
	store := &workerStore{numbers: []string{"1", "2", "3", "4", "5", "6", "7", "8"}}
	started := make(chan struct{}, 8)
	release := make(chan struct{})
	var active, peak atomic.Int32
	worker := NewWorker(store, orderClientFunc(func(ctx context.Context, number string) (Order, error) {
		n := active.Add(1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		defer active.Add(-1)
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return Order{}, ctx.Err()
		}
		return Order{Number: number, Status: "INVALID"}, nil
	}), nil)
	done := make(chan struct{})
	go func() { defer close(done); worker.poll(context.Background()) }()
	for range 4 {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			<-done
			t.Fatal("pool did not run four requests concurrently")
		}
	}
	close(release)
	<-done
	if peak.Load() != 4 || len(store.updates) != 8 {
		t.Fatalf("peak=%d updates=%d", peak.Load(), len(store.updates))
	}
}

func TestWorker429CancelsInflightAndStopsDispatch(t *testing.T) {
	store := &workerStore{numbers: []string{"1", "2", "3", "4", "5", "6"}}
	started := make(chan struct{}, 4)
	var calls atomic.Int32
	worker := NewWorker(store, orderClientFunc(func(ctx context.Context, number string) (Order, error) {
		calls.Add(1)
		started <- struct{}{}
		if number == "1" {
			for range 4 {
				<-started
			}
			return Order{}, &RateLimitError{RetryAfter: time.Minute, Header: "bad", ParseError: fmt.Errorf("invalid header")}
		}
		<-ctx.Done()
		return Order{}, ctx.Err()
	}), func(err error) {
		if !strings.Contains(err.Error(), "bad") {
			t.Error(err)
		}
	})
	done := make(chan time.Duration, 1)
	go func() { done <- worker.poll(context.Background()) }()
	select {
	case delay := <-done:
		if delay != time.Minute || calls.Load() != 4 {
			t.Fatalf("delay=%s calls=%d", delay, calls.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("in-flight requests were not cancelled")
	}
}

func TestWorkerStopsDuringRateLimitPause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{})
	worker := NewWorker(&workerStore{numbers: []string{"18"}}, orderClientFunc(func(context.Context, string) (Order, error) {
		close(called)
		return Order{}, &RateLimitError{RetryAfter: time.Minute}
	}), nil)
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(ctx) }()
	<-called
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown blocked by rate limit")
	}
}
