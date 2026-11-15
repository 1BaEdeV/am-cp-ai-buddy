package mltools_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	appdb "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/db"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoprovider"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoseed"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
	accesshandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/access"
	requesthandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/accessrequest"
	authhandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/auth"
	mlhandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/mltools"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/issuanceworker"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/mltools"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	demodata "github.com/1BaEdeV/am-cp-ai-buddy/backend/seeds/demo"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/tests/integration/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const serviceToken = "integration-ml-service-token"

func system(t *testing.T, enableTools bool) (http.Handler, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ml.db")+"?_foreign_keys=on"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	return systemWithDatabase(t, enableTools, db)
}

func systemWithDatabase(t *testing.T, enableTools bool, db *gorm.DB) (http.Handler, *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, appdb.Migrate(ctx, db))
	if db.Name() == "sqlite" {
		require.NoError(t, db.Exec("CREATE UNIQUE INDEX test_ml_requests_id ON access_requests(id)").Error)
	}
	_, err := demoseed.Load(ctx, db, demodata.Files)
	require.NoError(t, err)
	accesses, requests, provider := access.NewService(db), accessrequest.NewService(db), demoprovider.NewProvider(db)
	var tools handler.MLToolEndpoints
	if enableTools {
		tools = mlhandler.NewHandler(mltools.NewService(db, accesses, requests, provider), serviceToken)
	}
	worker, err := issuanceworker.NewWorker(requests, provider, 5*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(workerCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("фоновый процесс не завершился")
		}
	})
	return handler.NewRouter(
		authhandler.NewAuthHandler(auth.NewService(db)),
		nil,
		nil,
		accesshandler.NewAccessHandler(accesses),
		requesthandler.NewRequestHandler(requests),
		tools,
	), db
}

