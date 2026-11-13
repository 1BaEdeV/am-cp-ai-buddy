package accessrequest

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureString struct{ value *string }

func (a captureString) Match(value driver.Value) bool {
	text, ok := value.(string)
	if ok {
		*a.value = text
	}
	return ok && text != ""
}

type unitProvider struct {
	check func(context.Context, GrantTarget) (ProviderAccess, error)
	grant func(context.Context, string, GrantTarget) error
}

func (p unitProvider) CheckAccess(ctx context.Context, target GrantTarget) (ProviderAccess, error) {
	return p.check(ctx, target)
}

func (p unitProvider) GrantAccess(ctx context.Context, id string, target GrantTarget) error {
	return p.grant(ctx, id, target)
}

func expectClaim(mock sqlmock.Sqlmock, token *string, resource models.Resource) {
	mock.ExpectBegin()
	selectFrom(mock, "access_issuance_jobs", unitNow, unitNow, models.AccessRequestIssuing, 1).WillReturnRows(jobRow(unitJob()))
	mock.ExpectExec(`UPDATE "access_issuance_jobs"`).
		WithArgs(2, captureString{token}, unitNow.Add(issuanceLease), unitNow, "request").WillReturnResult(sqlmock.NewResult(0, 1))
	selectFrom(mock, "access_requests", "request", 1).WillReturnRows(requestRow(unitRequest(models.AccessRequestIssuing)))
	selectFrom(mock, "resources", "repo").WillReturnRows(resourceRow(resource))
	mock.ExpectCommit()
}

