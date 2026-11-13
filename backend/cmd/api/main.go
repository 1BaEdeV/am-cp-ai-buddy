package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/config"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/db"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoprovider"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
	accesshandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/access"
	requesthandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/accessrequest"
	authhandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/issuanceworker"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
)

func main() {
	if err := run(); err != nil {
		slog.Error("сервер остановлен с ошибкой", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	conn, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	pool, err := conn.DB()
	if err != nil {
		return err
	}
	defer func() {
		if err := pool.Close(); err != nil {
			slog.Error("закрыть PostgreSQL", "error", err)
		}
	}()

	migrationCtx, cancelMigration := context.WithTimeout(ctx, time.Minute)
	err = conn.WithContext(migrationCtx).AutoMigrate(
		&models.Position{}, &models.Team{}, &models.Mentor{}, &models.Employee{}, &models.Profile{},
		&models.Resource{}, &models.AccessRequirement{}, &models.EmployeeAccess{},
		&models.AccessDraft{}, &models.AccessRequest{}, &models.AccessIssuanceJob{},
		&models.DemoProviderAccess{}, &models.DemoProviderOperation{},
	)
	cancelMigration()
	if err != nil {
		return fmt.Errorf("подготовить таблицы приложения: %w", err)
	}
	slog.Info("таблицы приложения готовы")

	requests := accessrequest.NewService(conn)
	worker, err := issuanceworker.NewWorker(requests, demoprovider.NewProvider(conn), cfg.IssuancePollInterval, slog.Default())
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: handler.NewRouter(
			authhandler.NewAuthHandler(auth.NewService(conn)),
			accesshandler.NewAccessHandler(access.NewService(conn)),
			requesthandler.NewRequestHandler(requests),
		),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	slog.Info("HTTP-сервер запускается", "addr", cfg.HTTPAddr)
	return serve(ctx, server, worker)
}
