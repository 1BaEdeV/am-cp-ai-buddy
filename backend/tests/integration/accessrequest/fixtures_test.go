package accessrequest_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	requestservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func expireDraft(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	createdAt := time.Now().UTC().Add(-2 * requestservice.DraftTTL)
	require.NoError(t, db.Model(&models.AccessDraft{}).Where("id = ?", id).
		Updates(map[string]any{"created_at": createdAt, "expires_at": createdAt.Add(requestservice.DraftTTL)}).Error)
}

func expireIssuanceLease(t *testing.T, db *gorm.DB, requestID string) {
	t.Helper()
	require.NoError(t, db.Model(&models.AccessIssuanceJob{}).Where("request_id = ?", requestID).
		Update("lease_until", time.Now().UTC().Add(-time.Second)).Error)
}

type workerResult struct {
	worked bool
	err    error
}

func awaitWorker(t *testing.T, ctx context.Context, results <-chan workerResult) workerResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-ctx.Done():
		t.Fatal("обработчик не завершился: ", ctx.Err())
		return workerResult{}
	}
}

type blockingProvider struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (p blockingProvider) CheckAccess(ctx context.Context, target requestservice.GrantTarget) (requestservice.ProviderAccess, error) {
	select {
	case p.started <- struct{}{}:
	case <-ctx.Done():
		return requestservice.ProviderAccess{}, ctx.Err()
	}
	select {
	case <-p.release:
		return requestservice.ProviderAccess{Granted: true, AccessLevel: target.AccessLevel}, nil
	case <-ctx.Done():
		return requestservice.ProviderAccess{}, ctx.Err()
	}
}

func (p blockingProvider) GrantAccess(context.Context, string, requestservice.GrantTarget) error {
	return requestservice.ErrIssuanceUnconfirmed // Проверка уже возвращает достаточные права; выдача не ожидается.
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "access.db")+"?_foreign_keys=on"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	seedAccessDB(t, db)
	return db
}

func seedAccessDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(
		&models.Team{}, &models.Position{}, &models.Mentor{}, &models.Employee{}, &models.Profile{},
		&models.Resource{}, &models.AccessRequirement{}, &models.EmployeeAccess{},
		&models.AccessDraft{}, &models.AccessRequest{}, &models.AccessIssuanceJob{},
	))
	// Драйвер SQLite делает autoIncrement-номер primary key вместо строкового ID.
	// Сохраняем уникальность ID для внешнего ключа задания; в PostgreSQL ID — PK.
	if db.Name() == "sqlite" {
		require.NoError(t, db.Exec("CREATE UNIQUE INDEX test_access_requests_id ON access_requests(id)").Error)
	}

	require.NoError(t, db.Create(&[]models.Team{{ID: "search", Name: "Поиск"}, {ID: "other", Name: "Другая команда"}}).Error)
	require.NoError(t, db.Create(&[]models.Position{{ID: "go", Name: "Go-разработчик"}, {ID: "qa", Name: "Тестировщик"}}).Error)
	require.NoError(t, db.Create(&[]models.Employee{
		{ID: "manager", Email: "manager@example.test", FirstName: "Дмитрий", SecondName: "Орлов", TeamID: "search", PositionID: "go", Role: models.RoleManager},
		{ID: "intern", Email: "intern@example.test", FirstName: "Алина", SecondName: "Смирнова", TeamID: "search", PositionID: "go", Role: models.RoleIntern},
		{ID: "other-intern", Email: "other@example.test", FirstName: "Иван", SecondName: "Петров", TeamID: "other", PositionID: "go", Role: models.RoleIntern},
	}).Error)
	require.NoError(t, db.Create(&models.Profile{ID: "profile", EmployeeID: "intern"}).Error)

	resources := []models.Resource{
		{ID: "wiki", Name: "Вики", Service: "wiki", ExternalID: "demo/wiki", AccessLevels: []string{"read"}},
		{ID: "youtrack", Name: "YouTrack", Service: "youtrack", ExternalID: "DEMO", AccessLevels: []string{"member"}},
		{ID: "gitlab", Name: "GitLab", Service: "gitlab", ExternalID: "demo/repo", AccessLevels: []string{"Reporter", "Developer"}},
		{ID: "grafana", Name: "Grafana", Service: "grafana", ExternalID: "demo", AccessLevels: []string{"Viewer"}},
	}
	require.NoError(t, db.Create(&resources).Error)
	for _, resource := range resources {
		requirement := models.AccessRequirement{
			ID: resource.ID, TeamID: "search", PositionID: "go", ResourceID: resource.ID,
			AccessLevel: resource.AccessLevels[len(resource.AccessLevels)-1], Purpose: "Для учебных задач", ApproverID: "manager",
		}
		require.NoError(t, db.Create(&requirement).Error)
	}
	// Эти правила помогают обнаружить выдачу ресурсов другой команды или должности.
	require.NoError(t, db.Create(&[]models.AccessRequirement{
		{ID: "other-team", TeamID: "other", PositionID: "go", ResourceID: "gitlab", AccessLevel: "Developer", Purpose: "Другой команды", ApproverID: "manager"},
		{ID: "other-position", TeamID: "search", PositionID: "qa", ResourceID: "gitlab", AccessLevel: "Reporter", Purpose: "Другой должности", ApproverID: "manager"},
	}).Error)
	require.NoError(t, db.Create(&[]models.EmployeeAccess{
		{ID: "wiki", EmployeeID: "intern", ResourceID: "wiki", AccessLevel: "read", VerifiedAt: time.Now()},
		{ID: "youtrack", EmployeeID: "intern", ResourceID: "youtrack", AccessLevel: "member", VerifiedAt: time.Now()},
		{ID: "other", EmployeeID: "other-intern", ResourceID: "gitlab", AccessLevel: "Developer", VerifiedAt: time.Now()},
	}).Error)
}

func requestService(t *testing.T) (*requestservice.Service, *gorm.DB) {
	t.Helper()
	db := testDB(t)
	s := requestservice.NewService(db)
	return s, db
}

func preparedDraft(t *testing.T, s *requestservice.Service, resourceID string) requestservice.Draft {
	t.Helper()
	preparation, err := s.PrepareAccessRequest(context.Background(), "intern", resourceID)
	require.NoError(t, err)
	require.Equal(t, requestservice.PreparationReady, preparation.Status)
	require.NotNil(t, preparation.Draft)
	require.Nil(t, preparation.Request)
	return *preparation.Draft
}

func pendingRequest(t *testing.T, s *requestservice.Service, resourceID string) requestservice.Request {
	t.Helper()
	draft := preparedDraft(t, s, resourceID)
	request, created, err := s.ConfirmAccessRequest(context.Background(), "intern", draft.ID)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, models.AccessRequestPending, request.Status)
	return request
}
