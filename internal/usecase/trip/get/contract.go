//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=get_test
package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/domain/trip"
)

type tripRepository interface {
	Fetch(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
}
