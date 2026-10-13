package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
