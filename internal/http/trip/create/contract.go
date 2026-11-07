//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=create_test
package create

import (
	"context"

	"github.com/epif4nov/template/internal/domain/trip"
	usecase "github.com/epif4nov/template/internal/usecase/trip/create"
)

type useCase interface {
	Execute(context.Context, usecase.Input) (*trip.Trip, error)
}
