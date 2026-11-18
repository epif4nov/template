package positions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/epif4nov/template/internal/domain/trip"
)

func TestScannerLimitsDatabaseQueryTime(t *testing.T) {
	queue, err := NewQueue(1)
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()

	parent, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	queryTimedOut := false
	repo := staleRepoFunc(func(ctx context.Context, _ time.Duration) ([]trip.StaleTrip, error) {
		deadline, ok := ctx.Deadline()
		if !ok || deadline.After(time.Now().Add(500*time.Millisecond)) {
			t.Fatal("database query has no separate short deadline")
		}
		<-ctx.Done()
		queryTimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		return nil, ctx.Err()
	})

	scanner, err := NewScanner(repo, queue, testLogger(), time.Second, time.Minute, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	scanner.scan(parent)
	if !queryTimedOut {
		t.Fatal("database query did not stop at its deadline")
	}
	if err := parent.Err(); err != nil {
		t.Fatalf("scanner context was canceled: %v", err)
	}
}
