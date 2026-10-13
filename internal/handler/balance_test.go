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
