package demoprovider_test

import (
	"context"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoprovider"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderSupportsDemoResources(t *testing.T) {
	p, db := providerDB(t)
	ctx := context.Background()
	for service, level := range map[string]string{
		"wiki": "read", "youtrack": "member", "gitlab": "Developer", "grafana": "Viewer",
		"figma": "Editor", "jupyterhub": "user", "s3": "read_write", "postgres": "read", "metabase": "Viewer",
	} {
		t.Run(service, func(t *testing.T) {
			target := accessrequest.GrantTarget{EmployeeID: "intern", Service: service, ExternalID: "demo/" + service, AccessLevel: level}
			actual, err := p.CheckAccess(ctx, target)
			require.NoError(t, err)
			assert.False(t, actual.Granted)
			require.NoError(t, p.GrantAccess(ctx, "operation/"+service, target))
			actual, err = demoprovider.NewProvider(db).CheckAccess(ctx, target)
			require.NoError(t, err)
			assert.Equal(t, accessrequest.ProviderAccess{Granted: true, AccessLevel: level}, actual)
			// Повтор после пересоздания провайдера использует сохранённую операцию.
			require.NoError(t, demoprovider.NewProvider(db).GrantAccess(ctx, "operation/"+service, target))
			other := target
			other.EmployeeID = "other"
			actual, err = p.CheckAccess(ctx, other)
			require.NoError(t, err)
			assert.False(t, actual.Granted)
		})
	}
	var operations, employeeAccesses int64
	require.NoError(t, db.Model(&models.DemoProviderOperation{}).Count(&operations).Error)
	assert.EqualValues(t, 9, operations)
	require.NoError(t, db.Model(&models.EmployeeAccess{}).Count(&employeeAccesses).Error)
	assert.Zero(t, employeeAccesses, "провайдер не должен самостоятельно подтверждать доступ в бизнес-слое")
}

func TestProviderOperationsAreBoundToOriginalTarget(t *testing.T) {
	p, db := providerDB(t)
	ctx := context.Background()
	target := accessrequest.GrantTarget{EmployeeID: "intern", Service: "gitlab", ExternalID: "demo/gitlab", AccessLevel: "Reporter"}
	require.NoError(t, p.GrantAccess(ctx, "operation", target))
	for _, field := range []string{"employee", "service", "resource", "level"} {
		t.Run(field, func(t *testing.T) {
			changed := target
			switch field {
			case "employee":
				changed.EmployeeID = "other"
			case "service":
				changed.Service = "s3"
			case "resource":
				changed.ExternalID = "another-repo"
			case "level":
				changed.AccessLevel = "Developer"
			}
			require.ErrorIs(t, p.GrantAccess(ctx, "operation", changed), demoprovider.ErrOperationConflict)
		})
	}
	// Новая операция повышает права, повтор старой не понижает их.
	higher := target
	higher.AccessLevel = "Developer"
	require.NoError(t, p.GrantAccess(ctx, "upgrade", higher))
	require.NoError(t, p.SetFailure(ctx, target, demoprovider.FailureGrant))
	require.NoError(t, demoprovider.NewProvider(db).GrantAccess(ctx, "operation", target))
	require.NoError(t, p.SeedAccess(ctx, target))
	require.NoError(t, p.SetFailure(ctx, target, demoprovider.FailureNone))
	require.NoError(t, p.GrantAccess(ctx, "new-operation-same-lower-level", target))
	actual, err := p.CheckAccess(ctx, target)
	require.NoError(t, err)
	assert.Equal(t, "Developer", actual.AccessLevel)
}

func TestProviderFailuresAndLostResponse(t *testing.T) {
	p, db := providerDB(t)
	ctx := context.Background()
	target := accessrequest.GrantTarget{EmployeeID: "intern", Service: "gitlab", ExternalID: "demo/gitlab", AccessLevel: "Developer"}
	require.NoError(t, p.SetFailure(ctx, target, demoprovider.FailureCheck))
	_, err := demoprovider.NewProvider(db).CheckAccess(ctx, target)
	require.ErrorIs(t, err, demoprovider.ErrCheckFailed)

	require.NoError(t, p.SetFailure(ctx, target, demoprovider.FailureGrant))
	require.ErrorIs(t, p.GrantAccess(ctx, "operation", target), demoprovider.ErrGrantFailed)
	actual, err := p.CheckAccess(ctx, target)
	require.NoError(t, err)
	assert.False(t, actual.Granted)
	var count int64
	require.NoError(t, db.Model(&models.DemoProviderOperation{}).Count(&count).Error)
	assert.Zero(t, count, "сбой не должен сохранять успешную операцию")

	require.NoError(t, p.SetFailure(ctx, target, demoprovider.FailureAfterGrant))
	require.ErrorIs(t, p.GrantAccess(ctx, "operation", target), context.DeadlineExceeded)
	restarted := demoprovider.NewProvider(db)
	actual, err = restarted.CheckAccess(ctx, target)
	require.NoError(t, err)
	assert.Equal(t, accessrequest.ProviderAccess{Granted: true, AccessLevel: "Developer"}, actual)
	require.NoError(t, restarted.GrantAccess(ctx, "operation", target))
	require.NoError(t, db.Model(&models.DemoProviderOperation{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestProviderRejectsUnknownTargets(t *testing.T) {
	p, db := providerDB(t)
	ctx := context.Background()
	for _, scenario := range []string{"employee", "resource", "level"} {
		t.Run(scenario, func(t *testing.T) {
			target := accessrequest.GrantTarget{EmployeeID: "intern", Service: "gitlab", ExternalID: "demo/gitlab", AccessLevel: "Reporter"}
			want := demoprovider.ErrUnsupportedLevel
			switch scenario {
			case "employee":
				target.EmployeeID, want = "unknown", demoprovider.ErrEmployeeNotFound
			case "resource":
				target.ExternalID, want = "unknown", demoprovider.ErrResourceNotFound
			case "level":
				target.AccessLevel = "Owner"
			}
			require.ErrorIs(t, p.GrantAccess(ctx, "operation", target), want)
			_, err := p.CheckAccess(ctx, target)
			require.ErrorIs(t, err, want)
		})
	}
	var count int64
	require.NoError(t, db.Model(&models.DemoProviderOperation{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestProviderRollsBackOperationWhenRightsCannotBeSaved(t *testing.T) {
	p, db := providerDB(t)
	ctx := context.Background()
	target := accessrequest.GrantTarget{EmployeeID: "intern", Service: "gitlab", ExternalID: "demo/gitlab", AccessLevel: "Developer"}
	require.NoError(t, db.Exec(`CREATE TRIGGER test_fail_demo_grant BEFORE UPDATE OF access_level ON demo_provider_accesses
BEGIN SELECT RAISE(ABORT, 'test write failure'); END`).Error)
	require.ErrorContains(t, p.GrantAccess(ctx, "operation", target), "test write failure")
	var operations, states int64
	require.NoError(t, db.Model(&models.DemoProviderOperation{}).Count(&operations).Error)
	require.NoError(t, db.Model(&models.DemoProviderAccess{}).Count(&states).Error)
	assert.Zero(t, operations)
	assert.Zero(t, states)
	require.NoError(t, db.Exec("DROP TRIGGER test_fail_demo_grant").Error)
	require.NoError(t, p.GrantAccess(ctx, "operation", target))
	actual, err := p.CheckAccess(ctx, target)
	require.NoError(t, err)
	assert.Equal(t, accessrequest.ProviderAccess{Granted: true, AccessLevel: "Developer"}, actual)
}
