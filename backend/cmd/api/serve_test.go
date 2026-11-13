package main

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type serverStub struct {
	listen   func() error
	shutdown func(context.Context) error
	close    func() error
}

func (s serverStub) ListenAndServe() error              { return s.listen() }
func (s serverStub) Shutdown(ctx context.Context) error { return s.shutdown(ctx) }
func (s serverStub) Close() error                       { return s.close() }

type workerFunc func(context.Context)

func (f workerFunc) Run(ctx context.Context) { f(ctx) }

func awaitSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("ожидаемое событие не наступило")
	}
}

func awaitServe(t *testing.T, results <-chan error) error {
	t.Helper()
	select {
	case err := <-results:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("сервер не завершил остановку")
		return nil
	}
}

func TestServeWaitsForWorkerBeforeReturning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	httpStarted, listenerClosed, shutdownCalled := make(chan struct{}), make(chan struct{}), make(chan struct{})
	workerStarted, workerStopping, releaseWorker := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var release sync.Once
	finishWorker := func() { release.Do(func() { close(releaseWorker) }) }
	defer finishWorker()
	server := serverStub{
		listen: func() error { close(httpStarted); <-listenerClosed; return http.ErrServerClosed },
		shutdown: func(shutdownCtx context.Context) error {
			assert.NoError(t, shutdownCtx.Err(), "завершение HTTP использует отдельный контекст")
			deadline, ok := shutdownCtx.Deadline()
			assert.True(t, ok)
			assert.WithinDuration(t, time.Now().Add(10*time.Second), deadline, time.Second)
			close(listenerClosed)
			close(shutdownCalled)
			return nil
		},
		close: func() error {
			t.Error("штатная остановка не требует принудительного Close")
			return nil
		},
	}
	worker := workerFunc(func(workerCtx context.Context) {
		close(workerStarted)
		<-workerCtx.Done()
		close(workerStopping)
		// Имитация сохранения результата после отмены сетевого вызова.
		<-releaseWorker
	})
	results := make(chan error, 1)
	go func() { results <- serve(ctx, server, worker) }()
	awaitSignal(t, httpStarted)
	awaitSignal(t, workerStarted)
	cancel()
	awaitSignal(t, shutdownCalled)
	awaitSignal(t, workerStopping)
	select {
	case err := <-results:
		t.Fatalf("нельзя возвращаться к закрытию БД, пока обработчик сохраняет результат: %v", err)
	default:
	}
	finishWorker()
	require.NoError(t, awaitServe(t, results))
}

func TestServeStopsWorkerWhenHTTPFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listenErr := errors.New("address already in use")
	workerStarted, workerStopping, releaseWorker := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var release sync.Once
	finishWorker := func() { release.Do(func() { close(releaseWorker) }) }
	defer finishWorker()
	server := serverStub{
		listen: func() error { <-workerStarted; return listenErr },
		shutdown: func(context.Context) error {
			t.Error("HTTP не запустился, Shutdown не требуется")
			return nil
		},
		close: func() error { t.Error("HTTP не запустился, Close не требуется"); return nil },
	}
	worker := workerFunc(func(workerCtx context.Context) {
		close(workerStarted)
		<-workerCtx.Done()
		close(workerStopping)
		<-releaseWorker
	})
	results := make(chan error, 1)
	go func() { results <- serve(ctx, server, worker) }()
	awaitSignal(t, workerStopping)
	assert.NoError(t, ctx.Err(), "ошибка HTTP должна сама остановить обработчик без сигнала завершения")
	select {
	case <-results:
		t.Fatal("после ошибки HTTP нужно дождаться остановки обработчика")
	default:
	}
	finishWorker()
	require.ErrorIs(t, awaitServe(t, results), listenErr)
}

func TestServeForcesCloseAfterShutdownFailure(t *testing.T) {
	for _, closeErr := range []error{nil, errors.New("close failed")} {
		name := "close succeeds"
		if closeErr != nil {
			name = "close fails"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started, listenerClosed, workerStopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
			shutdownErr := errors.New("HTTP shutdown deadline")
			closeCalls := 0
			server := serverStub{
				listen:   func() error { close(started); <-listenerClosed; return http.ErrServerClosed },
				shutdown: func(context.Context) error { return shutdownErr },
				close:    func() error { closeCalls++; close(listenerClosed); return closeErr },
			}
			worker := workerFunc(func(ctx context.Context) { <-ctx.Done(); close(workerStopped) })
			results := make(chan error, 1)
			go func() { results <- serve(ctx, server, worker) }()
			awaitSignal(t, started)
			cancel()
			err := awaitServe(t, results)
			require.ErrorIs(t, err, shutdownErr)
			if closeErr != nil {
				require.ErrorIs(t, err, closeErr)
			}
			assert.Equal(t, 1, closeCalls)
			awaitSignal(t, workerStopped)
		})
	}
}

func TestServeDoesNotStartAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, serve(ctx, nil, nil))
}
