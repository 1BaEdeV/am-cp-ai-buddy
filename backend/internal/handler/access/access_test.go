package access_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	accessservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
	accesshandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/access"
	authhandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/auth"
	"github.com/stretchr/testify/require"
)

const taskPath = "/api/v1/onboarding/tasks/working-accesses"

type accessStub func(context.Context, string) (accessservice.WorkingAccesses, error)

func (s accessStub) ListRequiredAccesses(ctx context.Context, employeeID string) (accessservice.WorkingAccesses, error) {
	return s(ctx, employeeID)
}

func newRouter(service accesshandler.AccessService) http.Handler {
	return handler.NewRouter(authhandler.NewAuthHandler(nil), accesshandler.NewAccessHandler(service), nil)
}

func TestWorkingAccessesTask(t *testing.T) {
	for _, tc := range []struct {
		completed  int
		inProgress int
		failed     int
		status     string
	}{
		{completed: 0, status: "not_started"},
		{completed: 2, status: "in_progress"},
		{completed: 4, status: "completed"},
		{inProgress: 1, status: "in_progress"},
		{failed: 1, status: "in_progress"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			accesses := accessservice.WorkingAccesses{
				Resources: []accessservice.RequiredResource{{
					ID: "gitlab", Service: "gitlab", Name: "GitLab", AccessLevel: "Developer",
					Purpose: "Работа с кодом", Status: accessservice.StatusRequestable, CanRequest: true,
				}},
				Progress: accessservice.Progress{Completed: tc.completed, Total: 4, InProgress: tc.inProgress, Failed: tc.failed},
			}
			request := httptest.NewRequest(http.MethodGet, taskPath, nil)
			request.Header.Set("X-Demo-Employee-Id", " intern-alina ")
			router := newRouter(accessStub(func(ctx context.Context, employeeID string) (accessservice.WorkingAccesses, error) {
				require.Equal(t, "intern-alina", employeeID)
				require.Equal(t, request.Context(), ctx)
				return accesses, nil
			}))
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
			require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			var task accesshandler.WorkingAccessesTask
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &task))
			require.Equal(t, "working-accesses", task.ID)
			require.Equal(t, "Рабочие доступы", task.Title)
			require.Equal(t, "Ресурсы команды", task.Subtitle)
			require.NotEmpty(t, task.Description)
			require.Equal(t, tc.status, task.Status)
			require.True(t, task.ChatEnabled)
			require.Equal(t, accesses, task.WorkingAccesses)
		})
	}
}

func TestWorkingAccessesRequiresIdentity(t *testing.T) {
	for _, employeeID := range []string{"", "  "} {
		request := httptest.NewRequest(http.MethodGet, taskPath, nil)
		request.Header.Set("X-Demo-Employee-Id", employeeID)
		recorder := httptest.NewRecorder()
		// Пустой сервис обнаружит ошибочный вызов до проверки заголовка.
		newRouter(nil).ServeHTTP(recorder, request)
		require.Equal(t, http.StatusUnauthorized, recorder.Code)
		require.Contains(t, recorder.Body.String(), "demo_identity_required")
	}
}

func TestWorkingAccessesServiceErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{err: auth.ErrEmployeeNotFound, status: 401, code: "demo_identity_required"},
		{err: auth.ErrForbidden, status: 403, code: "forbidden"},
		{err: auth.ErrProfileRequired, status: 403, code: "profile_required"},
		{err: accessservice.ErrRequirementsNotConfigured, status: 500, code: "internal_error"},
		{err: accessservice.ErrInvalidRequirement, status: 500, code: "internal_error"},
		{err: errors.New("secret database details"), status: 500, code: "internal_error"},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			router := newRouter(accessStub(func(context.Context, string) (accessservice.WorkingAccesses, error) {
				return accessservice.WorkingAccesses{}, fmt.Errorf("сервис: %w", tc.err)
			}))
			request := httptest.NewRequest(http.MethodGet, taskPath, nil)
			request.Header.Set("X-Demo-Employee-Id", "intern-alina")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, tc.status, recorder.Code)
			var body struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			require.Equal(t, tc.code, body.Code)
			require.NotEmpty(t, body.Message)
			require.NotContains(t, recorder.Body.String(), "secret database details")
		})
	}
}

func TestWorkingAccessesRouting(t *testing.T) {
	for _, tc := range []struct {
		method string
		path   string
		status int
	}{
		{method: http.MethodPost, path: taskPath, status: http.StatusMethodNotAllowed},
		{method: http.MethodGet, path: "/api/v1/onboarding/tasks/unknown", status: http.StatusNotFound},
	} {
		recorder := httptest.NewRecorder()
		newRouter(nil).ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.path, nil))
		require.Equal(t, tc.status, recorder.Code)
	}
}
