package chat

import (
	"context"
	"fmt"

	accessservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/onboarding"
)

type teamIntroReader interface {
	GetTeamIntro(context.Context, string) (onboarding.TaskDetails, error)
}

type requiredAccessesReader interface {
	ListRequiredAccesses(context.Context, string) (accessservice.WorkingAccesses, error)
}

type accessRequestsReader interface {
	ListAccessRequests(context.Context, string, accessrequest.RequestFilter) (accessrequest.RequestList, error)
}

// AppContextBuilder supplies only data the trainee is allowed to see.
type AppContextBuilder struct {
	teamIntro teamIntroReader
	accesses  requiredAccessesReader
	requests  accessRequestsReader
}

func NewAppContextBuilder(
	teamIntro teamIntroReader,
	accesses requiredAccessesReader,
	requests accessRequestsReader,
) *AppContextBuilder {
	return &AppContextBuilder{teamIntro: teamIntro, accesses: accesses, requests: requests}
}

func (b *AppContextBuilder) Build(ctx context.Context, employeeID, taskID string) (any, error) {
	intro, err := b.teamIntro.GetTeamIntro(ctx, employeeID)
	if err != nil {
		return nil, fmt.Errorf("load team context: %w", err)
	}
	switch taskID {
	case "team-intro":
		return map[string]any{"teamIntro": intro.TeamIntro}, nil
	case "working-accesses":
		accesses, err := b.accesses.ListRequiredAccesses(ctx, employeeID)
		if err != nil {
			return nil, fmt.Errorf("load access context: %w", err)
		}
		requests, err := b.requests.ListAccessRequests(ctx, employeeID, accessrequest.RequestFilter{})
		if err != nil {
			return nil, fmt.Errorf("load request context: %w", err)
		}
		return map[string]any{
			"team":            intro.TeamIntro.Team,
			"teamIntro":       intro.TeamIntro,
			"workingAccesses": accesses,
			"requestCounts": map[string]int64{
				"pending":  requests.Counts.Pending,
				"issuing":  requests.Counts.Issuing,
				"granted":  requests.Counts.Granted,
				"rejected": requests.Counts.Rejected,
				"failed":   requests.Counts.Failed,
			},
		}, nil
	default:
		return nil, ErrInvalidTask
	}
}
