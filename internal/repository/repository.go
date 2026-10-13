package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"github.com/selis18/gomart_dipl/internal/model"
)

var ErrLoginEmpty = errors.New("login is empty")
var ErrLoginExisted = errors.New("login is already existed")
var ErrPasswordEmpty = errors.New("password is empty")
var ErrLoginNotExisted = errors.New("login don't exist")

type Storage interface {
	GetBalance(ctx context.Context, userID string) (model.Balance, error)
	GetOrdersByUserID(ctx context.Context, userID string) ([]model.Order, error)
	AddOrder(ctx context.Context, userID, number string) error
	RegisterUser(ctx context.Context, l string, p string) (string, error)
	LoginUser(ctx context.Context, l string) (string, string, error)
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

func (s *StorageRepo) LoginUser(ctx context.Context, l string) (string, string, error) {
	var passwordHash string
	var userID string
	if l == "" {
		return "", "", ErrLoginEmpty
	}

	err := s.db.QueryRowContext(ctx, `
		SELECT id,password 
		FROM users
		WHERE login=$1
	`, l).Scan(&userID, &passwordHash)

	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrLoginNotExisted
	}
	if err != nil {
		return "", "", err
	}

	return userID, passwordHash, nil
}

var ErrOrderExists = errors.New("order already belongs to this user")
var ErrOrderConflict = errors.New("order belongs to another user")

func (s *StorageRepo) AddOrder(ctx context.Context, userID, number string) error {
	var inserted string
	err := s.db.QueryRowContext(ctx, `INSERT INTO orders (number, user_id)
		VALUES ($1, $2) ON CONFLICT (number) DO NOTHING RETURNING number`, number, userID).Scan(&inserted)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	// A separate statement sees the committed winner of a concurrent insert.
	var owner string
	if err := s.db.QueryRowContext(ctx, `SELECT user_id FROM orders WHERE number = $1`, number).Scan(&owner); err != nil {
		return err
	}
	if owner == userID {
		return ErrOrderExists
	}
	return ErrOrderConflict
}
