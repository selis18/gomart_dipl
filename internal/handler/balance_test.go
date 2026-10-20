package handler

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/selis18/gomart_dipl/internal/auth"
	"github.com/selis18/gomart_dipl/internal/config"
	"github.com/selis18/gomart_dipl/internal/model"
	"github.com/selis18/gomart_dipl/internal/repository"
)

type balanceStorage struct {
	repository.Storage
	get func(context.Context, string) (model.Balance, error)
}

func (s balanceStorage) GetBalance(ctx context.Context, id string) (model.Balance, error) {
	return s.get(ctx, id)
}

func TestGetBalance(t *testing.T) {
	t.Setenv("SECRET_KEY", "balance-test-secret")
	config.ParseConfig()
	token, err := auth.BuildToken("alice")
	if err != nil {
		t.Fatal(err)
	}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"UserID": "alice", "exp": time.Now().Add(-time.Hour).Unix()}).SignedString([]byte(config.GetSecretKey()))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, cookie string
		balance      model.Balance
		err          error
		status       int
		called       bool
	}{
		{"balance", token, model.Balance{Current: 500.5, Withdrawn: 42}, nil, 200, true},
		{"no operations", token, model.Balance{}, nil, 200, true},
		{"all spent", token, model.Balance{Withdrawn: 42.5}, nil, 200, true},
		{"missing cookie", "", model.Balance{}, nil, 401, false},
		{"invalid cookie", "invalid", model.Balance{}, nil, 401, false},
		{"expired cookie", expired, model.Balance{}, nil, 401, false},
		{"database error", token, model.Balance{}, errors.New("private storage error"), 500, true},
		{"serialization error", token, model.Balance{Current: math.NaN()}, nil, 500, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			type key struct{}
			ctx := context.WithValue(context.Background(), key{}, "request")
			calls := 0
			s := balanceStorage{get: func(got context.Context, id string) (model.Balance, error) {
				calls++
				if id != "alice" || got.Value(key{}) != "request" {
					t.Error("wrong user or request context")
				}
				return tt.balance, tt.err
			}}
			r := httptest.NewRequest("GET", "/api/user/balance", nil).WithContext(ctx)
			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{Name: "user", Value: tt.cookie})
			}
			w := httptest.NewRecorder()
			NewHandlerStorage(s).GetBalance(w, r)
			if w.Code != tt.status {
				t.Fatalf("status=%d, want %d", w.Code, tt.status)
			}
			if (calls == 1) != tt.called || calls > 1 {
				t.Errorf("storage calls=%d", calls)
			}
			if strings.Contains(w.Body.String(), "private storage") {
				t.Error("leaked storage error")
			}
			if tt.status != 200 {
				return
			}
			if w.Header().Get("Content-Type") != "application/json" {
				t.Error("expected JSON content type")
			}
			var got map[string]float64
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 {
				t.Fatalf("expected both fields, got %v", got)
			}
			current, hasCurrent := got["current"]
			withdrawn, hasWithdrawn := got["withdrawn"]
			if !hasCurrent || !hasWithdrawn || current != tt.balance.Current || withdrawn != tt.balance.Withdrawn {
				t.Errorf("unexpected balance: %v", got)
			}
		})
	}
}

type withdrawalStorage struct {
	repository.Storage
	withdraw func(context.Context, string, string, string) error
}

func (s withdrawalStorage) Withdraw(ctx context.Context, user, order, sum string) error {
	return s.withdraw(ctx, user, order, sum)
}
func TestWithdraw(t *testing.T) {
	t.Setenv("SECRET_KEY", "withdraw-test-secret")
	config.ParseConfig()
	token, err := auth.BuildToken("alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, body, cookie, contentType string
		storageErr                      error
		status                          int
		called                          bool
	}{
		{"success", `{"order":"0018","sum":0.10}`, token, "application/json", nil, 200, true},
		{"insufficient", `{"order":"0018","sum":0.10}`, token, "application/json", repository.ErrInsufficientFunds, 402, true},
		{"conflict", `{"order":"0018","sum":0.10}`, token, "application/json", repository.ErrWithdrawalConflict, 409, true},
		{"storage failure", `{"order":"0018","sum":0.10}`, token, "application/json", errors.New("private error"), 500, true},
		{"missing auth", `{}`, "", "application/json", nil, 401, false},
		{"invalid auth", `{}`, "invalid", "application/json", nil, 401, false},
		{"bad order", `{"order":"19","sum":1}`, token, "application/json", nil, 422, false},
		{"empty order", `{"sum":1}`, token, "application/json", nil, 422, false},
		{"bad json", `{`, token, "application/json", nil, 400, false},
		{"trailing json", `{"order":"18","sum":1}{}`, token, "application/json", nil, 400, false},
		{"missing sum", `{"order":"18"}`, token, "application/json", nil, 400, false},
		{"null sum", `{"order":"18","sum":null}`, token, "application/json", nil, 400, false},
		{"zero sum", `{"order":"18","sum":0}`, token, "application/json", nil, 400, false},
		{"negative sum", `{"order":"18","sum":-1}`, token, "application/json", nil, 400, false},
		{"string sum", `{"order":"18","sum":"1"}`, token, "application/json", nil, 400, false},
		{"overflow", `{"order":"18","sum":1e999}`, token, "application/json", nil, 400, false},
		{"wrong media", `{}`, token, "text/plain", nil, 400, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			ctx := context.WithValue(context.Background(), orderContextKeyForWithdrawal{}, "request")
			s := withdrawalStorage{withdraw: func(got context.Context, user, order, sum string) error {
				calls++
				if got != ctx || user != "alice" || order != "0018" || sum != "0.10" {
					t.Errorf("wrong arguments: %s %s %s", user, order, sum)
				}
				return tt.storageErr
			}}
			r := httptest.NewRequest("POST", "/api/user/balance/withdraw", strings.NewReader(tt.body)).WithContext(ctx)
			r.Header.Set("Content-Type", tt.contentType)
			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{Name: "user", Value: tt.cookie})
			}
			w := httptest.NewRecorder()
			NewHandlerStorage(s).Withdraw(w, r)
			if w.Code != tt.status {
				t.Errorf("status=%d want %d", w.Code, tt.status)
			}
			if (calls == 1) != tt.called || calls > 1 {
				t.Errorf("calls=%d", calls)
			}
			if strings.Contains(w.Body.String(), "private") {
				t.Error("storage error exposed")
			}
		})
	}
}

