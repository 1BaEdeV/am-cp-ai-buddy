package access_test

import (
	"context"
	"testing"
	"time"

	accessservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestListRequiredAccesses(t *testing.T) {
	db := testDB(t)
	result, err := accessservice.NewService(db).ListRequiredAccesses(context.Background(), " intern ")
	require.NoError(t, err)
	require.Equal(t, accessservice.Progress{Completed: 2, Total: 4}, result.Progress)
	require.Len(t, result.Resources, 4)
	require.Equal(t, "gitlab", result.Resources[0].ID)
	require.Equal(t, "grafana", result.Resources[1].ID)
	require.Equal(t, "wiki", result.Resources[2].ID)
	require.Equal(t, "youtrack", result.Resources[3].ID)
	for _, resource := range result.Resources {
		switch resource.ID {
		case "wiki", "youtrack":
			require.Equal(t, accessservice.StatusGranted, resource.Status)
			require.False(t, resource.CanRequest)
		default:
			require.Equal(t, accessservice.StatusRequestable, resource.Status)
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
		{name: "правила не настроены", employeeID: "intern", wantErr: accessservice.ErrRequirementsNotConfigured,
			prepare: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Where("team_id = ? AND position_id = ?", "search", "go").Delete(&models.AccessRequirement{}).Error)
			}},
		{name: "недопустимый уровень", employeeID: "intern", wantErr: accessservice.ErrInvalidRequirement,
			prepare: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Model(&models.AccessRequirement{}).Where("id = ?", "gitlab").Update("access_level", "Admin").Error)
			}},
		{name: "согласующий стажёр", employeeID: "intern", wantErr: accessservice.ErrInvalidRequirement,
			prepare: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Model(&models.AccessRequirement{}).Where("id = ?", "gitlab").Update("approver_id", "intern").Error)
			}},
		{name: "согласующий другой команды", employeeID: "intern", wantErr: accessservice.ErrInvalidRequirement,
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
			result, err := accessservice.NewService(db).ListRequiredAccesses(context.Background(), tc.employeeID)
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
			result, err := accessservice.NewService(db).ListRequiredAccesses(context.Background(), "intern")
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
	_, err := accessservice.NewService(db).ListRequiredAccesses(ctx, "intern")
	require.ErrorIs(t, err, context.Canceled)

	require.NoError(t, db.Migrator().DropTable(&models.EmployeeAccess{}))
	_, err = accessservice.NewService(db).ListRequiredAccesses(context.Background(), "intern")
	require.ErrorContains(t, err, "загрузить подтверждённые доступы")
}