func expectFinish(mock sqlmock.Sqlmock, token string, failed bool, publicError *string) {
	mock.ExpectBegin()
	expectEmployee(mock, models.RoleIntern)
	selectFrom(mock, "access_requests", "request", 1).WillReturnRows(requestRow(unitRequest(models.AccessRequestIssuing)))
	job := unitJob()
	job.LeaseToken = token
	selectFrom(mock, "access_issuance_jobs", "request", 1).WillReturnRows(jobRow(job))
	if !failed {
		selectFrom(mock, "resources", "repo", 1).WillReturnRows(resourceRow(unitRequirement().Resource))
		mock.ExpectExec(`INSERT INTO "employee_accesses"`).WithArgs(sqlmock.AnyArg(), "intern", "repo", "Developer", unitNow).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE "access_requests"`).WithArgs(unitNow, "", models.AccessRequestGranted, unitNow, "request").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE "access_issuance_jobs"`).WithArgs(unitNow, "", nil, unitNow, "request").WillReturnResult(sqlmock.NewResult(0, 1))
	} else {
		mock.ExpectExec(`UPDATE "access_requests"`).WithArgs(captureString{publicError}, models.AccessRequestFailed, unitNow, "request").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE "access_issuance_jobs"`).WithArgs("", nil, unitNow, "request").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
}

func TestProcessNextIssuanceProviderOutcomes(t *testing.T) {
	providerErr := errors.New("provider unavailable")
	for _, scenario := range []string{"already granted", "grant", "check error", "grant error", "verification error", "unconfirmed", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			s, mock := mockRequestService(t)
			var token, publicError string
			expectClaim(mock, &token, unitRequirement().Resource)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			checkCalls, grantCalls := 0, 0
			failed := scenario != "already granted" && scenario != "grant"
			provider := unitProvider{
				check: func(providerCtx context.Context, target GrantTarget) (ProviderAccess, error) {
					checkCalls++
					assert.Equal(t, GrantTarget{EmployeeID: "intern", Service: "gitlab", ExternalID: "42", AccessLevel: "Reporter"}, target)
					deadline, ok := providerCtx.Deadline()
					assert.True(t, ok)
					assert.WithinDuration(t, time.Now().Add(providerTimeout), deadline, time.Second)
					if checkCalls == 1 {
						require.NotEmpty(t, token)
						expectFinish(mock, token, failed, &publicError)
					}
					if scenario == "cancelled" {
						cancel()
						return ProviderAccess{}, providerCtx.Err()
					}
					if scenario == "check error" || (scenario == "verification error" && checkCalls == 2) {
						return ProviderAccess{}, providerErr
					}
					if scenario == "already granted" || (checkCalls == 2 && scenario == "grant") {
						return ProviderAccess{Granted: true, AccessLevel: "Developer"}, nil
					}
					return ProviderAccess{}, nil
				},
				grant: func(_ context.Context, operationID string, target GrantTarget) error {
					grantCalls++
					assert.Equal(t, "request", operationID, "повторная выдача использует ID той же заявки")
					assert.Equal(t, "42", target.ExternalID)
					if scenario == "grant error" {
						return providerErr
					}
					return nil
				},
			}
			processed, err := s.ProcessNextIssuance(ctx, provider)
			assert.True(t, processed)
			switch scenario {
			case "already granted", "grant":
				require.NoError(t, err)
			case "unconfirmed":
				require.ErrorIs(t, err, ErrIssuanceUnconfirmed)
			case "cancelled":
				require.ErrorIs(t, err, context.Canceled)
			default:
				require.ErrorIs(t, err, providerErr)
			}
			if failed {
				assert.NotEmpty(t, publicError, "ошибка должна сохраниться и при отмене исходного контекста")
			}
			if scenario == "already granted" || scenario == "check error" || scenario == "cancelled" {
				assert.Equal(t, 1, checkCalls)
				assert.Zero(t, grantCalls)
			} else {
				assert.Equal(t, 1, grantCalls)
				if scenario == "grant error" {
					assert.Equal(t, 1, checkCalls)
				} else {
					assert.Equal(t, 2, checkCalls)
				}
			}
		})
	}
}

func TestProcessNextIssuanceRejectsChangedResource(t *testing.T) {
	s, mock := mockRequestService(t)
	resource := unitRequirement().Resource
	resource.ExternalID = "another-repo"
	var token string
	expectClaim(mock, &token, resource)
	mock.ExpectBegin()
	expectEmployee(mock, models.RoleIntern)
	selectFrom(mock, "access_requests").WillReturnRows(requestRow(unitRequest(models.AccessRequestIssuing)))
	selectFrom(mock, "access_issuance_jobs").WillReturnRows(jobRow(models.AccessIssuanceJob{LeaseToken: "other-worker"}))
	mock.ExpectRollback()
	provider := unitProvider{
		check: func(context.Context, GrantTarget) (ProviderAccess, error) {
			t.Error("изменившийся ресурс нельзя проверять или выдавать")
			return ProviderAccess{}, nil
		},
		grant: func(context.Context, string, GrantTarget) error {
			t.Error("изменившийся ресурс нельзя выдавать")
			return nil
		},
	}
	processed, err := s.ProcessNextIssuance(context.Background(), provider)
	assert.True(t, processed)
	require.ErrorIs(t, err, ErrIssuanceResourceChanged)
	require.ErrorIs(t, err, ErrIssuanceLeaseLost, "ошибки провайдера и сохранения должны остаться доступны через errors.Is")
}

func TestClaimIssuanceOutcomes(t *testing.T) {
	storageErr := errors.New("queue unavailable")
	for _, stage := range []string{"empty", "select error", "lease error", "request error", "success"} {
		t.Run(stage, func(t *testing.T) {
			s, mock := mockRequestService(t)
			var token string
			if stage == "success" {
				expectClaim(mock, &token, unitRequirement().Resource)
			} else {
				mock.ExpectBegin()
				q := selectFrom(mock, "access_issuance_jobs", unitNow, unitNow, models.AccessRequestIssuing, 1)
				if stage == "empty" {
					q.WillReturnRows(noRows())
					mock.ExpectCommit()
				} else {
					if stage == "select error" {
						q.WillReturnError(storageErr)
					} else {
						q.WillReturnRows(jobRow(unitJob()))
						update := mock.ExpectExec(`UPDATE "access_issuance_jobs"`)
						if stage == "lease error" {
							update.WillReturnError(storageErr)
						} else {
							update.WillReturnResult(sqlmock.NewResult(0, 1))
							selectFrom(mock, "access_requests").WillReturnError(storageErr)
						}
					}
					mock.ExpectRollback()
				}
			}
			job, request, err := s.claimIssuance(context.Background())
			if stage == "success" {
				require.NoError(t, err)
				require.NotNil(t, job)
				assert.Equal(t, token, job.LeaseToken)
				assert.NotEqual(t, "lease", job.LeaseToken)
				assert.Equal(t, 2, job.Attempts)
				assert.Equal(t, unitNow.Add(issuanceLease), *job.LeaseUntil)
				assert.Equal(t, "request", request.ID)
				assert.Equal(t, unitRequirement().Resource, request.Resource)
			} else {
				assert.Nil(t, job)
				assert.Empty(t, request)
				if stage == "empty" {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, storageErr)
				}
			}
		})
	}
}

func TestProcessNextIssuanceDoesNotCallProviderWithoutJob(t *testing.T) {
	storageErr := errors.New("cannot claim job")
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty queue", true: "claim error"}[fail], func(t *testing.T) {
			s, mock := mockRequestService(t)
			mock.ExpectBegin()
			q := selectFrom(mock, "access_issuance_jobs")
			if fail {
				q.WillReturnError(storageErr)
				mock.ExpectRollback()
			} else {
				q.WillReturnRows(noRows())
				mock.ExpectCommit()
			}
			processed, err := s.ProcessNextIssuance(context.Background(), unitProvider{})
			assert.False(t, processed)
			if fail {
				require.ErrorIs(t, err, storageErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
