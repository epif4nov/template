package create_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/epif4nov/template/internal/domain/trip"
	"github.com/epif4nov/template/internal/usecase/trip/create"
)

func validInput() create.Input {
	return create.Input{
		UserID:     uuid.New(),
		DriverID:   uuid.New(),
		StartPoint: trip.Coordinates{Latitude: 56.84, Longitude: 60.61},
		EndPoint:   trip.Coordinates{Latitude: 56.85, Longitude: 60.63},
		Price:      500,
	}
}

func fixedClock() time.Time {
	return time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
}

func TestExecuteSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)

	var savedID uuid.UUID

	tx.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		})

	repo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, value *trip.Trip) error {
			savedID = value.ID()

			if value.Status() != trip.StatusActive {
				t.Errorf("expected active status, got %s", value.Status())
			}
			return nil
		})

	repo.EXPECT().
		AddStatusHistory(
			gomock.Any(),
			gomock.Any(),
			gomock.Nil(),
			trip.StatusActive,
			"trip created",
		).
		DoAndReturn(func(
			_ context.Context,
			id uuid.UUID,
			_ *trip.Status,
			_ trip.Status,
			_ string,
		) error {
			if id != savedID {
				t.Errorf("history has wrong trip ID")
			}
			return nil
		})

	uc := create.New(repo, tx, fixedClock)

	result, err := uc.Execute(context.Background(), validInput())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if result == nil || result.ID() != savedID {
		t.Fatal("unexpected result")
	}
}

func TestExecuteInvalidPrice(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)

	uc := create.New(repo, tx, fixedClock)

	input := validInput()
	input.Price = -100

	result, err := uc.Execute(context.Background(), input)

	if !errors.Is(err, trip.ErrInvalidTrip) {
		t.Fatalf("expected ErrInvalidTrip, got %v", err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}
}

func TestExecuteRepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)

	dbErr := errors.New("database unavailable")

	tx.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		})

	repo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(dbErr)

	uc := create.New(repo, tx, fixedClock)

	result, err := uc.Execute(context.Background(), validInput())

	if !errors.Is(err, dbErr) {
		t.Fatalf("expected wrapped database error, got %v", err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}
}

func TestExecuteHistoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)

	historyErr := errors.New("history insertion failed")

	tx.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		})

	repo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(nil)

	repo.EXPECT().
		AddStatusHistory(
			gomock.Any(),
			gomock.Any(),
			gomock.Nil(),
			trip.StatusActive,
			"trip created",
		).
		Return(historyErr)

	uc := create.New(repo, tx, fixedClock)

	result, err := uc.Execute(context.Background(), validInput())

	if !errors.Is(err, historyErr) {
		t.Fatalf("expected wrapped history error, got %v", err)
	}

	if result != nil {
		t.Fatal("expected nil result")
	}
}

func TestExecuteDriverBusy(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })
	repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(trip.ErrDriverBusy)
	got, err := create.New(repo, tx, fixedClock).Execute(context.Background(), validInput())
	if got != nil || !errors.Is(err, trip.ErrDriverBusy) {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestExecuteTransactionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	failure := errors.New("begin failed")
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).Return(failure)
	got, err := create.New(repo, tx, fixedClock).Execute(context.Background(), validInput())
	if got != nil || !errors.Is(err, failure) {
		t.Fatalf("got %v, %v", got, err)
	}
}
