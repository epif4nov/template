package get_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/epif4nov/template/internal/domain/trip"
	"github.com/epif4nov/template/internal/usecase/trip/get"
)

func TestExecute(t *testing.T) {
	value, err := trip.New(uuid.New(), uuid.New(), trip.Coordinates{}, trip.Coordinates{}, 100, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	dbErr := errors.New("database unavailable")
	for _, tc := range []struct {
		name    string
		id      uuid.UUID
		repoErr error
		want    error
	}{
		{name: "success", id: value.ID()},
		{name: "invalid UUID", id: uuid.Nil, want: trip.ErrInvalidTrip},
		{name: "not found", id: value.ID(), repoErr: trip.ErrTripNotFound, want: trip.ErrTripNotFound},
		{name: "repository error", id: value.ID(), repoErr: dbErr, want: dbErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewMocktripRepository(gomock.NewController(t))
			ctx := context.Background()
			if tc.id != uuid.Nil {
				repo.EXPECT().Fetch(ctx, tc.id).Return(value, tc.repoErr)
			}
			got, err := get.New(repo).Execute(ctx, tc.id)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if tc.want != nil {
				if got != nil {
					t.Fatal("expected nil on failure")
				}
				return
			}
			if got != value {
				t.Fatal("returned a different trip")
			}
		})
	}
}
