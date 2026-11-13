package accessrequest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	accessservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	requestservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type providerStub struct {
	actual         requestservice.ProviderAccess
	check          func(context.Context, requestservice.GrantTarget) (requestservice.ProviderAccess, error)
	grant          func(context.Context, string, requestservice.GrantTarget) error
	checks, grants int
	operationID    string
	target         requestservice.GrantTarget
}

func (p *providerStub) CheckAccess(ctx context.Context, target requestservice.GrantTarget) (requestservice.ProviderAccess, error) {
	p.checks++
	p.target = target
	if p.check != nil {
		return p.check(ctx, target)
	}
	return p.actual, nil
}

func (p *providerStub) GrantAccess(ctx context.Context, operationID string, target requestservice.GrantTarget) error {
	p.grants++
	p.operationID, p.target = operationID, target
	if p.grant != nil {
		return p.grant(ctx, operationID, target)
	}
	p.actual = requestservice.ProviderAccess{Granted: true, AccessLevel: target.AccessLevel}
	return nil
}

func approvedRequest(t *testing.T, s *requestservice.Service) requestservice.Request {
	t.Helper()
	request := pendingRequest(t, s, "gitlab")
	approved, err := s.DecideAccessRequest(context.Background(), "manager", request.ID, requestservice.DecisionInput{Decision: requestservice.DecisionApprove})
	require.NoError(t, err)
	return approved
}

func TestApprovedRequestIsIssuedAndUpdatesProgress(t *testing.T) {
	s, db := requestService(t)
	// Существующие недостаточные права обновляются, вторая запись не создаётся.
	require.NoError(t, db.Create(&models.EmployeeAccess{ID: "lower", EmployeeID: "intern", ResourceID: "gitlab", AccessLevel: "Reporter", VerifiedAt: time.Now().UTC()}).Error)
	request := approvedRequest(t, s)
	card, err := accessservice.NewService(db).ListRequiredAccesses(context.Background(), "intern")
	require.NoError(t, err)
	require.Equal(t, accessservice.Progress{Completed: 2, Total: 4, InProgress: 1}, card.Progress)
	for _, resource := range card.Resources {
		if resource.ID == "gitlab" {
			require.Equal(t, accessservice.StatusIssuing, resource.Status)
			require.False(t, resource.CanRequest)
			require.Equal(t, request.ID, resource.Request.ID)
		}
	}
	// Переименование не меняет сохранённые параметры заявки и не мешает выдаче.
	require.NoError(t, db.Model(&models.Resource{}).Where("id = ?", "gitlab").Update("name", "Новое имя").Error)
	provider := &providerStub{actual: requestservice.ProviderAccess{Granted: true, AccessLevel: "Reporter"}}
	worked, err := s.ProcessNextIssuance(context.Background(), provider)
	require.NoError(t, err)
	require.True(t, worked)
	require.Equal(t, 1, provider.grants)
	require.Equal(t, 2, provider.checks)
	require.Equal(t, request.ID, provider.operationID)
	require.Equal(t, "demo/repo", provider.target.ExternalID)
	result, err := s.GetAccessRequest(context.Background(), "intern", request.ID)
	require.NoError(t, err)
	require.Equal(t, models.AccessRequestGranted, result.Status)
	require.NotNil(t, result.GrantedAt)
	var access models.EmployeeAccess
	require.NoError(t, db.Take(&access, "employee_id = ? AND resource_id = ?", "intern", "gitlab").Error)
	require.Equal(t, "lower", access.ID)
	require.Equal(t, "Developer", access.AccessLevel)
	var job models.AccessIssuanceJob
	require.NoError(t, db.Take(&job, "request_id = ?", request.ID).Error)
	require.Equal(t, 1, job.Attempts)
	require.NotNil(t, job.CompletedAt)
	require.Nil(t, job.LeaseUntil)
	card, err = accessservice.NewService(db).ListRequiredAccesses(context.Background(), "intern")
	require.NoError(t, err)
	require.Equal(t, accessservice.Progress{Completed: 3, Total: 4}, card.Progress)
	worked, err = s.ProcessNextIssuance(context.Background(), provider)
	require.NoError(t, err)
	require.False(t, worked)
	require.Equal(t, 1, provider.grants)
}

