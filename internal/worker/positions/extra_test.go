package positions

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/goleak"

	"github.com/epif4nov/template/internal/domain/trip"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type staleRepoFunc func(context.Context, time.Duration) ([]trip.StaleTrip, error)

func (f staleRepoFunc) FindStaleTrips(ctx context.Context, d time.Duration) ([]trip.StaleTrip, error) {
	return f(ctx, d)
}

type manualTicker struct {
	ticks   chan time.Time
	stopped chan struct{}
	once    sync.Once
}

func (t *manualTicker) Chan() <-chan time.Time { return t.ticks }
func (t *manualTicker) Stop()                  { t.once.Do(func() { close(t.stopped) }) }

func TestScannerTickDeduplicatesAndDrops(t *testing.T) {
	first := trip.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}
	second := trip.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}
	q, err := NewQueue(1)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()

	repo := staleRepoFunc(func(_ context.Context, _ time.Duration) ([]trip.StaleTrip, error) {
		return []trip.StaleTrip{first, second}, nil
	})
	s, err := NewScanner(repo, q, testLogger(), time.Second, time.Minute, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	s.scan(context.Background())
	s.scan(context.Background())
	if got := q.Len(); got != 1 {
		t.Fatalf("queue length = %d", got)
	}
	if !q.IsCurrent(<-q.tasks) {
		t.Fatal("first trip not queued")
	}
	if _, exists := q.pending[second.ID]; exists {
		t.Fatal("dropped trip marked pending")
	}
	if _, exists := q.pending[first.ID]; !exists {
		t.Fatal("first trip not pending")
	}
}

func TestScannerRunStopsTicker(t *testing.T) {
	q, err := NewQueue(1)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	s, err := NewScanner(staleRepoFunc(func(context.Context, time.Duration) ([]trip.StaleTrip, error) { return nil, nil }), q, testLogger(), time.Second, time.Minute, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	manual := &manualTicker{ticks: make(chan time.Time), stopped: make(chan struct{})}
	ready := make(chan struct{})
	s.newTicker = func(time.Duration) ticker { close(ready); return manual }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	<-ready
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scanner did not stop")
	}
	select {
	case <-manual.stopped:
	default:
		t.Fatal("ticker not stopped")
	}
}

func TestQueueOldDonePreservesReplacement(t *testing.T) {
	q, err := NewQueue(2)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	value := trip.StaleTrip{ID: uuid.New()}
	q.Enqueue(value)
	old := <-q.tasks
	q.Cancel(value.ID)
	q.Enqueue(value)
	newTask := <-q.tasks
	q.Done(old)
	if !q.IsCurrent(newTask) {
		t.Fatal("old Done removed replacement")
	}
}

func TestQueueCancelRunningTask(t *testing.T) {
	q, err := NewQueue(2)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	value := trip.StaleTrip{ID: uuid.New()}
	q.Enqueue(value)
	old := <-q.tasks
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if !q.Register(old, cancel) {
		t.Fatal("could not register running task")
	}
	q.Cancel(value.ID)
	select {
	case <-ctx.Done():
	default:
		t.Fatal("running task was not cancelled")
	}
	if got := q.Enqueue(value); got != Queued {
		t.Fatalf("replacement = %s", got)
	}
	replacement := <-q.tasks
	q.Done(old)
	if !q.IsCurrent(replacement) {
		t.Fatal("old task removed replacement")
	}
}

func TestServiceShutdownDrainsQueue(t *testing.T) {
	q, err := NewQueue(2)
	if err != nil {
		t.Fatal(err)
	}
	requests := make(chan uuid.UUID, 2)
	sender := fakePush(func(_ context.Context, _, id uuid.UUID) error { requests <- id; return nil })
	p, err := NewPool(q, sender, testLogger(), 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewScanner(staleRepoFunc(func(context.Context, time.Duration) ([]trip.StaleTrip, error) {
		return nil, errors.New("unexpected scan")
	}), q, testLogger(), time.Hour, time.Minute, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(s, q, p)
	service.Start(context.Background())
	for i := 0; i < 2; i++ {
		if got := q.Enqueue(trip.StaleTrip{ID: uuid.New()}); got != Queued {
			t.Fatalf("enqueue = %s", got)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := service.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 {
		t.Fatalf("processed %d tasks", len(requests))
	}
}

func TestServiceShutdownCancelsInFlight(t *testing.T) {
	q, err := NewQueue(1)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	stopped := make(chan struct{})
	sender := fakePush(func(ctx context.Context, _, _ uuid.UUID) error {
		close(started)
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	})
	p, err := NewPool(q, sender, testLogger(), 1, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewScanner(staleRepoFunc(func(context.Context, time.Duration) ([]trip.StaleTrip, error) { return nil, nil }), q, testLogger(), time.Hour, time.Minute, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(s, q, p)
	q.Enqueue(trip.StaleTrip{ID: uuid.New()})
	service.Start(context.Background())
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.Shutdown(ctx); err == nil {
		t.Fatal("expected deadline error")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("worker still running after shutdown")
	}
}
