package main

import (
	"context"
	"flag"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/config"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/db"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoseed"
	demodata "github.com/1BaEdeV/am-cp-ai-buddy/backend/seeds/demo"
)

func main() {
	dir := flag.String("dir", "", "папка YAML-файлов; по умолчанию используются встроенные seeds/demo")
	flag.Parse()
	if err := run(*dir); err != nil {
		slog.Error("загрузить демо-данные", "error", err)
		os.Exit(1)
	}
}

func run(dir string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
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
	if err := db.Migrate(ctx, conn); err != nil {
		return err
	}
	var files fs.FS = demodata.Files
	if dir != "" {
		files = os.DirFS(dir)
	}
	counts, err := demoseed.Load(ctx, conn, files)
	if err != nil {
		return err
	}
	slog.Info("демо-данные загружены; существующие записи сохранены", "counts", counts)
	return nil
}
