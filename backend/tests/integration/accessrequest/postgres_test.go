package accessrequest_test

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	requestservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Каждый тест работает в собственной схеме. Нужна тестовая PostgreSQL
// с разрешением CREATE SCHEMA; существующие таблицы приложения не затрагиваются.
func postgresRequestService(t *testing.T) (*requestservice.Service, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("ACCESS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("для проверки блокировок PostgreSQL задай ACCESS_TEST_DATABASE_URL")
	}
	config, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	config.ConnectTimeout = 5 * time.Second
	adminPool := stdlib.OpenDB(*config)
	t.Cleanup(func() { require.NoError(t, adminPool.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	schema := "access_test_" + strings.ToLower(rand.Text())
	_, err = adminPool.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := adminPool.ExecContext(ctx, `DROP SCHEMA "`+schema+`" CASCADE`)
		require.NoError(t, err)
	})
	config.RuntimeParams["search_path"] = schema
	pool := stdlib.OpenDB(*config)
	pool.SetMaxOpenConns(8)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	seedAccessDB(t, db)
	// Повторная миграция должна сохранять sequence и ограничения.
	require.NoError(t, db.AutoMigrate(&models.AccessDraft{}, &models.AccessRequest{}, &models.AccessIssuanceJob{}))
	return requestservice.NewService(db), db
}

func TestPostgresParallelConfirmation(t *testing.T) {
	for _, sameDraft := range []bool{true, false} {
		name := "разные черновики"
		if sameDraft {
			name = "один черновик"
		}
		t.Run(name, func(t *testing.T) {
			s, db := postgresRequestService(t)
			first := preparedDraft(t, s, "gitlab")
			second := first
			if !sameDraft {
				second = preparedDraft(t, s, "gitlab")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			type outcome struct {
				request requestservice.Request
				created bool
				err     error
			}
			results := make(chan outcome, 2)
			start := make(chan struct{})
			for _, draft := range []requestservice.Draft{first, second} {
				go func(id string) {
					<-start
					r, created, err := s.ConfirmAccessRequest(ctx, "intern", id)
					results <- outcome{r, created, err}
				}(draft.ID)
			}
			close(start)
			a, b := <-results, <-results
			created := 0
			for _, result := range []outcome{a, b} {
				if result.created {
					created++
				}
			}
			require.Equal(t, 1, created)
			if sameDraft {
				require.NoError(t, a.err)
				require.NoError(t, b.err)
				require.Equal(t, a.request.ID, b.request.ID)
				require.Positive(t, a.request.Number)
			} else {
				require.True(t, (a.err == nil && errors.Is(b.err, requestservice.ErrActiveRequestExists)) || (b.err == nil && errors.Is(a.err, requestservice.ErrActiveRequestExists)))
			}
			var count int64
			require.NoError(t, db.Model(&models.AccessRequest{}).Count(&count).Error)
			require.EqualValues(t, 1, count)
			next := pendingRequest(t, s, "grafana")
			require.Greater(t, next.Number, int64(1), "номер назначает PostgreSQL")
		})
	}
}

func TestPostgresParallelManagerDecisions(t *testing.T) {
	s, db := postgresRequestService(t)
	request := pendingRequest(t, s, "gitlab")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	results := make(chan error, 2)
	start := make(chan struct{})
	for _, decision := range []requestservice.Decision{requestservice.DecisionApprove, requestservice.DecisionReject} {
		go func(decision requestservice.Decision) {
			<-start
			_, err := s.DecideAccessRequest(ctx, "manager", request.ID, requestservice.DecisionInput{Decision: decision, Reason: "Причина"})
			results <- err
		}(decision)
	}
	close(start)
	a, b := <-results, <-results
	require.True(t, (a == nil && errors.Is(b, requestservice.ErrRequestAlreadyDecided)) || (b == nil && errors.Is(a, requestservice.ErrRequestAlreadyDecided)))
	saved, err := s.GetAccessRequest(ctx, "intern", request.ID)
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.Model(&models.AccessIssuanceJob{}).Count(&count).Error)
	if saved.Status == models.AccessRequestIssuing {
		require.EqualValues(t, 1, count)
	} else {
		require.Equal(t, models.AccessRequestRejected, saved.Status)
		require.Zero(t, count)
	}
}

func TestPostgresParallelWorkersClaimOnlyOnce(t *testing.T) {
	s, _ := postgresRequestService(t)
	approvedRequest(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	results := make(chan workerResult, 2)
	start := make(chan struct{})
	started, release := make(chan struct{}, 2), make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	provider := blockingProvider{started: started, release: release}
	for range 2 {
		go func() {
			<-start
			worked, err := s.ProcessNextIssuance(ctx, provider)
			results <- workerResult{worked, err}
		}()
	}
	close(start)
	select {
	case <-started:
	case result := <-results:
		t.Fatalf("обработчик не дошёл до провайдера: %+v", result)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Один обработчик удерживает задание и ждёт провайдера. Второй должен
	// вернуть отсутствие задания, не ожидая завершения первой выдачи.
	a := awaitWorker(t, ctx, results)
	require.NoError(t, a.err)
	require.False(t, a.worked)
	close(release)
	released = true
	b := awaitWorker(t, ctx, results)
	require.NoError(t, b.err)
	require.True(t, b.worked)
}
