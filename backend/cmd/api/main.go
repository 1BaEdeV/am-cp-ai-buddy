package main

import (
	"context"
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
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoseed"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
	accesshandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/access"
	requesthandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/accessrequest"
	authhandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/auth"
	chathandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/chat"
	mlhandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/mltools"
	onboardinghandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/onboarding"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/issuanceworker"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/mltools"
	onboardingservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/onboarding"
	demodata "github.com/1BaEdeV/am-cp-ai-buddy/backend/seeds/demo"
)

type applicationWorkers struct {
	chat     *chatservice.Service
	issuance interface{ Run(context.Context) }
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
	err = db.Migrate(migrationCtx, conn)
	cancelMigration()
	if err != nil {
		return err
	}
	slog.Info("таблицы приложения готовы")
	if cfg.DemoSeedOnStartup {
		seedCtx, cancelSeed := context.WithTimeout(ctx, time.Minute)
		counts, seedErr := demoseed.Load(seedCtx, conn, demodata.Files)
		cancelSeed()
		if seedErr != nil {
			return seedErr
		}
		slog.Info("демо-данные загружены", "counts", counts)
	}

	accesses := access.NewService(conn)
	requests := accessrequest.NewService(conn)
	provider := demoprovider.NewProvider(conn)
	issuance, err := issuanceworker.NewWorker(requests, provider, cfg.IssuancePollInterval, slog.Default())
	if err != nil {
		return err
	}
	onboarding := onboardingservice.NewServiceWithReaders(conn, accesses, requests)
	chat := chatservice.NewService(
		conn,
		chatservice.NewHTTPAgent(cfg.MLAgentURL, &http.Client{Timeout: 50 * time.Second}),
		chatservice.NewAppContextBuilder(onboarding, accesses, requests),
		requests,
	)

	var toolHandler handler.MLToolEndpoints
	if cfg.MLToolsToken != "" {
		toolHandler = mlhandler.NewHandler(mltools.NewService(conn, accesses, requests, provider), cfg.MLToolsToken)
	}
	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: handler.NewRouter(
			authhandler.NewAuthHandler(auth.NewService(conn)),
			onboardinghandler.NewOnboardingHandler(onboarding),
			chathandler.NewChatHandler(chat),
			accesshandler.NewAccessHandler(accesses),
			requesthandler.NewRequestHandler(requests),
			toolHandler,
		),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	slog.Info("HTTP-сервер запускается", "addr", cfg.HTTPAddr)
	return serve(ctx, server, applicationWorkers{chat: chat, issuance: issuance})
}
