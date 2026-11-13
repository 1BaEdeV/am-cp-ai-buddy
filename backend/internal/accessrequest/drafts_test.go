package accessrequest

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareAccessRequestOutcomes(t *testing.T) {
	for _, outcome := range []PreparationStatus{PreparationReady, PreparationAlreadyGranted, PreparationRequestExists} {
		t.Run(string(outcome), func(t *testing.T) {
			s, mock := mockRequestService(t)
			mock.ExpectBegin()
			expectEmployee(mock, models.RoleIntern)
			expectProfile(mock)
			expectRequirement(mock)
			if outcome == PreparationAlreadyGranted {
				selectFrom(mock, "employee_accesses", "intern", "repo", 1).WillReturnRows(mockRow(map[string]driver.Value{
					"access_level": "Developer", "verified_at": unitNow,
				}))
			} else {
				expectNoAccess(mock)
				if outcome == PreparationRequestExists {
					selectFrom(mock, "access_requests").WillReturnRows(requestRow(unitRequest(models.AccessRequestFailed)))
				} else {
					expectNoActiveRequest(mock)
					mock.ExpectExec(`INSERT INTO "access_drafts"`).
						WithArgs(sqlmock.AnyArg(), "intern", "team", "position", "repo", "manager", "Backend", "gitlab", "42", "Reporter", "Работа с кодом", "Анна Иванова", unitNow, unitNow.Add(DraftTTL), nil).
						WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}
			mock.ExpectCommit()
			result, err := s.PrepareAccessRequest(context.Background(), "intern", " repo ")
			require.NoError(t, err)
			assert.Equal(t, outcome, result.Status)
			switch outcome {
			case PreparationReady:
				require.NotNil(t, result.Draft)
				assert.NotEmpty(t, result.Draft.ID)
				assert.Equal(t, "repo", result.Draft.ResourceID)
				assert.Equal(t, "manager", result.Draft.ApproverID)
				assert.Equal(t, parametersView(requirementParameters(unitRequirement())), result.Draft.Parameters)
				assert.Equal(t, unitNow, result.Draft.CreatedAt)
				assert.Equal(t, unitNow.Add(DraftTTL), result.Draft.ExpiresAt)
				assert.Nil(t, result.Request)
			case PreparationAlreadyGranted:
				assert.Nil(t, result.Draft)
				assert.Nil(t, result.Request)
			case PreparationRequestExists:
				assert.Nil(t, result.Draft)
				require.NotNil(t, result.Request)
				assert.Equal(t, "request", result.Request.ID)
				assert.Equal(t, models.AccessRequestFailed, result.Request.Status)
			}
		})
	}
}

func TestPrepareAccessRequestRollsBack(t *testing.T) {
	storageErr := errors.New("database unavailable")
	for _, stage := range []string{"employee", "profile", "requirement", "access", "active request", "insert draft"} {
		t.Run(stage, func(t *testing.T) {
			s, mock := mockRequestService(t)
			mock.ExpectBegin()
			setup := func() {
				if stage == "employee" {
					selectFrom(mock, "employees").WillReturnError(storageErr)
					return
				}
				expectEmployee(mock, models.RoleIntern)
				if stage == "profile" {
					selectFrom(mock, "profiles").WillReturnError(storageErr)
					return
				}
				expectProfile(mock)
				if stage == "requirement" {
					selectFrom(mock, "access_requirements").WillReturnError(storageErr)
					return
				}
				expectRequirement(mock)
				if stage == "access" {
					selectFrom(mock, "employee_accesses").WillReturnError(storageErr)
					return
				}
				expectNoAccess(mock)
				if stage == "active request" {
					selectFrom(mock, "access_requests").WillReturnError(storageErr)
					return
				}
				expectNoActiveRequest(mock)
				mock.ExpectExec(`INSERT INTO "access_drafts"`).WillReturnError(storageErr)
			}
			setup()
			mock.ExpectRollback()
			result, err := s.PrepareAccessRequest(context.Background(), "intern", "repo")
			require.ErrorIs(t, err, storageErr)
			assert.Empty(t, result)
		})
	}
}

func expectConfirmationDraft(mock sqlmock.Sqlmock, draft models.AccessDraft) {
	expectEmployee(mock, models.RoleIntern)
	selectFrom(mock, "access_drafts", "draft", "intern", 1).WillReturnRows(draftRow(draft))
	selectFrom(mock, "access_requests", "draft", "intern", 1).WillReturnRows(noRows())
	expectProfile(mock)
}

func TestConfirmAccessRequestCreatesPending(t *testing.T) {
	s, mock := mockRequestService(t)
	mock.ExpectBegin()
	draft := unitDraft()
	expectConfirmationDraft(mock, draft)
	expectRequirement(mock)
	expectNoAccess(mock)
	expectNoActiveRequest(mock)
	mock.ExpectQuery(`INSERT INTO "access_requests"`).
		WithArgs(sqlmock.AnyArg(), "draft", "intern", "team", "position", "repo", "manager", "Backend", "gitlab", "42", "Reporter", "Работа с кодом", "Анна Иванова", models.AccessRequestPending, unitNow, unitNow, nil, "", nil, "").
		WillReturnRows(sqlmock.NewRows([]string{"number"}).AddRow(123))
	mock.ExpectExec(`UPDATE "access_drafts" SET "confirmed_at"`).WithArgs(unitNow, "draft").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	result, created, err := s.ConfirmAccessRequest(context.Background(), "intern", " draft ")
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotEmpty(t, result.ID)
	assert.Equal(t, int64(123), result.Number)
	assert.Equal(t, models.AccessRequestPending, result.Status)
	assert.Equal(t, parametersView(draft.Parameters), result.Parameters)
	assert.Equal(t, "intern", result.EmployeeID)
	assert.Equal(t, "repo", result.ResourceID)
	assert.Nil(t, result.DecidedAt)
	assert.Nil(t, result.GrantedAt)
}

func TestConfirmAccessRequestRepeatReturnsExisting(t *testing.T) {
	s, mock := mockRequestService(t)
	mock.ExpectBegin()
	expectEmployee(mock, models.RoleIntern)
	draft := unitDraft()
	draft.ExpiresAt = unitNow.Add(-time.Hour)
	selectFrom(mock, "access_drafts").WillReturnRows(draftRow(draft))
	saved := unitRequest(models.AccessRequestGranted)
	selectFrom(mock, "access_requests", "draft", "intern", 1).WillReturnRows(requestRow(saved))
	mock.ExpectCommit()
	result, created, err := s.ConfirmAccessRequest(context.Background(), "intern", "draft")
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, requestView(saved), result, "повтор должен вернуть исходную заявку даже после истечения черновика")
}

