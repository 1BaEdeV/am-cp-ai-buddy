package accessrequest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	requestservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
	requesthandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/accessrequest"
	authhandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testProvider struct {
	actual  requestservice.ProviderAccess
	failure error
}

func (p *testProvider) CheckAccess(context.Context, requestservice.GrantTarget) (requestservice.ProviderAccess, error) {
	return p.actual, nil
}

func (p *testProvider) GrantAccess(_ context.Context, _ string, target requestservice.GrantTarget) error {
	if p.failure != nil {
		return p.failure
	}
	p.actual = requestservice.ProviderAccess{Granted: true, AccessLevel: target.AccessLevel}
	return nil
}

func integrationService(t *testing.T) (*requestservice.Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "http.db")+"?_foreign_keys=on"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	require.NoError(t, db.AutoMigrate(
		&models.Team{}, &models.Position{}, &models.Mentor{}, &models.Employee{}, &models.Profile{},
		&models.Resource{}, &models.AccessRequirement{}, &models.EmployeeAccess{},
		&models.AccessDraft{}, &models.AccessRequest{}, &models.AccessIssuanceJob{},
	))
	// SQLite выбирает autoIncrement-номер первичным ключом; для внешнего ключа
	// задания сохраняем уникальность строкового ID. В PostgreSQL ID уже primary key.
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX test_http_requests_id ON access_requests(id)").Error)
	require.NoError(t, db.Create(&models.Team{ID: "team", Name: "Команда"}).Error)
	require.NoError(t, db.Create(&models.Position{ID: "go", Name: "Разработчик"}).Error)
	require.NoError(t, db.Create(&[]models.Employee{
		{ID: "intern", Email: "intern@example.test", FirstName: "Стажёр", TeamID: "team", PositionID: "go", Role: models.RoleIntern},
		{ID: "other-intern", Email: "other-intern@example.test", FirstName: "Другой", TeamID: "team", PositionID: "go", Role: models.RoleIntern},
		{ID: "manager", Email: "manager@example.test", FirstName: "Руководитель", TeamID: "team", PositionID: "go", Role: models.RoleManager},
		{ID: "other-manager", Email: "other-manager@example.test", FirstName: "Другой", TeamID: "team", PositionID: "go", Role: models.RoleManager},
	}).Error)
	require.NoError(t, db.Create(&models.Profile{ID: "profile", EmployeeID: "intern"}).Error)
	require.NoError(t, db.Create(&models.Resource{ID: "repo", Name: "Репозиторий", Service: "gitlab", ExternalID: "team/repo", AccessLevels: []string{"Reporter", "Developer"}}).Error)
	require.NoError(t, db.Create(&models.AccessRequirement{ID: "rule", TeamID: "team", PositionID: "go", ResourceID: "repo", AccessLevel: "Developer", Purpose: "Работа с кодом", ApproverID: "manager"}).Error)
	return requestservice.NewService(db), db
}

const requestsPath = "/api/v1/access-requests"

func requestRouter(service requesthandler.RequestService) http.Handler {
	return handler.NewRouter(authhandler.NewAuthHandler(nil), nil, requesthandler.NewRequestHandler(service))
}

func send(router http.Handler, method, path, body, actor string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-Demo-Employee-Id", actor)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func checkAPIError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	require.Equal(t, status, w.Code, w.Body.String())
	var result struct{ Code, Message string }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Equal(t, code, result.Code)
	require.NotEmpty(t, result.Message)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
}
