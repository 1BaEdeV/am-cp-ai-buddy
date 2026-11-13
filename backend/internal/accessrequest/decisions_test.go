package accessrequest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecideAccessRequestTransitions(t *testing.T) {
	for _, decision := range []Decision{DecisionApprove, DecisionReject} {
		t.Run(string(decision), func(t *testing.T) {
			s, mock := mockRequestService(t)
			mock.ExpectBegin()
			expectEmployee(mock, models.RoleManager)
			selectFrom(mock, "access_requests", "request", "manager", 1).WillReturnRows(requestRow(unitRequest(models.AccessRequestPending)))
			status := models.AccessRequestRejected
			if decision == DecisionApprove {
				status = models.AccessRequestIssuing
			}
			mock.ExpectExec(`UPDATE "access_requests"`).WithArgs(unitNow, "Причина", status, unitNow, "request", models.AccessRequestPending).WillReturnResult(sqlmock.NewResult(0, 1))
			if decision == DecisionApprove {
				mock.ExpectExec(`INSERT INTO "access_issuance_jobs"`).WithArgs("request", 0, unitNow, nil, "", nil, unitNow, unitNow).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectCommit()
			result, err := s.DecideAccessRequest(context.Background(), "manager", " request ", DecisionInput{Decision: decision, Reason: " Причина \n"})
			require.NoError(t, err)
			assert.Equal(t, status, result.Status)
			assert.Equal(t, "Причина", result.DecisionReason)
			require.NotNil(t, result.DecidedAt)
			assert.Equal(t, unitNow, *result.DecidedAt)
			assert.Equal(t, unitNow, result.UpdatedAt)
		})
	}
}

func TestDecideAccessRequestIdempotency(t *testing.T) {
	for _, status := range []models.AccessRequestStatus{models.AccessRequestRejected, models.AccessRequestIssuing, models.AccessRequestGranted, models.AccessRequestFailed} {
		for _, scenario := range []string{"same", "opposite", "different reason", "missing decision date"} {
			t.Run(string(status)+"/"+scenario, func(t *testing.T) {
				s, mock := mockRequestService(t)
				saved := unitRequest(status)
				saved.DecisionReason = "Причина"
				decision := DecisionApprove
				if status == models.AccessRequestRejected {
					decision = DecisionReject
				}
				input := DecisionInput{Decision: decision, Reason: "Причина"}
				switch scenario {
				case "opposite":
					if decision == DecisionApprove {
						input.Decision = DecisionReject
					} else {
						input.Decision = DecisionApprove
					}
				case "different reason":
					input.Reason = "Другая причина"
				case "missing decision date":
					saved.DecidedAt = nil
				}
				mock.ExpectBegin()
				expectEmployee(mock, models.RoleManager)
				selectFrom(mock, "access_requests").WillReturnRows(requestRow(saved))
				if scenario == "same" {
					mock.ExpectCommit()
				} else {
					mock.ExpectRollback()
				}
				result, err := s.DecideAccessRequest(context.Background(), "manager", "request", input)
				if scenario == "same" {
					require.NoError(t, err)
					assert.Equal(t, requestView(saved), result)
				} else {
					require.ErrorIs(t, err, ErrRequestAlreadyDecided)
					assert.Empty(t, result)
				}
			})
		}
	}
}

func TestDecideAccessRequestFailures(t *testing.T) {
	storageErr := errors.New("write failed")
	for _, stage := range []string{"employee", "intern", "missing", "read", "update", "concurrent decision", "job"} {
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
				q.WillReturnRows(requestRow(unitRequest(models.AccessRequestPending)))
				update := mock.ExpectExec(`UPDATE "access_requests"`)
				switch stage {
				case "update":
					update.WillReturnError(storageErr)
				case "concurrent decision":
					update.WillReturnResult(sqlmock.NewResult(0, 0))
					want = ErrRequestAlreadyDecided
				default:
					update.WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectExec(`INSERT INTO "access_issuance_jobs"`).WillReturnError(storageErr)
				}
			}
			setup()
			mock.ExpectRollback()
			actor := "manager"
			if stage == "intern" {
				actor = "intern"
			}
			result, err := s.DecideAccessRequest(context.Background(), actor, "request", DecisionInput{Decision: DecisionApprove})
			require.ErrorIs(t, err, want)
			assert.Empty(t, result)
		})
	}
}

func TestDecideAccessRequestReasonLimitCountsCharacters(t *testing.T) {
	s, mock := mockRequestService(t)
	storageErr := errors.New("employee lookup reached")
	mock.ExpectBegin()
	selectFrom(mock, "employees").WillReturnError(storageErr)
	mock.ExpectRollback()
	// 2000 русских символов занимают 4000 байт, но проходят ограничение длины.
	_, err := s.DecideAccessRequest(context.Background(), "manager", "request", DecisionInput{Decision: DecisionReject, Reason: strings.Repeat("я", 2000)})
	require.ErrorIs(t, err, storageErr)
}
