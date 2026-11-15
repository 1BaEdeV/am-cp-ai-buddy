// Package testdb создаёт изолированные базы для интеграционных проверок.
package testdb

import (
	"context"
	"crypto/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Postgres выделяет тесту отдельную схему и удаляет её после закрытия пула.
// Подключение требует CREATE SCHEMA; таблицы приложения не затрагиваются.
func Postgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("ACCESS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("для проверки PostgreSQL задай ACCESS_TEST_DATABASE_URL")
	}
	config, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	config.ConnectTimeout = 5 * time.Second
	admin := stdlib.OpenDB(*config)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	schema := "integration_" + strings.ToLower(rand.Text())
	_, err = admin.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := admin.ExecContext(ctx, `DROP SCHEMA "`+schema+`" CASCADE`)
		require.NoError(t, err)
	})
	config.RuntimeParams["search_path"] = schema
	pool := stdlib.OpenDB(*config)
	pool.SetMaxOpenConns(8)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	return db
}
