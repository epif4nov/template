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
	"github.com/epif4nov/template/internal/usecase/trip/positions/save"
)

func TestPositionStorageAndStaleSelection(t *testing.T) {
	pool := openTestPool(t)
	repo := repository.New(pool)
	manager := transaction.NewManager(pool, 5*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	old := newTrip()
	old.ID, old.Status, old.StartedAt = uuid.New(), "active", time.Now().UTC().Add(-2*time.Hour)
	fresh := newTrip()
	fresh.ID, fresh.Status, fresh.StartedAt = uuid.New(), "active", time.Now().UTC()
	completed := newTrip()
	completed.ID, completed.Status, completed.StartedAt = uuid.New(), "completed", time.Now().UTC().Add(-2*time.Hour)
	finishedAt := time.Now().UTC()
	completed.FinishedAt = &finishedAt
	for _, value := range []repository.Trip{old, fresh, completed} {
		if err := repo.CreateTrip(ctx, value); err != nil {
			t.Fatal(err)
		}
		id := value.ID
		t.Cleanup(func() {
			cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			if _, err := pool.Exec(cleanupCtx, "DELETE FROM trips WHERE id = $1", id); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		})
	}

	selected, err := repo.FindStaleTrips(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	found := map[uuid.UUID]bool{}
	for _, value := range selected {
		found[value.ID] = true
	}
	if !found[old.ID] || found[fresh.ID] || found[completed.ID] {
		t.Fatalf("incorrect stale selection: old=%v fresh=%v completed=%v", found[old.ID], found[fresh.ID], found[completed.ID])
	}

	uc := save.New(repo, manager, func() time.Time { return time.Now().UTC() })
	var callbacks int
	uc.SetOnSaved(func(context.Context, uuid.UUID) { callbacks++ })
	newer := time.Now().UTC().Add(-10 * time.Second).Truncate(time.Microsecond)
	if _, err := uc.Execute(ctx, save.Input{TripID: old.ID, Latitude: 59, Longitude: 30, RecordedAt: newer}); err != nil {
		t.Fatal(err)
	}
	if callbacks != 1 {
		t.Fatalf("successful save callbacks = %d", callbacks)
	}
	older := newer.Add(-time.Hour)
	if _, err := uc.Execute(ctx, save.Input{TripID: old.ID, Latitude: 60, Longitude: 31, RecordedAt: older}); err != nil {
		t.Fatal(err)
	}
	var last time.Time
	if err := pool.QueryRow(ctx, "SELECT last_position_at FROM trips WHERE id = $1", old.ID).Scan(&last); err != nil {
		t.Fatal(err)
	}
	if !last.Equal(newer) {
		t.Fatalf("last_position_at = %s, want %s", last, newer)
	}
	trip, err := repo.Fetch(ctx, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if trip.LastPositionAt() == nil || !trip.LastPositionAt().Equal(newer) {
		t.Fatal("GET model lost last_position_at")
	}

	rollback := errors.New("force rollback")
	err = manager.Do(ctx, func(txCtx context.Context) error {
		if _, err := repo.InsertPosition(txCtx, old.ID, 61, 32, newer); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback error = %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM trip_positions WHERE trip_id = $1", old.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("positions after rollback = %d", count)
	}
}
