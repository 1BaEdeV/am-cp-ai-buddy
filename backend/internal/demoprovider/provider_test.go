package demoprovider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvalidInputDoesNotReachDatabase(t *testing.T) {
	for _, field := range []string{"employee", "service", "external id", "level"} {
		for _, blank := range []string{"", " \t\n\u00a0"} {
			t.Run(field+"/"+blank, func(t *testing.T) {
				input := target()
				switch field {
				case "employee":
					input.EmployeeID = blank
				case "service":
					input.Service = blank
				case "external id":
					input.ExternalID = blank
				case "level":
					input.AccessLevel = blank
				}
				p, ctx := NewProvider(nil), context.Background()
				actual, err := p.CheckAccess(ctx, input)
				require.ErrorIs(t, err, ErrInvalidInput)
				assert.Empty(t, actual)
				require.ErrorIs(t, p.GrantAccess(ctx, "operation", input), ErrInvalidInput)
				require.ErrorIs(t, p.SeedAccess(ctx, input), ErrInvalidInput)
				require.ErrorIs(t, p.SetFailure(ctx, input, FailureGrant), ErrInvalidInput)
			})
		}
	}
	p := NewProvider(nil)
	require.ErrorIs(t, p.GrantAccess(context.Background(), " \t", target()), ErrInvalidInput)
	require.ErrorIs(t, p.SetFailure(context.Background(), target(), "unknown"), ErrInvalidInput)
}

func TestCancelledContext(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancelled", true: "deadline exceeded"}[expired], func(t *testing.T) {
			var ctx context.Context
			var cancel context.CancelFunc
			want := context.Canceled
			if expired {
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				want = context.DeadlineExceeded
			} else {
				ctx, cancel = context.WithCancel(context.Background())
			}
			cancel()
			p := NewProvider(nil)
			_, err := p.CheckAccess(ctx, target())
			require.ErrorIs(t, err, want)
			require.ErrorIs(t, p.GrantAccess(ctx, "operation", target()), want)
			require.ErrorIs(t, p.SeedAccess(ctx, target()), want)
			require.ErrorIs(t, p.SetFailure(ctx, target(), FailureNone), want)
		})
	}
}

func TestCheckAccess(t *testing.T) {
	for _, scenario := range []string{"missing", "empty level", "lower level", "higher level", "check failure", "grant failure does not block checking", "storage error"} {
		t.Run(scenario, func(t *testing.T) {
			p, mock := mockProvider(t)
			expectTarget(mock)
			q := stateQuery(mock)
			storageErr := errors.New("state read failed")
			want := accessrequest.ProviderAccess{}
			var wantErr error
			switch scenario {
			case "missing":
				q.WillReturnRows(emptyRows())
			case "empty level":
				q.WillReturnRows(stateRows("", FailureNone))
			case "lower level":
				q.WillReturnRows(stateRows("Reporter", FailureNone))
				want = accessrequest.ProviderAccess{Granted: true, AccessLevel: "Reporter"}
			case "higher level":
				q.WillReturnRows(stateRows("Developer", FailureNone))
				want = accessrequest.ProviderAccess{Granted: true, AccessLevel: "Developer"}
			case "check failure":
				q.WillReturnRows(stateRows("Reporter", FailureCheck))
				wantErr = ErrCheckFailed
			case "grant failure does not block checking":
				q.WillReturnRows(stateRows("Reporter", FailureGrant))
				want = accessrequest.ProviderAccess{Granted: true, AccessLevel: "Reporter"}
			case "storage error":
				q.WillReturnError(storageErr)
				wantErr = storageErr
			}
			input := target()
			input.AccessLevel = "Developer"
			actual, err := p.CheckAccess(context.Background(), input)
			if wantErr != nil {
				require.ErrorIs(t, err, wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, want, actual)
		})
	}
}

func TestValidateTarget(t *testing.T) {
	storageErr := errors.New("catalog unavailable")
	for _, scenario := range []string{"employee missing", "employee error", "resource missing", "resource error", "unsupported level"} {
		t.Run(scenario, func(t *testing.T) {
			p, mock := mockProvider(t)
			want := storageErr
			switch scenario {
			case "employee missing":
				employeeQuery(mock).WillReturnRows(emptyRows())
				want = ErrEmployeeNotFound
			case "employee error":
				employeeQuery(mock).WillReturnError(storageErr)
			default:
				employeeQuery(mock).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("intern"))
				switch scenario {
				case "resource missing":
					resourceQuery(mock).WillReturnRows(emptyRows())
					want = ErrResourceNotFound
				case "resource error":
					resourceQuery(mock).WillReturnError(storageErr)
				case "unsupported level":
					resourceQuery(mock).WillReturnRows(sqlmock.NewRows([]string{"access_levels"}).AddRow(`["Developer"]`))
					want = ErrUnsupportedLevel
				}
			}
			_, err := p.CheckAccess(context.Background(), target())
			require.ErrorIs(t, err, want)
		})
	}
}

func TestSeedAccessPreservesHigherRights(t *testing.T) {
	for _, level := range []string{"", "Reporter", "Developer"} {
		t.Run(level, func(t *testing.T) {
			p, mock := mockProvider(t)
			mock.ExpectBegin()
			expectTarget(mock)
			expectLock(mock, level, FailureGrant)
			if level == "" {
				expectSave(mock, "Reporter")
			}
			mock.ExpectCommit()
			require.NoError(t, p.SeedAccess(context.Background(), target()))
		})
	}
}

func TestSetFailure(t *testing.T) {
	for _, mode := range []FailureMode{FailureNone, FailureCheck, FailureGrant, FailureAfterGrant} {
		t.Run(string(mode), func(t *testing.T) {
			p, mock := mockProvider(t)
			mock.ExpectBegin()
			expectTarget(mock)
			expectLock(mock, "Developer", FailureGrant)
			mock.ExpectExec(`UPDATE "demo_provider_accesses" SET "failure_mode"`).
				WithArgs(string(mode), testNow, "intern", "gitlab", "repo").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			require.NoError(t, p.SetFailure(context.Background(), target(), mode))
		})
	}
}

func TestSameOperationBindsAllParameters(t *testing.T) {
	saved := models.DemoProviderOperation{EmployeeID: "intern", Service: "gitlab", ExternalID: "repo", AccessLevel: "Reporter"}
	require.NoError(t, sameOperation(saved, target()))
	for _, field := range []string{"employee", "service", "external id", "level"} {
		t.Run(field, func(t *testing.T) {
			changed := target()
			switch field {
			case "employee":
				changed.EmployeeID = "other"
			case "service":
				changed.Service = "figma"
			case "external id":
				changed.ExternalID = "other"
			case "level":
				changed.AccessLevel = "Developer"
			}
			require.ErrorIs(t, sameOperation(saved, changed), ErrOperationConflict)
		})
	}
}
