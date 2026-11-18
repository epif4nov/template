package positions

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Интерфейс отправки запроса на получение координаты.
// Реализацию HTTP-гейтвея добавим следующим этапом.
type pushSender interface {
	RequestPosition(
		ctx context.Context,
		driverID uuid.UUID,
		tripID uuid.UUID,
	) error
}

type Pool struct {
	queue          *Queue
	sender         pushSender
	logger         *slog.Logger
	workers        int
	requestTimeout time.Duration

	startOnce sync.Once
	wg        sync.WaitGroup
}

func NewPool(
	queue *Queue,
	sender pushSender,
	logger *slog.Logger,
	workers int,
	requestTimeout time.Duration,
) (*Pool, error) {
	if queue == nil || sender == nil {
		return nil, fmt.Errorf("queue and sender are required")
	}

	if workers <= 0 {
		return nil, fmt.Errorf("workers must be positive")
	}

	if requestTimeout <= 0 {
		return nil, fmt.Errorf("request timeout must be positive")
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &Pool{
		queue:          queue,
		sender:         sender,
		logger:         logger,
		workers:        workers,
		requestTimeout: requestTimeout,
	}, nil
}

// Start запускает фиксированное количество воркеров.
func (p *Pool) Start(ctx context.Context) {
	p.startOnce.Do(func() {
		for i := 0; i < p.workers; i++ {
			p.wg.Add(1)

			go p.runWorker(ctx, i+1)
		}
	})
}

func (p *Pool) runWorker(
	ctx context.Context,
	workerID int,
) {
	defer p.wg.Done()

	p.logger.Info(
		"position worker started",
		"worker_id", workerID,
	)

	for task := range p.queue.tasks {
		p.process(ctx, workerID, task)
	}

	p.logger.Info(
		"position worker stopped",
		"worker_id", workerID,
	)
}

func (p *Pool) process(
	ctx context.Context,
	workerID int,
	task positionTask,
) {
	defer p.queue.Done(task)

	// Проверяем отмену сервиса и актуальность задачи.
	if ctx.Err() != nil || !p.queue.IsCurrent(task) {
		return
	}

	// Ограничиваем время одного обращения к Push Service.
	requestCtx, cancel := context.WithTimeout(
		ctx,
		p.requestTimeout,
	)
	defer cancel()

	err := p.sender.RequestPosition(
		requestCtx,
		task.Trip.DriverID,
		task.Trip.ID,
	)

	if err != nil {
		p.logger.Warn(
			"position request failed",
			"worker_id", workerID,
			"trip_id", task.Trip.ID,
			"error", err,
		)
		return
	}

	p.logger.Debug(
		"position request sent",
		"worker_id", workerID,
		"trip_id", task.Trip.ID,
	)
}

// Wait ожидает завершения всех запущенных воркеров.
// Перед вызовом нужно остановить сканер и закрыть очередь.
func (p *Pool) Wait() {
	p.wg.Wait()
}
