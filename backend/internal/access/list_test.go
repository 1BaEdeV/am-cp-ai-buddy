package access

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func mockAccessService(t *testing.T) (*Service, sqlmock.Sqlmock) {
	t.Helper()
	conn, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mock.ExpectationsWereMet())
		mock.ExpectClose()
		assert.NoError(t, conn.Close())
	})
	return NewService(db), mock
}

func expectAccessEmployee(mock sqlmock.Sqlmock, role models.EmployeeRole) {
	mock.ExpectQuery(`SELECT .* FROM "employees"`).WithArgs("intern", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "team_id", "position_id", "role"}).AddRow("intern", "team", "position", role))
}

func expectAccessProfile(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT "id" FROM "profiles"`).WithArgs("intern", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("profile"))
}

func expectAccessRequirements(mock sqlmock.Sqlmock, count int, invalid bool) {
	requirements := sqlmock.NewRows([]string{"id", "resource_id", "approver_id", "access_level", "purpose"})
	resources := sqlmock.NewRows([]string{"id", "name", "service", "external_id", "access_levels"})
	ids := make([]driver.Value, count)
	for i := range count {
		id := fmt.Sprintf("repo-%d", i)
		ids[i] = id
		requirements.AddRow(fmt.Sprintf("rule-%d", i), id, "manager", "Reporter", "Работа с кодом")
		name := "Backend"
		if invalid {
			name = ""
		}
		resources.AddRow(id, name, "gitlab", fmt.Sprintf("%d", i), `["Reporter","Developer"]`)
	}
	mock.ExpectQuery(`SELECT .* FROM "access_requirements"`).WithArgs("team", "position").WillReturnRows(requirements)
	if count > 0 {
		mock.ExpectQuery(`SELECT .* FROM "employees"`).WithArgs("manager").
			WillReturnRows(sqlmock.NewRows([]string{"id", "role", "team_id"}).AddRow("manager", models.RoleManager, "team"))
		mock.ExpectQuery(`SELECT .* FROM "resources"`).WithArgs(ids...).WillReturnRows(resources)
	}
}

func TestListRequiredAccessesProgress(t *testing.T) {
	s, mock := mockAccessService(t)
	mock.ExpectBegin()
	expectAccessEmployee(mock, models.RoleIntern)
	expectAccessProfile(mock)
	expectAccessRequirements(mock, 6, false)
	verified := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT .* FROM "employee_accesses"`).
		WithArgs("intern", "repo-0", "repo-1", "repo-2", "repo-3", "repo-4", "repo-5").
		WillReturnRows(sqlmock.NewRows([]string{"resource_id", "access_level", "verified_at"}).
			AddRow("repo-0", "Developer", verified).
			AddRow("repo-4", "Reporter", time.Time{}).
			AddRow("repo-5", "Guest", verified))
	mock.ExpectQuery(`SELECT .* FROM "access_requests"`).
		WithArgs("intern", "repo-0", "repo-1", "repo-2", "repo-3", "repo-4", "repo-5", models.AccessRequestPending, models.AccessRequestIssuing, models.AccessRequestFailed).
		WillReturnRows(sqlmock.NewRows([]string{"id", "number", "resource_id", "status"}).
			AddRow("old-request", 10, "repo-0", models.AccessRequestPending).
			AddRow("pending", 11, "repo-1", models.AccessRequestPending).
			AddRow("issuing", 12, "repo-2", models.AccessRequestIssuing).
			AddRow("failed", 13, "repo-3", models.AccessRequestFailed))
	mock.ExpectCommit()

	result, err := s.ListRequiredAccesses(context.Background(), "  intern\t")
	require.NoError(t, err)
	assert.Equal(t, Progress{Total: 6, Completed: 1, InProgress: 2, Failed: 1}, result.Progress)
	require.Len(t, result.Resources, 6)
	for i, status := range []ResourceStatus{StatusGranted, StatusPending, StatusIssuing, StatusFailed, StatusRequestable, StatusRequestable} {
		resource := result.Resources[i]
		assert.Equal(t, status, resource.Status)
		assert.Equal(t, i >= 4, resource.CanRequest)
		assert.Equal(t, "Reporter", resource.AccessLevel)
		assert.Equal(t, "Работа с кодом", resource.Purpose)
		if i >= 1 && i <= 3 {
			require.NotNil(t, resource.Request)
			assert.Equal(t, int64(10+i), resource.Request.Number)
			assert.Equal(t, models.AccessRequestStatus(status), resource.Request.Status)
		} else {
			assert.Nil(t, resource.Request)
		}
	}
}

func TestListRequiredAccessesFailures(t *testing.T) {
	storageErr := errors.New("storage unavailable")
	for _, stage := range []string{"begin", "employee missing", "employee error", "manager", "profile missing", "profile error", "requirements error", "no requirements", "invalid requirement", "accesses error", "requests error", "commit"} {
		t.Run(stage, func(t *testing.T) {
			s, mock := mockAccessService(t)
			want := storageErr
			if stage == "begin" {
				mock.ExpectBegin().WillReturnError(storageErr)
			} else {
				mock.ExpectBegin()
				setup := func() {
					if stage == "employee missing" || stage == "employee error" {
						query := mock.ExpectQuery(`SELECT .* FROM "employees"`).WithArgs("intern", 1)
						if stage == "employee missing" {
							query.WillReturnRows(sqlmock.NewRows([]string{"id"}))
							want = auth.ErrEmployeeNotFound
						} else {
							query.WillReturnError(storageErr)
						}
						return
					}
					if stage == "manager" {
						expectAccessEmployee(mock, models.RoleManager)
						want = auth.ErrForbidden
						return
					}
					expectAccessEmployee(mock, models.RoleIntern)
					if stage == "profile missing" || stage == "profile error" {
						query := mock.ExpectQuery(`SELECT "id" FROM "profiles"`).WithArgs("intern", 1)
						if stage == "profile missing" {
							query.WillReturnRows(sqlmock.NewRows([]string{"id"}))
							want = auth.ErrProfileRequired
						} else {
							query.WillReturnError(storageErr)
						}
						return
					}
					expectAccessProfile(mock)
					if stage == "requirements error" {
						mock.ExpectQuery(`SELECT .* FROM "access_requirements"`).WillReturnError(storageErr)
						return
					}
					if stage == "no requirements" {
						expectAccessRequirements(mock, 0, false)
						want = ErrRequirementsNotConfigured
						return
					}
					expectAccessRequirements(mock, 1, stage == "invalid requirement")
					if stage == "invalid requirement" {
						want = ErrInvalidRequirement
						return
					}
					query := mock.ExpectQuery(`SELECT .* FROM "employee_accesses"`)
					if stage == "accesses error" {
						query.WillReturnError(storageErr)
						return
					}
					query.WillReturnRows(sqlmock.NewRows([]string{"id"}))
					query = mock.ExpectQuery(`SELECT .* FROM "access_requests"`)
					if stage == "requests error" {
						query.WillReturnError(storageErr)
						return
					}
					query.WillReturnRows(sqlmock.NewRows([]string{"id"}))
				}
				setup()
				if stage == "commit" {
					mock.ExpectCommit().WillReturnError(storageErr)
				} else {
					mock.ExpectRollback()
				}
			}
			result, err := s.ListRequiredAccesses(context.Background(), "intern")
			require.ErrorIs(t, err, want)
			assert.Empty(t, result, "при ошибке нельзя возвращать частично рассчитанный прогресс")
		})
	}
}
