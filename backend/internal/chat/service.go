package chat

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrEmployeeNotFound     = errors.New("сотрудник не найден")
	ErrForbidden            = errors.New("чат доступен только стажёру")
	ErrProfileRequired      = errors.New("сначала заполни профиль")
	ErrInvalidTask          = errors.New("неизвестный этап чата")
	ErrInvalidMessage       = errors.New("проверь текст вопроса и ключ повтора")
	ErrChatBusy             = errors.New("в этом чате уже готовится ответ")
	ErrIdempotencyKeyReused = errors.New("ключ повтора уже использован для другого текста")
)

type Service struct {
	db       *gorm.DB
	agent    Agent
	context  ContextBuilder
	requests accessRequestPreparer
}

type accessRequestPreparer interface {
	PrepareAccessRequest(context.Context, string, string) (accessrequest.Preparation, error)
}

func NewService(db *gorm.DB, agent Agent, contextBuilder ContextBuilder, requests accessRequestPreparer) *Service {
	return &Service{db: db, agent: agent, context: contextBuilder, requests: requests}
}

func (s *Service) GetTaskChat(
	ctx context.Context,
	employeeID string,
	taskID string,
) (TaskChat, error) {
	if taskID != "team-intro" && taskID != "working-accesses" {
		return TaskChat{}, ErrInvalidTask
	}

	var employee models.Employee
	err := s.db.WithContext(ctx).
		Take(&employee, "id = ?", employeeID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TaskChat{}, ErrEmployeeNotFound
	}
	if err != nil {
		return TaskChat{}, fmt.Errorf("найти сотрудника для чата: %w", err)
	}
	if employee.Role != models.RoleIntern {
		return TaskChat{}, ErrForbidden
	}

	var profile models.Profile
	err = s.db.WithContext(ctx).
		Select("id").
		Take(&profile, "employee_id = ?", employee.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TaskChat{}, ErrProfileRequired
	}
	if err != nil {
		return TaskChat{}, fmt.Errorf("проверить профиль для чата: %w", err)
	}

	var storedMessages []models.ChatMessage
	err = s.db.WithContext(ctx).
		Where("employee_id = ? AND task_id = ?", employee.ID, taskID).
		Order("created_at ASC").
		Order("id ASC").
		Find(&storedMessages).Error
	if err != nil {
		return TaskChat{}, fmt.Errorf("прочитать сообщения чата: %w", err)
	}

	result := TaskChat{
		TaskID:   taskID,
		Messages: make([]Message, 0, len(storedMessages)),
	}

	for _, stored := range storedMessages {
		result.Messages = append(result.Messages, Message{
			ID:               stored.ID,
			Role:             stored.Role,
			Text:             stored.Text,
			CreatedAt:        stored.CreatedAt,
			ReplyToMessageID: stored.ReplyToMessageID,
			SourceLabel:      stored.SourceLabel,
		})
		if stored.AccessDraftJSON != nil {
			var draft accessrequest.Draft
			if err := json.Unmarshal([]byte(*stored.AccessDraftJSON), &draft); err != nil {
				return TaskChat{}, fmt.Errorf("прочитать черновик из сообщения: %w", err)
			}
			result.Messages[len(result.Messages)-1].AccessDraft = &draft
		}
	}

	var storedRun models.ChatRun
	err = s.db.WithContext(ctx).
		Where("employee_id = ? AND task_id = ?", employee.ID, taskID).
		Order("created_at DESC").
		Order("id DESC").
		Take(&storedRun).Error

	if err == nil {
		result.LatestRun = &Run{
			ID:                 storedRun.ID,
			UserMessageID:      storedRun.UserMessageID,
			Status:             storedRun.Status,
			AssistantMessageID: storedRun.AssistantMessageID,
		}
		if storedRun.ErrorCode != nil && storedRun.ErrorMessage != nil {
			result.LatestRun.Error = &APIError{
				Code:    *storedRun.ErrorCode,
				Message: *storedRun.ErrorMessage,
			}
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return TaskChat{}, fmt.Errorf("прочитать состояние чата: %w", err)
	}

	return result, nil
}

func (s *Service) SendMessage(
	ctx context.Context,
	employeeID string,
	taskID string,
	idempotencyKey string,
	input MessageInput,
) (Run, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)

	if taskID != "team-intro" && taskID != "working-accesses" {
		return Run{}, ErrInvalidTask
	}
	if strings.TrimSpace(input.Text) == "" ||
		utf8.RuneCountInString(input.Text) > 450 ||
		idempotencyKey == "" {
		return Run{}, ErrInvalidMessage
	}

	var result Run

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var employee models.Employee
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Take(&employee, "id = ?", employeeID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrEmployeeNotFound
		}
		if err != nil {
			return fmt.Errorf("найти стажёра для чата: %w", err)
		}
		if employee.Role != models.RoleIntern {
			return ErrForbidden
		}

		var profile models.Profile
		err = tx.Select("id").
			Take(&profile, "employee_id = ?", employee.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProfileRequired
		}
		if err != nil {
			return fmt.Errorf("проверить профиль стажёра: %w", err)
		}

		var existing models.ChatRun
		err = tx.Take(
			&existing,
			"employee_id = ? AND task_id = ? AND idempotency_key = ?",
			employee.ID,
			taskID,
			idempotencyKey,
		).Error

		if err == nil {
			var originalMessage models.ChatMessage
			err = tx.Take(&originalMessage, "id = ?", existing.UserMessageID).Error
			if err != nil {
				return fmt.Errorf("найти исходный вопрос: %w", err)
			}
			if originalMessage.Text != input.Text {
				return ErrIdempotencyKeyReused
			}

			result = runView(existing)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("проверить повтор вопроса: %w", err)
		}

		var active models.ChatRun
		err = tx.Take(
			&active,
			"employee_id = ? AND task_id = ? AND status IN ?",
			employee.ID,
			taskID,
			[]string{"queued", "running"},
		).Error
		if err == nil {
			return ErrChatBusy
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("проверить активный запуск чата: %w", err)
		}

		now := time.Now().UTC()
		userMessage := models.ChatMessage{
			ID:         rand.Text(),
			EmployeeID: employee.ID,
			TaskID:     taskID,
			Role:       "user",
			Text:       input.Text,
			CreatedAt:  now,
		}
		if err := tx.Create(&userMessage).Error; err != nil {
			return fmt.Errorf("сохранить вопрос: %w", err)
		}

		run := models.ChatRun{
			ID:             rand.Text(),
			EmployeeID:     employee.ID,
			TaskID:         taskID,
			UserMessageID:  userMessage.ID,
			IdempotencyKey: idempotencyKey,
			Status:         "queued",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&run).Error; err != nil {
			return fmt.Errorf("поставить вопрос в очередь: %w", err)
		}

		result = runView(run)
		return nil
	})

	if err != nil {
		return Run{}, err
	}

	return result, nil
}

func runView(run models.ChatRun) Run {
	view := Run{
		ID:                 run.ID,
		UserMessageID:      run.UserMessageID,
		Status:             run.Status,
		AssistantMessageID: run.AssistantMessageID,
	}
	if run.ErrorCode != nil && run.ErrorMessage != nil {
		view.Error = &APIError{
			Code:    *run.ErrorCode,
			Message: *run.ErrorMessage,
		}
	}
	return view
}
