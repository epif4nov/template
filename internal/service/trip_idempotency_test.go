package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/epif4nov/template/internal/repository"
	"github.com/epif4nov/template/internal/transaction"
)

func setupIdempotencyTest(t *testing.T) (
	*Service, *pgxpool.Pool, context.Context,
) {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 45*time.Second,
	)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	svc := New(
		repository.New(pool),
		transaction.NewManager(pool, 20*time.Second),
	)
	return svc, pool, ctx
}

func newIdempotencyTrip() repository.Trip {
	return repository.Trip{
		UserID:         uuid.New(),
		DriverID:       uuid.New(),
		StartLatitude:  59.9398,
		StartLongitude: 30.3146,
		EndLatitude:    59.9290,
		EndLongitude:   30.3626,
		Price:          1450,
	}
}

func hashTrip(t *testing.T, trip repository.Trip) [32]byte {
	t.Helper()

	data, err := json.Marshal(trip)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func cleanupIdempotencyTrip(
	t *testing.T,
	pool *pgxpool.Pool,
	key string,
	id uuid.UUID,
) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cancel()

		queries := []struct {
			sql string
			arg any
		}{
			{
				"DELETE FROM trip_idempotency_keys WHERE idempotency_key = $1",
				key,
			},
			{
				"DELETE FROM trip_status_history WHERE trip_id = $1",
				id,
			},
			{
				"DELETE FROM trips WHERE id = $1",
				id,
			},
		}

		for _, q := range queries {
			if _, err := pool.Exec(ctx, q.sql, q.arg); err != nil {
				t.Errorf("cleanup idempotency test: %v", err)
			}
		}
	})
}

func TestIdempotentTripReplay(t *testing.T) {
	svc, pool, ctx := setupIdempotencyTest(t)

	trip := newIdempotencyTrip()
	key := uuid.NewString()
	hash := hashTrip(t, trip)

	first, err := svc.CreateTripIdempotent(
		ctx, trip, key, hash,
	)
	if err != nil {
		t.Fatal(err)
	}
	cleanupIdempotencyTrip(t, pool, key, first.ID)

	second, err := svc.CreateTripIdempotent(
		ctx, trip, key, hash,
	)
	if err != nil {
		t.Fatal(err)
	}

	if first.ID != second.ID {
		t.Fatalf("different IDs: %s and %s", first.ID, second.ID)
	}

	changed := trip
	changed.Price = 1500

	_, err = svc.CreateTripIdempotent(
		ctx, changed, key, hashTrip(t, changed),
	)
	if !errors.Is(err, repository.ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	finished, err := svc.FinishTrip(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}

	if finished.Status != "completed" {
		t.Fatalf("unexpected status: %s", finished.Status)
	}

	replayed, err := svc.CreateTripIdempotent(
		ctx, trip, key, hash,
	)
	if err != nil {
		t.Fatal(err)
	}

	if replayed.ID != first.ID || replayed.Status != "active" {
		t.Fatalf("incorrect replay: %+v", replayed)
	}

	var status string
	err = pool.QueryRow(
		ctx,
		"SELECT status FROM trips WHERE id = $1",
		first.ID,
	).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}

	if status != "completed" {
		t.Fatalf("database status changed to %s", status)
	}
}

func TestConcurrentIdempotentCreate(t *testing.T) {
	svc, pool, ctx := setupIdempotencyTest(t)

	trip := newIdempotencyTrip()
	key := uuid.NewString()
	hash := hashTrip(t, trip)

	const workers = 10

	ids := make([]uuid.UUID, workers)
	errs := make([]error, workers)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()

			result, err := svc.CreateTripIdempotent(
				ctx, trip, key, hash,
			)

			errs[index] = err
			if err == nil {
				ids[index] = result.ID
			}
		}(i)
	}

	wg.Wait()

	for i := 0; i < workers; i++ {
		if errs[i] != nil {
			t.Fatalf("request %d: %v", i, errs[i])
		}
		if ids[i] != ids[0] {
			t.Fatalf("request %d returned a different trip", i)
		}
	}

	cleanupIdempotencyTrip(t, pool, key, ids[0])

	var count int

	err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM trips WHERE driver_id = $1",
		trip.DriverID,
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Fatalf("expected one trip, got %d", count)
	}

	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM trip_status_history WHERE trip_id = $1",
		ids[0],
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Fatalf("expected one history record, got %d", count)
	}
}
