package positions

import (
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/domain/trip"
)

func TestQueueDuplicate(t *testing.T) {
	q, err := NewQueue(2)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()

	trip := trip.StaleTrip{ID: uuid.New()}

	if got := q.Enqueue(trip); got != Queued {
		t.Fatalf("first enqueue = %s", got)
	}

	if got := q.Enqueue(trip); got != Duplicate {
		t.Fatalf("second enqueue = %s", got)
	}
}

func TestQueueFull(t *testing.T) {
	q, err := NewQueue(1)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()

	first := trip.StaleTrip{ID: uuid.New()}
	second := trip.StaleTrip{ID: uuid.New()}

	if got := q.Enqueue(first); got != Queued {
		t.Fatalf("first enqueue = %s", got)
	}

	if got := q.Enqueue(second); got != Dropped {
		t.Fatalf("expected dropped, got %s", got)
	}
}

func TestQueueCancelAndRequeue(t *testing.T) {
	q, err := NewQueue(2)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()

	value := trip.StaleTrip{ID: uuid.New()}

	q.Enqueue(value)

	oldTask := <-q.tasks

	q.Cancel(value.ID)

	if q.IsCurrent(oldTask) {
		t.Fatal("cancelled task is still current")
	}

	if got := q.Enqueue(value); got != Queued {
		t.Fatalf("requeue = %s", got)
	}

	newTask := <-q.tasks

	q.Done(oldTask)

	if !q.IsCurrent(newTask) {
		t.Fatal("old task removed the new task")
	}

	q.Done(newTask)

	if q.IsCurrent(newTask) {
		t.Fatal("completed task is still pending")
	}
}

func TestQueueConcurrentEnqueue(t *testing.T) {
	q, err := NewQueue(1)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()

	value := trip.StaleTrip{ID: uuid.New()}

	const callers = 20

	results := make(chan EnqueueResult, callers)
	var wg sync.WaitGroup

	for i := 0; i < callers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			results <- q.Enqueue(value)
		}()
	}

	wg.Wait()
	close(results)

	queued := 0
	duplicates := 0

	for result := range results {
		switch result {
		case Queued:
			queued++
		case Duplicate:
			duplicates++
		default:
			t.Fatalf("unexpected result: %s", result)
		}
	}

	if queued != 1 || duplicates != callers-1 {
		t.Fatalf(
			"queued=%d duplicates=%d",
			queued, duplicates,
		)
	}
}
