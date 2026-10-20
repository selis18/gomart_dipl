package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/selis18/gomart_dipl/internal/auth"
	"github.com/selis18/gomart_dipl/internal/config"
	"github.com/selis18/gomart_dipl/internal/handler"
	"github.com/selis18/gomart_dipl/internal/model"
	"github.com/selis18/gomart_dipl/internal/repository"
)

type routerStorage struct{ repository.Storage }

func (routerStorage) GetOrdersByUserID(context.Context, string) ([]model.Order, error) {
	return nil, nil
}

func TestGetOrdersRoute(t *testing.T) {
	t.Setenv("SECRET_KEY", "router-test-secret")
	config.ParseConfig()
	token, err := auth.BuildToken("alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, loggedIn := range []bool{true, false} {
		r := httptest.NewRequest("GET", "/api/user/orders", nil)
		want := http.StatusUnauthorized
		if loggedIn {
			r.AddCookie(&http.Cookie{Name: "user", Value: token})
			want = http.StatusNoContent
		}
		w := httptest.NewRecorder()
		newRouter(handler.NewHandlerStorage(routerStorage{})).ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("status = %d, want %d", w.Code, want)
		}
	}
}

func (routerStorage) AddOrder(context.Context, string, string) error { return nil }
func TestOrdersRoute(t *testing.T) {
	t.Setenv("SECRET_KEY", "router-test-secret")
	config.ParseConfig()
	token, err := auth.BuildToken("alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, loggedIn := range []bool{true, false} {
		r := httptest.NewRequest("POST", "/api/user/orders", strings.NewReader("12345678903"))
		r.Header.Set("Content-Type", "text/plain")
		want := http.StatusUnauthorized
		if loggedIn {
			r.AddCookie(&http.Cookie{Name: "user", Value: token})
			want = http.StatusAccepted
		}
		w := httptest.NewRecorder()
		newRouter(handler.NewHandlerStorage(routerStorage{})).ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("status = %d, want %d", w.Code, want)
		}
	}
}

func (routerStorage) GetBalance(context.Context, string) (model.Balance, error) {
	return model.Balance{}, nil
}

func TestBalanceRoute(t *testing.T) {
	t.Setenv("SECRET_KEY", "router-test-secret")
	config.ParseConfig()
	token, err := auth.BuildToken("alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, loggedIn := range []bool{true, false} {
		r := httptest.NewRequest("GET", "/api/user/balance", nil)
		want := http.StatusUnauthorized
		if loggedIn {
			r.AddCookie(&http.Cookie{Name: "user", Value: token})
			want = http.StatusOK
		}
		w := httptest.NewRecorder()
		newRouter(handler.NewHandlerStorage(routerStorage{})).ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("status=%d, want %d", w.Code, want)
		}
	}
}

func (routerStorage) Withdraw(context.Context, string, string, string) error { return nil }

func TestWithdrawRoute(t *testing.T) {
	t.Setenv("SECRET_KEY", "router-test-secret")
	config.ParseConfig()
	token, err := auth.BuildToken("alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, loggedIn := range []bool{true, false} {
		r := httptest.NewRequest("POST", "/api/user/balance/withdraw", strings.NewReader(`{"order":"18","sum":1}`))
		r.Header.Set("Content-Type", "application/json")
		want := 401
		if loggedIn {
			r.AddCookie(&http.Cookie{Name: "user", Value: token})
			want = 200
		}
		w := httptest.NewRecorder()
		newRouter(handler.NewHandlerStorage(routerStorage{})).ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("status=%d want %d", w.Code, want)
		}
	}
}

func (routerStorage) GetWithdrawals(context.Context, string) ([]model.Withdrawal, error) {
	return nil, nil
}
func TestWithdrawalsRoute(t *testing.T) {
	t.Setenv("SECRET_KEY", "router-test-secret")
	config.ParseConfig()
	token, err := auth.BuildToken("alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, loggedIn := range []bool{true, false} {
		r := httptest.NewRequest("GET", "/api/user/withdrawals", nil)
		want := http.StatusUnauthorized
		if loggedIn {
			r.AddCookie(&http.Cookie{Name: "user", Value: token})
			want = http.StatusNoContent
		}
		w := httptest.NewRecorder()
		newRouter(handler.NewHandlerStorage(routerStorage{})).ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("status=%d want %d", w.Code, want)
		}
	}
}
