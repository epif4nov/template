//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=finish_test
package finish

import (
	"context"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/domain/trip"
)

type tripRepository interface {
	Fetch(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
	Complete(ctx context.Context, value *trip.Trip) error
	AddStatusHistory(ctx context.Context, id uuid.UUID, from *trip.Status, to trip.Status, reason string) error
}
type txManager interface {
	Do(ctx context.Context, fn func(context.Context) error) error
}
