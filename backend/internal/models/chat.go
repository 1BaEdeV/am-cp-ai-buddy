package models

import "time"

type ChatMessage struct { // cохранённое сообщение стажёра или AI-buddy.
	ID         string    `gorm:"primaryKey"`
	EmployeeID string    `gorm:"not null;index:idx_chat_messages_owner_task_time,priority:1"`
	TaskID     string    `gorm:"not null;index:idx_chat_messages_owner_task_time,priority:2"`
	Role       string    `gorm:"not null"`
	Text       string    `gorm:"type:text;not null"`
	CreatedAt  time.Time `gorm:"not null;index:idx_chat_messages_owner_task_time,priority:3"`

	ReplyToMessageID *string
	SourceLabel      *string
	AccessDraftJSON  *string `gorm:"type:text"`
}

type ChatRun struct { //  состояние обработки одного вопроса в чате
	ID             string `gorm:"primaryKey"`
	EmployeeID     string `gorm:"not null;uniqueIndex:ux_chat_runs_idempotency,priority:1;uniqueIndex:ux_chat_runs_active,priority:1,where:status = 'queued' OR status = 'running'"`
	TaskID         string `gorm:"not null;uniqueIndex:ux_chat_runs_idempotency,priority:2;uniqueIndex:ux_chat_runs_active,priority:2,where:status = 'queued' OR status = 'running'"`
	UserMessageID  string `gorm:"not null;uniqueIndex"`
	IdempotencyKey string `gorm:"not null;uniqueIndex:ux_chat_runs_idempotency,priority:3"`

	Status             string `gorm:"not null"`
	AssistantMessageID *string
	ErrorCode          *string
	ErrorMessage       *string

	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}
