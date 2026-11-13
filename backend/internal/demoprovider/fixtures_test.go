package demoprovider

import (
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var testNow = time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

func mockProvider(t *testing.T) (*Provider, sqlmock.Sqlmock) {
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
	p := NewProvider(db)
	p.now = func() time.Time { return testNow }
	return p, mock
}

func target() accessrequest.GrantTarget {
	return accessrequest.GrantTarget{EmployeeID: "intern", Service: "gitlab", ExternalID: "repo", AccessLevel: "Reporter"}
}

func employeeQuery(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(`SELECT "id" FROM "employees"`).WithArgs("intern", 1)
}

func resourceQuery(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(`SELECT .* FROM "resources"`).WithArgs("gitlab", "repo", 1)
}

func expectTarget(mock sqlmock.Sqlmock) {
	employeeQuery(mock).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("intern"))
	resourceQuery(mock).WillReturnRows(sqlmock.NewRows([]string{"id", "access_levels"}).AddRow("resource", `["Reporter","Developer"]`))
}

func stateRows(level string, mode FailureMode) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"employee_id", "service", "external_id", "access_level", "failure_mode"}).
		AddRow("intern", "gitlab", "repo", level, string(mode))
}

func stateQuery(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(`SELECT .* FROM "demo_provider_accesses"`).WithArgs("intern", "gitlab", "repo", 1)
}

func expectLock(mock sqlmock.Sqlmock, level string, mode FailureMode) {
	mock.ExpectExec(`INSERT INTO "demo_provider_accesses"`).
		WithArgs("intern", "gitlab", "repo", "", "", testNow, testNow).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "demo_provider_accesses".*FOR UPDATE`).WithArgs("intern", "gitlab", "repo", 1).WillReturnRows(stateRows(level, mode))
}

func expectSave(mock sqlmock.Sqlmock, level string) {
	mock.ExpectExec(`UPDATE "demo_provider_accesses" SET "access_level"`).
		WithArgs(level, testNow, "intern", "gitlab", "repo").WillReturnResult(sqlmock.NewResult(0, 1))
}

func operationQuery(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(`SELECT .* FROM "demo_provider_operations"`).WithArgs("operation", 1)
}

func operationRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "employee_id", "service", "external_id", "access_level"}).
		AddRow("operation", "intern", "gitlab", "repo", "Reporter")
}

func emptyRows() *sqlmock.Rows { return sqlmock.NewRows([]string{"id"}) }

func operationInsert(mock sqlmock.Sqlmock) *sqlmock.ExpectedExec {
	return mock.ExpectExec(`INSERT INTO "demo_provider_operations"`).WithArgs("operation", "intern", "gitlab", "repo", "Reporter", testNow)
}

func expectNewOperation(mock sqlmock.Sqlmock) {
	operationQuery(mock).WillReturnRows(emptyRows())
	expectTarget(mock)
	operationInsert(mock).WillReturnResult(sqlmock.NewResult(0, 1))
}
