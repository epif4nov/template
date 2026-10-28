package finish_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/epif4nov/template/internal/domain/trip"
	"github.com/epif4nov/template/internal/usecase/trip/finish"
)

type contextKey struct{}

func TestExecute(t *testing.T) {
	dbErr := errors.New("database unavailable")
	historyErr := errors.New("history failed")
	commitErr := errors.New("commit failed")
	for _, tc := range []struct {
		name                                       string
		invalid, completed                         bool
		fetchErr, saveErr, historyErr, txErr, want error
	}{
		{name: "success"},
		{name: "invalid UUID", invalid: true, want: trip.ErrInvalidTrip},
		{name: "already completed", completed: true, want: trip.ErrTripCompleted},
		{name: "missing trip", fetchErr: trip.ErrTripNotFound, want: trip.ErrTripNotFound},
		{name: "fetch error", fetchErr: dbErr, want: dbErr},
		{name: "update error", saveErr: dbErr, want: dbErr},
		{name: "concurrent completion", saveErr: trip.ErrTripCompleted, want: trip.ErrTripCompleted},
		{name: "history error", historyErr: historyErr, want: historyErr},
		{name: "commit error", txErr: commitErr, want: commitErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			at := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
			value, err := trip.New(uuid.New(), uuid.New(), trip.Coordinates{}, trip.Coordinates{}, 100, at.Add(-time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			if tc.completed {
				if err := value.Finish(at.Add(-time.Minute)); err != nil {
					t.Fatal(err)
				}
			}
			id := value.ID()
			ctx := context.Background()
			txCtx := context.WithValue(ctx, contextKey{}, true)
			if tc.invalid {
				id = uuid.Nil
			} else {
				tx.EXPECT().Do(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, fn func(context.Context) error) error {
					err := fn(txCtx)
					if err != nil {
						return err
					}
					return tc.txErr
				})
				fetch := repo.EXPECT().Fetch(txCtx, id).Return(value, tc.fetchErr)
				if tc.fetchErr == nil && !tc.completed {
					complete := repo.EXPECT().Complete(txCtx, value).After(fetch).DoAndReturn(func(_ context.Context, got *trip.Trip) error {
						if got.Status() != trip.StatusCompleted || got.FinishedAt() == nil || !got.FinishedAt().Equal(at) {
							t.Fatal("incorrect completed state or clock")
						}
						return tc.saveErr
					})
					if tc.saveErr == nil {
						previous := trip.StatusActive
						repo.EXPECT().AddStatusHistory(txCtx, id, &previous, trip.StatusCompleted, "trip finished").After(complete).Return(tc.historyErr)
					}
				}
			}
			got, err := finish.New(repo, tx, func() time.Time { return at }).Execute(ctx, id)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if tc.want != nil {
				if got != nil {
					t.Fatal("expected nil result after failure")
				}
				return
			}
			if got != value || !got.FinishedAt().Equal(at) {
				t.Fatal("unexpected result")
			}
		})
	}
}