func send(router http.Handler, method, path, body, employee, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if employee != "" {
		r.Header.Set("X-Demo-Employee-Id", employee)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func tool(router http.Handler, name, body, employee string) *httptest.ResponseRecorder {
	return send(router, http.MethodPost, "/api/v1/internal/ml/tools/"+name, body, employee, serviceToken)
}

func TestMLToolsWithSeededDataAndBackgroundIssuance(t *testing.T) {
	router, db := system(t, true)
	checkMLLifecycle(t, router, db)
}

func TestPostgresMLToolsWithSeededDataAndBackgroundIssuance(t *testing.T) {
	router, db := systemWithDatabase(t, true, testdb.Postgres(t))
	checkMLLifecycle(t, router, db)
}

func checkMLLifecycle(t *testing.T, router http.Handler, db *gorm.DB) {
	t.Helper()
	w := send(router, http.MethodGet, "/api/v1/internal/ml/tools", "", "", serviceToken)
	require.Equal(t, 200, w.Code)
	var definitions []mltools.Definition
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &definitions))
	require.Len(t, definitions, 4)
	w = tool(router, "get_role_tools", `{}`, "intern-alina")
	require.Equal(t, 200, w.Code, w.Body.String())
	var card access.WorkingAccesses
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &card))
	assert.Equal(t, access.Progress{Completed: 2, Total: 4}, card.Progress)
	w = tool(router, "check_tool_access", `{"resourceId":"team-wiki"}`, "intern-alina")
	require.Equal(t, 200, w.Code, w.Body.String())
	var actual mltools.ToolAccess
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &actual))
	assert.True(t, actual.Granted)
	assert.True(t, actual.Sufficient)
	w = tool(router, "prepare_access_request", `{"resourceId":"team-wiki"}`, "intern-alina")
	require.Equal(t, 200, w.Code)
	var already accessrequest.Preparation
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &already))
	assert.Equal(t, accessrequest.PreparationAlreadyGranted, already.Status)
	assert.Nil(t, already.Draft)

	for i, resourceID := range []string{"gitlab-search-api", "grafana-search"} {
		body := `{"resourceId":"` + resourceID + `"}`
		w = tool(router, "check_tool_access", body, "intern-alina")
		require.Equal(t, 200, w.Code)
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &actual))
		assert.False(t, actual.Sufficient)
		w = tool(router, "prepare_access_request", body, "intern-alina")
		require.Equal(t, 200, w.Code, w.Body.String())
		var preparation accessrequest.Preparation
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preparation))
		require.NotNil(t, preparation.Draft)
		var count int64
		require.NoError(t, db.Model(&models.AccessRequest{}).Count(&count).Error)
		assert.EqualValues(t, i, count, "подготовка инструмента ещё не создаёт заявку")
		confirmPath := "/api/v1/access-requests/drafts/" + preparation.Draft.ID + "/confirm"
		w = send(router, http.MethodPost, confirmPath, `{"confirmed":true}`, "intern-alina", "")
		require.Equal(t, 201, w.Code, w.Body.String())
		var request accessrequest.Request
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &request))
		w = send(router, http.MethodPost, confirmPath, `{"confirmed":true}`, "intern-alina", "")
		require.Equal(t, 200, w.Code)
		w = tool(router, "get_access_request_status", `{"requestId":"`+request.ID+`"}`, "intern-ivan")
		require.Equal(t, 404, w.Code)
		w = send(router, http.MethodPost, "/api/v1/access-requests/"+request.ID+"/decision", `{"decision":"approve"}`, "manager-dmitry", "")
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Eventually(t, func() bool {
			response := tool(router, "get_access_request_status", `{"requestId":"`+request.ID+`"}`, "intern-alina")
			var result accessrequest.Request
			return response.Code == 200 && json.Unmarshal(response.Body.Bytes(), &result) == nil && result.Status == models.AccessRequestGranted
		}, 5*time.Second, 5*time.Millisecond)
		w = tool(router, "get_role_tools", `{}`, "intern-alina")
		require.Equal(t, 200, w.Code)
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &card))
		assert.Equal(t, 3+i, card.Progress.Completed)
	}
	_, err := demoseed.Load(context.Background(), db, demodata.Files)
	require.NoError(t, err)
	w = tool(router, "get_role_tools", `{}`, "intern-alina")
	require.Equal(t, 200, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &card))
	assert.Equal(t, access.Progress{Completed: 4, Total: 4}, card.Progress)
}

func TestMLRoutesEnforceServiceAndEmployeeBoundaries(t *testing.T) {
	router, _ := system(t, true)
	for _, tt := range []struct {
		name, body, employee, token string
		status                      int
	}{
		{"prepare_access_request", `{"resourceId":"gitlab-search-api"}`, "intern-alina", "", 401},
		{"prepare_access_request", `{"resourceId":"gitlab-search-api"}`, "intern-alina", "wrong", 401},
		{"get_role_tools", `{}`, "", serviceToken, 401},
		{"get_role_tools", `{}`, "unknown", serviceToken, 401},
		{"get_role_tools", `{}`, "manager-dmitry", serviceToken, 403},
		{"get_role_tools", `{}`, "intern-ivan", serviceToken, 403},
		{"prepare_access_request", `{"resourceId":"gitlab-search-api","employeeId":"manager-dmitry"}`, "intern-alina", serviceToken, 400},
		{"prepare_access_request", `{"resourceId":"gitlab-search-api","accessLevel":"Owner"}`, "intern-alina", serviceToken, 400},
		{"check_tool_access", `{"resourceId":"figma-design"}`, "intern-alina", serviceToken, 404},
		{"prepare_access_request", `{"resourceId":"figma-design"}`, "intern-alina", serviceToken, 404},
		{"confirm_access_request", `{}`, "intern-alina", serviceToken, 404},
		{"grant_access", `{}`, "intern-alina", serviceToken, 404},
	} {
		w := send(router, http.MethodPost, "/api/v1/internal/ml/tools/"+tt.name, tt.body, tt.employee, tt.token)
		assert.Equal(t, tt.status, w.Code, "%+v: %s", tt, w.Body.String())
	}
	assert.Equal(t, http.StatusMethodNotAllowed, send(router, http.MethodPost, "/api/v1/access-requests/drafts", `{"resourceId":"gitlab-search-api"}`, "intern-alina", "").Code)
	disabled, _ := system(t, false)
	assert.Equal(t, 404, tool(disabled, "get_role_tools", `{}`, "intern-alina").Code)
}

