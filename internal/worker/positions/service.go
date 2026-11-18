package positions

import (
	"context"
	"fmt"
)

type Service struct {
	scanner *Scanner
	queue   *Queue
	pool    *Pool

	stopScanner context.CancelFunc
	stopWorkers context.CancelFunc
	scannerDone chan struct{}
}

func NewService(
	scanner *Scanner,
	queue *Queue,
	pool *Pool,
) *Service {
	return &Service{
		scanner: scanner,
		queue:   queue,
		pool:    pool,
	}
}

// Start запускается один раз при старте приложения.
func (s *Service) Start(ctx context.Context) {
	scanCtx, stopScanner := context.WithCancel(ctx)
	workCtx, stopWorkers := context.WithCancel(ctx)

	s.stopScanner = stopScanner
	s.stopWorkers = stopWorkers
	s.scannerDone = make(chan struct{})

	// Сначала запускаем воркеров.
	s.pool.Start(workCtx)

	// Затем запускаем один сканер.
	go func() {
		defer close(s.scannerDone)
		s.scanner.Run(scanCtx)
	}()
}

// Shutdown останавливает сканер, затем даёт
// воркерам доработать очередь.
func (s *Service) Shutdown(ctx context.Context) error {
	if s.stopScanner == nil {
		return nil
	}

	// Новые задачи больше не должны поступать.
	s.stopScanner()

	select {
	case <-s.scannerDone:
		// Сканер завершился.
	case <-ctx.Done():
		// The repository uses this canceled context. Wait for it to release
		// its query before closing the database connection pool.
		<-s.scannerDone
		s.queue.Close()
		s.stopWorkers()
		s.pool.Wait()
		return fmt.Errorf("stop scanner: %w", ctx.Err())
	}

	// Закрываем канал после остановки сканера.
	s.queue.Close()

	if err := s.pool.WaitContext(ctx); err != nil {
		// Cancel in-flight HTTP requests, then join every worker before
		// the caller closes the HTTP transport and PostgreSQL pool.
		s.stopWorkers()
		s.pool.Wait()
		return fmt.Errorf("stop workers: %w", err)
	}
	s.stopWorkers()
	return nil
}
