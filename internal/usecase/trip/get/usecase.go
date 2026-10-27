package get

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/domain/trip"
)

type UseCase struct{ repo tripRepository }

func New(repo tripRepository) *UseCase { return &UseCase{repo: repo} }

func (u *UseCase) Execute(ctx context.Context, id uuid.UUID) (*trip.Trip, error) {
	if id == uuid.Nil {
		return nil, trip.ErrInvalidTrip
	}
	value, err := u.repo.Fetch(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get trip: %w", err)
	}
	return value, nil
}
