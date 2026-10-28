package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selis18/gomart_dipl/internal/accrual"
	"github.com/selis18/gomart_dipl/internal/config"
	"github.com/selis18/gomart_dipl/internal/handler"
	"github.com/selis18/gomart_dipl/internal/logger"
	"github.com/selis18/gomart_dipl/internal/migrations"
	"github.com/selis18/gomart_dipl/internal/repository"
	"go.uber.org/zap"
)

func InitServer(cfg config.Config) error {
	dbURI := cfg.DatabaseURI
	if dbURI == "" {
		return fmt.Errorf("DATABASE_URI is empty; use ./run-local.ps1 or set the environment variable")
	}

	db, err := pgxpool.New(context.Background(), dbURI)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = db.Ping(ctx)
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

	storage := repository.NewStorageRepo(db)
	handlers := handler.NewHandlerStorage(storage, cfg.SecretKey)
	var worker *accrual.Worker
	if cfg.AccrualAddress != "" {
		client, err := accrual.NewClient(cfg.AccrualAddress, nil)
		if err != nil {
			return err
		}
		worker = accrual.NewWorker(storage, client, func(err error) { logger.Log.Error("poll accrual", zap.Error(err)) })
	}
	return startServer(handlers, cfg.Address, worker)
}

func startServer(h *handler.HandlerStorage, address string, worker *accrual.Worker) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	r := newRouter(h)
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	if worker != nil {
		go func() { defer close(workerDone); worker.Run(workerCtx) }()
	} else {
		close(workerDone)
	}
	defer func() { stopWorker(); <-workerDone }()

	server := &http.Server{Addr: address, Handler: r, ReadHeaderTimeout: 5 * time.Second}
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

func newRouter(h *handler.HandlerStorage) http.Handler {
	r := chi.NewRouter()
	r.Use(logger.RequestLogger)

	r.Route("/api", func(r chi.Router) {
		r.Route("/user", func(r chi.Router) {
			r.With(middleware.AllowContentType("application/json")).Post("/register", h.Register)
			r.With(middleware.AllowContentType("application/json")).Post("/login", h.Auth)
			r.Post("/orders", h.PostOrder)
			r.Get("/orders", h.GetOrdersByUserID)
			r.Get("/balance", h.GetBalance)
			r.Post("/balance/withdraw", h.Withdraw)
			r.Get("/withdrawals", h.GetWithdrawals)
		})
	})

	return r
}
