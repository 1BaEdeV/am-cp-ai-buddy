package access

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
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
	require.NoError(t, db.AutoMigrate(
		&models.Team{}, &models.Position{}, &models.Mentor{}, &models.Employee{}, &models.Profile{},
		&models.Resource{}, &models.AccessRequirement{}, &models.EmployeeAccess{},
	))

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
	return db
}

func TestListRequiredAccesses(t *testing.T) {
	db := testDB(t)
	result, err := NewService(db).ListRequiredAccesses(context.Background(), " intern ")
	require.NoError(t, err)
	require.Equal(t, Progress{Completed: 2, Total: 4}, result.Progress)
	require.Len(t, result.Resources, 4)
	require.Equal(t, "gitlab", result.Resources[0].ID)
	require.Equal(t, "grafana", result.Resources[1].ID)
	require.Equal(t, "wiki", result.Resources[2].ID)
	require.Equal(t, "youtrack", result.Resources[3].ID)
	for _, resource := range result.Resources {
		switch resource.ID {
		case "wiki", "youtrack":
			require.Equal(t, StatusGranted, resource.Status)
			require.False(t, resource.CanRequest)
		default:
			require.Equal(t, StatusRequestable, resource.Status)
			require.True(t, resource.CanRequest)
		}
	}
}

func TestListRequiredAccessesRejectsInvalidState(t *testing.T) {
	cases := []struct {
		name       string
		employeeID string
		prepare    func(t *testing.T, db *gorm.DB)
		wantErr    error
	}{
		{name: "пустой ID", employeeID: " ", wantErr: auth.ErrEmployeeNotFound},
		{name: "сотрудник отсутствует", employeeID: "missing", wantErr: auth.ErrEmployeeNotFound},
		{name: "руководитель", employeeID: "manager", wantErr: auth.ErrForbidden},
		{name: "нет профиля", employeeID: "other-intern", wantErr: auth.ErrProfileRequired},
		{name: "правила не настроены", employeeID: "intern", wantErr: ErrRequirementsNotConfigured,
			prepare: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Where("team_id = ? AND position_id = ?", "search", "go").Delete(&models.AccessRequirement{}).Error)
			}},
		{name: "недопустимый уровень", employeeID: "intern", wantErr: ErrInvalidRequirement,
			prepare: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Model(&models.AccessRequirement{}).Where("id = ?", "gitlab").Update("access_level", "Admin").Error)
			}},
		{name: "согласующий стажёр", employeeID: "intern", wantErr: ErrInvalidRequirement,
			prepare: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Model(&models.AccessRequirement{}).Where("id = ?", "gitlab").Update("approver_id", "intern").Error)
			}},
		{name: "согласующий другой команды", employeeID: "intern", wantErr: ErrInvalidRequirement,
			prepare: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Model(&models.Employee{}).Where("id = ?", "manager").Update("team_id", "other").Error)
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := testDB(t)
			if tc.prepare != nil {
				tc.prepare(t, db)
			}
			result, err := NewService(db).ListRequiredAccesses(context.Background(), tc.employeeID)
			require.ErrorIs(t, err, tc.wantErr)
			require.Empty(t, result.Resources)
			require.Zero(t, result.Progress)
		})
	}
}

func TestListRequiredAccessesChecksConfirmedLevel(t *testing.T) {
	cases := []struct {
		name      string
		service   string
		required  string
		actual    string
		verified  bool
		completed int
	}{
		{name: "нужный уровень подтверждён", service: "gitlab", required: "Developer", actual: "Developer", verified: true, completed: 3},
		{name: "уровень недостаточен", service: "gitlab", required: "Developer", actual: "Reporter", verified: true, completed: 2},
		{name: "нет времени подтверждения", service: "gitlab", required: "Developer", actual: "Developer", completed: 2},
		{name: "Developer включает Reporter", service: "gitlab", required: "Reporter", actual: "Developer", verified: true, completed: 3},
		{name: "Editor включает Viewer", service: "figma", required: "Viewer", actual: "Editor", verified: true, completed: 3},
		{name: "read_write включает read", service: "s3", required: "read", actual: "read_write", verified: true, completed: 3},
		{name: "права одного сервиса не распространяются на другой", service: "wiki", required: "read", actual: "read_write", verified: true, completed: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := testDB(t)
			resource := models.Resource{ID: "gitlab", Name: "Ресурс", Service: tc.service, ExternalID: "demo/levels", AccessLevels: []string{tc.required, tc.actual}}
			require.NoError(t, db.Save(&resource).Error)
			require.NoError(t, db.Model(&models.AccessRequirement{}).Where("id = ?", "gitlab").Update("access_level", tc.required).Error)
			access := models.EmployeeAccess{ID: "new-access", EmployeeID: "intern", ResourceID: "gitlab", AccessLevel: tc.actual}
			if tc.verified {
				access.VerifiedAt = time.Now()
			}
			require.NoError(t, db.Create(&access).Error)
			result, err := NewService(db).ListRequiredAccesses(context.Background(), "intern")
			require.NoError(t, err)
			require.Equal(t, tc.completed, result.Progress.Completed)
			require.Equal(t, tc.required, result.Resources[0].AccessLevel)
		})
	}
}

func TestListRequiredAccessesPreservesErrors(t *testing.T) {
	db := testDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewService(db).ListRequiredAccesses(ctx, "intern")
	require.ErrorIs(t, err, context.Canceled)

	require.NoError(t, db.Migrator().DropTable(&models.EmployeeAccess{}))
	_, err = NewService(db).ListRequiredAccesses(context.Background(), "intern")
	require.ErrorContains(t, err, "загрузить подтверждённые доступы")
}
