package demoprovider_test

import (
	"context"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoprovider"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderWithIssuanceServiceUpdatesProgress(t *testing.T) {
	for _, mode := range []demoprovider.FailureMode{demoprovider.FailureNone, demoprovider.FailureGrant, demoprovider.FailureAfterGrant} {
		t.Run(string(mode), func(t *testing.T) {
			p, db := providerDB(t)
			ctx := context.Background()
			requests, accesses := accessrequest.NewService(db), access.NewService(db)
			for service, level := range map[string]string{"wiki": "read", "youtrack": "member"} {
				target := accessrequest.GrantTarget{EmployeeID: "intern", Service: service, ExternalID: "demo/" + service, AccessLevel: level}
				require.NoError(t, p.SeedAccess(ctx, target))
				require.NoError(t, db.Create(&models.EmployeeAccess{ID: service, EmployeeID: "intern", ResourceID: service, AccessLevel: level, VerifiedAt: time.Now().UTC()}).Error)
			}
			card, err := accesses.ListRequiredAccesses(ctx, "intern")
			require.NoError(t, err)
			assert.Equal(t, 2, card.Progress.Completed)
			assert.Equal(t, 4, card.Progress.Total)

			for i, resourceID := range []string{"gitlab", "grafana"} {
				preparation, err := requests.PrepareAccessRequest(ctx, "intern", resourceID)
				require.NoError(t, err)
				require.NotNil(t, preparation.Draft)
				request, created, err := requests.ConfirmAccessRequest(ctx, "intern", preparation.Draft.ID)
				require.NoError(t, err)
				assert.True(t, created)
				_, err = requests.DecideAccessRequest(ctx, "manager", request.ID, accessrequest.DecisionInput{Decision: accessrequest.DecisionApprove})
				require.NoError(t, err)
				target := accessrequest.GrantTarget{EmployeeID: "intern", Service: request.Parameters.ResourceService, ExternalID: request.Parameters.ResourceExternalID, AccessLevel: request.Parameters.AccessLevel}
				if i == 0 && mode != demoprovider.FailureNone {
					require.NoError(t, p.SetFailure(ctx, target, mode))
					processed, err := requests.ProcessNextIssuance(ctx, p)
					assert.True(t, processed)
					if mode == demoprovider.FailureGrant {
						require.ErrorIs(t, err, demoprovider.ErrGrantFailed)
					} else {
						require.ErrorIs(t, err, context.DeadlineExceeded)
					}
					failed, err := requests.GetAccessRequest(ctx, "intern", request.ID)
					require.NoError(t, err)
					assert.Equal(t, models.AccessRequestFailed, failed.Status)
					assert.NotEmpty(t, failed.LastError)
					card, err = accesses.ListRequiredAccesses(ctx, "intern")
					require.NoError(t, err)
					assert.Equal(t, 2, card.Progress.Completed)
					if mode == demoprovider.FailureGrant {
						require.NoError(t, p.SetFailure(ctx, target, demoprovider.FailureNone))
					}
					_, err = requests.RetryAccessIssuance(ctx, "manager", request.ID)
					require.NoError(t, err)
				}
				processed, err := requests.ProcessNextIssuance(ctx, demoprovider.NewProvider(db))
				require.NoError(t, err)
				assert.True(t, processed)
				granted, err := requests.GetAccessRequest(ctx, "intern", request.ID)
				require.NoError(t, err)
				assert.Equal(t, models.AccessRequestGranted, granted.Status)
				require.NotNil(t, granted.GrantedAt)
				card, err = accesses.ListRequiredAccesses(ctx, "intern")
				require.NoError(t, err)
				assert.Equal(t, 3+i, card.Progress.Completed)
			}
			var operations, completedJobs int64
			require.NoError(t, db.Model(&models.DemoProviderOperation{}).Count(&operations).Error)
			assert.EqualValues(t, 2, operations, "потерянный ответ не должен приводить к повторной выдаче")
			require.NoError(t, db.Model(&models.AccessIssuanceJob{}).Where("completed_at IS NOT NULL").Count(&completedJobs).Error)
			assert.EqualValues(t, 2, completedJobs)
		})
	}
}
