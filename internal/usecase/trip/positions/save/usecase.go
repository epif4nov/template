package save

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/domain/trip"
)

type Input struct {
	TripID     uuid.UUID
	Latitude   float64
	Longitude  float64
	RecordedAt time.Time
}

type Position struct {
	ID         int64
	TripID     uuid.UUID
	Latitude   float64
	Longitude  float64
	RecordedAt time.Time
}

type UseCase struct {
	repo positionRepository
	tx   txManager
	now  func() time.Time
}

func New(
	repo positionRepository,
	tx txManager,
	now func() time.Time,
) *UseCase {
	return &UseCase{repo: repo, tx: tx, now: now}
}

func (u *UseCase) Execute(
	ctx context.Context,
	input Input,
) (*Position, error) {
	if input.TripID == uuid.Nil {
		return nil, &trip.ValidationError{
			Field:  "trip_id",
			Reason: "must not be zero UUID",
		}
	}

	if math.IsNaN(input.Latitude) ||
		math.IsInf(input.Latitude, 0) ||
		input.Latitude < -90 ||
		input.Latitude > 90 {
		return nil, &trip.ValidationError{
			Field:  "latitude",
			Reason: "must be between -90 and 90",
		}
	}

	if math.IsNaN(input.Longitude) ||
		math.IsInf(input.Longitude, 0) ||
		input.Longitude < -180 ||
		input.Longitude > 180 {
		return nil, &trip.ValidationError{
			Field:  "longitude",
			Reason: "must be between -180 and 180",
		}
	}

	if input.RecordedAt.IsZero() ||
		input.RecordedAt.After(u.now().Add(time.Minute)) {
		return nil, &trip.ValidationError{
			Field:  "recorded_at",
			Reason: "invalid or too far in the future",
		}
	}

	var id int64
	err := u.tx.Do(ctx, func(txCtx context.Context) error {
		if err := u.repo.LockActiveTrip(
			txCtx, input.TripID,
		); err != nil {
			return err
		}

		var err error
		id, err = u.repo.InsertPosition(
			txCtx,
			input.TripID,
			input.Latitude,
			input.Longitude,
			input.RecordedAt,
		)
		if err != nil {
			return err
		}

		return u.repo.UpdateLastPositionAt(
			txCtx,
			input.TripID,
			input.RecordedAt,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("save position: %w", err)
	}

	return &Position{
		ID:         id,
		TripID:     input.TripID,
		Latitude:   input.Latitude,
		Longitude:  input.Longitude,
		RecordedAt: input.RecordedAt,
	}, nil
}
