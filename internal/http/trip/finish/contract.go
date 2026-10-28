//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=finish_test
package finish

import (
	"context"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/domain/trip"
)

type useCase interface {
	Execute(context.Context, uuid.UUID) (*trip.Trip, error)
}
