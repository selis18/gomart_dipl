package handler

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"

	"github.com/selis18/gomart_dipl/internal/auth"
	"github.com/selis18/gomart_dipl/internal/logger"
	"github.com/selis18/gomart_dipl/internal/repository"
	"go.uber.org/zap"
)

func (h *HandlerStorage) Withdraw(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("user")
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := auth.ParseToken(cookie.Value, h.secret)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		http.Error(w, "expected application/json", http.StatusBadRequest)
		return
	}

	var request struct {
		Order string          `json:"order"`
		Sum   json.RawMessage `json:"sum"`
	}

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if !validOrderNumber(request.Order) {
		http.Error(w, "invalid order number", http.StatusUnprocessableEntity)
		return
	}
	var sum float64
	if err := json.Unmarshal(request.Sum, &sum); err != nil || sum <= 0 || math.IsInf(sum, 0) {
		http.Error(w, "invalid sum", http.StatusBadRequest)
		return
	}

	err = h.storage.Withdraw(r.Context(), userID, request.Order, string(request.Sum))

	switch {
	case err == nil:
		w.WriteHeader(http.StatusOK)
	case errors.Is(err, repository.ErrInsufficientFunds):
		http.Error(w, "insufficient funds", http.StatusPaymentRequired)
	case errors.Is(err, repository.ErrWithdrawalConflict):
		http.Error(w, "withdrawal already exists with different details", http.StatusConflict)
	default:
		logger.Log.Error("balance request failed", zap.String("operation", r.Method+" "+r.URL.Path), zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func (h *HandlerStorage) GetBalance(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("user")
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID, err := auth.ParseToken(cookie.Value, h.secret)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	balance, err := h.storage.GetBalance(r.Context(), userID)
	if err != nil {
		logger.Log.Error("balance request failed", zap.String("operation", r.Method+" "+r.URL.Path), zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	body, err := json.Marshal(balance)
	if err != nil {
		logger.Log.Error("balance request failed", zap.String("operation", r.Method+" "+r.URL.Path), zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h *HandlerStorage) GetWithdrawals(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("user")
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID, err := auth.ParseToken(cookie.Value, h.secret)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	withdrawals, err := h.storage.GetWithdrawals(r.Context(), userID)
	if err != nil {
		logger.Log.Error("balance request failed", zap.String("operation", r.Method+" "+r.URL.Path), zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if len(withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	body, err := json.Marshal(withdrawals)
	if err != nil {
		logger.Log.Error("balance request failed", zap.String("operation", r.Method+" "+r.URL.Path), zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