func TestConfirmAccessRequestRejectsChangedDraft(t *testing.T) {
	for _, scenario := range []string{"expired", "expiry boundary", "confirmed", "team changed", "position changed", "approver changed", "parameters changed", "rule removed", "invalid rule", "already granted", "active request"} {
		t.Run(scenario, func(t *testing.T) {
			s, mock := mockRequestService(t)
			draft := unitDraft()
			want := ErrDraftChanged
			switch scenario {
			case "expired":
				draft.ExpiresAt, want = unitNow.Add(-time.Second), ErrDraftExpired
			case "expiry boundary":
				draft.ExpiresAt, want = unitNow, ErrDraftExpired
			case "confirmed":
				draft.ConfirmedAt = &unitNow
			case "team changed":
				draft.TeamID = "other"
			case "position changed":
				draft.PositionID = "other"
			case "approver changed":
				draft.ApproverID = "other"
			case "parameters changed":
				draft.Parameters.AccessLevel = "Developer"
			}
			mock.ExpectBegin()
			expectConfirmationDraft(mock, draft)
			switch scenario {
			case "rule removed":
				selectFrom(mock, "access_requirements").WillReturnRows(noRows())
			case "invalid rule":
				selectFrom(mock, "access_requirements").WillReturnRows(requirementRow(unitRequirement()))
				selectFrom(mock, "resources").WillReturnRows(noRows())
			case "approver changed", "parameters changed":
				expectRequirement(mock)
			case "already granted":
				expectRequirement(mock)
				selectFrom(mock, "employee_accesses").WillReturnRows(mockRow(map[string]driver.Value{"access_level": "Reporter", "verified_at": unitNow}))
				want = ErrAccessAlreadyGranted
			case "active request":
				expectRequirement(mock)
				expectNoAccess(mock)
				selectFrom(mock, "access_requests").WillReturnRows(requestRow(unitRequest(models.AccessRequestPending)))
				want = ErrActiveRequestExists
			}
			mock.ExpectRollback()
			result, created, err := s.ConfirmAccessRequest(context.Background(), "intern", "draft")
			require.ErrorIs(t, err, want)
			assert.False(t, created)
			assert.Empty(t, result)
		})
	}
}

func TestConfirmAccessRequestRejectsManager(t *testing.T) {
	s, mock := mockRequestService(t)
	mock.ExpectBegin()
	expectEmployee(mock, models.RoleManager)
	mock.ExpectRollback()
	_, created, err := s.ConfirmAccessRequest(context.Background(), "manager", "draft")
	require.ErrorIs(t, err, auth.ErrForbidden)
	assert.False(t, created)
}
