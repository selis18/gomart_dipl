package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/selis18/gomart_dipl/internal/logger"
	"go.uber.org/zap"
)

func CookieMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookies, err := r.Cookie("user")
		if err == nil {
			_, err := ParseToken(cookies.Value)
			if err != nil {
				logger.Log.Error("Parser cookie: ", zap.Error(err))
			}
		}

		if err != nil {
			_, err := CreateCookie(w)
			if err != nil {
				logger.Log.Error("Create user cookie: ", zap.Error(err))
			}
		}

		h.ServeHTTP(w, r)
	})
}

func CreateCookie(w http.ResponseWriter) (string, error) {
	user, err := makeHexUserID()
	if err != nil {
		return "", err
	}

	tokenString, err := BuildToken(user)
	if err != nil {
		return "", err
	}

	cookie := &http.Cookie{
		Name:     "user",
		Value:    tokenString,
		HttpOnly: true,
		Path:     "/",
	}

	http.SetCookie(w, cookie)
	return user, nil
}

func makeHexUserID() (string, error) {
	user := make([]byte, 16)
	_, err := rand.Read(user)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(user), nil
}
