package issuanceworker

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type processorFunc func(context.Context, accessrequest.AccessProvider) (bool, error)

func (f processorFunc) ProcessNextIssuance(ctx context.Context, provider accessrequest.AccessProvider) (bool, error) {
	return f(ctx, provider)
}

type unusedProvider struct{}

func (*unusedProvider) CheckAccess(context.Context, accessrequest.GrantTarget) (accessrequest.ProviderAccess, error) {
	panic("провайдер должен вызывать сервис заявок, а не фоновый цикл")
}

func (*unusedProvider) GrantAccess(context.Context, string, accessrequest.GrantTarget) error {
	panic("провайдер должен вызывать сервис заявок, а не фоновый цикл")
}

func TestNewWorkerValidation(t *testing.T) {
	processor := processorFunc(func(context.Context, accessrequest.AccessProvider) (bool, error) { return false, nil })
	provider := &unusedProvider{}
	for _, scenario := range []string{"no processor", "no provider", "zero interval", "negative interval"} {
		t.Run(scenario, func(t *testing.T) {
			var service Processor = processor
			var adapter accessrequest.AccessProvider = provider
			interval := time.Second
			switch scenario {
			case "no processor":
				service = nil
			case "no provider":
				adapter = nil
			case "zero interval":
				interval = 0
			case "negative interval":
				interval = -time.Second
			}
			worker, err := NewWorker(service, adapter, interval, nil)
			require.ErrorIs(t, err, ErrInvalidInput)
			assert.Nil(t, worker)
		})
	}
	worker, err := NewWorker(processor, provider, time.Second, nil)
	require.NoError(t, err)
	assert.Same(t, slog.Default(), worker.logger)
}

func TestRunDrainsQueueAndWaitsOnlyWhenEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output bytes.Buffer
	provider := &unusedProvider{}
	calls, waits := 0, 0
	results := []bool{true, true, false, true, false}
	worker, err := NewWorker(processorFunc(func(actualCtx context.Context, actualProvider accessrequest.AccessProvider) (bool, error) {
		require.NoError(t, actualCtx.Err())
		assert.Same(t, provider, actualProvider)
		require.Less(t, calls, len(results), "цикл должен остановиться при отмене")
		result := results[calls]
		calls++
		return result, nil
	}), provider, 100*time.Millisecond, slog.New(slog.NewTextHandler(&output, nil)))
	require.NoError(t, err)
	worker.wait = func(actualCtx context.Context, interval time.Duration) bool {
		assert.Same(t, ctx, actualCtx)
		assert.Equal(t, 100*time.Millisecond, interval)
		waits++
		if waits == 1 {
			assert.Equal(t, 3, calls, "первые два задания выполняются без паузы")
			return true
		}
		cancel()
		return false
	}
	worker.Run(ctx)
	assert.Equal(t, 5, calls)
	assert.Equal(t, 2, waits)
	assert.NotContains(t, output.String(), "level=ERROR")
}

func TestRunContinuesAfterJobAndStorageErrors(t *testing.T) {
	for _, processed := range []bool{false, true} {
		for _, interval := range []time.Duration{100 * time.Millisecond, 2 * time.Second} {
			t.Run(interval.String()+"/"+map[bool]string{false: "storage", true: "job"}[processed], func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var output bytes.Buffer
				calls, waits := 0, 0
				worker, err := NewWorker(processorFunc(func(context.Context, accessrequest.AccessProvider) (bool, error) {
					calls++
					if calls == 1 {
						return processed, errors.New("temporary failure")
					}
					return calls == 2, nil
				}), &unusedProvider{}, interval, slog.New(slog.NewTextHandler(&output, nil)))
				require.NoError(t, err)
				worker.wait = func(_ context.Context, delay time.Duration) bool {
					waits++
					if waits == 1 {
						assert.Equal(t, max(interval, time.Second), delay)
						return true
					}
					assert.Equal(t, interval, delay)
					cancel()
					return false
				}
				worker.Run(ctx)
				assert.Equal(t, 3, calls, "ошибка не должна останавливать обработку следующего задания")
				assert.Equal(t, 2, waits)
				assert.Contains(t, output.String(), "temporary failure")
			})
		}
	}
}

func TestRunCancellation(t *testing.T) {
	for _, scenario := range []string{"before start", "during processing", "during error backoff", "between iterations"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var output bytes.Buffer
			calls := 0
			worker, err := NewWorker(processorFunc(func(context.Context, accessrequest.AccessProvider) (bool, error) {
				calls++
				if scenario == "during processing" {
					cancel()
					return true, ctx.Err()
				}
				return false, errors.New("queue unavailable")
			}), &unusedProvider{}, time.Second, slog.New(slog.NewTextHandler(&output, nil)))
			require.NoError(t, err)
			worker.wait = func(context.Context, time.Duration) bool {
				cancel()
				return scenario == "between iterations"
			}
			if scenario == "before start" {
				cancel()
			}
			worker.Run(ctx)
			if scenario == "before start" {
				assert.Zero(t, calls)
			} else {
				assert.Equal(t, 1, calls)
			}
			if scenario == "during processing" {
				assert.NotContains(t, output.String(), "level=ERROR", "остановка приложения не является сбоем очереди")
			}
		})
	}
}

func TestWaitHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan bool, 1)
	go func() { finished <- wait(ctx, time.Hour) }()
	cancel()
	select {
	case result := <-finished:
		assert.False(t, result)
	case <-time.After(time.Second):
		t.Fatal("ожидание не завершилось после отмены")
	}
	assert.True(t, wait(context.Background(), 0))
}
