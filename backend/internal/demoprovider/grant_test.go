package demoprovider

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGrantAccess(t *testing.T) {
	for _, scenario := range []string{"new rights", "keep higher rights", "grant failure", "lost response", "repeat", "parallel repeat", "conflicting repeat"} {
		t.Run(scenario, func(t *testing.T) {
			p, mock := mockProvider(t)
			mock.ExpectBegin()
			var want error
			switch scenario {
			case "repeat", "conflicting repeat":
				rows := operationRows()
				if scenario == "conflicting repeat" {
					rows = sqlmock.NewRows([]string{"employee_id", "service", "external_id", "access_level"}).AddRow("other", "gitlab", "repo", "Reporter")
					want = ErrOperationConflict
				}
				operationQuery(mock).WillReturnRows(rows)
			case "parallel repeat":
				operationQuery(mock).WillReturnRows(emptyRows())
				expectTarget(mock)
				operationInsert(mock).WillReturnResult(sqlmock.NewResult(0, 0))
				operationQuery(mock).WillReturnRows(operationRows())
			default:
				expectNewOperation(mock)
				level, mode := "", FailureNone
				switch scenario {
				case "keep higher rights":
					level = "Developer"
				case "grant failure":
					mode, want = FailureGrant, ErrGrantFailed
				case "lost response":
					mode, want = FailureAfterGrant, context.DeadlineExceeded
				}
				expectLock(mock, level, mode)
				if scenario == "new rights" || scenario == "lost response" {
					expectSave(mock, "Reporter")
				}
			}
			if want != nil && !errors.Is(want, context.DeadlineExceeded) {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			err := p.GrantAccess(context.Background(), "operation", target())
			if want != nil {
				require.ErrorIs(t, err, want)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGrantStorageFailures(t *testing.T) {
	storageErr := errors.New("grant transaction failed")
	for _, stage := range []string{"begin", "read operation", "validate target", "insert operation", "parallel operation", "create state", "lock state", "save rights", "commit"} {
		t.Run(stage, func(t *testing.T) {
			p, mock := mockProvider(t)
			if stage == "begin" {
				mock.ExpectBegin().WillReturnError(storageErr)
			} else {
				mock.ExpectBegin()
				setup := func() {
					q := operationQuery(mock)
					if stage == "read operation" {
						q.WillReturnError(storageErr)
						return
					}
					q.WillReturnRows(emptyRows())
					if stage == "validate target" {
						employeeQuery(mock).WillReturnError(storageErr)
						return
					}
					expectTarget(mock)
					insert := operationInsert(mock)
					switch stage {
					case "insert operation":
						insert.WillReturnError(storageErr)
						return
					case "parallel operation":
						insert.WillReturnResult(sqlmock.NewResult(0, 0))
						operationQuery(mock).WillReturnError(storageErr)
						return
					default:
						insert.WillReturnResult(sqlmock.NewResult(0, 1))
					}
					insert = mock.ExpectExec(`INSERT INTO "demo_provider_accesses"`)
					if stage == "create state" {
						insert.WillReturnError(storageErr)
						return
					}
					insert.WillReturnResult(sqlmock.NewResult(0, 1))
					q = stateQuery(mock)
					if stage == "lock state" {
						q.WillReturnError(storageErr)
						return
					}
					q.WillReturnRows(stateRows("", FailureNone))
					update := mock.ExpectExec(`UPDATE "demo_provider_accesses"`)
					if stage == "save rights" {
						update.WillReturnError(storageErr)
					} else {
						update.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
				setup()
				if stage == "commit" {
					mock.ExpectCommit().WillReturnError(storageErr)
				} else {
					mock.ExpectRollback()
				}
			}
			require.ErrorIs(t, p.GrantAccess(context.Background(), "operation", target()), storageErr)
		})
	}
}

func TestAdministrationRollsBackOnFailure(t *testing.T) {
	storageErr := errors.New("cannot change demo state")
	for _, stage := range []string{"validate", "lock", "seed update", "failure update"} {
		t.Run(stage, func(t *testing.T) {
			p, mock := mockProvider(t)
			mock.ExpectBegin()
			if stage == "validate" {
				employeeQuery(mock).WillReturnError(storageErr)
			} else {
				expectTarget(mock)
				if stage == "lock" {
					mock.ExpectExec(`INSERT INTO "demo_provider_accesses"`).WillReturnError(storageErr)
				} else {
					expectLock(mock, "", FailureNone)
					mock.ExpectExec(`UPDATE "demo_provider_accesses"`).WillReturnError(storageErr)
				}
			}
			mock.ExpectRollback()
			var err error
			if stage == "failure update" {
				err = p.SetFailure(context.Background(), target(), FailureGrant)
			} else {
				err = p.SeedAccess(context.Background(), target())
			}
			require.ErrorIs(t, err, storageErr)
		})
	}
}
