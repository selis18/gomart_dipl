package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/selis18/gomart_dipl/internal/auth"
	"github.com/selis18/gomart_dipl/internal/config"
	"github.com/selis18/gomart_dipl/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type registerStorageFunc func(context.Context, string, string) (string, error)

func (f registerStorageFunc) RegisterUser(ctx context.Context, login, hash string) (string, error) {
	return f(ctx, login, hash)
}

func TestRegisterSuccess(t *testing.T) {
	t.Setenv("SECRET_KEY", "handler-test-secret")
	config.ParseConfig()
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request-value")
	calls := 0
	storage := registerStorageFunc(func(gotCtx context.Context, login, hash string) (string, error) {
		calls++
		if gotCtx.Value(contextKey{}) != "request-value" {
			t.Error("request context was not passed to storage")
		}
		if login != "alice" {
			t.Errorf("login = %q, want alice", login)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("password")); err != nil {
			t.Errorf("stored password is not a valid hash: %v", err)
		}
		return "database-user-id", nil
	})
	r := httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(`{"login":"alice","password":"password"}`)).WithContext(ctx)
	w := httptest.NewRecorder()
	NewHandlerStorage(storage).Register(w, r)
	if w.Code != http.StatusOK || calls != 1 {
		t.Fatalf("status = %d, storage calls = %d; want 200 and 1", w.Code, calls)
	}
	response := w.Result()
	defer response.Body.Close()
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "user" || cookie.Path != "/" || !cookie.HttpOnly {
		t.Errorf("unexpected cookie attributes: %+v", cookie)
	}
	id, err := auth.ParseToken(cookie.Value)
	if err != nil || id != "database-user-id" {
		t.Fatalf("cookie user ID = %q, error = %v", id, err)
	}
}

func TestRegisterInvalidRequest(t *testing.T) {
	longPassword, err := json.Marshal(map[string]string{"login": "alice", "password": strings.Repeat("я", 37)})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct{ name, body string }{
		{"empty body", ""},
		{"broken JSON", `{"login":`},
		{"wrong field type", `{"login":42,"password":"secret"}`},
		{"empty login", `{"login":"","password":"secret"}`},
		{"blank login", `{"login":"  ","password":"secret"}`},
		{"empty password", `{"login":"alice","password":""}`},
		{"null", `null`},
		{"password over 72 bytes", string(longPassword)},
		{"oversized body", `{"login":"` + strings.Repeat("a", 1<<20) + `","password":"secret"}`},
		{"second JSON object", `{"login":"alice","password":"secret"} {}`},
		{"trailing garbage", `{"login":"alice","password":"secret"} garbage`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			storage := registerStorageFunc(func(context.Context, string, string) (string, error) {
				calls++
				return "", errors.New("invalid request reached storage")
			})
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(tt.body))
			NewHandlerStorage(storage).Register(w, r)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
			if calls != 0 {
				t.Errorf("storage called %d times for invalid input", calls)
			}
			if len(w.Header().Values("Set-Cookie")) != 0 {
				t.Error("invalid request received a cookie")
			}
		})
	}
}

func TestRegisterStorageErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
	}{
		{"duplicate login", repository.ErrLoginExisted, http.StatusConflict},
		{"wrapped duplicate", fmt.Errorf("insert: %w", repository.ErrLoginExisted), http.StatusConflict},
		{"database failure", errors.New("database unavailable"), http.StatusInternalServerError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			storage := registerStorageFunc(func(context.Context, string, string) (string, error) { return "", tt.err })
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(`{"login":"alice","password":"secret"}`))
			NewHandlerStorage(storage).Register(w, r)
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d", w.Code, tt.status)
			}
			if len(w.Header().Values("Set-Cookie")) != 0 {
				t.Error("failed registration received a cookie")
			}
		})
	}
}
