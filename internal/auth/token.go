package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/selis18/gomart_dipl/internal/config"
)

type claims struct {
	jwt.RegisteredClaims
	UserID string
}

var ErrSecretKeyEmpty = errors.New("Secret key is empty")

func BuildToken(id string) (string, error) {
	secret := config.GetSecretKey()
	if secret == "" {
		return "", ErrSecretKeyEmpty
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(3 * time.Hour)),
		},
		UserID: id,
	})
	return token.SignedString([]byte(secret))
}

func ParseToken(tokenString string) (string, error) {
	secret := config.GetSecretKey()
	if secret == "" {
		return "", ErrSecretKeyEmpty
	}

	c := &claims{}
	token, err := jwt.ParseWithClaims(tokenString, c, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", err
	}
	if token == nil || !token.Valid || c.UserID == "" {
		return "", fmt.Errorf("invalid token")
	}
	return c.UserID, nil
}
