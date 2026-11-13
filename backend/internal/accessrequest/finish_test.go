package accessrequest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestFinishIssuanceRejectsLostLease(t *testing.T) {
	for _, scenario := range []string{"another worker", "no lease", "expired", "expiry boundary", "completed", "request not issuing"} {
		t.Run(scenario, func(t *testing.T) {
			s, mock := mockRequestService(t)
			job, saved := unitJob(), unitRequest(models.AccessRequestIssuing)
			want := ErrIssuanceLeaseLost
			switch scenario {
			case "another worker":
				job.LeaseToken = "another-lease"
			case "no lease":
				job.LeaseUntil = nil
			case "expired":
				expired := unitNow.Add(-time.Second)
				job.LeaseUntil = &expired
			case "expiry boundary":
				job.LeaseUntil = &unitNow
			case "completed":
				job.CompletedAt = &unitNow
			case "request not issuing":
				saved.Status, want = models.AccessRequestGranted, ErrInvalidRequestState
			}
			mock.ExpectBegin()
			expectEmployee(mock, models.RoleIntern)
			selectFrom(mock, "access_requests").WillReturnRows(requestRow(saved))
			selectFrom(mock, "access_issuance_jobs").WillReturnRows(jobRow(job))
			mock.ExpectRollback()
			err := s.finishIssuance(context.Background(), unitJob(), unitRequest(models.AccessRequestIssuing), ProviderAccess{Granted: true, AccessLevel: "Developer"}, nil, "")
			require.ErrorIs(t, err, want)
		})
	}
}

func TestFinishIssuanceResourceChangedDuringProviderCall(t *testing.T) {
	s, mock := mockRequestService(t)
	mock.ExpectBegin()
	expectEmployee(mock, models.RoleIntern)
	selectFrom(mock, "access_requests").WillReturnRows(requestRow(unitRequest(models.AccessRequestIssuing)))
	selectFrom(mock, "access_issuance_jobs").WillReturnRows(jobRow(unitJob()))
	resource := unitRequirement().Resource
	resource.ExternalID = "another-resource"
	selectFrom(mock, "resources").WillReturnRows(resourceRow(resource))
	mock.ExpectExec(`UPDATE "access_requests"`).WithArgs(resourceChangedMessage, models.AccessRequestFailed, unitNow, "request").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "access_issuance_jobs"`).WithArgs("", nil, unitNow, "request").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err := s.finishIssuance(context.Background(), unitJob(), unitRequest(models.AccessRequestIssuing), ProviderAccess{Granted: true, AccessLevel: "Developer"}, nil, "")
	require.ErrorIs(t, err, ErrIssuanceResourceChanged)
}

func TestFinishIssuanceStorageFailures(t *testing.T) {
	storageErr := errors.New("result storage failed")
	for _, stage := range []string{"employee", "request", "job", "resource", "access", "request update", "job update"} {
		t.Run(stage, func(t *testing.T) {
			s, mock := mockRequestService(t)
			mock.ExpectBegin()
			setup := func() {
				if stage == "employee" {
					selectFrom(mock, "employees").WillReturnError(storageErr)
					return
				}
				expectEmployee(mock, models.RoleIntern)
				q := selectFrom(mock, "access_requests")
				if stage == "request" {
					q.WillReturnError(storageErr)
					return
				}
				q.WillReturnRows(requestRow(unitRequest(models.AccessRequestIssuing)))
				q = selectFrom(mock, "access_issuance_jobs")
				if stage == "job" {
					q.WillReturnError(storageErr)
					return
				}
				q.WillReturnRows(jobRow(unitJob()))
				q = selectFrom(mock, "resources")
				if stage == "resource" {
					q.WillReturnError(storageErr)
					return
				}
				q.WillReturnRows(resourceRow(unitRequirement().Resource))
				update := mock.ExpectExec(`INSERT INTO "employee_accesses"`)
				if stage == "access" {
					update.WillReturnError(storageErr)
					return
				}
				update.WillReturnResult(sqlmock.NewResult(0, 1))
				update = mock.ExpectExec(`UPDATE "access_requests"`)
				if stage == "request update" {
					update.WillReturnError(storageErr)
					return
				}
				update.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`UPDATE "access_issuance_jobs"`).WillReturnError(storageErr)
			}
			setup()
			mock.ExpectRollback()
			err := s.finishIssuance(context.Background(), unitJob(), unitRequest(models.AccessRequestIssuing), ProviderAccess{Granted: true, AccessLevel: "Developer"}, nil, "")
			require.ErrorIs(t, err, storageErr)
		})
	}
}
