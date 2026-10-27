package accrual

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetOrder(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		wantErr    bool
	}{
		{"processed", `{"order":"0018","status":"PROCESSED","accrual":500.5}`, 200, false},
		{"zero", `{"order":"0018","status":"PROCESSED","accrual":0}`, 200, false},
		{"no accrual", `{"order":"0018","status":"PROCESSED"}`, 200, false},
		{"registered", `{"order":"0018","status":"REGISTERED"}`, 200, false},
		{"processing", `{"order":"0018","status":"PROCESSING"}`, 200, false},
		{"invalid", `{"order":"0018","status":"INVALID"}`, 200, false},
		{"not registered", "", 204, true},
		{"rate limited", "limit", 429, true},
		{"server error", "private details", 500, true},
		{"unexpected status", "", 404, true},
		{"bad JSON", "{", 200, true},
		{"multiple values", `{"order":"0018","status":"INVALID"}{}`, 200, true},
		{"wrong number", `{"order":"18","status":"PROCESSED"}`, 200, true},
		{"unknown status", `{"order":"0018","status":"NEW"}`, 200, true},
		{"missing status", `{"order":"0018"}`, 200, true},
		{"null response", `null`, 200, true},
		{"negative accrual", `{"order":"0018","status":"PROCESSED","accrual":-1}`, 200, true},
		{"oversized", strings.Repeat(" ", (1<<20)+1), 200, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/api/orders/0018" || r.ContentLength != 0 || r.Header.Get("Accept") != "application/json" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(tt.status)
				_, _ = fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			client, err := NewClient(server.URL+"/", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			order, err := client.GetOrder(context.Background(), "0018")
			if (err != nil) != tt.wantErr {
				t.Fatalf("order=%+v error=%v", order, err)
			}
			if tt.status == 204 && !errors.Is(err, ErrNotRegistered) {
				t.Fatalf("error=%v", err)
			}
			if tt.status == 429 {
				var limited *RateLimitError
				if !errors.As(err, &limited) || limited.RetryAfter != time.Minute {
					t.Fatalf("error=%v", err)
				}
			}
			if err != nil && strings.Contains(err.Error(), "private details") {
				t.Fatal("leaked upstream response")
			}
			if !tt.wantErr {
				if order.Number != "0018" {
					t.Fatalf("lost number: %+v", order)
				}
				switch tt.name {
				case "processed":
					if order.Accrual == nil || *order.Accrual != 500.5 {
						t.Fatal("wrong accrual")
					}
				case "zero":
					if order.Accrual == nil || *order.Accrual != 0 {
						t.Fatal("lost zero accrual")
					}
				default:
					if order.Accrual != nil {
						t.Fatal("absent accrual must remain nil")
					}
				}
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRequestContextAndTransportError(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "request")
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	client, err := NewClient("http://localhost:8081", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Context().Value(key{}) != "request" {
			t.Error("context not propagated")
		}
		if _, ok := r.Context().Deadline(); !ok {
			t.Error("missing timeout")
		}
		return nil, r.Context().Err()
	})})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetOrder(ctx, "0018"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	for _, number := range []string{"", "../18", "18?x=y", "1 8", "１８"} {
		if _, err := client.GetOrder(context.Background(), number); err == nil {
			t.Errorf("accepted %q", number)
		}
	}
}

func TestNewClient(t *testing.T) {
	for _, address := range []string{"", "localhost:8081", "ftp://localhost", "http://", "http://localhost?x=1", "http://localhost/#fragment", "http://user:password@localhost"} {
		if _, err := NewClient(address, nil); err == nil {
			t.Errorf("accepted %q", address)
		}
	}
	client, err := NewClient("https://example.com/", nil)
	if err != nil || client.http.Timeout != 10*time.Second {
		t.Fatalf("client=%+v error=%v", client, err)
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		value string
		want  time.Duration
	}{
		{"60", time.Minute}, {"0", 0}, {"", time.Minute}, {"bad", time.Minute},
		{"-1", time.Minute}, {"9223372036854775807", time.Minute},
		{now.Add(2 * time.Minute).Format(http.TimeFormat), 2 * time.Minute},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0},
	} {
		if got, err := retryAfter(tt.value, now); got != tt.want {
			t.Errorf("%q: got %s want %s (%v)", tt.value, got, tt.want, err)
		}
	}
}

func TestGetOrderRetriesServerErrors(t *testing.T) {
	for _, recover := range []bool{true, false} {
		t.Run(fmt.Sprint(recover), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if !recover || n < 3 {
					w.WriteHeader(503)
					return
				}
				fmt.Fprint(w, `{"order":"18","status":"PROCESSED"}`)
			}))
			defer server.Close()
			client, _ := NewClient(server.URL, server.Client())
			order, err := client.GetOrder(context.Background(), "18")
			if recover {
				if err != nil || order.Status != "PROCESSED" || calls.Load() != 3 {
					t.Fatalf("order=%+v err=%v calls=%d", order, err, calls.Load())
				}
			} else if err == nil || calls.Load() != 4 {
				t.Fatalf("err=%v calls=%d", err, calls.Load())
			}
		})
	}
}

func TestGetOrderCancelsRetryDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(503)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, server.Client())
	done := make(chan error, 1)
	go func() { _, err := client.GetOrder(ctx, "18"); done <- err }()
	deadline := time.After(time.Second)
	for calls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("request did not start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("retry did not stop")
	}
	if calls.Load() != 1 {
		t.Fatal("retried after cancellation")
	}
}

func TestRetryAfterSignalsFallback(t *testing.T) {
	for _, value := range []string{"", "bad", "-1", "9223372036854775807"} {
		delay, err := retryAfter(value, time.Now())
		if delay != time.Minute || err == nil || !strings.Contains(err.Error(), value) {
			t.Fatalf("value=%q delay=%s err=%v", value, delay, err)
		}
	}
}
