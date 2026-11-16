package chat_test

import (
	"context"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/chat"
	appdb "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/db"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoseed"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	demodata "github.com/1BaEdeV/am-cp-ai-buddy/backend/seeds/demo"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/tests/integration/testdb"
	"github.com/stretchr/testify/require"
)

func TestPostgresInterruptedQuestionReleasesChat(t *testing.T) {
	db := testdb.Postgres(t)
	ctx := context.Background()
	require.NoError(t, appdb.Migrate(ctx, db))
	_, err := demoseed.Load(ctx, db, demodata.Files)
	require.NoError(t, err)
	s := chat.NewService(db, nil, nil, nil)
	run, err := s.SendMessage(ctx, "intern-alina", "team-intro", "first", chat.MessageInput{Text: "Кто мой наставник?"})
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.ChatRun{}).Where("id = ?", run.ID).
		Updates(map[string]any{"status": "running", "updated_at": time.Now().UTC().Add(-3 * time.Minute)}).Error)
	_, err = s.SendMessage(ctx, "intern-alina", "team-intro", "second", chat.MessageInput{Text: "Расскажи о команде"})
	require.ErrorIs(t, err, chat.ErrChatBusy)
	processed, err := s.ProcessNextRun(ctx)
	require.NoError(t, err)
	require.False(t, processed)
	history, err := s.GetTaskChat(ctx, "intern-alina", "team-intro")
	require.NoError(t, err)
	require.Equal(t, "failed", history.LatestRun.Status)
	require.Equal(t, "processing_interrupted", history.LatestRun.Error.Code)
	next, err := s.SendMessage(ctx, "intern-alina", "team-intro", "second", chat.MessageInput{Text: "Расскажи о команде"})
	require.NoError(t, err)
	require.Equal(t, "queued", next.Status)
	require.NotEqual(t, run.ID, next.ID)
	repeated, err := s.SendMessage(ctx, "intern-alina", "team-intro", "first", chat.MessageInput{Text: "Кто мой наставник?"})
	require.NoError(t, err)
	require.Equal(t, run.ID, repeated.ID)
	require.Equal(t, "failed", repeated.Status)
}
