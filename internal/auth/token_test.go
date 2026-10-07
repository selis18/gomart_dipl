package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/selis18/gomart_dipl/internal/config"
)

func TestTokenRoundTrip(t *testing.T) {
	t.Setenv("SECRET_KEY", "token-test-secret")
	config.ParseConfig()
	token, err := BuildToken("database-user-id")
	if err != nil {
		t.Fatal(err)
	}
	id, err := ParseToken(token)
	if err != nil || id != "database-user-id" {
		t.Fatalf("ParseToken = (%q, %v), want database-user-id", id, err)
	}
}

func TestParseTokenRejectsInvalidTokens(t *testing.T) {
	const secret = "token-test-secret"
	t.Setenv("SECRET_KEY", secret)
	config.ParseConfig()
	for _, tt := range []struct {
		name, id, key string
		expires       time.Time
	}{
		{"expired", "user-id", secret, time.Now().Add(-time.Hour)},
		{"wrong signature", "user-id", "other-secret", time.Now().Add(time.Hour)},
		{"missing user ID", "", secret, time.Now().Add(time.Hour)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
				RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(tt.expires)},
				UserID:           tt.id,
			}).SignedString([]byte(tt.key))
			if err != nil {
				t.Fatal(err)
			}
			if id, err := ParseToken(token); err == nil || id != "" {
				t.Errorf("ParseToken = (%q, %v), want rejection", id, err)
			}
		})
	}
	if _, err := ParseToken("not-a-jwt"); err == nil {
		t.Error("malformed token accepted")
	}
}
