package mltools

import (
	"context"
	"errors"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type resourcesStub struct {
	result   access.WorkingAccesses
	err      error
	employee string
}

func (s *resourcesStub) ListRequiredAccesses(_ context.Context, employee string) (access.WorkingAccesses, error) {
	s.employee = employee
	return s.result, s.err
}

type requestsStub struct {
	employee, argument string
	err                error
}

func (s *requestsStub) PrepareAccessRequest(_ context.Context, employee, resource string) (accessrequest.Preparation, error) {
	s.employee, s.argument = employee, resource
	return accessrequest.Preparation{Status: accessrequest.PreparationReady}, s.err
}
func (s *requestsStub) GetAccessRequest(_ context.Context, employee, request string) (accessrequest.Request, error) {
	s.employee, s.argument = employee, request
	return accessrequest.Request{ID: request}, s.err
}

type providerStub struct {
	actual accessrequest.ProviderAccess
	err    error
	target accessrequest.GrantTarget
	calls  int
}

func (p *providerStub) CheckAccess(_ context.Context, target accessrequest.GrantTarget) (accessrequest.ProviderAccess, error) {
	p.calls++
	p.target = target
	return p.actual, p.err
}
func (*providerStub) GrantAccess(context.Context, string, accessrequest.GrantTarget) error {
	panic("инструмент не должен выдавать права")
}

func TestContextIsRequiredForEveryTool(t *testing.T) {
	for _, scenario := range []string{"missing", "blank", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			want := auth.ErrEmployeeNotFound
			if scenario == "blank" {
				ctx = WithEmployeeID(ctx, " \t")
			}
			if scenario == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want = context.Canceled
			}
			s := NewService(nil, nil, nil, nil)
			_, err := s.GetRoleTools(ctx)
			require.ErrorIs(t, err, want)
			_, err = s.CheckToolAccess(ctx, "resource")
			require.ErrorIs(t, err, want)
			_, err = s.PrepareAccessRequest(ctx, "resource")
			require.ErrorIs(t, err, want)
			_, err = s.GetAccessRequestStatus(ctx, "request")
			require.ErrorIs(t, err, want)
		})
	}
}

func TestToolsUseEmployeeFromContext(t *testing.T) {
	ctx := WithEmployeeID(context.Background(), " intern ")
	resources, requests := &resourcesStub{result: access.WorkingAccesses{Progress: access.Progress{Completed: 2, Total: 4}}}, &requestsStub{}
	s := NewService(nil, resources, requests, nil)
	result, err := s.GetRoleTools(ctx)
	require.NoError(t, err)
	assert.Equal(t, resources.result, result)
	assert.Equal(t, "intern", resources.employee)
	prepared, err := s.PrepareAccessRequest(ctx, "repo")
	require.NoError(t, err)
	assert.Equal(t, accessrequest.PreparationReady, prepared.Status)
	assert.Equal(t, "intern", requests.employee)
	assert.Equal(t, "repo", requests.argument)
	request, err := s.GetAccessRequestStatus(ctx, "request")
	require.NoError(t, err)
	assert.Equal(t, "request", request.ID)
	assert.Equal(t, "intern", requests.employee)
	requests.err = accessrequest.ErrRequestNotFound
	_, err = s.GetAccessRequestStatus(ctx, "foreign")
	require.ErrorIs(t, err, accessrequest.ErrRequestNotFound)
	requests.err = auth.ErrForbidden
	_, err = s.PrepareAccessRequest(ctx, "repo")
	require.ErrorIs(t, err, auth.ErrForbidden)
}

