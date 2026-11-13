package demoprovider_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoprovider"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/issuanceworker"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func startWorker(t *testing.T, service *accessrequest.Service, provider *demoprovider.Provider, db *gorm.DB) {
	t.Helper()
	pool, err := db.DB()
	require.NoError(t, err)
	// Конкуренцию блокировок проверяют PostgreSQL-тесты. Здесь SQLite использует
	// одно соединение для чтения статусов и работы фонового процесса.
	pool.SetMaxOpenConns(1)
	worker, err := issuanceworker.NewWorker(service, provider, 5*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("фоновый обработчик не остановился до закрытия БД")
		}
	})
}

func confirmResource(t *testing.T, service *accessrequest.Service, resourceID string) accessrequest.Request {
	t.Helper()
	ctx := context.Background()
	preparation, err := service.PrepareAccessRequest(ctx, "intern", resourceID)
	require.NoError(t, err)
	require.NotNil(t, preparation.Draft)
	request, created, err := service.ConfirmAccessRequest(ctx, "intern", preparation.Draft.ID)
	require.NoError(t, err)
	require.True(t, created)
	return request
}

func awaitStatus(t *testing.T, service *accessrequest.Service, id string, status models.AccessRequestStatus) accessrequest.Request {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.Eventually(t, func() bool {
		result, err := service.GetAccessRequest(ctx, "intern", id)
		return err == nil && result.Status == status
	}, 5*time.Second, 5*time.Millisecond, "заявка %s не перешла в %s", id, status)
	result, err := service.GetAccessRequest(ctx, "intern", id)
	require.NoError(t, err)
	return result
}

func TestBackgroundWorkerIssuesApprovalsAndContinuesAfterFailure(t *testing.T) {
	p, db := providerDB(t)
	ctx := context.Background()
	service := accessrequest.NewService(db)
	for resourceID, level := range map[string]string{"wiki": "read", "youtrack": "member"} {
		require.NoError(t, p.SeedAccess(ctx, accessrequest.GrantTarget{EmployeeID: "intern", Service: resourceID, ExternalID: "demo/" + resourceID, AccessLevel: level}))
		require.NoError(t, db.Create(&models.EmployeeAccess{ID: resourceID, EmployeeID: "intern", ResourceID: resourceID, AccessLevel: level, VerifiedAt: time.Now().UTC()}).Error)
	}
	gitlab, grafana := confirmResource(t, service, "gitlab"), confirmResource(t, service, "grafana")
	target := accessrequest.GrantTarget{EmployeeID: "intern", Service: "gitlab", ExternalID: "demo/gitlab", AccessLevel: "Developer"}
	require.NoError(t, p.SetFailure(ctx, target, demoprovider.FailureGrant))
	startWorker(t, service, p, db)
	for _, id := range []string{gitlab.ID, grafana.ID} {
		_, err := service.DecideAccessRequest(ctx, "manager", id, accessrequest.DecisionInput{Decision: accessrequest.DecisionApprove})
		require.NoError(t, err)
	}
	failed := awaitStatus(t, service, gitlab.ID, models.AccessRequestFailed)
	assert.NotEmpty(t, failed.LastError)
	granted := awaitStatus(t, service, grafana.ID, models.AccessRequestGranted)
	assert.NotNil(t, granted.GrantedAt)
	card, err := access.NewService(db).ListRequiredAccesses(ctx, "intern")
	require.NoError(t, err)
	assert.Equal(t, access.Progress{Completed: 3, Total: 4, Failed: 1}, card.Progress)

	// После исправления провайдера руководитель повторяет ту же заявку;
	// никаких прямых вызовов ProcessNextIssuance из теста нет.
	require.NoError(t, p.SetFailure(ctx, target, demoprovider.FailureNone))
	_, err = service.RetryAccessIssuance(ctx, "manager", gitlab.ID)
	require.NoError(t, err)
	awaitStatus(t, service, gitlab.ID, models.AccessRequestGranted)
	card, err = access.NewService(db).ListRequiredAccesses(ctx, "intern")
	require.NoError(t, err)
	assert.Equal(t, access.Progress{Completed: 4, Total: 4}, card.Progress)
	var operations, completed int64
	require.NoError(t, db.Model(&models.DemoProviderOperation{}).Count(&operations).Error)
	require.NoError(t, db.Model(&models.AccessIssuanceJob{}).Where("completed_at IS NOT NULL").Count(&completed).Error)
	assert.EqualValues(t, 2, operations)
	assert.EqualValues(t, 2, completed)
}

func TestBackgroundWorkerProcessesQueueSavedBeforeStartup(t *testing.T) {
	p, db := providerDB(t)
	ctx := context.Background()
	service := accessrequest.NewService(db)
	request := confirmResource(t, service, "gitlab")
	_, err := service.DecideAccessRequest(ctx, "manager", request.ID, accessrequest.DecisionInput{Decision: accessrequest.DecisionApprove})
	require.NoError(t, err)
	// Задание уже сохранено. При запуске новый сервис подхватывает очередь из БД.
	restarted := accessrequest.NewService(db)
	startWorker(t, restarted, demoprovider.NewProvider(db), db)
	granted := awaitStatus(t, restarted, request.ID, models.AccessRequestGranted)
	assert.NotNil(t, granted.GrantedAt)
	actual, err := p.CheckAccess(ctx, accessrequest.GrantTarget{EmployeeID: "intern", Service: "gitlab", ExternalID: "demo/gitlab", AccessLevel: "Developer"})
	require.NoError(t, err)
	assert.True(t, actual.Granted)
}
