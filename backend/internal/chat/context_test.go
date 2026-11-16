package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/onboarding"
	"github.com/stretchr/testify/require"
)

type introContextStub struct {
	result onboarding.TaskDetails
	err    error
}

func (s introContextStub) GetTeamIntro(context.Context, string) (onboarding.TaskDetails, error) {
	return s.result, s.err
}

type accessesContextStub struct {
	result access.WorkingAccesses
	err    error
}

func (s accessesContextStub) ListRequiredAccesses(context.Context, string) (access.WorkingAccesses, error) {
	return s.result, s.err
}

type requestsContextStub struct {
	result accessrequest.RequestList
	err    error
}

func (s requestsContextStub) ListAccessRequests(context.Context, string, accessrequest.RequestFilter) (accessrequest.RequestList, error) {
	return s.result, s.err
}

func TestAppContextBuilderBuildsTaskContexts(t *testing.T) {
	intro := onboarding.TaskDetails{TeamIntro: &onboarding.TeamIntro{Team: onboarding.TeamRef{ID: "team-search", Name: "Поиск"}}}
	accesses := access.WorkingAccesses{Progress: access.Progress{Completed: 2, Total: 4}}
	requests := accessrequest.RequestList{Counts: accessrequest.RequestCounts{Pending: 1, Issuing: 2, Granted: 3, Rejected: 4, Failed: 5}}
	builder := NewAppContextBuilder(introContextStub{result: intro}, accessesContextStub{result: accesses}, requestsContextStub{result: requests})

	teamContext, err := builder.Build(context.Background(), "intern-1", "team-intro")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"teamIntro": intro.TeamIntro}, teamContext)

	accessContext, err := builder.Build(context.Background(), "intern-1", "working-accesses")
	require.NoError(t, err)
	values := accessContext.(map[string]any)
	require.Equal(t, intro.TeamIntro.Team, values["team"])
	require.Equal(t, intro.TeamIntro, values["teamIntro"])
	require.Equal(t, accesses, values["workingAccesses"])
	require.Equal(t, map[string]int64{"pending": 1, "issuing": 2, "granted": 3, "rejected": 4, "failed": 5}, values["requestCounts"])

	_, err = builder.Build(context.Background(), "intern-1", "first-task")
	require.ErrorIs(t, err, ErrInvalidTask)
}

func TestAppContextBuilderReturnsSourceErrors(t *testing.T) {
	introErr := errors.New("intro unavailable")
	_, err := NewAppContextBuilder(introContextStub{err: introErr}, accessesContextStub{}, requestsContextStub{}).
		Build(context.Background(), "intern-1", "team-intro")
	require.ErrorIs(t, err, introErr)

	accessErr := errors.New("access unavailable")
	_, err = NewAppContextBuilder(introContextStub{result: onboarding.TaskDetails{TeamIntro: &onboarding.TeamIntro{}}}, accessesContextStub{err: accessErr}, requestsContextStub{}).
		Build(context.Background(), "intern-1", "working-accesses")
	require.ErrorIs(t, err, accessErr)

	requestErr := errors.New("requests unavailable")
	_, err = NewAppContextBuilder(introContextStub{result: onboarding.TaskDetails{TeamIntro: &onboarding.TeamIntro{}}}, accessesContextStub{}, requestsContextStub{err: requestErr}).
		Build(context.Background(), "intern-1", "working-accesses")
	require.ErrorIs(t, err, requestErr)
}
