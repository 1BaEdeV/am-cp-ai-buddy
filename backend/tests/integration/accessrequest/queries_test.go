package accessrequest_test

import (
	"context"
	"errors"
	"testing"

	requestservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
)

func TestRequestVisibilityFiltersAndCounts(t *testing.T) {
	s, db := requestService(t)
	rejected := pendingRequest(t, s, "gitlab")
	_, err := s.DecideAccessRequest(context.Background(), "manager", rejected.ID, requestservice.DecisionInput{Decision: requestservice.DecisionReject, Reason: "Причина"})
	require.NoError(t, err)
	pending := pendingRequest(t, s, "gitlab")
	other := pendingRequest(t, s, "grafana")
	_, err = s.DecideAccessRequest(context.Background(), "manager", other.ID, requestservice.DecisionInput{Decision: requestservice.DecisionApprove})
	require.NoError(t, err)
	for _, actor := range []string{"intern", "manager"} {
		counts := requestservice.RequestCounts{Total: 3, Active: 2, Completed: 1, Pending: 1, Issuing: 1, Rejected: 1}
		if actor == "manager" {
			counts = requestservice.RequestCounts{Total: 1, Active: 1, Pending: 1}
		}
		list, err := s.ListAccessRequests(context.Background(), actor, requestservice.RequestFilter{Group: "active", ResourceID: "gitlab", Limit: 1})
		require.NoError(t, err)
		require.EqualValues(t, 1, list.Total)
		require.Len(t, list.Items, 1)
		require.Equal(t, pending.ID, list.Items[0].ID)
		require.Equal(t, counts, list.Counts)
		page, err := s.ListAccessRequests(context.Background(), actor, requestservice.RequestFilter{Limit: 1, Offset: 3})
		require.NoError(t, err)
		require.Empty(t, page.Items)
		require.Equal(t, counts.Total, page.Total)
	}
	require.NoError(t, db.Create(&models.Employee{ID: "other-manager", Email: "other-manager@example.test", TeamID: "search", PositionID: "go", Role: models.RoleManager}).Error)
	for _, actor := range []string{"other-intern", "other-manager"} {
		_, err := s.GetAccessRequest(context.Background(), actor, pending.ID)
		require.ErrorIs(t, err, requestservice.ErrRequestNotFound)
		list, err := s.ListAccessRequests(context.Background(), actor, requestservice.RequestFilter{})
		require.NoError(t, err)
		require.Empty(t, list.Items)
		require.Zero(t, list.Counts.Total)
		if actor == "other-manager" {
			_, err = s.DecideAccessRequest(context.Background(), actor, pending.ID, requestservice.DecisionInput{Decision: requestservice.DecisionApprove})
			require.ErrorIs(t, err, requestservice.ErrRequestNotFound)
		}
	}
	for _, filter := range []requestservice.RequestFilter{{Limit: -1}, {Limit: 101}, {Offset: -1}, {Status: "draft"}, {Group: "unknown"}} {
		_, err := s.ListAccessRequests(context.Background(), "intern", filter)
		require.ErrorIs(t, err, requestservice.ErrInvalidInput)
	}
}

func TestManagerListIsPendingApprovalQueue(t *testing.T) {
	s, _ := requestService(t)
	first := pendingRequest(t, s, "gitlab")
	second := pendingRequest(t, s, "grafana")
	ctx := context.Background()
	// Фильтры стажёра для руководителя не действуют, остаётся только пагинация.
	page, err := s.ListAccessRequests(ctx, "manager", requestservice.RequestFilter{
		Group: "completed", Status: models.AccessRequestGranted, ResourceID: "missing", Limit: 1, Offset: 1,
	})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, models.AccessRequestPending, page.Items[0].Status)
	require.EqualValues(t, 2, page.Total)
	require.Equal(t, requestservice.RequestCounts{Total: 2, Active: 2, Pending: 2}, page.Counts)

	_, err = s.DecideAccessRequest(ctx, "manager", first.ID, requestservice.DecisionInput{Decision: requestservice.DecisionApprove})
	require.NoError(t, err)
	assertQueue := func() {
		t.Helper()
		list, err := s.ListAccessRequests(ctx, "manager", requestservice.RequestFilter{})
		require.NoError(t, err)
		require.Len(t, list.Items, 1)
		require.Equal(t, second.ID, list.Items[0].ID)
		require.Equal(t, requestservice.RequestCounts{Total: 1, Active: 1, Pending: 1}, list.Counts)
	}
	assertQueue()
	card, err := s.GetAccessRequest(ctx, "manager", first.ID)
	require.NoError(t, err)
	require.Equal(t, models.AccessRequestIssuing, card.Status)

	provider := &providerStub{grant: func(context.Context, string, requestservice.GrantTarget) error {
		return errors.New("выдача недоступна")
	}}
	_, err = s.ProcessNextIssuance(ctx, provider)
	require.Error(t, err)
	assertQueue()
	card, err = s.GetAccessRequest(ctx, "manager", first.ID)
	require.NoError(t, err)
	require.Equal(t, models.AccessRequestFailed, card.Status)
	_, err = s.RetryAccessIssuance(ctx, "manager", first.ID)
	require.NoError(t, err)
	provider.grant = nil
	_, err = s.ProcessNextIssuance(ctx, provider)
	require.NoError(t, err)
	assertQueue()
	card, err = s.GetAccessRequest(ctx, "manager", first.ID)
	require.NoError(t, err)
	require.Equal(t, models.AccessRequestGranted, card.Status)

	_, err = s.DecideAccessRequest(ctx, "manager", second.ID, requestservice.DecisionInput{Decision: requestservice.DecisionReject, Reason: "Отказ"})
	require.NoError(t, err)
	empty, err := s.ListAccessRequests(ctx, "manager", requestservice.RequestFilter{})
	require.NoError(t, err)
	require.Empty(t, empty.Items)
	require.Zero(t, empty.Total)
	require.Equal(t, requestservice.RequestCounts{}, empty.Counts)
	card, err = s.GetAccessRequest(ctx, "manager", second.ID)
	require.NoError(t, err)
	require.Equal(t, models.AccessRequestRejected, card.Status)
}
