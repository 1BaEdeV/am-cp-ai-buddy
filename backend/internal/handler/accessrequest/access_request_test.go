package accessrequest_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	requestservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
	requesthandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/accessrequest"
	authhandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
)

const requestsPath = "/api/v1/access-requests"

type requestStub struct {
	confirm func(context.Context, string, string) (requestservice.Request, bool, error)
	list    func(context.Context, string, requestservice.RequestFilter) (requestservice.RequestList, error)
	get     func(context.Context, string, string) (requestservice.Request, error)
	decide  func(context.Context, string, string, requestservice.DecisionInput) (requestservice.Request, error)
	retry   func(context.Context, string, string) (requestservice.Request, error)
}

func (s requestStub) ConfirmAccessRequest(ctx context.Context, actor, id string) (requestservice.Request, bool, error) {
	return s.confirm(ctx, actor, id)
}
func (s requestStub) ListAccessRequests(ctx context.Context, actor string, filter requestservice.RequestFilter) (requestservice.RequestList, error) {
	return s.list(ctx, actor, filter)
}
func (s requestStub) GetAccessRequest(ctx context.Context, actor, id string) (requestservice.Request, error) {
	return s.get(ctx, actor, id)
}
func (s requestStub) DecideAccessRequest(ctx context.Context, actor, id string, input requestservice.DecisionInput) (requestservice.Request, error) {
	return s.decide(ctx, actor, id, input)
}
func (s requestStub) RetryAccessIssuance(ctx context.Context, actor, id string) (requestservice.Request, error) {
	return s.retry(ctx, actor, id)
}

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

func TestConfirmDraftCreatedAndRepeated(t *testing.T) {
	for _, created := range []bool{true, false} {
		t.Run(fmt.Sprint(created), func(t *testing.T) {
			expected := requestservice.Request{ID: "request-1", Number: 1, DraftID: "draft-1", Status: models.AccessRequestPending}
			r := httptest.NewRequest(http.MethodPost, requestsPath+"/drafts/draft-1/confirm", strings.NewReader(`{"confirmed":true}`))
			r.Header.Set("X-Demo-Employee-Id", " intern ")
			r.Header.Set("Content-Type", "application/json")
			router := requestRouter(requestStub{confirm: func(ctx context.Context, actor, id string) (requestservice.Request, bool, error) {
				require.Equal(t, r.Context(), ctx)
				require.Equal(t, "intern", actor)
				require.Equal(t, "draft-1", id)
				return expected, created, nil
			}})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			status := http.StatusOK
			if created {
				status = http.StatusCreated
				require.Equal(t, requestsPath+"/request-1", w.Header().Get("Location"))
			}
			require.Equal(t, status, w.Code)
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			var actual requestservice.Request
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &actual))
			require.Equal(t, expected, actual)
		})
	}
}

func TestRequestEndpointsForwardIdentityAndArguments(t *testing.T) {
	request := requestservice.Request{ID: "request-1", Number: 1, Status: models.AccessRequestIssuing}
	router := requestRouter(requestStub{
		get: func(_ context.Context, actor, id string) (requestservice.Request, error) {
			require.Equal(t, "intern", actor)
			require.Equal(t, request.ID, id)
			return request, nil
		},
		decide: func(_ context.Context, actor, id string, input requestservice.DecisionInput) (requestservice.Request, error) {
			require.Equal(t, "manager", actor)
			require.Equal(t, request.ID, id)
			require.Equal(t, requestservice.DecisionInput{Decision: requestservice.DecisionReject, Reason: "Причина"}, input)
			return request, nil
		},
		retry: func(_ context.Context, actor, id string) (requestservice.Request, error) {
			require.Equal(t, "manager", actor)
			require.Equal(t, request.ID, id)
			return request, nil
		},
	})
	for _, tc := range []struct{ method, path, body, actor string }{
		{http.MethodGet, requestsPath + "/request-1", "", " intern "},
		{http.MethodPost, requestsPath + "/request-1/decision", `{"decision":"reject","reason":"Причина"}`, " manager "},
		{http.MethodPost, requestsPath + "/request-1/retry", "", "manager"},
	} {
		w := send(router, tc.method, tc.path, tc.body, tc.actor)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var actual requestservice.Request
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &actual))
		require.Equal(t, request, actual)
	}
}

