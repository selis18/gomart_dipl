package handler

import (
	"github.com/selis18/gomart_dipl/internal/repository"
)

type HandlerStorage struct {
	storage repository.Storage
}

func NewHandlerStorage(s repository.Storage) *HandlerStorage {
	return &HandlerStorage{
		storage: s,
	}
}
