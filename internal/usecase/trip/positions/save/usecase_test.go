package save

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/domain/trip"
)

type fakePositionRepo struct {
	lockErr, insertErr, updateErr error
	inserts                       int
}

func (r *fakePositionRepo) LockActiveTrip(context.Context, uuid.UUID) error { return r.lockErr }
func (r *fakePositionRepo) InsertPosition(context.Context, uuid.UUID, float64, float64, time.Time) (int64, error) {
	r.inserts++
	return 42, r.insertErr
}

func (r *fakePositionRepo) UpdateLastPositionAt(context.Context, uuid.UUID, time.Time) error {
	return r.updateErr
}

type fakeTx struct{ commitErr error }

func (tx fakeTx) Do(ctx context.Context, fn func(context.Context) error) error {
	if err := fn(ctx); err != nil {
		return err
	}
	return tx.commitErr
}

func TestSaveCallsObserverOnlyAfterCommit(t *testing.T) {
	now := time.Now().UTC()
	input := Input{TripID: uuid.New(), Latitude: 59, Longitude: 30, RecordedAt: now}
	for _, tc := range []struct {
		name         string
		repo         fakePositionRepo
		commitErr    error
		wantErr      bool
		wantCallback int
	}{
		{name: "success", wantCallback: 1},
		{name: "missing trip", repo: fakePositionRepo{lockErr: trip.ErrTripNotFound}, wantErr: true},
		{name: "update failure", repo: fakePositionRepo{updateErr: errors.New("update failed")}, wantErr: true},
		{name: "commit failure", commitErr: errors.New("commit failed"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uc := New(&tc.repo, fakeTx{commitErr: tc.commitErr}, func() time.Time { return now })
			var callbacks int
			uc.SetOnSaved(func(_ context.Context, id uuid.UUID) {
				if id != input.TripID {
					t.Error("wrong trip")
				}
				callbacks++
			})
			position, err := uc.Execute(context.Background(), input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v", err)
			}
			if !tc.wantErr && (position == nil || position.ID != 42 || position.TripID != input.TripID) {
				t.Fatalf("position = %+v", position)
			}
			if callbacks != tc.wantCallback {
				t.Fatalf("callbacks = %d", callbacks)
			}
		})
	}
}

func TestSaveRejectsFutureAndInvalidCoordinates(t *testing.T) {
	now := time.Now().UTC()
	for _, input := range []Input{
		{TripID: uuid.New(), Latitude: 91, Longitude: 30, RecordedAt: now},
		{TripID: uuid.New(), Latitude: 59, Longitude: -181, RecordedAt: now},
		{TripID: uuid.New(), Latitude: 59, Longitude: 30, RecordedAt: now.Add(time.Minute + time.Nanosecond)},
		{TripID: uuid.New(), Latitude: 59, Longitude: 30},
	} {
		repo := &fakePositionRepo{}
		uc := New(repo, fakeTx{}, func() time.Time { return now })
		if _, err := uc.Execute(context.Background(), input); err == nil {
			t.Fatalf("accepted invalid input: %+v", input)
		}
		if repo.inserts != 0 {
			t.Fatal("invalid input reached repository")
		}
	}
}
