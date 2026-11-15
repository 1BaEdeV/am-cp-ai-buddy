package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type agentStub struct {
	response AgentResponse
	err      error
	request  AgentRequest
}

func (s *agentStub) Reply(_ context.Context, request AgentRequest) (AgentResponse, error) {
	s.request = request
	return s.response, s.err
}

type contextBuilderStub struct{ value any }

func (s contextBuilderStub) Build(context.Context, string, string) (any, error) { return s.value, nil }

type requestPreparerStub struct {
	preparation accessrequest.Preparation
	resourceID  string
}

func (s *requestPreparerStub) PrepareAccessRequest(_ context.Context, _, resourceID string) (accessrequest.Preparation, error) {
	s.resourceID = resourceID
	return s.preparation, nil
}

func TestProcessNextRunPersistsAgentReply(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Employee{}, &models.Profile{}, &models.ChatMessage{}, &models.ChatRun{}))
	message := models.ChatMessage{ID: "message-1", EmployeeID: "intern-1", TaskID: "team-intro", Role: "user", Text: "Кто мой наставник?", CreatedAt: time.Now().UTC()}
	require.NoError(t, db.Create(&message).Error)
	run := models.ChatRun{ID: "run-1", EmployeeID: "intern-1", TaskID: "team-intro", UserMessageID: message.ID, IdempotencyKey: "key-1", Status: "queued", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	require.NoError(t, db.Create(&run).Error)
	agent := &agentStub{response: AgentResponse{Text: "Твой наставник — Катя."}}
	service := NewService(db, agent, contextBuilderStub{value: map[string]string{"mentor": "Катя Волкова"}}, nil)

	processed, err := service.ProcessNextRun(context.Background())
	require.NoError(t, err)
	require.True(t, processed)
	require.Equal(t, "team-intro", agent.request.TaskID)
	require.Equal(t, message.Text, agent.request.Question)
	require.Equal(t, "user", agent.request.History[0].Role)
	require.NotNil(t, agent.request.Context)
	var savedRun models.ChatRun
	require.NoError(t, db.Take(&savedRun, "id = ?", run.ID).Error)
	require.Equal(t, "completed", savedRun.Status)
	require.NotNil(t, savedRun.AssistantMessageID)
	var answer models.ChatMessage
	require.NoError(t, db.Take(&answer, "id = ?", *savedRun.AssistantMessageID).Error)
	require.Equal(t, "Твой наставник — Катя.", answer.Text)
	require.Equal(t, message.ID, *answer.ReplyToMessageID)
	require.Equal(t, "Карточка команды", *answer.SourceLabel)
}

func TestProcessNextRunMarksAgentFailure(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Employee{}, &models.Profile{}, &models.ChatMessage{}, &models.ChatRun{}))
	message := models.ChatMessage{ID: "message-1", EmployeeID: "intern-1", TaskID: "working-accesses", Role: "user", Text: "Нужен доступ", CreatedAt: time.Now().UTC()}
	require.NoError(t, db.Create(&message).Error)
	run := models.ChatRun{ID: "run-1", EmployeeID: "intern-1", TaskID: "working-accesses", UserMessageID: message.ID, IdempotencyKey: "key-1", Status: "queued", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	require.NoError(t, db.Create(&run).Error)
	service := NewService(db, &agentStub{err: errors.New("provider unavailable")}, contextBuilderStub{value: map[string]string{}}, nil)

	processed, err := service.ProcessNextRun(context.Background())
	require.Error(t, err)
	require.True(t, processed)
	var savedRun models.ChatRun
	require.NoError(t, db.Take(&savedRun, "id = ?", run.ID).Error)
	require.Equal(t, "failed", savedRun.Status)
	require.Equal(t, "processing_failed", *savedRun.ErrorCode)
}

func TestProcessNextRunPreparesAndPersistsAccessDraft(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Employee{}, &models.Profile{}, &models.ChatMessage{}, &models.ChatRun{}))
	require.NoError(t, db.Create(&models.Employee{ID: "intern-1", Role: models.RoleIntern}).Error)
	require.NoError(t, db.Create(&models.Profile{ID: "profile-1", EmployeeID: "intern-1"}).Error)
	now := time.Now().UTC()
	message := models.ChatMessage{ID: "message-1", EmployeeID: "intern-1", TaskID: "working-accesses", Role: "user", Text: "Подготовь черновик GitLab", CreatedAt: now}
	require.NoError(t, db.Create(&message).Error)
	run := models.ChatRun{ID: "run-1", EmployeeID: "intern-1", TaskID: "working-accesses", UserMessageID: message.ID, IdempotencyKey: "key-1", Status: "queued", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, db.Create(&run).Error)
	draft := accessrequest.Draft{ID: "draft-1", ResourceID: "gitlab-search-api", ApproverID: "manager-1", CreatedAt: now, ExpiresAt: now.Add(15 * time.Minute)}
	preparer := &requestPreparerStub{preparation: accessrequest.Preparation{Status: accessrequest.PreparationReady, Draft: &draft}}
	agent := &agentStub{response: AgentResponse{Text: "Сейчас подготовлю.", PrepareAccessRequestResourceID: "gitlab-search-api"}}
	service := NewService(db, agent, contextBuilderStub{value: map[string]string{}}, preparer)

	processed, err := service.ProcessNextRun(context.Background())
	require.NoError(t, err)
	require.True(t, processed)
	require.Equal(t, "gitlab-search-api", preparer.resourceID)
	var saved models.ChatMessage
	require.NoError(t, db.Take(&saved, "role = ?", "assistant").Error)
	require.NotNil(t, saved.AccessDraftJSON)
	var savedDraft accessrequest.Draft
	require.NoError(t, json.Unmarshal([]byte(*saved.AccessDraftJSON), &savedDraft))
	require.Equal(t, draft.ID, savedDraft.ID)

	chat, err := service.GetTaskChat(context.Background(), "intern-1", "working-accesses")
	require.NoError(t, err)
	require.Len(t, chat.Messages, 2)
	require.NotNil(t, chat.Messages[1].AccessDraft)
	require.Equal(t, draft.ID, chat.Messages[1].AccessDraft.ID)
	require.Contains(t, chat.Messages[1].Text, "подтверди")
}
