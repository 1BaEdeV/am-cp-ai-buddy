package demoprovider_test

import (
	"path/filepath"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoprovider"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func providerDB(t *testing.T) (*demoprovider.Provider, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "provider.db")+"?_foreign_keys=on"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	seedProviderDB(t, db)
	return demoprovider.NewProvider(db), db
}

func seedProviderDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(
		&models.Team{}, &models.Position{}, &models.Employee{}, &models.Mentor{}, &models.Profile{},
		&models.Resource{}, &models.AccessRequirement{}, &models.EmployeeAccess{},
		&models.AccessDraft{}, &models.AccessRequest{}, &models.AccessIssuanceJob{},
		&models.DemoProviderAccess{}, &models.DemoProviderOperation{},
	))
	if db.Name() == "sqlite" {
		// SQLite назначает autoIncrement-номеру заявки primary key; ID нужен для FK задания.
		require.NoError(t, db.Exec("CREATE UNIQUE INDEX test_provider_request_id ON access_requests(id)").Error)
	}
	require.NoError(t, db.Create(&models.Team{ID: "team", Name: "Поиск"}).Error)
	require.NoError(t, db.Create(&models.Position{ID: "position", Name: "Go-разработчик"}).Error)
	require.NoError(t, db.Create(&[]models.Employee{
		{ID: "intern", Email: "intern@example.test", Role: models.RoleIntern, TeamID: "team", PositionID: "position"},
		{ID: "other", Email: "other@example.test", Role: models.RoleIntern, TeamID: "team", PositionID: "position"},
		{ID: "manager", Email: "manager@example.test", FirstName: "Анна", SecondName: "Иванова", Role: models.RoleManager, TeamID: "team", PositionID: "position"},
	}).Error)
	require.NoError(t, db.Create(&models.Profile{ID: "profile", EmployeeID: "intern"}).Error)
	resources := []models.Resource{
		{ID: "wiki", Service: "wiki", AccessLevels: []string{"read"}},
		{ID: "youtrack", Service: "youtrack", AccessLevels: []string{"member"}},
		{ID: "gitlab", Service: "gitlab", AccessLevels: []string{"Reporter", "Developer"}},
		{ID: "grafana", Service: "grafana", AccessLevels: []string{"Viewer"}},
		{ID: "figma", Service: "figma", AccessLevels: []string{"Viewer", "Editor"}},
		{ID: "jupyterhub", Service: "jupyterhub", AccessLevels: []string{"user"}},
		{ID: "s3", Service: "s3", AccessLevels: []string{"read", "read_write"}},
		{ID: "postgres", Service: "postgres", AccessLevels: []string{"read"}},
		{ID: "metabase", Service: "metabase", AccessLevels: []string{"Viewer"}},
	}
	for i := range resources {
		resources[i].Name, resources[i].ExternalID = resources[i].ID, "demo/"+resources[i].ID
	}
	require.NoError(t, db.Create(&resources).Error)
	for _, r := range resources[:4] {
		require.NoError(t, db.Create(&models.AccessRequirement{
			ID: r.ID, TeamID: "team", PositionID: "position", ResourceID: r.ID,
			AccessLevel: r.AccessLevels[len(r.AccessLevels)-1], Purpose: "Учебные задачи", ApproverID: "manager",
		}).Error)
	}
}
