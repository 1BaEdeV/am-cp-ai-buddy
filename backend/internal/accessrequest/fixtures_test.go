package accessrequest

import (
	"database/sql/driver"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var unitNow = time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

// Реального подключения нет: драйвер возвращает только заданные тестом строки.
func mockRequestService(t *testing.T) (*Service, sqlmock.Sqlmock) {
	t.Helper()
	conn, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mock.ExpectationsWereMet())
		mock.ExpectClose()
		assert.NoError(t, conn.Close())
	})
	s := NewService(db)
	s.now = func() time.Time { return unitNow }
	return s, mock
}

func mockRow(values map[string]driver.Value) *sqlmock.Rows {
	columns := make([]string, 0, len(values))
	for column := range values {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	row := make([]driver.Value, len(columns))
	for i, column := range columns {
		row[i] = values[column]
	}
	return sqlmock.NewRows(columns).AddRow(row...)
}

func selectFrom(mock sqlmock.Sqlmock, table string, args ...driver.Value) *sqlmock.ExpectedQuery {
	q := mock.ExpectQuery(`SELECT .* FROM "` + table + `"`)
	if len(args) > 0 {
		q.WithArgs(args...)
	}
	return q
}

func noRows() *sqlmock.Rows { return sqlmock.NewRows([]string{"id"}) }

func unitEmployee(role models.EmployeeRole) models.Employee {
	id := "intern"
	if role == models.RoleManager {
		id = "manager"
	}
	return models.Employee{ID: id, TeamID: "team", PositionID: "position", Role: role, FirstName: "Анна", SecondName: "Иванова"}
}

func employeeRow(e models.Employee) *sqlmock.Rows {
	return mockRow(map[string]driver.Value{"id": e.ID, "team_id": e.TeamID, "position_id": e.PositionID, "role": e.Role, "first_name": e.FirstName, "second_name": e.SecondName})
}

func expectEmployee(mock sqlmock.Sqlmock, role models.EmployeeRole) {
	e := unitEmployee(role)
	selectFrom(mock, "employees", e.ID, 1).WillReturnRows(employeeRow(e))
}

func expectProfile(mock sqlmock.Sqlmock) {
	selectFrom(mock, "profiles", "intern", 1).WillReturnRows(mockRow(map[string]driver.Value{"id": "profile"}))
}

func unitRequirement() models.AccessRequirement {
	return models.AccessRequirement{
		ID: "rule", TeamID: "team", PositionID: "position", ResourceID: "repo", ApproverID: "manager",
		Resource: models.Resource{ID: "repo", Name: "Backend", Service: "gitlab", ExternalID: "42", AccessLevels: []string{"Reporter", "Developer"}},
		Approver: unitEmployee(models.RoleManager), AccessLevel: "Reporter", Purpose: "Работа с кодом",
	}
}

func resourceRow(r models.Resource) *sqlmock.Rows {
	levels, _ := json.Marshal(r.AccessLevels)
	return mockRow(map[string]driver.Value{"id": r.ID, "name": r.Name, "service": r.Service, "external_id": r.ExternalID, "access_levels": string(levels)})
}

func requirementRow(r models.AccessRequirement) *sqlmock.Rows {
	return mockRow(map[string]driver.Value{"id": r.ID, "resource_id": r.ResourceID, "approver_id": r.ApproverID, "access_level": r.AccessLevel, "purpose": r.Purpose})
}

func expectRequirement(mock sqlmock.Sqlmock) {
	r := unitRequirement()
	selectFrom(mock, "access_requirements", "team", "position", "repo", 1).WillReturnRows(requirementRow(r))
	selectFrom(mock, "resources", "repo", 1).WillReturnRows(resourceRow(r.Resource))
	selectFrom(mock, "employees", "manager", 1).WillReturnRows(employeeRow(r.Approver))
}

func unitDraft() models.AccessDraft {
	return models.AccessDraft{
		ID: "draft", EmployeeID: "intern", TeamID: "team", PositionID: "position", ResourceID: "repo", ApproverID: "manager",
		Parameters: requirementParameters(unitRequirement()), CreatedAt: unitNow.Add(-time.Minute), ExpiresAt: unitNow.Add(DraftTTL),
	}
}

func snapshotValues(p models.AccessRequestParameters) map[string]driver.Value {
	return map[string]driver.Value{
		"snapshot_resource_name": p.ResourceName, "snapshot_resource_service": p.ResourceService,
		"snapshot_resource_external_id": p.ResourceExternalID, "snapshot_access_level": p.AccessLevel,
		"snapshot_purpose": p.Purpose, "snapshot_approver_name": p.ApproverName,
	}
}

func draftRow(d models.AccessDraft) *sqlmock.Rows {
	v := snapshotValues(d.Parameters)
	v["id"], v["employee_id"], v["team_id"], v["position_id"] = d.ID, d.EmployeeID, d.TeamID, d.PositionID
	v["resource_id"], v["approver_id"] = d.ResourceID, d.ApproverID
	v["created_at"], v["expires_at"], v["confirmed_at"] = d.CreatedAt, d.ExpiresAt, d.ConfirmedAt
	return mockRow(v)
}

func unitRequest(status models.AccessRequestStatus) models.AccessRequest {
	d := unitDraft()
	r := models.AccessRequest{
		ID: "request", Number: 123, DraftID: d.ID, EmployeeID: d.EmployeeID, TeamID: d.TeamID, PositionID: d.PositionID,
		ResourceID: d.ResourceID, Resource: unitRequirement().Resource, ApproverID: d.ApproverID, Parameters: d.Parameters,
		Status: status, CreatedAt: d.CreatedAt, UpdatedAt: unitNow,
	}
	if status != models.AccessRequestPending {
		decided := unitNow.Add(-time.Second)
		r.DecidedAt = &decided
	}
	return r
}

func requestRow(r models.AccessRequest) *sqlmock.Rows {
	v := snapshotValues(r.Parameters)
	v["id"], v["number"], v["draft_id"], v["employee_id"] = r.ID, r.Number, r.DraftID, r.EmployeeID
	v["team_id"], v["position_id"], v["resource_id"], v["approver_id"] = r.TeamID, r.PositionID, r.ResourceID, r.ApproverID
	v["status"], v["created_at"], v["updated_at"] = r.Status, r.CreatedAt, r.UpdatedAt
	v["decided_at"], v["decision_reason"], v["granted_at"], v["last_error"] = r.DecidedAt, r.DecisionReason, r.GrantedAt, r.LastError
	return mockRow(v)
}

func expectNoAccess(mock sqlmock.Sqlmock) {
	selectFrom(mock, "employee_accesses", "intern", "repo", 1).WillReturnRows(noRows())
}

func expectNoActiveRequest(mock sqlmock.Sqlmock) {
	selectFrom(mock, "access_requests", "intern", "repo", models.AccessRequestPending, models.AccessRequestIssuing, models.AccessRequestFailed, 1).WillReturnRows(noRows())
}

func unitJob() models.AccessIssuanceJob {
	until := unitNow.Add(issuanceLease)
	return models.AccessIssuanceJob{RequestID: "request", LeaseToken: "lease", LeaseUntil: &until, Attempts: 1, AvailableAt: unitNow}
}

func jobRow(j models.AccessIssuanceJob) *sqlmock.Rows {
	return mockRow(map[string]driver.Value{"request_id": j.RequestID, "lease_token": j.LeaseToken, "lease_until": j.LeaseUntil, "attempts": j.Attempts, "available_at": j.AvailableAt, "completed_at": j.CompletedAt})
}
