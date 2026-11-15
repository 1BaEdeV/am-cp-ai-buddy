package access

import (
	"context"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasRequiredLevel(t *testing.T) {
	tests := []struct {
		name, service, actual, required string
		want                            bool
	}{
		{"gitlab exact lower level", "gitlab", "Reporter", "Reporter", true},
		{"gitlab exact higher level", "gitlab", "Developer", "Developer", true},
		{"gitlab higher level includes lower", "gitlab", "Developer", "Reporter", true},
		{"gitlab lower level is insufficient", "gitlab", "Reporter", "Developer", false},
		{"figma exact level", "figma", "Viewer", "Viewer", true},
		{"figma higher level includes lower", "figma", "Editor", "Viewer", true},
		{"figma lower level is insufficient", "figma", "Viewer", "Editor", false},
		{"s3 exact level", "s3", "read", "read", true},
		{"s3 higher level includes lower", "s3", "read_write", "read", true},
		{"s3 lower level is insufficient", "s3", "read", "read_write", false},
		{"custom service accepts exact level", "youtrack", "Reader", "Reader", true},
		{"custom service has no implicit hierarchy", "youtrack", "Admin", "Reader", false},
		{"gitlab levels do not apply to figma", "figma", "Developer", "Reporter", false},
		{"figma levels do not apply to s3", "s3", "Editor", "Viewer", false},
		{"s3 levels do not apply to gitlab", "gitlab", "read_write", "read", false},
		{"missing actual level", "gitlab", "", "Reporter", false},
		{"unknown actual level", "gitlab", "Owner", "Reporter", false},
		{"level comparison is case sensitive", "gitlab", "developer", "Reporter", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasRequiredLevel(tt.service, tt.actual, tt.required))
		})
	}
}

func TestValidRequirement(t *testing.T) {
	tests := []struct {
		name   string
		change func(*models.AccessRequirement)
		want   bool
	}{
		{"configured requirement", func(r *models.AccessRequirement) {}, true},
		{"missing resource", func(r *models.AccessRequirement) { r.Resource.ID = "" }, false},
		{"empty resource name", func(r *models.AccessRequirement) { r.Resource.Name = "" }, false},
		{"blank resource name", func(r *models.AccessRequirement) { r.Resource.Name = " \t\n" }, false},
		{"empty service", func(r *models.AccessRequirement) { r.Resource.Service = "" }, false},
		{"blank service", func(r *models.AccessRequirement) { r.Resource.Service = "\t " }, false},
		{"empty external id", func(r *models.AccessRequirement) { r.Resource.ExternalID = "" }, false},
		{"blank external id", func(r *models.AccessRequirement) { r.Resource.ExternalID = " \n" }, false},
		{"missing access level", func(r *models.AccessRequirement) { r.AccessLevel = "" }, false},
		{"unsupported access level", func(r *models.AccessRequirement) { r.AccessLevel = "Owner" }, false},
		{"no resource levels", func(r *models.AccessRequirement) { r.Resource.AccessLevels = nil }, false},
		{"empty purpose", func(r *models.AccessRequirement) { r.Purpose = "" }, false},
		{"blank purpose", func(r *models.AccessRequirement) { r.Purpose = "\u00a0\t " }, false},
		{"missing approver", func(r *models.AccessRequirement) { r.Approver = models.Employee{} }, false},
		{"intern cannot approve", func(r *models.AccessRequirement) { r.Approver.Role = models.RoleIntern }, false},
		{"unknown role cannot approve", func(r *models.AccessRequirement) { r.Approver.Role = "admin" }, false},
		{"manager from another team", func(r *models.AccessRequirement) { r.Approver.TeamID = "other-team" }, false},
		{"another supported level", func(r *models.AccessRequirement) { r.AccessLevel = "Developer" }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := models.AccessRequirement{
				Resource: models.Resource{
					ID: "repo", Name: "Backend", Service: "gitlab", ExternalID: "42",
					AccessLevels: []string{"Reporter", "Developer"},
				},
				AccessLevel: "Reporter", Purpose: "Работа с кодом команды",
				Approver: models.Employee{Role: models.RoleManager, TeamID: "team"},
			}
			tt.change(&r)
			assert.Equal(t, tt.want, ValidRequirement(r, "team"))
		})
	}
}

func TestActiveRequestStatuses(t *testing.T) {
	want := []models.AccessRequestStatus{
		models.AccessRequestPending, models.AccessRequestIssuing, models.AccessRequestFailed,
	}
	assert.ElementsMatch(t, want, ActiveRequestStatuses(), "ошибка выдачи тоже блокирует создание дубля")

	statuses := ActiveRequestStatuses()
	require.NotEmpty(t, statuses)
	statuses[0] = models.AccessRequestGranted
	assert.ElementsMatch(t, want, ActiveRequestStatuses(), "вызывающий код не должен менять общие правила")
}

func TestListRequiredAccessesRejectsBlankEmployee(t *testing.T) {
	for _, id := range []string{"", " ", "\t\n", "\u00a0"} {
		t.Run(id, func(t *testing.T) {
			// nil вместо подключения: неправильный ID должен отклоняться до запроса к БД.
			result, err := NewService(nil).ListRequiredAccesses(context.Background(), id)
			require.ErrorIs(t, err, auth.ErrEmployeeNotFound)
			assert.Empty(t, result)
		})
	}
}
