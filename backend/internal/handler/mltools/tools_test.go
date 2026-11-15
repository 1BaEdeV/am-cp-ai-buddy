package mltools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoprovider"
	tools "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/mltools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const token = "test-ml-service-secret"

type serviceStub struct {
	name, argument, employee string
	calls                    int
	err                      error
}

func (s *serviceStub) record(ctx context.Context, name, arg string) {
	s.calls++
	s.name, s.argument = name, arg
	s.employee, _ = tools.EmployeeID(ctx)
}
func (s *serviceStub) GetRoleTools(ctx context.Context) (access.WorkingAccesses, error) {
	s.record(ctx, "get_role_tools", "")
	return access.WorkingAccesses{Progress: access.Progress{Total: 4}}, s.err
}
func (s *serviceStub) CheckToolAccess(ctx context.Context, id string) (tools.ToolAccess, error) {
	s.record(ctx, "check_tool_access", id)
	return tools.ToolAccess{ResourceID: id}, s.err
}
func (s *serviceStub) PrepareAccessRequest(ctx context.Context, id string) (accessrequest.Preparation, error) {
	s.record(ctx, "prepare_access_request", id)
	return accessrequest.Preparation{Status: accessrequest.PreparationReady}, s.err
}
func (s *serviceStub) GetAccessRequestStatus(ctx context.Context, id string) (accessrequest.Request, error) {
	s.record(ctx, "get_access_request_status", id)
	return accessrequest.Request{ID: id}, s.err
}

func call(h *Handler, name, body, authorization, employee string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/internal/ml/tools/"+name, strings.NewReader(body))
	r.SetPathValue("toolName", name)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", authorization)
	r.Header.Set("X-Demo-Employee-Id", employee)
	w := httptest.NewRecorder()
	h.CallTool(w, r)
	return w
}

func TestCallsUseServerIdentity(t *testing.T) {
	for _, tt := range []struct{ name, body, argument string }{
		{"get_role_tools", `{}`, ""}, {"check_tool_access", `{"resourceId":"repo"}`, "repo"},
		{"prepare_access_request", `{"resourceId":"repo"}`, "repo"}, {"get_access_request_status", `{"requestId":"request"}`, "request"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &serviceStub{}
			w := call(NewHandler(s, token), tt.name, tt.body, "Bearer "+token, " intern ")
			require.Equal(t, 200, w.Code, w.Body.String())
			assert.Equal(t, tt.name, s.name)
			assert.Equal(t, tt.argument, s.argument)
			assert.Equal(t, "intern", s.employee)
			assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		})
	}
}

func TestRejectsUnauthenticatedAndForgedCalls(t *testing.T) {
	for _, authorization := range []string{"", "Bearer wrong", "Basic " + token, "Bearer " + token + " extra"} {
		s := &serviceStub{}
		w := call(NewHandler(s, token), "prepare_access_request", `{"resourceId":"repo"}`, authorization, "intern")
		assert.Equal(t, 401, w.Code)
		assert.Zero(t, s.calls)
	}
	s := &serviceStub{}
	assert.Equal(t, 401, call(NewHandler(s, ""), "get_role_tools", `{}`, "Bearer ", "intern").Code)
	assert.Equal(t, 401, call(NewHandler(s, token), "get_role_tools", `{}`, "Bearer "+token, " ").Code)
	for _, body := range []string{`{"resourceId":"repo","employeeId":"manager"}`, `{"resourceId":"repo","approverId":"manager"}`, `{"resourceId":"repo","accessLevel":"Owner"}`, `{"resourceId":"repo","confirmed":true}`, `null`, `[]`, `{`, `{} {}`, `{}`, `{"resourceId":" "}`} {
		w := call(NewHandler(s, token), "prepare_access_request", body, "Bearer "+token, "intern")
		assert.Equal(t, 400, w.Code, body)
	}
	assert.Equal(t, 400, call(NewHandler(s, token), "get_access_request_status", `{}`, "Bearer "+token, "intern").Code)
	assert.Equal(t, 400, call(NewHandler(s, token), "get_access_request_status", `null`, "Bearer "+token, "intern").Code)
	assert.Equal(t, 400, call(NewHandler(s, token), "get_role_tools", `{"employeeId":"other"}`, "Bearer "+token, "intern").Code)
	assert.Equal(t, 404, call(NewHandler(s, token), "confirm_access_request", `{}`, "Bearer "+token, "intern").Code)
	assert.Zero(t, s.calls)
}

func TestDiscoveryAndJSONLimits(t *testing.T) {
	h := NewHandler(nil, token)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/internal/ml/tools", nil)
	w := httptest.NewRecorder()
	h.ListTools(w, r)
	assert.Equal(t, 401, w.Code)
	r.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	h.ListTools(w, r)
	require.Equal(t, 200, w.Code)
	var definitions []tools.Definition
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &definitions))
	assert.Len(t, definitions, 4)
	w = call(h, "prepare_access_request", `{"resourceId":"`+strings.Repeat("a", 1<<20)+`"}`, "Bearer "+token, "intern")
	assert.Equal(t, 413, w.Code)
	r = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	r.SetPathValue("toolName", "get_role_tools")
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Demo-Employee-Id", "intern")
	w = httptest.NewRecorder()
	h.CallTool(w, r)
	assert.Equal(t, 415, w.Code)
}

func TestErrorMapping(t *testing.T) {
	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{
		{auth.ErrEmployeeNotFound, 401, "demo_identity_required"}, {auth.ErrForbidden, 403, "forbidden"}, {auth.ErrProfileRequired, 403, "profile_required"},
		{accessrequest.ErrInvalidInput, 400, "invalid_input"}, {accessrequest.ErrResourceNotRequired, 404, "resource_not_required"}, {accessrequest.ErrRequestNotFound, 404, "request_not_found"},
		{tools.ErrResourceChanged, 409, "resource_changed"}, {demoprovider.ErrResourceNotFound, 409, "resource_changed"}, {demoprovider.ErrUnsupportedLevel, 409, "resource_changed"},
		{access.ErrRequirementsNotConfigured, 409, "requirements_not_configured"}, {access.ErrInvalidRequirement, 409, "requirements_not_configured"},
		{demoprovider.ErrCheckFailed, 503, "access_check_unavailable"}, {context.DeadlineExceeded, 503, "access_check_unavailable"}, {errors.New("private database details"), 500, "internal_error"},
	} {
		t.Run(tt.err.Error(), func(t *testing.T) {
			w := call(NewHandler(&serviceStub{err: tt.err}, token), "get_role_tools", `{}`, "Bearer "+token, "intern")
			require.Equal(t, tt.status, w.Code)
			var body map[string]string
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, tt.code, body["code"])
			assert.NotContains(t, w.Body.String(), "private database details")
		})
	}
}