func TestIssuanceFailuresDoNotGrantAccess(t *testing.T) {
	for _, stage := range []string{"check", "grant", "unconfirmed"} {
		t.Run(stage, func(t *testing.T) {
			s, db := requestService(t)
			request := approvedRequest(t, s)
			provider := &providerStub{}
			failure := errors.New("секретный ответ провайдера")
			switch stage {
			case "check":
				provider.check = func(context.Context, requestservice.GrantTarget) (requestservice.ProviderAccess, error) {
					return requestservice.ProviderAccess{}, failure
				}
			case "grant":
				provider.grant = func(context.Context, string, requestservice.GrantTarget) error { return failure }
			case "unconfirmed":
				provider.grant = func(context.Context, string, requestservice.GrantTarget) error { return nil }
			}
			worked, err := s.ProcessNextIssuance(context.Background(), provider)
			require.True(t, worked)
			require.Error(t, err)
			if stage == "unconfirmed" {
				require.ErrorIs(t, err, requestservice.ErrIssuanceUnconfirmed)
			} else {
				require.ErrorIs(t, err, failure)
			}
			result, err := s.GetAccessRequest(context.Background(), "intern", request.ID)
			require.NoError(t, err)
			require.Equal(t, models.AccessRequestFailed, result.Status)
			require.NotEmpty(t, result.LastError)
			require.NotContains(t, result.LastError, "секретный")
			require.Nil(t, result.GrantedAt)
			var count int64
			require.NoError(t, db.Model(&models.EmployeeAccess{}).Where("employee_id = ? AND resource_id = ?", "intern", "gitlab").Count(&count).Error)
			require.Zero(t, count)
			card, err := accessservice.NewService(db).ListRequiredAccesses(context.Background(), "intern")
			require.NoError(t, err)
			require.Equal(t, accessservice.Progress{Completed: 2, Total: 4, Failed: 1}, card.Progress)
			preparation, err := s.PrepareAccessRequest(context.Background(), "intern", "gitlab")
			require.NoError(t, err)
			require.Equal(t, requestservice.PreparationRequestExists, preparation.Status)
			worked, err = s.ProcessNextIssuance(context.Background(), provider)
			require.NoError(t, err)
			require.False(t, worked, "ошибку повторяет руководитель, а не бесконечный цикл")
		})
	}
}

func TestChangedExternalResourceIsNotCountedAsGranted(t *testing.T) {
	for _, changeDuringIssuance := range []bool{false, true} {
		name := "до выдачи"
		if changeDuringIssuance {
			name = "во время выдачи"
		}
		t.Run(name, func(t *testing.T) {
			s, db := requestService(t)
			request := approvedRequest(t, s)
			provider := &providerStub{}
			remap := func() {
				require.NoError(t, db.Model(&models.Resource{}).Where("id = ?", "gitlab").Update("external_id", "another/repo").Error)
			}
			if changeDuringIssuance {
				provider.grant = func(_ context.Context, _ string, target requestservice.GrantTarget) error {
					provider.actual = requestservice.ProviderAccess{Granted: true, AccessLevel: target.AccessLevel}
					remap()
					return nil
				}
			} else {
				remap()
			}
			worked, err := s.ProcessNextIssuance(context.Background(), provider)
			require.True(t, worked)
			require.ErrorIs(t, err, requestservice.ErrIssuanceResourceChanged)
			if !changeDuringIssuance {
				require.Zero(t, provider.checks)
				require.Zero(t, provider.grants)
			}
			result, err := s.GetAccessRequest(context.Background(), "intern", request.ID)
			require.NoError(t, err)
			require.Equal(t, models.AccessRequestFailed, result.Status)
			require.NotEmpty(t, result.LastError)
			require.Nil(t, result.GrantedAt)
			var count int64
			require.NoError(t, db.Model(&models.EmployeeAccess{}).Where("employee_id = ? AND resource_id = ?", "intern", "gitlab").Count(&count).Error)
			require.Zero(t, count)
			card, err := accessservice.NewService(db).ListRequiredAccesses(context.Background(), "intern")
			require.NoError(t, err)
			require.Equal(t, accessservice.Progress{Completed: 2, Total: 4, Failed: 1}, card.Progress)

			// После восстановления исходного назначения можно продолжить прежнюю
			// заявку. Если права успели выдать, повторно выдавать их не нужно.
			require.NoError(t, db.Model(&models.Resource{}).Where("id = ?", "gitlab").Update("external_id", "demo/repo").Error)
			_, err = s.RetryAccessIssuance(context.Background(), "manager", request.ID)
			require.NoError(t, err)
			provider.grant = nil
			_, err = s.ProcessNextIssuance(context.Background(), provider)
			require.NoError(t, err)
			require.Equal(t, 1, provider.grants)
		})
	}
}

func TestRetryChecksUnknownPreviousOutcome(t *testing.T) {
	s, db := requestService(t)
	request := approvedRequest(t, s)
	provider := &providerStub{}
	provider.grant = func(_ context.Context, operationID string, target requestservice.GrantTarget) error {
		provider.actual = requestservice.ProviderAccess{Granted: true, AccessLevel: "Developer"}
		return context.DeadlineExceeded // Провайдер успел выдать доступ, ответ потерялся.
	}
	worked, err := s.ProcessNextIssuance(context.Background(), provider)
	require.True(t, worked)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	_, err = s.RetryAccessIssuance(context.Background(), "intern", request.ID)
	require.ErrorIs(t, err, auth.ErrForbidden)
	_, err = s.RetryAccessIssuance(context.Background(), "manager", request.ID)
	require.NoError(t, err)
	_, err = s.RetryAccessIssuance(context.Background(), "manager", request.ID)
	require.NoError(t, err)
	worked, err = s.ProcessNextIssuance(context.Background(), provider)
	require.NoError(t, err)
	require.True(t, worked)
	require.Equal(t, 1, provider.grants, "повтор сначала проверяет уже выданные права")
	require.Equal(t, request.ID, provider.operationID)
	result, err := s.RetryAccessIssuance(context.Background(), "manager", request.ID)
	require.NoError(t, err)
	require.Equal(t, models.AccessRequestGranted, result.Status)
	var job models.AccessIssuanceJob
	require.NoError(t, db.Take(&job, "request_id = ?", request.ID).Error)
	require.Equal(t, 2, job.Attempts)
}

