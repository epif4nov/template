package positions

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/epif4nov/template/internal/infra/telemetry"
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
	done      chan struct{}
	signals   *telemetry.Positions
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
		done:           make(chan struct{}),
	}, nil
}

func (p *Pool) SetSignals(signals *telemetry.Positions) { p.signals = signals }

// Start запускает фиксированное количество воркеров.
func (p *Pool) Start(ctx context.Context) {
	p.startOnce.Do(func() {
		for i := 0; i < p.workers; i++ {
			p.wg.Add(1)

			go p.runWorker(ctx, i+1)
		}
		go func() { p.wg.Wait(); close(p.done) }()
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
		if p.signals != nil {
			p.signals.RequestDropped(ctx)
		}
		return
	}

	if p.signals != nil {
		var span trace.Span
		ctx, span = p.signals.StartTask(ctx, task.Link)
		defer span.End()
		p.signals.WorkerActive(ctx, 1)
		defer p.signals.WorkerActive(ctx, -1)
	}

	// Ограничиваем время одного обращения к Push Service.
	requestCtx, cancel := context.WithTimeout(
		ctx,
		p.requestTimeout,
	)
	defer cancel()
	if !p.queue.Register(task, cancel) {
		if p.signals != nil {
			p.signals.RequestDropped(ctx)
		}
		return
	}

	start := time.Now()
	var pushSpan trace.Span
	if p.signals != nil {
		requestCtx, pushSpan = p.signals.StartPush(requestCtx)
	}
	err := p.sender.RequestPosition(
		requestCtx,
		task.Trip.DriverID,
		task.Trip.ID,
	)

	if pushSpan != nil {
		pushSpan.End()
	}
	if p.signals != nil {
		p.signals.RequestResult(ctx, err, time.Since(start))
	}

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
	<-p.done
}

func (p *Pool) WaitContext(ctx context.Context) error {
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
