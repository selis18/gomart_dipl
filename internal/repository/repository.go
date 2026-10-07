package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
)

var ErrLoginEmpty = errors.New("login is empty")
var ErrLoginExisted = errors.New("login already exists")
var ErrPasswordEmpty = errors.New("password is empty")

type Storage interface {
	RegisterUser(ctx context.Context, l string, p string) (string, error)
}

type StorageRepo struct {
	db *sql.DB
}

func NewStorageRepo(db *sql.DB) *StorageRepo {
	return &StorageRepo{
		db: db,
	}
}

func (s *StorageRepo) RegisterUser(ctx context.Context, l string, p string) (string, error) {
	if l == "" {
		return "", ErrLoginEmpty
	}
	if p == "" {
		return "", ErrPasswordEmpty
	}

	var randomID [16]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(randomID[:])

	var userID string
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO users (id, login, password)
		VALUES ($1, $2, $3)
		ON CONFLICT (login) DO NOTHING
		RETURNING id
	`, id, l, p).Scan(&userID)

	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrLoginExisted
	}

	if err != nil {
		return "", err
	}
	return userID, nil
}
