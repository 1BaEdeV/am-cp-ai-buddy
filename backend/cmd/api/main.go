package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	chatservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/chat"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/config"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/db"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoprovider"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
	accesshandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/access"
	requesthandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/accessrequest"
	authhandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/auth"
	chathandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/chat"
	onboardinghandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/onboarding"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/issuanceworker"
	onboardingservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/onboarding"
)

type applicationWorkers struct {
	chat     *chatservice.Service
	issuance interface {
		Run(context.Context)
	}
}

func (w applicationWorkers) Run(ctx context.Context) {
	var workers sync.WaitGroup
	workers.Add(2)

	go func() {
		defer workers.Done()
		w.chat.RunWorker(ctx, time.Second)
	}()

	go func() {
		defer workers.Done()
		w.issuance.Run(ctx)
	}()

	workers.Wait()
}

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

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
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
	err = db.Migrate(migrationCtx, conn)
	cancelMigration()
	if err != nil {
		return fmt.Errorf("подготовить таблицы приложения: %w", err)
	}
	slog.Info("таблицы приложения готовы")

	accesses := access.NewService(conn)
	requests := accessrequest.NewService(conn)
	issuance, err := issuanceworker.NewWorker(
		requests,
		demoprovider.NewProvider(conn),
		cfg.IssuancePollInterval,
		slog.Default(),
	)
	if err != nil {
		return err
	}

	onboardingService := onboardingservice.NewServiceWithReaders(conn, accesses, requests)
	chatService := chatservice.NewService(
		conn,
		chatservice.NewHTTPAgent(cfg.MLAgentURL, &http.Client{Timeout: 50 * time.Second}),
		chatservice.NewAppContextBuilder(onboardingService, accesses, requests),
		requests,
	)

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: handler.NewRouter(
			authhandler.NewAuthHandler(auth.NewService(conn)),
			onboardinghandler.NewOnboardingHandler(onboardingService),
			chathandler.NewChatHandler(chatService),
			accesshandler.NewAccessHandler(accesses),
			requesthandler.NewRequestHandler(requests),
		),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	slog.Info("HTTP-сервер запускается", "addr", cfg.HTTPAddr)
	return serve(ctx, server, applicationWorkers{
		chat:     chatService,
		issuance: issuance,
	})
}
