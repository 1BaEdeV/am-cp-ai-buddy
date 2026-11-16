package chat

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type replyFunc func(context.Context, AgentRequest) (AgentResponse, error)

func (f replyFunc) Reply(ctx context.Context, request AgentRequest) (AgentResponse, error) {
	return f(ctx, request)
}

func recoveryDatabase(t *testing.T) (*gorm.DB, models.ChatRun) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	require.NoError(t, db.AutoMigrate(&models.ChatMessage{}, &models.ChatRun{}))
	now := time.Now().UTC()
	message := models.ChatMessage{ID: "question", EmployeeID: "intern", TaskID: "team-intro", Role: "user", Text: "Расскажи о команде", CreatedAt: now}
	require.NoError(t, db.Create(&message).Error)
	run := models.ChatRun{ID: "run", EmployeeID: message.EmployeeID, TaskID: message.TaskID,
		UserMessageID: message.ID, IdempotencyKey: "key", Status: "queued", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, db.Create(&run).Error)
	return db, run
}

func TestProcessNextRunSavesFailureAfterCancellation(t *testing.T) {
	db, run := recoveryDatabase(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agent := replyFunc(func(agentCtx context.Context, _ AgentRequest) (AgentResponse, error) {
		cancel()
		return AgentResponse{}, agentCtx.Err()
	})
	s := NewService(db, agent, contextBuilderStub{}, nil)
	processed, err := s.ProcessNextRun(ctx)
	require.True(t, processed)
	require.ErrorIs(t, err, context.Canceled)
	var saved models.ChatRun
	require.NoError(t, db.Take(&saved, "id = ?", run.ID).Error)
	require.Equal(t, "failed", saved.Status)
	require.Equal(t, "processing_failed", *saved.ErrorCode)
}

func TestProcessNextRunRecoversOnlyStaleRunningQuestions(t *testing.T) {
	for _, test := range []struct {
		name       string
		age        time.Duration
		wantStatus string
	}{
		{"interrupted", staleRunTimeout + time.Second, "failed"},
		{"still processing", staleRunTimeout - time.Second, "running"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, run := recoveryDatabase(t)
			require.NoError(t, db.Model(&models.ChatRun{}).Where("id = ?", run.ID).
				Updates(map[string]any{"status": "running", "updated_at": time.Now().UTC().Add(-test.age)}).Error)
			processed, err := NewService(db, nil, nil, nil).ProcessNextRun(context.Background())
			require.NoError(t, err)
			require.False(t, processed)
			var saved models.ChatRun
			require.NoError(t, db.Take(&saved, "id = ?", run.ID).Error)
			require.Equal(t, test.wantStatus, saved.Status)
			if saved.Status == "failed" {
				require.Equal(t, "processing_interrupted", *saved.ErrorCode)
			} else {
				require.Nil(t, saved.ErrorCode)
			}
		})
	}
}

func TestProcessNextRunDiscardsReplyForAlreadyFailedRun(t *testing.T) {
	db, run := recoveryDatabase(t)
	agent := replyFunc(func(context.Context, AgentRequest) (AgentResponse, error) {
		// Другой worker уже завершил зависший запуск. Поздний ответ не сохраняется.
		require.NoError(t, db.Model(&models.ChatRun{}).Where("id = ?", run.ID).Update("status", "failed").Error)
		return AgentResponse{Text: "Поздний ответ"}, nil
	})
	processed, err := NewService(db, agent, contextBuilderStub{}, nil).ProcessNextRun(context.Background())
	require.True(t, processed)
	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&models.ChatMessage{}).Where("role = ?", "assistant").Count(&count).Error)
	require.Zero(t, count)
	var saved models.ChatRun
	require.NoError(t, db.Take(&saved, "id = ?", run.ID).Error)
	require.Equal(t, "failed", saved.Status)
	require.Nil(t, saved.AssistantMessageID)
}
