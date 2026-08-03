package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/yourusername/traccar-billing/internal/api"
	"github.com/yourusername/traccar-billing/internal/billing"
	"github.com/yourusername/traccar-billing/internal/config"
	"github.com/yourusername/traccar-billing/internal/scheduler"
	"github.com/yourusername/traccar-billing/internal/storage"
	"github.com/yourusername/traccar-billing/internal/traccar"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repo, closeDB, err := openRepository(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer closeDB()

	if err := storage.RunMigrations(repoDB(repo), cfg.DBDriver); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	logger.Info("migrations applied")

	client := traccar.NewClient()

	sched := scheduler.New(repo, client, cfg.SyncInterval, logger)
	server := api.NewServer(repo, client, cfg.SessionSecret, logger)

	httpServer := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: server.Router(),
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := sched.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("scheduler stopped unexpectedly", "error", err)
		}
	}()

	go func() {
		defer wg.Done()
		logger.Info("http server listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server stopped unexpectedly", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown error", "error", err)
	}

	wg.Wait()
	logger.Info("shutdown complete")
	return nil
}

func openRepository(ctx context.Context, cfg config.Config) (billing.Repository, func(), error) {
	switch cfg.DBDriver {
	case "mysql":
		repo, err := storage.OpenMySQL(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, nil, err
		}
		return repo, func() { repo.Close() }, nil
	case "sqlite":
		repo, err := storage.OpenSQLite(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, nil, err
		}
		return repo, func() { repo.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("unsupported DB_DRIVER %q", cfg.DBDriver)
	}
}

func repoDB(repo billing.Repository) *sql.DB {
	type dber interface{ DB() *sql.DB }
	return repo.(dber).DB()
}
