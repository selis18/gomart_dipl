package handler

import (
	"github.com/selis18/gomart_dipl/internal/repository"
)

type HandlerStorage struct {
	storage repository.Storage
	secret  string
}

func NewHandlerStorage(s repository.Storage, secret string) *HandlerStorage {
	return &HandlerStorage{
		storage: s,
		secret:  secret,
	}
}
