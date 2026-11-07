package create

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/domain/trip"
)

type Input struct {
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartPoint trip.Coordinates
	EndPoint   trip.Coordinates
	Price      int64
}

type UseCase struct {
	repo tripRepository
	tx   txManager
	now  func() time.Time
}

func New(
	repo tripRepository,
	tx txManager,
	now func() time.Time,
) *UseCase {
	return &UseCase{
		repo: repo,
		tx:   tx,
		now:  now,
	}
}

func (u *UseCase) Execute(
	ctx context.Context,
	input Input,
) (*trip.Trip, error) {
	created, err := trip.New(
		input.UserID,
		input.DriverID,
		input.StartPoint,
		input.EndPoint,
		input.Price,
		u.now(),
	)
	if err != nil {
		return nil, fmt.Errorf("validate trip: %w", err)
	}

	err = u.tx.Do(ctx, func(txCtx context.Context) error {
		if err := u.repo.Save(txCtx, created); err != nil {
			return err
		}

		return u.repo.AddStatusHistory(
			txCtx,
			created.ID(),
			nil,
			trip.StatusActive,
			"trip created",
		)
	})
	if err != nil {
		return nil, fmt.Errorf("create trip: %w", err)
	}

	return created, nil
}
