package save

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type positionRepository interface {
	LockActiveTrip(context.Context, uuid.UUID) error

	InsertPosition(
		context.Context,
		uuid.UUID,
		float64,
		float64,
		time.Time,
	) (int64, error)

	UpdateLastPositionAt(
		context.Context,
		uuid.UUID,
		time.Time,
	) error
}

type txManager interface {
	Do(context.Context, func(context.Context) error) error
}
