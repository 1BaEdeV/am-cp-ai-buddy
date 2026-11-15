package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/config"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/db"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoseed"
)

func main() {
	if err := run(); err != nil {
		slog.Error("не удалось загрузить демо-данные", "error", err)
		os.Exit(1)
	}
}

func run() error {
	directory := flag.String("dir", "seeds/demo", "directory with demo YAML files")
	flag.Parse()
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
	migrationCtx, cancel := context.WithTimeout(ctx, time.Minute)
	err = db.Migrate(migrationCtx, conn)
	cancel()
	if err != nil {
		return fmt.Errorf("подготовить таблицы: %w", err)
	}
	if err := demoseed.Seed(ctx, conn, *directory); err != nil {
		return err
	}
	slog.Info("демо-данные загружены", "directory", *directory)
	return nil
}
