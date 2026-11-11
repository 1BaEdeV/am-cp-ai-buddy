package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Open создаёт пул PostgreSQL и проверяет соединение до запуска приложения.
func Open(ctx context.Context, dsn string) (*gorm.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("не задан DATABASE_URL")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		return nil, fmt.Errorf("открыть PostgreSQL: %w", err)
	}
	pool, err := conn.DB()
	if err != nil {
		return nil, fmt.Errorf("получить пул PostgreSQL: %w", err)
	}
	pool.SetMaxOpenConns(20)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(30 * time.Minute)
	pool.SetConnMaxIdleTime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		return nil, errors.Join(fmt.Errorf("подключиться к PostgreSQL: %w", err), pool.Close())
	}
	return conn, nil
}
