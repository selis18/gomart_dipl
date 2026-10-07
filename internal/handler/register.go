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

func (h *HandlerStorage) Register(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req model.RegisterRequest
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

	if len(req.Password) > 72 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		logger.Log.Error("hash registration password", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	userID, err := h.storage.RegisterUser(r.Context(), req.Login, string(passwordHash))
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrLoginExisted):
			w.WriteHeader(http.StatusConflict)
			return
		default:
			logger.Log.Error("register user in database", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	token, err := auth.BuildToken(userID)
	if err != nil {
		logger.Log.Error("build registration token", zap.Error(err))
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
