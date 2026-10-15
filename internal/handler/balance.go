package handler

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"

	"github.com/selis18/gomart_dipl/internal/auth"
	"github.com/selis18/gomart_dipl/internal/repository"
)

func (h *HandlerStorage) Withdraw(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("user")
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := auth.ParseToken(cookie.Value)
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
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func (h *HandlerStorage) GetBalance(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("user")
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID, err := auth.ParseToken(cookie.Value)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	balance, err := h.storage.GetBalance(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	body, err := json.Marshal(balance)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
