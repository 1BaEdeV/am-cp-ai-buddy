package demoseed_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	appdb "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/db"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoprovider"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoseed"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	demodata "github.com/1BaEdeV/am-cp-ai-buddy/backend/seeds/demo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func database(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "seed.db")+"?_foreign_keys=on"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	require.NoError(t, appdb.Migrate(context.Background(), db))
	// SQLite делает автоинкрементный номер primary key; FK заданий нужен уникальный ID.
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX test_seed_requests_id ON access_requests(id)").Error)
	return db
}

func TestLoadDemoAndPreserveRuntimeState(t *testing.T) {
	db, ctx := database(t), context.Background()
	counts, err := demoseed.Load(ctx, db, demodata.Files)
	require.NoError(t, err)
	assert.Equal(t, 20, counts.Employees)
	assert.Equal(t, 11, counts.Profiles)
	assert.Equal(t, 2, counts.Accesses)
	login, err := auth.NewService(db).Login(ctx, "intern-alina")
	require.NoError(t, err)
	require.NotNil(t, login.Profile)
	assert.Equal(t, auth.LoginReady, login.Status)
	card, err := access.NewService(db).ListRequiredAccesses(ctx, "intern-alina")
	require.NoError(t, err)
	assert.Equal(t, access.Progress{Completed: 2, Total: 4}, card.Progress)
	provider := demoprovider.NewProvider(db)
	wiki := accessrequest.GrantTarget{EmployeeID: "intern-alina", Service: "wiki", ExternalID: "demo/onboarding", AccessLevel: "read"}
	actual, err := provider.CheckAccess(ctx, wiki)
	require.NoError(t, err)
	assert.Equal(t, accessrequest.ProviderAccess{Granted: true, AccessLevel: "read"}, actual)
	_, created, err := auth.NewService(db).Register(ctx, "intern-ivan", auth.RegistrationInput{PositionID: "position-frontend", MentorID: "mentor-oleg", InternshipStartDate: "2026-11-14"})
	require.NoError(t, err)
	assert.True(t, created)
	require.NoError(t, db.Model(&models.Employee{}).Where("id = ?", "intern-alina").Update("first_name", "Новое имя").Error)
	newDate := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Model(&models.Profile{}).Where("employee_id = ?", "intern-alina").Update("internship_start_date", newDate).Error)
	requests := accessrequest.NewService(db)
	preparation, err := requests.PrepareAccessRequest(ctx, "intern-alina", "gitlab-search-api")
	require.NoError(t, err)
	require.NotNil(t, preparation.Draft)
	request, _, err := requests.ConfirmAccessRequest(ctx, "intern-alina", preparation.Draft.ID)
	require.NoError(t, err)
	_, err = requests.DecideAccessRequest(ctx, "manager-dmitry", request.ID, accessrequest.DecisionInput{Decision: accessrequest.DecisionApprove})
	require.NoError(t, err)
	processed, err := requests.ProcessNextIssuance(ctx, provider)
	require.NoError(t, err)
	assert.True(t, processed)
	require.NoError(t, provider.SetFailure(ctx, wiki, demoprovider.FailureCheck))
	_, err = demoseed.Load(ctx, db, demodata.Files)
	require.NoError(t, err)
	card, err = access.NewService(db).ListRequiredAccesses(ctx, "intern-alina")
	require.NoError(t, err)
	assert.Equal(t, access.Progress{Completed: 3, Total: 4}, card.Progress)
	profile, err := auth.NewService(db).Me(ctx, "intern-alina")
	require.NoError(t, err)
	assert.Equal(t, "Новое имя", profile.FirstName)
	assert.Equal(t, "2026-11-01", profile.InternshipStartDate)
	_, err = auth.NewService(db).Me(ctx, "intern-ivan")
	require.NoError(t, err)
	saved, err := requests.GetAccessRequest(ctx, "intern-alina", request.ID)
	require.NoError(t, err)
	assert.Equal(t, models.AccessRequestGranted, saved.Status)
	_, err = provider.CheckAccess(ctx, wiki)
	require.ErrorIs(t, err, demoprovider.ErrCheckFailed)
	for _, tt := range []struct {
		model any
		count int64
	}{
		{&models.Employee{}, 20}, {&models.Profile{}, 12}, {&models.Resource{}, 10},
		{&models.EmployeeAccess{}, 3}, {&models.DemoProviderOperation{}, 1}, {&models.AccessRequest{}, 1},
	} {
		var count int64
		require.NoError(t, db.Model(tt.model).Count(&count).Error)
		assert.Equal(t, tt.count, count)
	}
}

func TestLoadRollsBackAllTablesOnFailure(t *testing.T) {
	for _, table := range []string{"resources", "demo_provider_accesses"} {
		t.Run(table, func(t *testing.T) {
			db, ctx := database(t), context.Background()
			require.NoError(t, db.Exec("CREATE TRIGGER fail_seed BEFORE INSERT ON "+table+" BEGIN SELECT RAISE(ABORT, 'seed write failure'); END").Error)
			counts, err := demoseed.Load(ctx, db, demodata.Files)
			require.ErrorContains(t, err, "seed write failure")
			assert.Empty(t, counts)
			for _, model := range []any{&models.Team{}, &models.Position{}, &models.Employee{}, &models.Profile{}, &models.Resource{}, &models.EmployeeAccess{}, &models.DemoProviderAccess{}} {
				var count int64
				require.NoError(t, db.Model(model).Count(&count).Error)
				assert.Zero(t, count)
			}
			require.NoError(t, db.Exec("DROP TRIGGER fail_seed").Error)
			_, err = demoseed.Load(ctx, db, demodata.Files)
			require.NoError(t, err)
		})
	}
}

func TestLoadDoesNotTransferRightsToChangedExternalResource(t *testing.T) {
	db, ctx := database(t), context.Background()
	_, err := demoseed.Load(ctx, db, demodata.Files)
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.Resource{}).Where("id = ?", "team-wiki").Update("external_id", "another-wiki").Error)
	_, err = demoseed.Load(ctx, db, demodata.Files)
	require.ErrorIs(t, err, demoseed.ErrInvalidData)
	var count int64
	require.NoError(t, db.Model(&models.DemoProviderAccess{}).Where("external_id = ?", "another-wiki").Count(&count).Error)
	assert.Zero(t, count)
}
