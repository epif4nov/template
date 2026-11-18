package trip

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTripFinish(t *testing.T) {
	startedAt := time.Now()
	finishedAt := startedAt.Add(10 * time.Minute)

	trip, err := New(
		uuid.New(),
		uuid.New(),
		Coordinates{Latitude: 56.84, Longitude: 60.61},
		Coordinates{Latitude: 56.85, Longitude: 60.63},
		500,
		startedAt,
	)
	if err != nil {
		t.Fatalf("create trip: %v", err)
	}

	if trip.Status() != StatusActive {
		t.Fatal("new trip must be active")
	}

	if err := trip.Finish(finishedAt); err != nil {
		t.Fatalf("finish trip: %v", err)
	}

	if trip.Status() != StatusCompleted {
		t.Fatal("trip must be completed")
	}

	if got := trip.FinishedAt(); got == nil || !got.Equal(finishedAt) {
		t.Fatal("incorrect finish time")
	}

	err = trip.Finish(finishedAt.Add(time.Minute))
	if !errors.Is(err, ErrTripCompleted) {
		t.Fatalf("expected ErrTripCompleted, got %v", err)
	}

	if !trip.FinishedAt().Equal(finishedAt) {
		t.Fatal("repeated finish changed finish time")
	}
}

func TestNewTripInvalidPrice(t *testing.T) {
	_, err := New(
		uuid.New(),
		uuid.New(),
		Coordinates{Latitude: 56.84, Longitude: 60.61},
		Coordinates{Latitude: 56.85, Longitude: 60.63},
		-100,
		time.Now(),
	)

	if !errors.Is(err, ErrInvalidTrip) {
		t.Fatalf("expected ErrInvalidTrip, got %v", err)
	}
}
