package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v4"
	"github.com/selis18/gomart_dipl/internal/model"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/selis18/gomart_dipl/internal/auth"
	"github.com/selis18/gomart_dipl/internal/config"
	"github.com/selis18/gomart_dipl/internal/repository"
)

type orderStorage struct {
	repository.Storage
	add func(context.Context, string, string) error
}

func (s orderStorage) AddOrder(ctx context.Context, user, number string) error {
	return s.add(ctx, user, number)
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("read failure") }

func TestPostOrder(t *testing.T) {
	t.Setenv("SECRET_KEY", "order-test-secret")
	config.ParseConfig()
	token, err := auth.BuildToken("alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, body, contentType, cookie string
		storageErr                      error
		status                          int
		called                          bool
	}{
		{"new", "12345678903", "text/plain", token, nil, 202, true},
		{"charset", "12345678903", "text/plain; charset=utf-8", token, nil, 202, true},
		{"same owner", "12345678903", "text/plain", token, fmt.Errorf("save: %w", repository.ErrOrderExists), 200, true},
		{"other owner", "12345678903", "text/plain", token, fmt.Errorf("save: %w", repository.ErrOrderConflict), 409, true},
		{"database failure", "12345678903", "text/plain", token, errors.New("private database details"), 500, true},
		{"missing cookie", "12345678903", "text/plain", "", nil, 401, false},
		{"invalid cookie", "12345678903", "text/plain", "invalid", nil, 401, false},
		{"empty", "", "text/plain", token, nil, 400, false},
		{"json", "12345678903", "application/json", token, nil, 400, false},
		{"missing content type", "12345678903", "", token, nil, 400, false},
		{"checksum", "12345678904", "text/plain", token, nil, 422, false},
		{"letters", "abc", "text/plain", token, nil, 422, false},
		{"whitespace", "12345678903\n", "text/plain", token, nil, 422, false},
		{"negative", "-12345678903", "text/plain", token, nil, 422, false},
		{"long number", strings.Repeat("0", 100) + "12345678903", "text/plain", token, nil, 202, true},
		{"read failure", "broken-reader", "text/plain", token, nil, 400, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			type key struct{}
			ctx := context.WithValue(context.Background(), key{}, "context")
			calls := 0
			s := orderStorage{add: func(got context.Context, user, number string) error {
				calls++
				if got.Value(key{}) != "context" || user != "alice" || number != tt.body {
					t.Errorf("unexpected storage arguments: %s %s", user, number)
				}
				return tt.storageErr
			}}
			var body io.Reader = strings.NewReader(tt.body)
			if tt.body == "broken-reader" {
				body = brokenBody{}
			}
			r := httptest.NewRequest("POST", "/api/user/orders", body).WithContext(ctx)
			r.Header.Set("Content-Type", tt.contentType)
			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{Name: "user", Value: tt.cookie})
			}
			w := httptest.NewRecorder()
			NewHandlerStorage(s).PostOrder(w, r)
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d", w.Code, tt.status)
			}
			if (calls == 1) != tt.called || calls > 1 {
				t.Errorf("storage calls = %d", calls)
			}
			if strings.Contains(w.Body.String(), "private database") {
				t.Error("leaked storage error")
			}
		})
	}
}

func TestValidOrderNumber(t *testing.T) {
	for _, number := range []string{"0", "18", "79927398713", "12345678903", "00018", "9278923470"} {
		if !validOrderNumber(number) {
			t.Errorf("rejected %q", number)
		}
	}
	for _, number := range []string{"", "1", "79927398714", "１２", "1 8", "+18", "18.0"} {
		if validOrderNumber(number) {
			t.Errorf("accepted %q", number)
		}
	}
}

type listStorage struct {
	repository.Storage
	list func(context.Context, string) ([]model.Order, error)
}

func (s listStorage) GetOrdersByUserID(ctx context.Context, id string) ([]model.Order, error) {
	return s.list(ctx, id)
}

func TestGetOrders(t *testing.T) {
	t.Setenv("SECRET_KEY", "list-secret")
	config.ParseConfig()
	token, err := auth.BuildToken("alice")
	if err != nil {
		t.Fatal(err)
	}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"UserID": "alice", "exp": time.Now().Add(-time.Hour).Unix()}).SignedString([]byte(config.GetSecretKey()))
	if err != nil {
		t.Fatal(err)
	}
	amount, zero, bad := 500.5, 0.0, math.NaN()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	orders := []model.Order{
		{Number: "0018", Status: "PROCESSED", Accrual: &amount, UploadedAt: now},
		{Number: "12345678903", Status: "NEW", UploadedAt: now.Add(-time.Hour)},
		{Number: "9278923470", Status: "PROCESSED", Accrual: &zero, UploadedAt: now.Add(-2 * time.Hour)},
		{Number: "346436439", Status: "INVALID", UploadedAt: now.Add(-3 * time.Hour)},
		{Number: "79927398713", Status: "PROCESSING", UploadedAt: now.Add(-4 * time.Hour)},
	}
	for _, tt := range []struct {
		name, cookie string
		orders       []model.Order
		err          error
		status       int
		called       bool
	}{
		{"orders", token, orders, nil, 200, true},
		{"nil list", token, nil, nil, 204, true},
		{"empty list", token, []model.Order{}, nil, 204, true},
		{"database error", token, orders, errors.New("private database error"), 500, true},
		{"invalid JSON value", token, []model.Order{{Accrual: &bad}}, nil, 500, true},
		{"missing cookie", "", nil, nil, 401, false},
		{"invalid cookie", "bad", nil, nil, 401, false},
		{"expired cookie", expired, nil, nil, 401, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			type key struct{}
			ctx := context.WithValue(context.Background(), key{}, "request")
			calls := 0
			storage := listStorage{list: func(got context.Context, id string) ([]model.Order, error) {
				calls++
				if id != "alice" || got.Value(key{}) != "request" {
					t.Errorf("wrong user or context: %q", id)
				}
				return tt.orders, tt.err
			}}
			r := httptest.NewRequest("GET", "/api/user/orders", nil).WithContext(ctx)
			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{Name: "user", Value: tt.cookie})
			}
			w := httptest.NewRecorder()
			NewHandlerStorage(storage).GetOrdersByUserID(w, r)
			if w.Code != tt.status {
				t.Fatalf("status=%d, want %d", w.Code, tt.status)
			}
			if (calls == 1) != tt.called || calls > 1 {
				t.Errorf("calls=%d", calls)
			}
			if strings.Contains(w.Body.String(), "private database") {
				t.Error("leaked error")
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
			var got []model.Order
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, orders) {
				t.Errorf("unexpected orders: %+v", got)
			}
			var raw []map[string]json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
				t.Fatal(err)
			}
			if _, exists := raw[1]["accrual"]; exists {
				t.Error("absent accrual must be omitted")
			}
			if string(raw[2]["accrual"]) != "0" {
				t.Error("zero accrual must be included")
			}
			for _, item := range raw {
				var timestamp string
				if err := json.Unmarshal(item["uploaded_at"], &timestamp); err != nil {
					t.Fatal(err)
				}
				if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
