package demoprovider_test

import (
	"context"
	"crypto/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoprovider"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func postgresProvider(t *testing.T) (*demoprovider.Provider, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("ACCESS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("для проверки конкурирующих выдач задай ACCESS_TEST_DATABASE_URL")
	}
	config, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	config.ConnectTimeout = 5 * time.Second
	admin := stdlib.OpenDB(*config)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	schema := "provider_test_" + strings.ToLower(rand.Text())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = admin.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := admin.ExecContext(cleanupCtx, `DROP SCHEMA "`+schema+`" CASCADE`)
		require.NoError(t, err)
	})
	config.RuntimeParams["search_path"] = schema
	pool := stdlib.OpenDB(*config)
	pool.SetMaxOpenConns(4)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	seedProviderDB(t, db)
	return demoprovider.NewProvider(db), db
}

func TestPostgresConcurrentProviderOperations(t *testing.T) {
	for _, scenario := range []string{"same operation", "conflicting operation", "different levels"} {
		t.Run(scenario, func(t *testing.T) {
			p, db := postgresProvider(t)
			first := accessrequest.GrantTarget{EmployeeID: "intern", Service: "gitlab", ExternalID: "demo/gitlab", AccessLevel: "Reporter"}
			second, secondID := first, "operation"
			wantOperations := int64(1)
			switch scenario {
			case "conflicting operation":
				second.EmployeeID = "other"
			case "different levels":
				second.AccessLevel, secondID = "Developer", "upgrade"
				wantOperations = 2
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			start, results := make(chan struct{}), make(chan error, 2)
			go func() { <-start; results <- p.GrantAccess(ctx, "operation", first) }()
			go func() { <-start; results <- p.GrantAccess(ctx, secondID, second) }()
			close(start)
			var outcomes []error
			for range 2 {
				select {
				case err := <-results:
					outcomes = append(outcomes, err)
				case <-ctx.Done():
					t.Fatal("конкурирующие выдачи не завершились: ", ctx.Err())
				}
			}
			conflicts := 0
			for _, err := range outcomes {
				if err != nil && scenario == "conflicting operation" {
					require.ErrorIs(t, err, demoprovider.ErrOperationConflict)
					conflicts++
				} else {
					require.NoError(t, err)
				}
			}
			if scenario == "conflicting operation" {
				assert.Equal(t, 1, conflicts)
			} else {
				actual, err := p.CheckAccess(ctx, first)
				require.NoError(t, err)
				wantLevel := "Reporter"
				if scenario == "different levels" {
					wantLevel = "Developer"
				}
				assert.Equal(t, accessrequest.ProviderAccess{Granted: true, AccessLevel: wantLevel}, actual)
			}
			var count int64
			require.NoError(t, db.Model(&models.DemoProviderOperation{}).Count(&count).Error)
			assert.Equal(t, wantOperations, count)
		})
	}
}
