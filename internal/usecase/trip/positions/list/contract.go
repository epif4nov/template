package list

import (
	"context"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/domain/trip"
)

type positionRepository interface {
	ListPositions(
		context.Context,
		uuid.UUID,
	) ([]trip.Position, error)
}
