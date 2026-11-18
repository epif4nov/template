package positions

import (
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/domain/trip"
)

// Одна задача для будущего воркера.
type positionTask struct {
	Trip       trip.StaleTrip
	Generation uint64
}

type Queue struct {
	mu sync.Mutex

	tasks   chan positionTask
	pending map[uuid.UUID]uint64

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
	}, nil
}

// Enqueue ставит задачу без ожидания свободного места.
func (q *Queue) Enqueue(t trip.StaleTrip) EnqueueResult {
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
	defer q.mu.Unlock()

	delete(q.pending, tripID)
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
