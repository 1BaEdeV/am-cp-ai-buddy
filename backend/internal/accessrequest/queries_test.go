package accessrequest

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAccessRequestVisibility(t *testing.T) {
	for _, role := range []models.EmployeeRole{models.RoleIntern, models.RoleManager} {
		t.Run(string(role), func(t *testing.T) {
			s, mock := mockRequestService(t)
			mock.ExpectBegin()
			expectEmployee(mock, role)
			ownerColumn := "employee_id"
			if role == models.RoleManager {
				ownerColumn = "approver_id"
			}
			saved := unitRequest(models.AccessRequestGranted)
			mock.ExpectQuery(`SELECT .* FROM "access_requests" WHERE `+ownerColumn+` = \$1 AND id = \$2`).
				WithArgs(unitEmployee(role).ID, "request", 1).WillReturnRows(requestRow(saved))
			mock.ExpectCommit()
			result, err := s.GetAccessRequest(context.Background(), unitEmployee(role).ID, " request ")
			require.NoError(t, err)
			assert.Equal(t, requestView(saved), result)
		})
	}
}

func TestGetAccessRequestFailures(t *testing.T) {
	storageErr := errors.New("read unavailable")
	for _, stage := range []string{"employee", "missing or foreign request", "request"} {
		t.Run(stage, func(t *testing.T) {
			s, mock := mockRequestService(t)
			mock.ExpectBegin()
			want := storageErr
			if stage == "employee" {
				selectFrom(mock, "employees").WillReturnError(storageErr)
			} else {
				expectEmployee(mock, models.RoleIntern)
				q := selectFrom(mock, "access_requests", "intern", "request", 1)
				if stage == "missing or foreign request" {
					q.WillReturnRows(noRows())
					want = ErrRequestNotFound
				} else {
					q.WillReturnError(storageErr)
				}
			}
			mock.ExpectRollback()
			result, err := s.GetAccessRequest(context.Background(), "intern", "request")
			require.ErrorIs(t, err, want)
			assert.Empty(t, result)
		})
	}
}

func TestListAccessRequestsFiltersAndCounts(t *testing.T) {
	for _, scenario := range []string{"all", "active", "completed", "status and resource", "manager ignores filters", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			s, mock := mockRequestService(t)
			mock.ExpectBegin()
			role := models.RoleIntern
			filter := RequestFilter{}
			ownerColumn := "employee_id"
			args := []driver.Value{"intern"}
			groups := sqlmock.NewRows([]string{"status", "count"}).
				AddRow("pending", 2).AddRow("issuing", 1).AddRow("granted", 3).AddRow("rejected", 2).AddRow("failed", 1)
			wantCounts := RequestCounts{Total: 9, Active: 4, Completed: 5, Pending: 2, Issuing: 1, Granted: 3, Rejected: 2, Failed: 1}
			saved := unitRequest(models.AccessRequestPending)
			countArgs := append([]driver.Value{}, args...)
			switch scenario {
			case "active":
				filter.Group = " active "
				countArgs = append(countArgs, models.AccessRequestPending, models.AccessRequestIssuing, models.AccessRequestFailed)
			case "completed":
				filter.Group = "completed"
				countArgs = append(countArgs, models.AccessRequestGranted, models.AccessRequestRejected)
				saved.Status = models.AccessRequestGranted
			case "status and resource":
				filter.Status, filter.ResourceID, filter.Limit, filter.Offset = models.AccessRequestPending, " repo ", 10, 5
				countArgs = append(countArgs, models.AccessRequestPending, "repo")
			case "manager ignores filters":
				role, ownerColumn = models.RoleManager, "approver_id"
				filter = RequestFilter{Group: "invalid", Status: "invalid", ResourceID: "other"}
				args = []driver.Value{"manager", models.AccessRequestPending}
				countArgs = append([]driver.Value{}, args...)
				groups = sqlmock.NewRows([]string{"status", "count"}).AddRow("pending", 2)
				wantCounts = RequestCounts{Total: 2, Active: 2, Pending: 2}
			case "empty":
				groups = sqlmock.NewRows([]string{"status", "count"})
				wantCounts = RequestCounts{}
			}
			expectEmployee(mock, role)
			mock.ExpectQuery(`SELECT status, count\(\*\) AS count FROM "access_requests" WHERE ` + ownerColumn + ` = \$1`).WithArgs(args...).WillReturnRows(groups)
			var total int64 = 1
			if scenario == "empty" {
				total = 0
			}
			mock.ExpectQuery(`SELECT count\(\*\) FROM "access_requests" WHERE ` + ownerColumn + ` = \$1`).WithArgs(countArgs...).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(total))
			limit := filter.Limit
			if limit == 0 {
				limit = 20
			}
			findArgs := append(countArgs, limit)
			if filter.Offset > 0 {
				findArgs = append(findArgs, filter.Offset)
			}
			rows := requestRow(saved)
			if scenario == "empty" {
				rows = noRows()
			}
			mock.ExpectQuery(`SELECT .* FROM "access_requests" WHERE ` + ownerColumn + ` = \$1`).WithArgs(findArgs...).WillReturnRows(rows)
			mock.ExpectCommit()
			result, err := s.ListAccessRequests(context.Background(), unitEmployee(role).ID, filter)
			require.NoError(t, err)
			assert.Equal(t, wantCounts, result.Counts)
			assert.Equal(t, total, result.Total)
			assert.Equal(t, limit, result.Limit)
			assert.Equal(t, filter.Offset, result.Offset)
			require.NotNil(t, result.Items)
			if total == 0 {
				assert.Empty(t, result.Items)
			} else {
				require.Len(t, result.Items, 1)
				assert.Equal(t, requestView(saved), result.Items[0])
			}
		})
	}
}

func TestListAccessRequestsFailures(t *testing.T) {
	storageErr := errors.New("query failed")
	for _, stage := range []string{"employee", "invalid group", "invalid status", "counts", "total", "items"} {
		t.Run(stage, func(t *testing.T) {
			s, mock := mockRequestService(t)
			mock.ExpectBegin()
			filter := RequestFilter{}
			want := storageErr
			setup := func() {
				if stage == "employee" {
					selectFrom(mock, "employees").WillReturnError(storageErr)
					return
				}
				expectEmployee(mock, models.RoleIntern)
				if stage == "invalid group" || stage == "invalid status" {
					want = ErrInvalidInput
					if stage == "invalid group" {
						filter.Group = "unknown"
					} else {
						filter.Status = "unknown"
					}
					return
				}
				q := mock.ExpectQuery(`SELECT status, count\(\*\) AS count`)
				if stage == "counts" {
					q.WillReturnError(storageErr)
					return
				}
				q.WillReturnRows(sqlmock.NewRows([]string{"status", "count"}).AddRow("pending", 1))
				q = mock.ExpectQuery(`SELECT count\(\*\)`)
				if stage == "total" {
					q.WillReturnError(storageErr)
					return
				}
				q.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				selectFrom(mock, "access_requests").WillReturnError(storageErr)
			}
			setup()
			mock.ExpectRollback()
			result, err := s.ListAccessRequests(context.Background(), "intern", filter)
			require.ErrorIs(t, err, want)
			assert.Empty(t, result)
		})
	}
}
