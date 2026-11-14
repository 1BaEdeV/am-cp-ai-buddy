package chat

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RunWorker периодически проверяет очередь и обрабатывает вопросы.
// Он останавливается, когда отменяется ctx, например при Ctrl+C.
func (s *Service) RunWorker(ctx context.Context, pollInterval time.Duration) {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		processed, err := s.ProcessNextRun(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.ErrorContext(ctx, "обработать вопрос чата", "error", err)
		}

		// Если вопрос был найден, сразу проверяем очередь ещё раз.
		if processed {
			continue
		}

		// Если очередь пуста, ждём перед следующей проверкой.
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessNextRun обрабатывает один вопрос из очереди.
// Возвращает false, если очередь пуста.
func (s *Service) ProcessNextRun(ctx context.Context) (bool, error) {
	var run models.ChatRun
	found := false

	// Берём первую ожидающую заявку и помечаем её running
	// в одной транзакции.
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.Locking{
			Strength: "UPDATE",
			Options:  "SKIP LOCKED",
		}).
			Where("status = ?", "queued").
			Order("created_at ASC").
			Order("id ASC").
			Limit(1).
			Find(&run)

		if result.Error != nil {
			return fmt.Errorf("взять вопрос из очереди: %w", result.Error)
		}

		if result.RowsAffected == 0 {
			// Очередь пуста — это обычное состояние.
			return nil
		}

		now := time.Now().UTC()
		update := tx.Model(&models.ChatRun{}).
			Where("id = ? AND status = ?", run.ID, "queued").
			Updates(map[string]any{
				"status":     "running",
				"updated_at": now,
			})

		if update.Error != nil {
			return fmt.Errorf("пометить вопрос как выполняемый: %w", update.Error)
		}
		if update.RowsAffected != 1 {
			return fmt.Errorf("не удалось захватить вопрос %q из очереди", run.ID)
		}

		run.Status = "running"
		run.UpdatedAt = now
		found = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}

	// Загружаем текст вопроса, для которого нужно подготовить ответ.
	var userMessage models.ChatMessage
	err = s.db.WithContext(ctx).
		Take(
			&userMessage,
			"id = ? AND employee_id = ? AND task_id = ?",
			run.UserMessageID,
			run.EmployeeID,
			run.TaskID,
		).Error
	if err != nil {
		failErr := s.failRun(ctx, run)
		return true, errors.Join(
			fmt.Errorf("загрузить вопрос из очереди: %w", err),
			failErr,
		)
	}

	// Временный ответ. Позже вместо него подключим ML-агента.
	replyTo := userMessage.ID
	sourceLabel := "Тестовый ответ"
	now := time.Now().UTC()

	assistantMessage := models.ChatMessage{
		ID:               newID(),
		EmployeeID:       run.EmployeeID,
		TaskID:           run.TaskID,
		Role:             "assistant",
		Text:             "Это тестовый ответ AI-buddy. Позже здесь будет ответ ML-агента.",
		CreatedAt:        now,
		ReplyToMessageID: &replyTo,
		SourceLabel:      &sourceLabel,
	}

	// Сохраняем ответ и завершаем обработку одной транзакцией.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&assistantMessage).Error; err != nil {
			return fmt.Errorf("сохранить ответ AI-buddy: %w", err)
		}

		update := tx.Model(&models.ChatRun{}).
			Where("id = ? AND status = ?", run.ID, "running").
			Updates(map[string]any{
				"status":               "completed",
				"assistant_message_id": assistantMessage.ID,
				"error_code":           nil,
				"error_message":        nil,
				"updated_at":           now,
			})

		if update.Error != nil {
			return fmt.Errorf("завершить запуск чата: %w", update.Error)
		}
		if update.RowsAffected != 1 {
			return fmt.Errorf("не удалось завершить запуск чата %q", run.ID)
		}

		return nil
	})
	if err != nil {
		failErr := s.failRun(ctx, run)
		return true, errors.Join(err, failErr)
	}

	return true, nil
}

// failRun сохраняет понятную ошибку, если worker не смог обработать вопрос.
func (s *Service) failRun(ctx context.Context, run models.ChatRun) error {
	update := s.db.WithContext(ctx).
		Model(&models.ChatRun{}).
		Where("id = ? AND status = ?", run.ID, "running").
		Updates(map[string]any{
			"status":        "failed",
			"error_code":    "processing_failed",
			"error_message": "Не удалось обработать вопрос. Попробуй отправить его ещё раз.",
			"updated_at":    time.Now().UTC(),
		})

	if update.Error != nil {
		return fmt.Errorf("сохранить ошибку запуска чата: %w", update.Error)
	}
	if update.RowsAffected != 1 {
		return fmt.Errorf("не удалось отметить запуск чата %q как failed", run.ID)
	}

	return nil
}

// newID создаёт случайный идентификатор сообщения.
func newID() string {
	return rand.Text()
}