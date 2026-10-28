package telemetry

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric"

	"github.com/epif4nov/template/internal/domain/trip"
	"github.com/epif4nov/template/internal/usecase/trip/create"
)

type Business struct {
	created   metric.Int64Counter
	completed metric.Int64Counter
	logger    *slog.Logger
}

func NewBusiness(mp metric.MeterProvider, logger *slog.Logger) (*Business, error) {
	m := mp.Meter("tripgo/business")
	created, err := m.Int64Counter("tripgo.trips.created", metric.WithUnit("{trip}"))
	if err != nil {
		return nil, err
	}
	completed, err := m.Int64Counter("tripgo.trips.completed", metric.WithUnit("{trip}"))
	if err != nil {
		return nil, err
	}
	return &Business{created: created, completed: completed, logger: logger}, nil
}

type CreateUseCase interface {
	Execute(context.Context, create.Input) (*trip.Trip, error)
}
type FinishUseCase interface {
	Execute(context.Context, uuid.UUID) (*trip.Trip, error)
}

type Create struct {
	Next    CreateUseCase
	Signals *Business
}

func (u Create) Execute(ctx context.Context, input create.Input) (*trip.Trip, error) {
	value, err := u.Next.Execute(ctx, input)
	if err == nil {
		u.Signals.created.Add(ctx, 1)
		u.Signals.logger.InfoContext(ctx, "trip created", "trip_id", value.ID())
	}
	return value, err
}

type Finish struct {
	Next    FinishUseCase
	Signals *Business
}

func (u Finish) Execute(ctx context.Context, id uuid.UUID) (*trip.Trip, error) {
	value, err := u.Next.Execute(ctx, id)
	if err == nil {
		u.Signals.completed.Add(ctx, 1)
		u.Signals.logger.InfoContext(ctx, "trip completed", "trip_id", value.ID())
	}
	return value, err
}
