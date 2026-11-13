package accessrequest_test

import (
	"context"
	"strings"
	"testing"

	requestservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
)

func TestManagerDecisionAndIdempotency(t *testing.T) {
	for _, decision := range []requestservice.Decision{requestservice.DecisionApprove, requestservice.DecisionReject} {
		t.Run(string(decision), func(t *testing.T) {
			s, db := requestService(t)
			request := pendingRequest(t, s, "gitlab")
			input := requestservice.DecisionInput{Decision: decision, Reason: "  Причина решения  "}
			_, err := s.DecideAccessRequest(context.Background(), "intern", request.ID, input)
			require.ErrorIs(t, err, auth.ErrForbidden)
			result, err := s.DecideAccessRequest(context.Background(), "manager", request.ID, input)
			require.NoError(t, err)
			require.NotNil(t, result.DecidedAt)
			require.Equal(t, "Причина решения", result.DecisionReason)
			again, err := s.DecideAccessRequest(context.Background(), "manager", request.ID, input)
			require.NoError(t, err)
			require.Equal(t, result.ID, again.ID)
			var jobs int64
			require.NoError(t, db.Model(&models.AccessIssuanceJob{}).Count(&jobs).Error)
			if decision == requestservice.DecisionApprove {
				require.Equal(t, models.AccessRequestIssuing, result.Status)
				require.EqualValues(t, 1, jobs)
				input.Decision = requestservice.DecisionReject
			} else {
				require.Equal(t, models.AccessRequestRejected, result.Status)
				require.Zero(t, jobs)
				input.Decision = requestservice.DecisionApprove
			}
			_, err = s.DecideAccessRequest(context.Background(), "manager", request.ID, input)
			require.ErrorIs(t, err, requestservice.ErrRequestAlreadyDecided)
		})
	}
}

func TestDecisionValidationAndAtomicApproval(t *testing.T) {
	s, db := requestService(t)
	request := pendingRequest(t, s, "gitlab")
	for _, input := range []requestservice.DecisionInput{
		{Decision: requestservice.DecisionReject, Reason: "  "}, {Decision: "unknown"},
		{Decision: requestservice.DecisionApprove, Reason: strings.Repeat("я", 2001)},
	} {
		_, err := s.DecideAccessRequest(context.Background(), "manager", request.ID, input)
		require.ErrorIs(t, err, requestservice.ErrInvalidInput)
	}
	require.NoError(t, db.Migrator().DropTable(&models.AccessIssuanceJob{}))
	_, err := s.DecideAccessRequest(context.Background(), "manager", request.ID, requestservice.DecisionInput{Decision: requestservice.DecisionApprove})
	require.Error(t, err)
	saved, err := s.GetAccessRequest(context.Background(), "intern", request.ID)
	require.NoError(t, err)
	require.Equal(t, models.AccessRequestPending, saved.Status)
	require.Nil(t, saved.DecidedAt)
}
