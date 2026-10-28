//go:build integration

package trip_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/infra/postgres/transaction"
	repository "github.com/epif4nov/template/internal/repository/trip"
)

func TestConcurrentCreateTrip(t *testing.T) {
	pool := openTestPool(t)
	repo := repository.New(pool)
	manager := transaction.NewManager(pool, 10*time.Second)
	svc := newApplication(repo, manager)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const requests = 20

	driverID := uuid.New()

	type result struct {
		trip repository.Trip
		err  error
	}

	results := make(chan result, requests)
	start := make(chan struct{})

	var wg sync.WaitGroup

	for i := 0; i < requests; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			trip := newTrip()
			trip.DriverID = driverID

			<-start

			created, err := svc.CreateTrip(ctx, trip)
			results <- result{trip: created, err: err}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	successes := 0
	conflicts := 0

	var createdIDs []uuid.UUID

	for res := range results {
		switch {
		case res.err == nil:
			successes++
			createdIDs = append(createdIDs, res.trip.ID)

		case errors.Is(res.err, repository.ErrDriverBusy):
			conflicts++

		default:
			t.Errorf("unexpected create error: %v", res.err)
		}
	}

	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer stop()

		for _, id := range createdIDs {
			if _, err := pool.Exec(
				cleanupCtx,
				"DELETE FROM trips WHERE id = $1",
				id,
			); err != nil {
				t.Errorf("cleanup trip: %v", err)
			}
		}
	})

	if successes != 1 || conflicts != requests-1 {
		t.Fatalf(
			"expected 1 success and 19 conflicts, got %d and %d",
			successes,
			conflicts,
		)
	}

	var activeCount int

	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM trips
		WHERE driver_id = $1 AND status = 'active'
	`, driverID).Scan(&activeCount)
	if err != nil {
		t.Fatalf("count active trips: %v", err)
	}

	if activeCount != 1 {
		t.Fatalf("expected 1 active trip, got %d", activeCount)
	}
}

func TestConcurrentFinishTrip(t *testing.T) {
	pool := openTestPool(t)
	repo := repository.New(pool)
	manager := transaction.NewManager(pool, 10*time.Second)
	svc := newApplication(repo, manager)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	created, err := svc.CreateTrip(ctx, newTrip())
	if err != nil {
		t.Fatalf("create trip: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer stop()

		if _, err := pool.Exec(
			cleanupCtx,
			"DELETE FROM trips WHERE id = $1",
			created.ID,
		); err != nil {
			t.Errorf("cleanup trip: %v", err)
		}
	})

	const requests = 20

	results := make(chan error, requests)
	start := make(chan struct{})

	var wg sync.WaitGroup

	for i := 0; i < requests; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			_, err := svc.FinishTrip(ctx, created.ID)
			results <- err
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	successes := 0
	conflicts := 0

	for err := range results {
		switch {
		case err == nil:
			successes++

		case errors.Is(err, repository.ErrTripCompleted):
			conflicts++

		default:
			t.Errorf("unexpected finish error: %v", err)
		}
	}

	if successes != 1 || conflicts != requests-1 {
		t.Fatalf(
			"expected 1 success and 19 conflicts, got %d and %d",
			successes,
			conflicts,
		)
	}

	var historyCount int

	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM trip_status_history
		WHERE trip_id = $1
		  AND from_status = 'active'
		  AND to_status = 'completed'
	`, created.ID).Scan(&historyCount)
	if err != nil {
		t.Fatalf("count completion history: %v", err)
	}

	if historyCount != 1 {
		t.Fatalf(
			"expected 1 completion history record, got %d",
			historyCount,
		)
	}
}