type orderContextKeyForWithdrawal struct{}

type withdrawalsStorage struct {
	repository.Storage
	get func(context.Context, string) ([]model.Withdrawal, error)
}

func (s withdrawalsStorage) GetWithdrawals(ctx context.Context, id string) ([]model.Withdrawal, error) {
	return s.get(ctx, id)
}
func TestGetWithdrawals(t *testing.T) {
	t.Setenv("SECRET_KEY", "withdrawals-test-secret")
	config.ParseConfig()
	token, err := auth.BuildToken("alice")
	if err != nil {
		t.Fatal(err)
	}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"UserID": "alice", "exp": time.Now().Add(-time.Hour).Unix()}).SignedString([]byte(config.GetSecretKey()))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.FixedZone("UTC+5", 5*3600))
	items := []model.Withdrawal{{Order: "0018", Sum: 500.5, ProcessedAt: now}, {Order: "12345678903", Sum: 42, ProcessedAt: now.Add(-time.Hour)}}
	for _, tt := range []struct {
		name, cookie string
		items        []model.Withdrawal
		err          error
		status       int
		called       bool
	}{
		{"history", token, items, nil, 200, true},
		{"empty", token, nil, nil, 204, true},
		{"empty slice", token, []model.Withdrawal{}, nil, 204, true},
		{"missing auth", "", nil, nil, 401, false},
		{"invalid auth", "invalid", nil, nil, 401, false},
		{"expired auth", expired, nil, nil, 401, false},
		{"storage failure", token, nil, errors.New("private database details"), 500, true},
		{"serialization failure", token, []model.Withdrawal{{Sum: math.NaN()}}, nil, 500, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), orderContextKeyForWithdrawal{}, "request")
			calls := 0
			s := withdrawalsStorage{get: func(got context.Context, id string) ([]model.Withdrawal, error) {
				calls++
				if got != ctx || id != "alice" {
					t.Error("wrong context or user")
				}
				return tt.items, tt.err
			}}
			r := httptest.NewRequest("GET", "/api/user/withdrawals", nil).WithContext(ctx)
			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{Name: "user", Value: tt.cookie})
			}
			w := httptest.NewRecorder()
			NewHandlerStorage(s).GetWithdrawals(w, r)
			if w.Code != tt.status {
				t.Fatalf("status=%d want %d", w.Code, tt.status)
			}
			if (calls == 1) != tt.called || calls > 1 {
				t.Errorf("calls=%d", calls)
			}
			if strings.Contains(w.Body.String(), "private") {
				t.Error("exposed storage error")
			}
			if tt.status == 204 && w.Body.Len() != 0 {
				t.Error("204 must have empty body")
			}
			if tt.status != 200 {
				return
			}
			if w.Header().Get("Content-Type") != "application/json" {
				t.Error("missing JSON content type")
			}
			var got []map[string]json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(items) {
				t.Fatalf("items=%d", len(got))
			}
			for i, item := range got {
				if len(item) != 3 {
					t.Errorf("unexpected fields: %v", item)
				}
				var order, stamp string
				var sum float64
				if err := json.Unmarshal(item["order"], &order); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(item["sum"], &sum); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(item["processed_at"], &stamp); err != nil {
					t.Fatal(err)
				}
				parsed, err := time.Parse(time.RFC3339, stamp)
				if err != nil || !parsed.Equal(items[i].ProcessedAt) || order != items[i].Order || sum != items[i].Sum {
					t.Errorf("wrong withdrawal: %v", item)
				}
			}
		})
	}
}
