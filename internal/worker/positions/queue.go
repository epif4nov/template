package positions

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/epif4nov/template/internal/domain/trip"
)

// positionTask передаёт воркеру поездку, поколение и контекст трассировки.
type positionTask struct {
	Trip       trip.StaleTrip
	Generation uint64
	Link       trace.SpanContext
}

type runningTask struct {
	generation uint64
	cancel     context.CancelFunc
}

type Queue struct {
	mu sync.Mutex

	tasks   chan positionTask
	pending map[uuid.UUID]uint64
	running map[uuid.UUID]runningTask

	nextGeneration uint64
	closed         bool
}

func NewQueue(size int) (*Queue, error) {
	if size <= 0 {
		return nil, fmt.Errorf("queue size must be positive")
	}

	return &Queue{
		tasks:   make(chan positionTask, size),
		pending: make(map[uuid.UUID]uint64),
		running: make(map[uuid.UUID]runningTask),
	}, nil
}

// Enqueue ставит задачу без ожидания свободного места.
func (q *Queue) Enqueue(t trip.StaleTrip) EnqueueResult {
	return q.EnqueueLinked(t, trace.SpanContext{})
}

func (q *Queue) EnqueueLinked(t trip.StaleTrip, link trace.SpanContext) EnqueueResult {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return Dropped
	}

	if _, exists := q.pending[t.ID]; exists {
		return Duplicate
	}

	q.nextGeneration++

	task := positionTask{
		Trip:       t,
		Generation: q.nextGeneration,
		Link:       link,
	}

	select {
	case q.tasks <- task:
		q.pending[t.ID] = task.Generation
		return Queued

	default:
		return Dropped
	}
}

// Cancel отменяет ожидающую задачу поездки.
func (q *Queue) Cancel(tripID uuid.UUID) {
	q.mu.Lock()
	delete(q.pending, tripID)
	running, exists := q.running[tripID]
	if exists {
		delete(q.running, tripID)
	}
	q.mu.Unlock()
	if exists {
		running.cancel()
	}
}

// Register marks a dequeued task as running and lets Cancel stop its request.
// The generation check and registration are one mutex-protected operation.
func (q *Queue) Register(task positionTask, cancel context.CancelFunc) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	generation, exists := q.pending[task.Trip.ID]
	if !exists || generation != task.Generation {
		return false
	}
	q.running[task.Trip.ID] = runningTask{generation: task.Generation, cancel: cancel}
	return true
}

// IsCurrent проверяет, не отменена ли задача.
func (q *Queue) IsCurrent(task positionTask) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	generation, exists := q.pending[task.Trip.ID]

	return exists && generation == task.Generation
}

// Done вызывается после обработки задачи воркером.
func (q *Queue) Done(task positionTask) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.pending[task.Trip.ID] == task.Generation {
		delete(q.pending, task.Trip.ID)
	}
	if running, exists := q.running[task.Trip.ID]; exists && running.generation == task.Generation {
		delete(q.running, task.Trip.ID)
	}
}

// Len возвращает число задач, ожидающих в канале.
func (q *Queue) Len() int {
	return len(q.tasks)
}

// Close прекращает приём задач.
// Уже поставленные задачи остаются доступными воркерам.
func (q *Queue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return
	}

	q.closed = true
	close(q.tasks)
}
