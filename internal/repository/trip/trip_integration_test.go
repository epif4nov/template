//go:build integration

package trip_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/epif4nov/template/internal/infra/postgres/transaction"
	repository "github.com/epif4nov/template/internal/repository/trip"
)

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	return pool
}

func newTrip() repository.Trip {
	return repository.Trip{
		UserID:         uuid.New(),
		DriverID:       uuid.New(),
		StartLatitude:  59.9398,
		StartLongitude: 30.3146,
		EndLatitude:    59.929,
		EndLongitude:   30.3626,
		Price:          1450,
	}
}

func TestTripLifecycle(t *testing.T) {
	pool := openTestPool(t)
	repo := repository.New(pool)
	manager := transaction.NewManager(pool, 5*time.Second)
	svc := newApplication(repo, manager)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	created, err := svc.CreateTrip(ctx, newTrip())
	if err != nil {
		t.Fatalf("create trip: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()

		if _, err := pool.Exec(cleanupCtx,
			"DELETE FROM trips WHERE id = $1", created.ID,
		); err != nil {
			t.Errorf("cleanup trip: %v", err)
		}
	})

	fetched, err := repo.Fetch(ctx, created.ID)
	if err != nil {
		t.Fatalf("fetch created trip: %v", err)
	}
	if fetched.ID() != created.ID || fetched.UserID() != created.UserID || fetched.DriverID() != created.DriverID || fetched.Price() != created.Price || fetched.FinishedAt() != nil {
		t.Fatal("persisted trip fields changed")
	}
	_, err = repo.Fetch(ctx, uuid.New())
	if !errors.Is(err, repository.ErrTripNotFound) {
		t.Fatalf("expected missing trip, got %v", err)
	}
	if created.Status != activeStatus {
		t.Fatalf("expected active, got %s", created.Status)
	}

	completed, err := svc.FinishTrip(ctx, created.ID)
	if err != nil {
		t.Fatalf("finish trip: %v", err)
	}

	if completed.Status != "completed" || completed.FinishedAt == nil {
		t.Fatalf("invalid completed trip: %+v", completed)
	}

	var activeCount, completedCount int

	err = pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (
				WHERE from_status IS NULL AND to_status = 'active'
			),
			COUNT(*) FILTER (
				WHERE from_status = 'active' AND to_status = 'completed'
			)
		FROM trip_status_history
		WHERE trip_id = $1
	`, created.ID).Scan(&activeCount, &completedCount)
	if err != nil {
		t.Fatalf("query history: %v", err)
	}

	if activeCount != 1 || completedCount != 1 {
		t.Fatalf(
			"unexpected history: active=%d completed=%d",
			activeCount,
			completedCount,
		)
	}

	_, err = svc.FinishTrip(ctx, created.ID)
	if !errors.Is(err, repository.ErrTripCompleted) {
		t.Fatalf("expected ErrTripCompleted, got %v", err)
	}
}

func TestRollbackWhenHistoryInsertFails(t *testing.T) {
	pool := openTestPool(t)
	repo := repository.New(pool)
	manager := transaction.NewManager(pool, 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	trip := newTrip()
	trip.ID = uuid.New()
	trip.Status = activeStatus
	trip.StartedAt = time.Now().UTC()

	err := manager.Do(ctx, func(txCtx context.Context) error {
		if err := repo.CreateTrip(txCtx, trip); err != nil {
			return err
		}

		return repo.AddStatusHistory(txCtx, trip.ID, nil, "invalid", "test rollback")
	})

	if err == nil {
		t.Fatal("expected transaction error")
	}

	var count int

	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM trips WHERE id = $1",
		trip.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("check rollback: %v", err)
	}

	if count != 0 {
		t.Fatal("trip remained after transaction rollback")
	}
}
