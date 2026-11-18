package list

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/domain/trip"
)

type UseCase struct {
	repo positionRepository
}

func New(repo positionRepository) *UseCase {
	return &UseCase{repo: repo}
}

func (u *UseCase) Execute(
	ctx context.Context,
	tripID uuid.UUID,
) ([]trip.Position, error) {
	if tripID == uuid.Nil {
		return nil, &trip.ValidationError{
			Field:  "trip_id",
			Reason: "must not be zero UUID",
		}
	}

	positions, err := u.repo.ListPositions(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}

	return positions, nil
}
