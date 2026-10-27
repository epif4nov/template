//go:build integration

package trip_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/infra/postgres/transaction"
	repository "github.com/epif4nov/template/internal/repository/trip"
)

func TestNestedTransactionRollback(t *testing.T) {
	pool := openTestPool(t)
	repo := repository.New(pool)
	manager := transaction.NewManager(pool, 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	trip := newTrip()
	trip.ID = uuid.New()
	trip.Status = "active"
	trip.StartedAt = time.Now().UTC()

	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer stop()

		if _, err := pool.Exec(
			cleanupCtx,
			"DELETE FROM trips WHERE id = $1",
			trip.ID,
		); err != nil {
			t.Errorf("cleanup trip: %v", err)
		}
	})

	rollbackErr := errors.New("force rollback")

	err := manager.Do(ctx, func(txCtx context.Context) error {
		err := manager.Do(txCtx, func(nestedCtx context.Context) error {
			return repo.CreateTrip(nestedCtx, trip)
		})
		if err != nil {
			return err
		}

		return rollbackErr
	})

	if !errors.Is(err, rollbackErr) {
		t.Fatalf("expected rollback error, got %v", err)
	}

	var count int
	err = pool.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM trips WHERE id = $1",
		trip.ID,
	).Scan(&count)

	if err != nil {
		t.Fatalf("check rollback: %v", err)
	}

	if count != 0 {
		t.Fatal("nested transaction was committed separately")
	}
}

func TestTransactionRollbackOnPanic(t *testing.T) {
	pool := openTestPool(t)
	repo := repository.New(pool)
	manager := transaction.NewManager(pool, 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	trip := newTrip()
	trip.ID = uuid.New()
	trip.Status = "active"
	trip.StartedAt = time.Now().UTC()

	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer stop()

		if _, err := pool.Exec(
			cleanupCtx,
			"DELETE FROM trips WHERE id = $1",
			trip.ID,
		); err != nil {
			t.Errorf("cleanup trip: %v", err)
		}
	})

	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Error("expected panic")
			}
		}()

		_ = manager.Do(ctx, func(txCtx context.Context) error {
			if err := repo.CreateTrip(txCtx, trip); err != nil {
				return err
			}

			panic("test panic")
		})
	}()

	var count int
	err := pool.QueryRow(
		ctx,
		"SELECT COUNT(*) FROM trips WHERE id = $1",
		trip.ID,
	).Scan(&count)

	if err != nil {
		t.Fatalf("check rollback: %v", err)
	}

	if count != 0 {
		t.Fatal("trip remained after panic")
	}
}
