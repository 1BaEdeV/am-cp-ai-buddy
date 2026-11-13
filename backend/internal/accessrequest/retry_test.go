package accessrequest

import (
	"context"
	"errors"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryAccessIssuanceStates(t *testing.T) {
	for _, status := range []models.AccessRequestStatus{models.AccessRequestFailed, models.AccessRequestIssuing, models.AccessRequestGranted, models.AccessRequestPending, models.AccessRequestRejected} {
		t.Run(string(status), func(t *testing.T) {
			s, mock := mockRequestService(t)
			mock.ExpectBegin()
			expectEmployee(mock, models.RoleManager)
			saved := unitRequest(status)
			if status == models.AccessRequestFailed {
				saved.LastError = "Провайдер недоступен"
			}
			selectFrom(mock, "access_requests", "request", "manager", 1).WillReturnRows(requestRow(saved))
			if status == models.AccessRequestFailed {
				mock.ExpectExec(`UPDATE "access_issuance_jobs".*WHERE request_id = \$5 AND completed_at IS NULL`).
					WithArgs(unitNow, "", nil, unitNow, "request").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`UPDATE "access_requests"`).WithArgs("", models.AccessRequestIssuing, unitNow, "request").WillReturnResult(sqlmock.NewResult(0, 1))
			}
			invalid := status == models.AccessRequestPending || status == models.AccessRequestRejected
			if invalid {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			result, err := s.RetryAccessIssuance(context.Background(), "manager", " request ")
			if invalid {
				require.ErrorIs(t, err, ErrInvalidRequestState)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				if status == models.AccessRequestFailed {
					saved.Status, saved.LastError, saved.UpdatedAt = models.AccessRequestIssuing, "", unitNow
				}
				assert.Equal(t, requestView(saved), result)
			}
		})
	}
}

func TestRetryAccessIssuanceFailures(t *testing.T) {
	storageErr := errors.New("retry storage failed")
	for _, stage := range []string{"employee", "intern", "missing", "read", "job update", "missing job", "request update"} {
		t.Run(stage, func(t *testing.T) {
			s, mock := mockRequestService(t)
			mock.ExpectBegin()
			want := storageErr
			setup := func() {
				if stage == "employee" {
					selectFrom(mock, "employees").WillReturnError(storageErr)
					return
				}
				if stage == "intern" {
					expectEmployee(mock, models.RoleIntern)
					want = auth.ErrForbidden
					return
				}
				expectEmployee(mock, models.RoleManager)
				q := selectFrom(mock, "access_requests")
				if stage == "missing" {
					q.WillReturnRows(noRows())
					want = ErrRequestNotFound
					return
				}
				if stage == "read" {
					q.WillReturnError(storageErr)
					return
				}
				q.WillReturnRows(requestRow(unitRequest(models.AccessRequestFailed)))
				update := mock.ExpectExec(`UPDATE "access_issuance_jobs"`)
				switch stage {
				case "job update":
					update.WillReturnError(storageErr)
				case "missing job":
					update.WillReturnResult(sqlmock.NewResult(0, 0))
				default:
					update.WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectExec(`UPDATE "access_requests"`).WillReturnError(storageErr)
				}
			}
			setup()
			mock.ExpectRollback()
			actor := "manager"
			if stage == "intern" {
				actor = "intern"
			}
			result, err := s.RetryAccessIssuance(context.Background(), actor, "request")
			if stage == "missing job" {
				require.ErrorContains(t, err, "отсутствует незавершённое задание")
			} else {
				require.ErrorIs(t, err, want)
			}
			assert.Empty(t, result)
		})
	}
}
