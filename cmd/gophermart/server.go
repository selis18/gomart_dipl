package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/selis18/gomart_dipl/internal/handler"
	"github.com/selis18/gomart_dipl/internal/logger"
	"github.com/selis18/gomart_dipl/internal/migrations"
	"github.com/selis18/gomart_dipl/internal/repository"
	"go.uber.org/zap"
)

func InitServer() error {
	dbURI := os.Getenv("DATABASE_URI")
	if dbURI == "" {
		return fmt.Errorf("DATABASE_URI is empty; use ./run-local.ps1 or set the environment variable")
	}

	db, err := sql.Open("pgx", dbURI)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = db.PingContext(ctx)
	cancel()
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	migrationCtx, migrationCancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = migrations.Apply(migrationCtx, db)
	migrationCancel()
	if err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}

	var storage repository.Storage = repository.NewStorageRepo(db)
	handlers := handler.NewHandlerStorage(storage)
	return startServer(handlers)
}

func startServer(h *handler.HandlerStorage) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	r := chi.NewRouter()

	r.Route("/api", func(r chi.Router) {
		r.Route("/user", func(r chi.Router) {
			r.Use(middleware.AllowContentType("application/json"))
			r.Post("/register", h.Register)
			r.Post("/login", h.Auth)
		})
	})

	server := &http.Server{Addr: "localhost:8080", Handler: r, ReadHeaderTimeout: 5 * time.Second}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server: %w", err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Log.Error("shutdown HTTP server", zap.Error(err))
			_ = server.Close()
			return err
		}
	}
	return nil
}
