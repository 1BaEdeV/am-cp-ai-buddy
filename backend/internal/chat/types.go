package chat

import "time"

type MessageInput struct { // текст вопроса, который стажёр отправляет в чат.
	Text string `json:"text"`
}


type Message struct { //  одно сообщение в переписке.
	ID               string    `json:"id"`
	Role             string    `json:"role"`
	Text             string    `json:"text"`
	CreatedAt        time.Time `json:"createdAt"`
	ReplyToMessageID *string   `json:"replyToMessageId,omitempty"`
	SourceLabel      *string   `json:"sourceLabel,omitempty"`
}


type APIError struct {  //ошибка обработки вопроса, возвращённая в состоянии failed.
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
	TaskID    string   `json:"taskId"`
	Messages  []Message `json:"messages"`
	LatestRun *Run      `json:"latestRun,omitempty"`
}