func TestListRequestsParsesFilters(t *testing.T) {
	for _, tc := range []struct {
		query  string
		filter requestservice.RequestFilter
	}{
		{"", requestservice.RequestFilter{}},
		{"?group=active&status=failed&resourceId=gitlab&limit=10&offset=20", requestservice.RequestFilter{
			Group: "active", Status: models.AccessRequestFailed, ResourceID: "gitlab", Limit: 10, Offset: 20,
		}},
	} {
		router := requestRouter(requestStub{list: func(_ context.Context, actor string, filter requestservice.RequestFilter) (requestservice.RequestList, error) {
			require.Equal(t, "intern", actor)
			require.Equal(t, tc.filter, filter)
			return requestservice.RequestList{Items: []requestservice.Request{}, Limit: 20}, nil
		}})
		w := send(router, http.MethodGet, requestsPath+tc.query, "", "intern")
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), `"items":[]`)
	}
}

func TestListRequestsRejectsMalformedQueries(t *testing.T) {
	router := requestRouter(requestStub{})
	for _, query := range []string{
		"?limit=abc", "?limit=", "?limit=0", "?limit=-1", "?limit=101", "?limit=9999999999999999999999999999999",
		"?offset=-1", "?offset=", "?offset=abc", "?limit=1&limit=2", "?employeeId=manager", "?offset=%zz", "?group=all;limit=1",
	} {
		t.Run(query, func(t *testing.T) {
			w := send(router, http.MethodGet, requestsPath+query, "", "intern")
			checkAPIError(t, w, http.StatusBadRequest, "invalid_input")
		})
	}
}

func TestRequestEndpointsRequireIdentity(t *testing.T) {
	router := requestRouter(requestStub{})
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, requestsPath + "/drafts/draft-1/confirm"},
		{http.MethodGet, requestsPath}, {http.MethodGet, requestsPath + "/request-1"},
		{http.MethodPost, requestsPath + "/request-1/decision"}, {http.MethodPost, requestsPath + "/request-1/retry"},
	} {
		w := send(router, tc.method, tc.path, `{}`, "  ")
		checkAPIError(t, w, http.StatusUnauthorized, "demo_identity_required")
	}
}

func TestRequestEndpointsRejectInvalidBodies(t *testing.T) {
	router := requestRouter(requestStub{})
	confirmPath := requestsPath + "/drafts/draft-1/confirm"
	for _, body := range []string{`{}`, `{"confirmed":false}`, `{"confirmed":null}`} {
		checkAPIError(t, send(router, http.MethodPost, confirmPath, body, "intern"), http.StatusBadRequest, "confirmation_required")
	}
	for _, tc := range []struct{ path, body string }{
		{confirmPath, `null`}, {confirmPath, `[]`}, {confirmPath, `"text"`}, {confirmPath, ``},
		{confirmPath, `{"confirmed":"true"}`}, {confirmPath, `{"confirmed":true} {}`},
		{confirmPath, `{"confirmed":true,"employeeId":"other"}`},
		{confirmPath, `{"confirmed":true,"accessLevel":"Owner"}`},
		{requestsPath + "/request-1/decision", `{"decision":"reject","comment":"старое поле"}`},
		{requestsPath + "/request-1/decision", `{"decision":"approve","approverId":"other"}`},
		{requestsPath + "/request-1/decision", `{"reason":42}`},
	} {
		checkAPIError(t, send(router, http.MethodPost, tc.path, tc.body, "intern"), http.StatusBadRequest, "invalid_input")
	}
	r := httptest.NewRequest(http.MethodPost, confirmPath, strings.NewReader(`{"confirmed":true}`))
	r.Header.Set("X-Demo-Employee-Id", "intern")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	checkAPIError(t, w, http.StatusUnsupportedMediaType, "unsupported_media_type")
	w = send(router, http.MethodPost, requestsPath+"/request-1/decision", `{"reason":"`+strings.Repeat("a", 1<<20)+`"}`, "manager")
	checkAPIError(t, w, http.StatusRequestEntityTooLarge, "request_too_large")
}