func TestPostgresRetryAfterLostProviderResponse(t *testing.T) {
	router, db := systemWithDatabase(t, true, testdb.Postgres(t))
	ctx := context.Background()
	provider := demoprovider.NewProvider(db)
	var resource models.Resource
	require.NoError(t, db.Take(&resource, "id = ?", "gitlab-search-api").Error)
	target := accessrequest.GrantTarget{EmployeeID: "intern-alina", Service: resource.Service, ExternalID: resource.ExternalID, AccessLevel: "Developer"}
	require.NoError(t, provider.SetFailure(ctx, target, demoprovider.FailureAfterGrant))
	w := tool(router, "prepare_access_request", `{"resourceId":"gitlab-search-api"}`, "intern-alina")
	require.Equal(t, 200, w.Code, w.Body.String())
	var preparation accessrequest.Preparation
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preparation))
	require.NotNil(t, preparation.Draft)
	w = send(router, http.MethodPost, "/api/v1/access-requests/drafts/"+preparation.Draft.ID+"/confirm", `{"confirmed":true}`, "intern-alina", "")
	require.Equal(t, 201, w.Code, w.Body.String())
	var request accessrequest.Request
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &request))
	w = send(router, http.MethodPost, "/api/v1/access-requests/"+request.ID+"/decision", `{"decision":"approve"}`, "manager-dmitry", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	awaitStatus := func(status models.AccessRequestStatus) {
		t.Helper()
		require.Eventually(t, func() bool {
			w := tool(router, "get_access_request_status", `{"requestId":"`+request.ID+`"}`, "intern-alina")
			var saved accessrequest.Request
			return w.Code == 200 && json.Unmarshal(w.Body.Bytes(), &saved) == nil && saved.Status == status
		}, 5*time.Second, 5*time.Millisecond)
	}
	awaitStatus(models.AccessRequestFailed)
	actual, err := provider.CheckAccess(ctx, target)
	require.NoError(t, err)
	assert.True(t, actual.Granted, "провайдер выдал права до потери ответа")
	card, err := access.NewService(db).ListRequiredAccesses(ctx, target.EmployeeID)
	require.NoError(t, err)
	assert.Equal(t, 2, card.Progress.Completed, "неподтверждённый результат ещё не увеличивает прогресс")
	w = tool(router, "prepare_access_request", `{"resourceId":"gitlab-search-api"}`, "intern-alina")
	require.Equal(t, 200, w.Code)
	var repeat accessrequest.Preparation
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &repeat))
	require.NotNil(t, repeat.Request)
	assert.Equal(t, accessrequest.PreparationRequestExists, repeat.Status)
	assert.Equal(t, request.ID, repeat.Request.ID)
	w = send(router, http.MethodPost, "/api/v1/access-requests/"+request.ID+"/retry", `{}`, "manager-dmitry", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	awaitStatus(models.AccessRequestGranted)
	card, err = access.NewService(db).ListRequiredAccesses(ctx, target.EmployeeID)
	require.NoError(t, err)
	assert.Equal(t, 3, card.Progress.Completed)
	var operations int64
	require.NoError(t, db.Model(&models.DemoProviderOperation{}).Where("id = ?", request.ID).Count(&operations).Error)
	assert.EqualValues(t, 1, operations, "повтор сначала проверяет права и не создаёт вторую выдачу")
	var job models.AccessIssuanceJob
	require.NoError(t, db.Take(&job, "request_id = ?", request.ID).Error)
	assert.Equal(t, 2, job.Attempts)
	assert.NotNil(t, job.CompletedAt)
}