func TestRetryKeepsOperationIDWhenGrantIsStillNeeded(t *testing.T) {
	s, _ := requestService(t)
	request := approvedRequest(t, s)
	provider := &providerStub{grant: func(context.Context, string, requestservice.GrantTarget) error { return errors.New("offline") }}
	_, err := s.ProcessNextIssuance(context.Background(), provider)
	require.Error(t, err)
	firstID := provider.operationID
	_, err = s.RetryAccessIssuance(context.Background(), "manager", request.ID)
	require.NoError(t, err)
	provider.grant = nil
	_, err = s.ProcessNextIssuance(context.Background(), provider)
	require.NoError(t, err)
	require.Equal(t, 2, provider.grants)
	require.Equal(t, firstID, provider.operationID)
}

func TestExpiredLeaseIsRecoveredAndOldWorkerCannotFinish(t *testing.T) {
	s, db := requestService(t)
	request := approvedRequest(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	started, release := make(chan struct{}, 1), make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	results := make(chan workerResult, 1)
	go func() {
		worked, err := s.ProcessNextIssuance(ctx, blockingProvider{started: started, release: release})
		results <- workerResult{worked, err}
	}()
	select {
	case <-started:
	case result := <-results:
		t.Fatalf("обработчик не дошёл до провайдера: %+v", result)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	worked, err := s.ProcessNextIssuance(ctx, &providerStub{})
	require.NoError(t, err)
	require.False(t, worked, "живую аренду другой обработчик не забирает")
	expireIssuanceLease(t, db, request.ID)
	provider := &providerStub{actual: requestservice.ProviderAccess{Granted: true, AccessLevel: "Developer"}}
	provider.check = func(context.Context, requestservice.GrantTarget) (requestservice.ProviderAccess, error) {
		close(release)
		released = true
		old := awaitWorker(t, ctx, results)
		require.True(t, old.worked)
		require.ErrorIs(t, old.err, requestservice.ErrIssuanceLeaseLost)
		return provider.actual, nil
	}
	worked, err = s.ProcessNextIssuance(ctx, provider)
	require.NoError(t, err)
	require.True(t, worked)
	require.Zero(t, provider.grants)
	var job models.AccessIssuanceJob
	require.NoError(t, db.Take(&job, "request_id = ?", request.ID).Error)
	require.Equal(t, 2, job.Attempts)
	require.NotNil(t, job.CompletedAt)
}

func TestIssuanceResultIsAtomic(t *testing.T) {
	s, db := requestService(t)
	request := approvedRequest(t, s)
	failure := errors.New("база не записала доступ")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail_access", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "employee_accesses" {
			require.ErrorIs(t, tx.AddError(failure), failure)
		}
	}))
	provider := &providerStub{}
	worked, err := s.ProcessNextIssuance(context.Background(), provider)
	require.True(t, worked)
	require.ErrorIs(t, err, failure)
	result, err := s.GetAccessRequest(context.Background(), "intern", request.ID)
	require.NoError(t, err)
	require.Equal(t, models.AccessRequestIssuing, result.Status)
	require.Nil(t, result.GrantedAt)
	var job models.AccessIssuanceJob
	require.NoError(t, db.Take(&job, "request_id = ?", request.ID).Error)
	require.Nil(t, job.CompletedAt)
	require.NoError(t, db.Callback().Create().Remove("test:fail_access"))
	expireIssuanceLease(t, db, request.ID)
	worked, err = s.ProcessNextIssuance(context.Background(), provider)
	require.NoError(t, err)
	require.True(t, worked)
	require.Equal(t, 1, provider.grants, "после сбоя БД повтор проверяет внешний результат")
}

func TestCancelledProviderCallStillSavesFailure(t *testing.T) {
	s, _ := requestService(t)
	request := approvedRequest(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider := &providerStub{check: func(context.Context, requestservice.GrantTarget) (requestservice.ProviderAccess, error) {
		cancel()
		return requestservice.ProviderAccess{}, context.Canceled
	}}
	worked, err := s.ProcessNextIssuance(ctx, provider)
	require.True(t, worked)
	require.ErrorIs(t, err, context.Canceled)
	result, err := s.GetAccessRequest(context.Background(), "intern", request.ID)
	require.NoError(t, err)
	require.Equal(t, models.AccessRequestFailed, result.Status)
}
