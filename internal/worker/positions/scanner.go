package positions

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/epif4nov/template/internal/domain/trip"
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

// Run выполняется в одной фоновой горутине.
func (s *Scanner) Run(ctx context.Context) {
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

		result := s.queue.Enqueue(t)

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
			s.logger.Warn(
				"position request queue full",
				"trip_id", t.ID,
			)
		}
	}
}
