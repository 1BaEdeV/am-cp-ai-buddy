package chat

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	runProcessingTimeout = time.Minute
	staleRunTimeout      = 2 * time.Minute
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
	ctx, cancelRun := context.WithTimeout(ctx, runProcessingTimeout)
	defer cancelRun()
	// После аварии процесса running не должен навсегда блокировать чат.
	// Старый вопрос завершаем ошибкой, не повторяя возможные действия агента.
	recoveryNow := time.Now().UTC()
	if err := s.db.WithContext(ctx).Model(&models.ChatRun{}).
		Where("status = ? AND updated_at <= ?", "running", recoveryNow.Add(-staleRunTimeout)).
		Updates(map[string]any{
			"status": "failed", "error_code": "processing_interrupted",
			"error_message": "Обработка вопроса прервалась. Отправь его ещё раз.", "updated_at": recoveryNow,
		}).Error; err != nil {
		return false, fmt.Errorf("завершить прерванные вопросы чата: %w", err)
	}
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

	if s.agent == nil || s.context == nil {
		err := errors.New("ML assistant or task context is not configured")
		return true, errors.Join(err, s.failRun(ctx, run))
	}
	appContext, err := s.context.Build(ctx, run.EmployeeID, run.TaskID)
	if err != nil {
		return true, errors.Join(fmt.Errorf("подготовить контекст AI-buddy: %w", err), s.failRun(ctx, run))
	}
	var historyRows []models.ChatMessage
	err = s.db.WithContext(ctx).
		Where("employee_id = ? AND task_id = ? AND created_at <= ?", run.EmployeeID, run.TaskID, userMessage.CreatedAt).
		Order("created_at ASC").
		Order("id ASC").
		Find(&historyRows).Error
	if err != nil {
		return true, errors.Join(fmt.Errorf("загрузить историю чата: %w", err), s.failRun(ctx, run))
	}
	if len(historyRows) > 20 {
		historyRows = historyRows[len(historyRows)-20:]
	}
	history := make([]Message, 0, len(historyRows))
	for _, row := range historyRows {
		history = append(history, Message{ID: row.ID, Role: row.Role, Text: row.Text, CreatedAt: row.CreatedAt})
	}
	agentCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	answer, err := s.agent.Reply(agentCtx, AgentRequest{
		TaskID: run.TaskID, Question: userMessage.Text, History: history, Context: appContext,
	})
	cancel()
	if err != nil {
		return true, errors.Join(fmt.Errorf("получить ответ AI-buddy: %w", err), s.failRun(ctx, run))
	}
	if strings.TrimSpace(answer.Text) == "" {
		return true, errors.Join(errors.New("AI-buddy вернул пустой ответ"), s.failRun(ctx, run))
	}
	var accessDraft *accessrequest.Draft
	var accessDraftJSON *string
	if resourceID := strings.TrimSpace(answer.PrepareAccessRequestResourceID); resourceID != "" {
		if run.TaskID != "working-accesses" || s.requests == nil {
			return true, errors.Join(errors.New("не настроена подготовка черновика доступа"), s.failRun(ctx, run))
		}
		preparation, err := s.requests.PrepareAccessRequest(ctx, run.EmployeeID, resourceID)
		if err != nil {
			if errors.Is(err, accessrequest.ErrInvalidInput) || errors.Is(err, accessrequest.ErrResourceNotRequired) {
				answer.Text = "Не получилось определить этот обязательный ресурс. Выбери его в карточке рабочих доступов и попроси подготовить черновик ещё раз."
				resourceID = ""
			} else {
				return true, errors.Join(fmt.Errorf("подготовить черновик доступа: %w", err), s.failRun(ctx, run))
			}
		}
		if resourceID != "" {
			switch preparation.Status {
			case accessrequest.PreparationReady:
				if preparation.Draft == nil {
					return true, errors.Join(errors.New("сервис не вернул подготовленный черновик"), s.failRun(ctx, run))
				}
				accessDraft = preparation.Draft
				encoded, err := json.Marshal(accessDraft)
				if err != nil {
					return true, errors.Join(fmt.Errorf("сериализовать черновик доступа: %w", err), s.failRun(ctx, run))
				}
				value := string(encoded)
				accessDraftJSON = &value
				answer.Text = strings.TrimSpace(answer.Text) + "\n\nПодготовил черновик доступа. Проверь его и подтверди отправку."
			case accessrequest.PreparationAlreadyGranted:
				answer.Text = "Этот обязательный доступ уже выдан. Проверь его статус в карточке рабочих доступов."
			case accessrequest.PreparationRequestExists:
				answer.Text = "По этому ресурсу уже есть активная заявка. Посмотри её статус в разделе «Мои заявки»."
			default:
				return true, errors.Join(fmt.Errorf("неизвестный результат подготовки черновика: %q", preparation.Status), s.failRun(ctx, run))
			}
		}
	}

	replyTo := userMessage.ID
	sourceLabel := "Карточка команды"
	if run.TaskID == "working-accesses" {
		sourceLabel = "Карточка рабочих доступов"
	}
	now := time.Now().UTC()

	assistantMessage := models.ChatMessage{
		ID:               newID(),
		EmployeeID:       run.EmployeeID,
		TaskID:           run.TaskID,
		Role:             "assistant",
		Text:             strings.TrimSpace(answer.Text),
		CreatedAt:        now,
		ReplyToMessageID: &replyTo,
		SourceLabel:      &sourceLabel,
		AccessDraftJSON:  accessDraftJSON,
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
	// Отмена запроса или остановка worker не должны мешать записи ошибки.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
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
