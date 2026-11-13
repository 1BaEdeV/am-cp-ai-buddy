package access_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

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
