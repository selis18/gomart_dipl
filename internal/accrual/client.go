// Package accrual implements the rewards calculation service protocol.
package accrual

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrNotRegistered = errors.New("order is not registered in accrual service")

type RateLimitError struct {
	RetryAfter time.Duration
	Header     string
	ParseError error
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("accrual service rate limit: retry after %s", e.RetryAfter)
}

type Order struct {
	Number  string   `json:"order"`
	Status  string   `json:"status"`
	Accrual *float64 `json:"accrual,omitempty"`
}

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(address string, client *http.Client) (*Client, error) {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, errors.New("invalid accrual service address")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(u.String(), "/"), http: client}, nil
}

func (c *Client) GetOrder(ctx context.Context, number string) (Order, error) {
	if number == "" || strings.IndexFunc(number, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return Order{}, errors.New("invalid order number")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	const attempts = 4
	var resp *http.Response
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/orders/"+number, nil)
		if err != nil {
			return Order{}, fmt.Errorf("create accrual request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		resp, err = c.http.Do(req)
		if err != nil {
			return Order{}, fmt.Errorf("get accrual order: %w", err)
		}
		if resp.StatusCode < 500 || resp.StatusCode > 599 || attempt == attempts-1 {
			break
		}
		// Bound draining so a large error body cannot delay the retry indefinitely.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		timer := time.NewTimer(100 * time.Millisecond << attempt)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Order{}, ctx.Err()
		case <-timer.C:
		}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return Order{}, ErrNotRegistered
	case http.StatusTooManyRequests:
		header := resp.Header.Get("Retry-After")
		delay, err := retryAfter(header, time.Now())
		return Order{}, &RateLimitError{RetryAfter: delay, Header: header, ParseError: err}
	case http.StatusOK:
	default:
		return Order{}, fmt.Errorf("accrual service returned HTTP %d", resp.StatusCode)
	}

	const maxBody = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return Order{}, fmt.Errorf("read accrual response: %w", err)
	}
	if len(body) > maxBody {
		return Order{}, errors.New("accrual response too large")
	}
	var order Order
	if err := json.Unmarshal(body, &order); err != nil {
		return Order{}, fmt.Errorf("decode accrual response: %w", err)
	}
	if order.Number != number {
		return Order{}, errors.New("accrual response order number mismatch")
	}
	switch order.Status {
	case "REGISTERED", "PROCESSING", "INVALID", "PROCESSED":
	default:
		return Order{}, errors.New("unknown accrual order status")
	}
	if order.Accrual != nil && (*order.Accrual < 0 || math.IsNaN(*order.Accrual) || math.IsInf(*order.Accrual, 0)) {
		return Order{}, errors.New("invalid accrual amount")
	}
	return order, nil
}

func retryAfter(value string, now time.Time) (time.Duration, error) {
	const fallback = time.Minute
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 && seconds <= math.MaxInt64/int64(time.Second) {
		return time.Duration(seconds) * time.Second, nil
	}
	if date, err := http.ParseTime(value); err == nil {
		if delay := date.Sub(now); delay > 0 {
			return delay, nil
		}
		return 0, nil
	}
	return fallback, fmt.Errorf("invalid Retry-After header %q; using %s", value, fallback)
}
