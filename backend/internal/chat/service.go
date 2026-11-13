package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
)

var (
	ErrEmployeeNotFound = errors.New("сотрудник не найден")
	ErrForbidden        = errors.New("чат доступен только стажёру")
	ErrProfileRequired  = errors.New("сначала заполни профиль")
	ErrInvalidTask      = errors.New("неизвестный этап чата")
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
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