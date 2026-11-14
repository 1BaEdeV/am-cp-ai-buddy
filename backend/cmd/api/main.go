package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	chatservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/chat"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/config"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/db"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
	authhandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/auth"
	chathandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/chat"
	onboardinghandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/onboarding"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	onboardingservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/onboarding"
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
	err = conn.WithContext(migrationCtx).AutoMigrate(
		&models.Position{},
		&models.Team{},
		&models.Mentor{},
		&models.Employee{},
		&models.Profile{},
		&models.TeamIntroduction{},
		&models.ChatMessage{},
		&models.ChatRun{},
	)
	cancelMigration()
	if err != nil {
		return fmt.Errorf("подготовить таблицы: %w", err)
	}
	slog.Info("таблицы базы данных готовы")

	chatService := chatservice.NewService(conn)

	authHTTPHandler := authhandler.NewAuthHandler(
		auth.NewService(conn),
	)
	onboardingHTTPHandler := onboardinghandler.NewOnboardingHandler(
		onboardingservice.NewService(conn),
	)
	chatHTTPHandler := chathandler.NewChatHandler(chatService)

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: handler.NewRouter(
			authHTTPHandler,
			onboardingHTTPHandler,
			chatHTTPHandler,
		),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}


	go chatService.RunWorker(ctx, time.Second)

	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("HTTP-сервер запускается", "addr", cfg.HTTPAddr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("запустить HTTP-сервер: %w", err)

	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return errors.Join(
				fmt.Errorf("остановить HTTP-сервер: %w", err),
				server.Close(),
			)
		}

		return nil
	}
}