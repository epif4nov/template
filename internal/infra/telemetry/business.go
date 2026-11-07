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
	created            metric.Int64Counter
	completed          metric.Int64Counter
	activeRegistration metric.Registration
	logger             *slog.Logger
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

type createUseCase interface {
	Execute(context.Context, create.Input) (*trip.Trip, error)
}
type finishUseCase interface {
	Execute(context.Context, uuid.UUID) (*trip.Trip, error)
}

type Create struct {
	Next    createUseCase
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
	Next    finishUseCase
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

func (b *Business) RegisterActiveTrips(
	mp metric.MeterProvider,
	countActive func(context.Context) (int64, error),
) error {
	m := mp.Meter("tripgo/business")

	gauge, err := m.Int64ObservableGauge(
		"tripgo.trips.active",
		metric.WithUnit("{trip}"),
		metric.WithDescription(
			"Number of currently active trips",
		),
	)
	if err != nil {
		return err
	}

	registration, err := m.RegisterCallback(
		func(ctx context.Context, observer metric.Observer) error {
			count, err := countActive(ctx)
			if err != nil {
				b.logger.ErrorContext(
					ctx,
					"count active trips failed",
					"error", err,
				)
				return err
			}

			observer.ObserveInt64(gauge, count)
			return nil
		},
		gauge,
	)
	if err != nil {
		return err
	}

	b.activeRegistration = registration
	return nil
}

func (b *Business) UnregisterActiveTrips() {
	if b.activeRegistration == nil {
		return
	}

	if err := b.activeRegistration.Unregister(); err != nil {
		b.logger.Error(
			"unregister active trips metric failed",
			"error", err,
		)
	}
}
