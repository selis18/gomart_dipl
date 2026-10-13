package repository

import (
	"context"
	"errors"
	"testing"
)

func TestRegisterUserRejectsEmptyCredentials(t *testing.T) {
	// No database is needed: validation must happen before any query.
	s := NewStorageRepo(nil)
	for _, tt := range []struct {
		name, login, password string
		want                  error
	}{
		{"empty login", "", "hash", ErrLoginEmpty},
		{"empty password", "alice", "", ErrPasswordEmpty},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id, err := s.RegisterUser(context.Background(), tt.login, tt.password)
			if id != "" || !errors.Is(err, tt.want) {
				t.Errorf("RegisterUser = (%q, %v), want empty ID and %v", id, err, tt.want)
			}
		})
	}
}