func TestCheckToolAccess(t *testing.T) {
	for _, scenario := range []string{"missing access", "sufficient", "insufficient", "unconfirmed", "blank resource", "foreign resource", "forbidden", "deleted resource", "read error", "changed service", "changed level", "provider error"} {
		t.Run(scenario, func(t *testing.T) {
			conn, mock, err := sqlmock.New()
			require.NoError(t, err)
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			t.Cleanup(func() {
				assert.NoError(t, mock.ExpectationsWereMet())
				mock.ExpectClose()
				assert.NoError(t, conn.Close())
			})
			resourceID := " repo "
			resources := &resourcesStub{result: access.WorkingAccesses{Resources: []access.RequiredResource{{ID: "repo", Service: "gitlab", AccessLevel: "Developer"}}}}
			provider := &providerStub{}
			var wantErr error
			switch scenario {
			case "blank resource":
				resourceID, wantErr = " ", accessrequest.ErrInvalidInput
			case "foreign resource":
				resourceID, wantErr = "foreign", accessrequest.ErrResourceNotRequired
			case "forbidden":
				resources.err, wantErr = auth.ErrForbidden, auth.ErrForbidden
			default:
				q := mock.ExpectQuery(`SELECT .* FROM "resources"`).WithArgs("repo", 1)
				switch scenario {
				case "deleted resource":
					q.WillReturnRows(sqlmock.NewRows([]string{"id"}))
					wantErr = accessrequest.ErrResourceNotRequired
				case "read error":
					wantErr = errors.New("catalog unavailable")
					q.WillReturnError(wantErr)
				default:
					service, levels := "gitlab", `["Reporter","Developer"]`
					if scenario == "changed service" {
						service, wantErr = "figma", ErrResourceChanged
					}
					if scenario == "changed level" {
						levels, wantErr = `["Reporter"]`, ErrResourceChanged
					}
					q.WillReturnRows(sqlmock.NewRows([]string{"id", "service", "external_id", "access_levels"}).AddRow("repo", service, "external-repo", levels))
				}
			}
			if scenario == "sufficient" {
				provider.actual = accessrequest.ProviderAccess{Granted: true, AccessLevel: "Developer"}
			}
			if scenario == "insufficient" {
				provider.actual = accessrequest.ProviderAccess{Granted: true, AccessLevel: "Reporter"}
			}
			if scenario == "unconfirmed" {
				provider.actual = accessrequest.ProviderAccess{AccessLevel: "Developer"}
			}
			if scenario == "provider error" {
				provider.err = errors.New("provider offline")
				wantErr = provider.err
			}
			result, err := NewService(db, resources, nil, provider).CheckToolAccess(WithEmployeeID(context.Background(), "intern"), resourceID)
			if wantErr != nil {
				require.ErrorIs(t, err, wantErr)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, ToolAccess{ResourceID: "repo", RequiredAccessLevel: "Developer", Granted: provider.actual.Granted, AccessLevel: provider.actual.AccessLevel, Sufficient: scenario == "sufficient"}, result)
				assert.Equal(t, accessrequest.GrantTarget{EmployeeID: "intern", Service: "gitlab", ExternalID: "external-repo", AccessLevel: "Developer"}, provider.target)
			}
			if wantErr != nil && scenario != "provider error" {
				assert.Zero(t, provider.calls, "проверка прав должна предшествовать вызову провайдера")
			}
		})
	}
}

func TestDefinitionsExposeOnlyAllowedOperations(t *testing.T) {
	definitions := Definitions()
	require.Len(t, definitions, 4)
	var names []string
	for _, d := range definitions {
		names = append(names, d.Name)
		assert.NotEmpty(t, d.Description)
		assert.Equal(t, false, d.InputSchema["additionalProperties"])
		properties := d.InputSchema["properties"].(map[string]any)
		for _, forbidden := range []string{"employeeId", "approverId", "accessLevel", "confirmed"} {
			assert.NotContains(t, properties, forbidden)
		}
	}
	assert.ElementsMatch(t, []string{"get_role_tools", "check_tool_access", "prepare_access_request", "get_access_request_status"}, names)
	definitions[0].InputSchema["additionalProperties"] = true
	assert.Equal(t, false, Definitions()[0].InputSchema["additionalProperties"])
}
