package accessrequest

import (
	"context"
	"strings"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationsRejectBlankIdentifiers(t *testing.T) {
	s := NewService(nil)
	ctx := context.Background()
	for _, id := range []string{"", " ", "\t\n", "\u00a0"} {
		t.Run(id, func(t *testing.T) {
			preparation, err := s.PrepareAccessRequest(ctx, "intern", id)
			require.ErrorIs(t, err, ErrInvalidInput)
			assert.Empty(t, preparation)

			request, created, err := s.ConfirmAccessRequest(ctx, "intern", id)
			require.ErrorIs(t, err, ErrInvalidInput)
			assert.Empty(t, request)
			assert.False(t, created)

			request, err = s.GetAccessRequest(ctx, "intern", id)
			require.ErrorIs(t, err, ErrInvalidInput)
			assert.Empty(t, request)

			request, err = s.DecideAccessRequest(ctx, "manager", id, DecisionInput{Decision: DecisionApprove})
			require.ErrorIs(t, err, ErrInvalidInput)
			assert.Empty(t, request)

			request, err = s.RetryAccessIssuance(ctx, "manager", id)
			require.ErrorIs(t, err, ErrInvalidInput)
			assert.Empty(t, request)

			employee, err := requestEmployee(nil, id, "UPDATE")
			require.ErrorIs(t, err, auth.ErrEmployeeNotFound)
			assert.Empty(t, employee)
		})
	}
}

func TestDecideAccessRequestRejectsInvalidDecision(t *testing.T) {
	tests := []struct {
		name  string
		input DecisionInput
	}{
		{"missing decision", DecisionInput{}},
		{"unknown decision", DecisionInput{Decision: "cancel"}},
		{"case sensitive decision", DecisionInput{Decision: "APPROVE"}},
		{"rejection without reason", DecisionInput{Decision: DecisionReject}},
		{"rejection with blank reason", DecisionInput{Decision: DecisionReject, Reason: " \t\n\u00a0"}},
		{"approval reason too long", DecisionInput{Decision: DecisionApprove, Reason: strings.Repeat("a", 2001)}},
		{"rejection reason too long", DecisionInput{Decision: DecisionReject, Reason: strings.Repeat("я", 2001)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := NewService(nil).DecideAccessRequest(context.Background(), "manager", "request", tt.input)
			require.ErrorIs(t, err, ErrInvalidInput)
			assert.Empty(t, request)
		})
	}
}

func TestListAccessRequestsRejectsInvalidPagination(t *testing.T) {
	for _, filter := range []RequestFilter{
		{Limit: -1}, {Limit: 101}, {Offset: -1}, {Limit: 20, Offset: -1},
	} {
		result, err := NewService(nil).ListAccessRequests(context.Background(), "intern", filter)
		require.ErrorIs(t, err, ErrInvalidInput, "filter: %+v", filter)
		assert.Empty(t, result)
	}
}

func TestRequireInternProfileRejectsOtherRoles(t *testing.T) {
	for _, role := range []models.EmployeeRole{models.RoleManager, "", "admin"} {
		t.Run(string(role), func(t *testing.T) {
			require.ErrorIs(t, requireInternProfile(nil, models.Employee{Role: role}), auth.ErrForbidden)
		})
	}
}

func TestProcessNextIssuanceRejectsMissingProvider(t *testing.T) {
	processed, err := NewService(nil).ProcessNextIssuance(context.Background(), nil)
	require.ErrorIs(t, err, ErrInvalidInput)
	assert.False(t, processed)
}
