package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/selis18/gomart_dipl/internal/auth"
	"github.com/selis18/gomart_dipl/internal/logger"
	"github.com/selis18/gomart_dipl/internal/model"
	"github.com/selis18/gomart_dipl/internal/repository"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

func (h *HandlerStorage) Auth(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req model.RegisterAuthRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Login) == "" || req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	userID, passwordHash, err := h.storage.LoginUser(r.Context(), req.Login)
	if err != nil {
		if errors.Is(err, repository.ErrLoginNotExisted) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		logger.Log.Error("get user from database", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	token, err := auth.BuildToken(userID)
	if err != nil {
		logger.Log.Error("build auth token", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "user",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
	})

	w.WriteHeader(http.StatusOK)
}
