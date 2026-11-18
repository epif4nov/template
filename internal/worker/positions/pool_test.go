package positions

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/domain/trip"
)

type fakePush func(context.Context, uuid.UUID, uuid.UUID) error

func (f fakePush) RequestPosition(
	ctx context.Context,
	driverID, tripID uuid.UUID,
) error {
	return f(ctx, driverID, tripID)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPoolWorkerLimit(t *testing.T) {
	q, err := NewQueue(8)
	if err != nil {
		t.Fatal(err)
	}

	var active atomic.Int32
	var maximum atomic.Int32
	var calls atomic.Int32

	started := make(chan struct{}, 8)
	release := make(chan struct{})

	sender := fakePush(func(
		ctx context.Context,
		_, _ uuid.UUID,
	) error {
		current := active.Add(1)
		defer active.Add(-1)

		calls.Add(1)

		for {
			old := maximum.Load()
			if current <= old ||
				maximum.CompareAndSwap(old, current) {
				break
			}
		}

		started <- struct{}{}

		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	pool, err := NewPool(q, sender, testLogger(), 3, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var once sync.Once
	finish := func() {
		once.Do(func() { close(release) })
		q.Close()
		pool.Wait()
	}
	t.Cleanup(finish)

	for i := 0; i < 8; i++ {
		value := trip.StaleTrip{
			ID:       uuid.New(),
			DriverID: uuid.New(),
		}
		if result := q.Enqueue(value); result != Queued {
			t.Fatalf("enqueue: %s", result)
		}
	}

	pool.Start(context.Background())

	// Убеждаемся, что три воркера начали обработку.
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("workers did not start")
		}
	}

	finish()

	if got := maximum.Load(); got != 3 {
		t.Fatalf("maximum active workers = %d, want 3", got)
	}

	if got := calls.Load(); got != 8 {
		t.Fatalf("processed tasks = %d, want 8", got)
	}
}

func TestPoolSkipsCancelledTask(t *testing.T) {
	q, err := NewQueue(2)
	if err != nil {
		t.Fatal(err)
	}

	first := trip.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}
	second := trip.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}

	requests := make(chan uuid.UUID, 2)
	release := make(chan struct{})
	var once sync.Once

	sender := fakePush(func(
		ctx context.Context,
		_, tripID uuid.UUID,
	) error {
		requests <- tripID

		if tripID == first.ID {
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		return nil
	})

	pool, err := NewPool(q, sender, testLogger(), 1, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	finish := func() {
		once.Do(func() { close(release) })
		q.Close()
		pool.Wait()
	}
	t.Cleanup(finish)

	q.Enqueue(first)
	q.Enqueue(second)

	pool.Start(context.Background())

	// Ждём начала первой задачи.
	select {
	case <-requests:
	case <-time.After(3 * time.Second):
		t.Fatal("first request did not start")
	}

	// Вторая задача ещё в очереди.
	q.Cancel(second.ID)

	finish()

	if len(requests) != 0 {
		t.Fatal("cancelled task reached Push Service")
	}
}

func TestPoolContinuesAfterPushError(t *testing.T) {
	q, err := NewQueue(2)
	if err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32

	sender := fakePush(func(
		_ context.Context,
		_, _ uuid.UUID,
	) error {
		if calls.Add(1) == 1 {
			return errors.New("push unavailable")
		}
		return nil
	})

	pool, err := NewPool(q, sender, testLogger(), 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	q.Enqueue(trip.StaleTrip{ID: uuid.New()})
	q.Enqueue(trip.StaleTrip{ID: uuid.New()})
	q.Close()

	pool.Start(context.Background())
	pool.Wait()

	if got := calls.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}
