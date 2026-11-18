package positions

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/epif4nov/template/internal/domain/trip"
	"github.com/epif4nov/template/internal/infra/telemetry"
)

// Репозиторий, который ищет устаревшие поездки.
type staleRepository interface {
	FindStaleTrips(
		context.Context,
		time.Duration,
	) ([]trip.StaleTrip, error)
}

// Возможные результаты постановки задачи в очередь.
type EnqueueResult string

const (
	Queued    EnqueueResult = "queued"
	Duplicate EnqueueResult = "duplicate"
	Dropped   EnqueueResult = "dropped"
)

// Очередь должна принимать задачи без блокировки.
type taskQueue interface {
	Enqueue(trip.StaleTrip) EnqueueResult
}

// Интерфейс тикера позволяет подменять время в тестах.
type ticker interface {
	Chan() <-chan time.Time
	Stop()
}

type realTicker struct {
	value *time.Ticker
}

func (t *realTicker) Chan() <-chan time.Time {
	return t.value.C
}

func (t *realTicker) Stop() {
	t.value.Stop()
}

type tickerFactory func(time.Duration) ticker

func newRealTicker(interval time.Duration) ticker {
	return &realTicker{
		value: time.NewTicker(interval),
	}
}

type Scanner struct {
	repo       staleRepository
	queue      taskQueue
	logger     *slog.Logger
	interval   time.Duration
	staleAfter time.Duration
	newTicker  tickerFactory
	signals    *telemetry.Positions
}

func NewScanner(
	repo staleRepository,
	queue taskQueue,
	logger *slog.Logger,
	interval time.Duration,
	staleAfter time.Duration,
) (*Scanner, error) {
	if interval <= 0 || staleAfter <= 0 {
		return nil, fmt.Errorf(
			"scanner intervals must be positive",
		)
	}

	return &Scanner{
		repo:       repo,
		queue:      queue,
		logger:     logger,
		interval:   interval,
		staleAfter: staleAfter,
		newTicker:  newRealTicker,
	}, nil
}

func (s *Scanner) SetSignals(signals *telemetry.Positions) { s.signals = signals }

// Run выполняется в одной фоновой горутине.
func (s *Scanner) Run(ctx context.Context) {
	s.logger.Info("position scanner started")
	ticker := s.newTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("position scanner stopped")
			return

		case <-ticker.Chan():
			if ctx.Err() != nil {
				return
			}
			s.scan(ctx)
		}
	}
}

func (s *Scanner) scan(ctx context.Context) {
	var link trace.SpanContext
	if s.signals != nil {
		var span trace.Span
		ctx, span = s.signals.StartScan(ctx)
		defer span.End()
		link = span.SpanContext()
	}
	trips, err := s.repo.FindStaleTrips(
		ctx,
		s.staleAfter,
	)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Error(
				"find stale trips failed",
				"error", err,
			)
		}
		return
	}

	for _, t := range trips {
		if ctx.Err() != nil {
			return
		}

		var result EnqueueResult
		if linked, ok := s.queue.(interface {
			EnqueueLinked(trip.StaleTrip, trace.SpanContext) EnqueueResult
		}); ok {
			result = linked.EnqueueLinked(t, link)
		} else {
			result = s.queue.Enqueue(t)
		}
		if s.signals != nil {
			s.signals.StaleCheck(ctx, string(result))
		}

		switch result {
		case Queued:
			s.logger.Debug(
				"position request queued",
				"trip_id", t.ID,
			)

		case Duplicate:
			s.logger.Debug(
				"position request already queued",
				"trip_id", t.ID,
			)

		case Dropped:
			if s.signals != nil {
				s.signals.RequestDropped(ctx)
			}
			s.logger.Warn(
				"position request queue full",
				"trip_id", t.ID,
			)
		}
	}
}
