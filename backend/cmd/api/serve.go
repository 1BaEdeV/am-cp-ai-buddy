package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type httpServer interface {
	ListenAndServe() error
	Shutdown(context.Context) error
	Close() error
}

type backgroundWorker interface {
	Run(context.Context)
}

// serve запускает HTTP и выдачу вместе. На любом пути выхода отменяет фоновые
// операции и ждёт сохранения результата, пока подключение к БД ещё открыто.
func serve(ctx context.Context, server httpServer, worker backgroundWorker) error {
	if ctx.Err() != nil {
		return nil
	}
	workerCtx, cancelWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		worker.Run(workerCtx)
	}()
	defer func() {
		cancelWorker()
		<-workerDone
	}()

	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	select {
	case err := <-serverErrors:
		return fmt.Errorf("запустить HTTP-сервер: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return errors.Join(fmt.Errorf("остановить HTTP-сервер: %w", err), server.Close())
		}
		return nil
	}
}
