package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/selis18/gomart_dipl/internal/auth"
	"github.com/selis18/gomart_dipl/internal/config"
	"github.com/selis18/gomart_dipl/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type loginStorageFunc func(context.Context, string) (string, string, error)

func (f loginStorageFunc) LoginUser(ctx context.Context, login string) (string, string, error) {
	return f(ctx, login)
}

func (f loginStorageFunc) RegisterUser(context.Context, string, string) (string, error) {
	panic("login must not register a user")
}

func TestAuthSuccess(t *testing.T) {
	t.Setenv("SECRET_KEY", "login-test-secret")
	config.ParseConfig()
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request-value")
	calls := 0
	storage := loginStorageFunc(func(gotCtx context.Context, login string) (string, string, error) {
		calls++
		if gotCtx.Value(contextKey{}) != "request-value" {
			t.Error("request context was not passed to storage")
		}
		if login != "alice" {
			t.Errorf("login = %q, want alice", login)
		}
		return "existing-user-id", string(hash), nil
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(`{"login":"alice","password":"password"}`)).WithContext(ctx)
	NewHandlerStorage(storage).Auth(w, r)
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
	if err != nil || id != "existing-user-id" {
		t.Fatalf("cookie user ID = %q, error = %v", id, err)
	}
}

func TestAuthInvalidRequest(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"empty body", ""},
		{"broken JSON", `{"login":`},
		{"wrong field type", `{"login":42,"password":"secret"}`},
		{"empty login", `{"login":"","password":"secret"}`},
		{"blank login", `{"login":"  ","password":"secret"}`},
		{"empty password", `{"login":"alice","password":""}`},
		{"null", `null`},
		{"oversized body", `{"login":"` + strings.Repeat("a", 1<<20) + `","password":"secret"}`},
		{"second JSON object", `{"login":"alice","password":"secret"} {}`},
		{"trailing garbage", `{"login":"alice","password":"secret"} garbage`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			storage := loginStorageFunc(func(context.Context, string) (string, string, error) {
				t.Fatal("invalid request reached storage")
				return "", "", nil
			})
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(tt.body))
			NewHandlerStorage(storage).Auth(w, r)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
			if len(w.Header().Values("Set-Cookie")) != 0 {
				t.Error("invalid request received a cookie")
			}
		})
	}
}

func TestAuthFailures(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, password, secret string
		storageErr             error
		status                 int
	}{
		{"wrong password", "wrong", "test-secret", nil, http.StatusUnauthorized},
		{"unknown login", "correct-password", "test-secret", repository.ErrLoginNotExisted, http.StatusUnauthorized},
		{"wrapped unknown login", "correct-password", "test-secret", fmt.Errorf("lookup: %w", repository.ErrLoginNotExisted), http.StatusUnauthorized},
		{"database failure", "correct-password", "test-secret", errors.New("database unavailable"), http.StatusInternalServerError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SECRET_KEY", tt.secret)
			config.ParseConfig()
			calls := 0
			storage := loginStorageFunc(func(context.Context, string) (string, string, error) {
				calls++
				return "existing-user-id", string(hash), tt.storageErr
			})
			w := httptest.NewRecorder()
			body := fmt.Sprintf(`{"login":"alice","password":%q}`, tt.password)
			r := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(body))
			NewHandlerStorage(storage).Auth(w, r)
			if w.Code != tt.status || calls != 1 {
				t.Errorf("status = %d, storage calls = %d; want %d and 1", w.Code, calls, tt.status)
			}
			if len(w.Header().Values("Set-Cookie")) != 0 {
				t.Error("failed login received a cookie")
			}
		})
	}
}

func TestAuthTokenFailure(t *testing.T) {
	// Config retains a previously loaded secret, so use a fresh process.
	if os.Getenv("LOGIN_TOKEN_FAILURE_HELPER") != "1" {
		t.Setenv("LOGIN_TOKEN_FAILURE_HELPER", "1")
		t.Setenv("SECRET_KEY", "")
		cmd := exec.Command(os.Args[0], "-test.run=^TestAuthTokenFailure$")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated token failure test: %v\n%s", err, output)
		}
		return
	}
	config.ParseConfig()
	if config.GetSecretKey() != "" {
		t.Fatal("test requires an empty signing key")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	storage := loginStorageFunc(func(context.Context, string) (string, string, error) {
		return "existing-user-id", string(hash), nil
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(`{"login":"alice","password":"password"}`))
	NewHandlerStorage(storage).Auth(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	if len(w.Header().Values("Set-Cookie")) != 0 {
		t.Error("token failure received a cookie")
	}
}
