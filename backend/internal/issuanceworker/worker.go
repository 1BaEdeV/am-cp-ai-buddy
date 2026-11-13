// Package issuanceworker запускает постоянную обработку очереди выдачи доступов.
package issuanceworker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
)

var ErrInvalidInput = errors.New("для обработчика выдачи нужны сервис, провайдер и положительный интервал опроса")

// Processor обрабатывает одно задание. Правила выдачи и сохранение результата
// остаются в сервисе заявок; цикл управляет только запуском и ожиданием.
type Processor interface {
	ProcessNextIssuance(context.Context, accessrequest.AccessProvider) (bool, error)
}

type Worker struct {
	processor    Processor
	provider     accessrequest.AccessProvider
	pollInterval time.Duration
	logger       *slog.Logger
	wait         func(context.Context, time.Duration) bool
}

func NewWorker(processor Processor, provider accessrequest.AccessProvider, pollInterval time.Duration, logger *slog.Logger) (*Worker, error) {
	if processor == nil || provider == nil || pollInterval <= 0 {
		return nil, ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{processor: processor, provider: provider, pollInterval: pollInterval, logger: logger, wait: wait}, nil
}

// Run работает до отмены ctx. Готовые задания выполняются последовательно без
// пауз, пустая очередь опрашивается с интервалом. Ошибка одного задания не
// останавливает цикл. Повтор failed-заявки по-прежнему запускает руководитель.
// Вызывающая сторона должна дождаться возврата Run перед закрытием БД.
func (w *Worker) Run(ctx context.Context) {
	w.logger.InfoContext(ctx, "обработчик выдачи доступов запущен")
	defer w.logger.InfoContext(ctx, "обработчик выдачи доступов остановлен")
	for ctx.Err() == nil {
		processed, err := w.processor.ProcessNextIssuance(ctx, w.provider)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			w.logger.ErrorContext(ctx, "обработать очередь выдачи доступов", "error", err)
			// Ограничиваем частоту обращений и логов при недоступной БД.
			if !w.wait(ctx, max(w.pollInterval, time.Second)) {
				return
			}
			continue
		}
		if !processed && !w.wait(ctx, w.pollInterval) {
			return
		}
	}
}

func wait(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
