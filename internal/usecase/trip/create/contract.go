//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=create_test
package create

import (
	"context"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/domain/trip"
)

type tripRepository interface {
	Save(ctx context.Context, value *trip.Trip) error

	AddStatusHistory(
		ctx context.Context,
		tripID uuid.UUID,
		from *trip.Status,
		to trip.Status,
		reason string,
	) error
}

type txManager interface {
	Do(ctx context.Context, fn func(context.Context) error) error
}