func TestRequestServiceErrorsHaveSafeHTTPResponses(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{auth.ErrEmployeeNotFound, 401, "demo_identity_required"}, {auth.ErrForbidden, 403, "forbidden"},
		{auth.ErrProfileRequired, 403, "profile_required"}, {requestservice.ErrInvalidInput, 400, "invalid_input"},
		{requestservice.ErrDraftNotFound, 404, "draft_not_found"}, {requestservice.ErrRequestNotFound, 404, "request_not_found"},
		{requestservice.ErrDraftExpired, 409, "draft_expired"}, {requestservice.ErrDraftChanged, 409, "draft_changed"},
		{requestservice.ErrAccessAlreadyGranted, 409, "access_already_granted"}, {requestservice.ErrActiveRequestExists, 409, "active_request_exists"},
		{requestservice.ErrRequestAlreadyDecided, 409, "request_already_decided"}, {requestservice.ErrInvalidRequestState, 409, "invalid_request_state"},
		{errors.New("secret database failure"), 500, "internal_error"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			failure := fmt.Errorf("secret service details: %w", tc.err)
			router := requestRouter(requestStub{
				confirm: func(context.Context, string, string) (requestservice.Request, bool, error) {
					return requestservice.Request{}, false, failure
				},
				get: func(context.Context, string, string) (requestservice.Request, error) {
					return requestservice.Request{}, failure
				},
				list: func(context.Context, string, requestservice.RequestFilter) (requestservice.RequestList, error) {
					return requestservice.RequestList{}, failure
				},
				decide: func(context.Context, string, string, requestservice.DecisionInput) (requestservice.Request, error) {
					return requestservice.Request{}, failure
				},
				retry: func(context.Context, string, string) (requestservice.Request, error) {
					return requestservice.Request{}, failure
				},
			})
			for _, endpoint := range []struct{ method, path, body string }{
				{http.MethodPost, requestsPath + "/drafts/draft-1/confirm", `{"confirmed":true}`},
				{http.MethodGet, requestsPath, ""}, {http.MethodGet, requestsPath + "/request-1", ""},
				{http.MethodPost, requestsPath + "/request-1/decision", `{"decision":"approve"}`},
				{http.MethodPost, requestsPath + "/request-1/retry", ""},
			} {
				w := send(router, endpoint.method, endpoint.path, endpoint.body, "intern")
				checkAPIError(t, w, tc.status, tc.code)
				require.NotContains(t, w.Body.String(), "secret")
			}
		})
	}
}

func TestRequestRoutesExposeOnlyUserActions(t *testing.T) {
	router := requestRouter(requestStub{})
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodPost, requestsPath, http.StatusMethodNotAllowed},
		{http.MethodPost, requestsPath + "/drafts", http.StatusMethodNotAllowed},
		{http.MethodGet, requestsPath + "/drafts/draft-1/confirm", http.StatusMethodNotAllowed},
		{http.MethodGet, requestsPath + "/request-1/decision", http.StatusMethodNotAllowed},
		{http.MethodGet, requestsPath + "/request-1/retry", http.StatusMethodNotAllowed},
		{http.MethodPost, requestsPath + "/request-1/issue", http.StatusNotFound},
		{http.MethodPost, requestsPath + "/request-1/prepare", http.StatusNotFound},
	} {
		w := send(router, tc.method, tc.path, `{}`, "intern")
		require.Equal(t, tc.status, w.Code, tc.path)
	}
}
