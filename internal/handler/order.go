package handler

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/selis18/gomart_dipl/internal/auth"
	"github.com/selis18/gomart_dipl/internal/logger"
	"github.com/selis18/gomart_dipl/internal/repository"
	"go.uber.org/zap"
)

func (h *HandlerStorage) PostOrder(w http.ResponseWriter, r *http.Request) {
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
	if err != nil || contentType != "text/plain" {
		http.Error(w, "expected text/plain", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	number := string(body)
	if !validOrderNumber(number) {
		http.Error(w, "invalid order number", http.StatusUnprocessableEntity)
		return
	}
	err = h.storage.AddOrder(r.Context(), userID, number)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusAccepted)
	case errors.Is(err, repository.ErrOrderExists):
		w.WriteHeader(http.StatusOK)
	case errors.Is(err, repository.ErrOrderConflict):
		http.Error(w, "order belongs to another user", http.StatusConflict)
	default:
		logger.Log.Error("order request failed", zap.String("operation", r.Method+" "+r.URL.Path), zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// Luhn algorithm
func validOrderNumber(number string) bool {
	if number == "" || len(number) > 128 {
		return false
	}
	sum, double := 0, false
	for i := len(number) - 1; i >= 0; i-- {
		if number[i] < '0' || number[i] > '9' {
			return false
		}
		digit := int(number[i] - '0')
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum = (sum + digit) % 10
		double = !double
	}
	return sum == 0
}

func (h *HandlerStorage) GetOrdersByUserID(w http.ResponseWriter, r *http.Request) {
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
	orders, err := h.storage.GetOrdersByUserID(r.Context(), userID)
	if err != nil {
		logger.Log.Error("order request failed", zap.String("operation", r.Method+" "+r.URL.Path), zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	body, err := json.Marshal(orders)
	if err != nil {
		logger.Log.Error("order request failed", zap.String("operation", r.Method+" "+r.URL.Path), zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
