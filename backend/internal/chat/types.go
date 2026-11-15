package chat

import (
	"context"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
)

type MessageInput struct { // текст вопроса, который стажёр отправляет в чат.
	Text string `json:"text"`
}

type Message struct { //  одно сообщение в переписке.
	ID               string               `json:"id"`
	Role             string               `json:"role"`
	Text             string               `json:"text"`
	CreatedAt        time.Time            `json:"createdAt"`
	ReplyToMessageID *string              `json:"replyToMessageId,omitempty"`
	SourceLabel      *string              `json:"sourceLabel,omitempty"`
	AccessDraft      *accessrequest.Draft `json:"accessDraft,omitempty"`
}

type APIError struct { //ошибка обработки вопроса, возвращённая в состоянии failed.
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Run struct { // состояние обработки одного вопроса AI-buddy.
	ID                 string    `json:"id"`
	UserMessageID      string    `json:"userMessageId"`
	Status             string    `json:"status"`
	AssistantMessageID *string   `json:"assistantMessageId,omitempty"`
	Error              *APIError `json:"error,omitempty"`
}

type TaskChat struct { //история переписки по одному этапу и состояние последнего ответа.
	TaskID    string    `json:"taskId"`
	Messages  []Message `json:"messages"`
	LatestRun *Run      `json:"latestRun,omitempty"`
}

// AgentRequest contains the question, recent chat history, and trusted app data.
type AgentRequest struct {
	TaskID   string    `json:"taskId"`
	Question string    `json:"question"`
	History  []Message `json:"history"`
	Context  any       `json:"context"`
}

// AgentResponse is the text generated for the trainee.
type AgentResponse struct {
	Text                           string `json:"text"`
	PrepareAccessRequestResourceID string `json:"prepareAccessRequestResourceId,omitempty"`
}

// Agent calls the ML assistant.
type Agent interface {
	Reply(context.Context, AgentRequest) (AgentResponse, error)
}

// ContextBuilder loads verified data needed to answer a task chat.
type ContextBuilder interface {
	Build(context.Context, string, string) (any, error)
}
