package accessrequest

import (
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestRequirementParametersPreservesSnapshot(t *testing.T) {
	r := models.AccessRequirement{
		Resource:    models.Resource{Name: "Backend", Service: "gitlab", ExternalID: "42"},
		AccessLevel: "Reporter", Purpose: "Чтение кода команды",
		Approver: models.Employee{FirstName: "Анна", SecondName: "Иванова"},
	}
	snapshot := requirementParameters(r)
	want := models.AccessRequestParameters{
		ResourceName: "Backend", ResourceService: "gitlab", ResourceExternalID: "42",
		AccessLevel: "Reporter", Purpose: "Чтение кода команды", ApproverName: "Анна Иванова",
	}
	assert.Equal(t, want, snapshot)

	// Изменение справочника не должно менять то, что уже показали сотруднику.
	r.Resource.Name, r.Resource.Service, r.Resource.ExternalID = "Other", "figma", "99"
	r.AccessLevel, r.Purpose, r.Approver.FirstName = "Editor", "Другая задача", "Ольга"
	assert.Equal(t, want, snapshot)
	assert.NotEqual(t, snapshot, requirementParameters(r))
}

func TestRequirementParametersFormatsApproverName(t *testing.T) {
	for _, tt := range []struct {
		name, first, second, want string
	}{
		{"full name", "Анна", "Иванова", "Анна Иванова"},
		{"first name only", "Анна", "", "Анна"},
		{"second name only", "", "Иванова", "Иванова"},
		{"outer spaces", "  Анна", "Иванова  ", "Анна Иванова"},
		{"no name", "", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := requirementParameters(models.AccessRequirement{
				Approver: models.Employee{FirstName: tt.first, SecondName: tt.second},
			})
			assert.Equal(t, tt.want, p.ApproverName)
		})
	}
}

func TestViewsUseConfirmedSnapshot(t *testing.T) {
	parameters := models.AccessRequestParameters{
		ResourceName: "Backend", ResourceService: "gitlab", ResourceExternalID: "42",
		AccessLevel: "Reporter", Purpose: "Чтение кода команды", ApproverName: "Анна Иванова",
	}
	want := RequestParameters{
		ResourceName: "Backend", ResourceService: "gitlab", ResourceExternalID: "42",
		AccessLevel: "Reporter", Purpose: "Чтение кода команды", ApproverName: "Анна Иванова",
	}
	// Загруженные связи содержат уже новые значения, но ответ должен показывать снимок.
	resource := models.Resource{Name: "Changed", Service: "figma", ExternalID: "99"}
	approver := models.Employee{FirstName: "Другой", SecondName: "Руководитель"}
	created := time.Date(2026, time.November, 13, 9, 0, 0, 0, time.UTC)
	expires := created.Add(DraftTTL)
	draft := draftView(models.AccessDraft{
		ID: "draft", ResourceID: "repo", ApproverID: "manager",
		Parameters: parameters, Resource: resource, Approver: approver,
		CreatedAt: created, ExpiresAt: expires,
	})
	assert.Equal(t, Draft{
		ID: "draft", ResourceID: "repo", ApproverID: "manager", Parameters: want,
		CreatedAt: created, ExpiresAt: expires,
	}, draft)

	decided := created.Add(time.Minute)
	updated := decided.Add(time.Minute)
	request := requestView(models.AccessRequest{
		ID: "request", Number: 123, DraftID: "draft", EmployeeID: "intern",
		ResourceID: "repo", ApproverID: "manager", Parameters: parameters,
		Resource: resource, Approver: approver, Status: models.AccessRequestFailed,
		CreatedAt: created, UpdatedAt: updated, DecidedAt: &decided,
		DecisionReason: "Одобрено", LastError: "Сервис недоступен",
	})
	assert.Equal(t, Request{
		ID: "request", Number: 123, DraftID: "draft", EmployeeID: "intern",
		ResourceID: "repo", ApproverID: "manager", Parameters: want,
		Status: models.AccessRequestFailed, CreatedAt: created, UpdatedAt: updated,
		DecidedAt: &decided, DecisionReason: "Одобрено", LastError: "Сервис недоступен",
	}, request)
}

func TestRequestViewPreservesIssuanceDates(t *testing.T) {
	created := time.Date(2026, time.November, 13, 9, 0, 0, 0, time.UTC)
	decided, granted := created.Add(time.Minute), created.Add(2*time.Minute)
	for _, tt := range []struct {
		status           models.AccessRequestStatus
		decided, granted *time.Time
	}{
		{models.AccessRequestPending, nil, nil},
		{models.AccessRequestIssuing, &decided, nil},
		{models.AccessRequestGranted, &decided, &granted},
		{models.AccessRequestRejected, &decided, nil},
	} {
		t.Run(string(tt.status), func(t *testing.T) {
			view := requestView(models.AccessRequest{Status: tt.status, DecidedAt: tt.decided, GrantedAt: tt.granted})
			assert.Equal(t, tt.status, view.Status)
			assert.Equal(t, tt.decided, view.DecidedAt)
			assert.Equal(t, tt.granted, view.GrantedAt)
		})
	}
}
