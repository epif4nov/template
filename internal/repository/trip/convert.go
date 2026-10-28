package trip

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domain "github.com/epif4nov/template/internal/domain/trip"
)

func (r *Repository) Save(ctx context.Context, t *domain.Trip) error {
	return r.CreateTrip(ctx, Trip{
		ID:             t.ID(),
		UserID:         t.UserID(),
		DriverID:       t.DriverID(),
		StartLatitude:  t.StartPoint().Latitude,
		StartLongitude: t.StartPoint().Longitude,
		EndLatitude:    t.EndPoint().Latitude,
		EndLongitude:   t.EndPoint().Longitude,
		Price:          t.Price(),
		Status:         string(t.Status()),
		StartedAt:      t.StartedAt(),
		FinishedAt:     t.FinishedAt(),
	})
}

func restore(t Trip) (*domain.Trip, error) {
	value, err := domain.Restore(
		t.ID, t.UserID, t.DriverID,
		domain.Coordinates{Latitude: t.StartLatitude, Longitude: t.StartLongitude},
		domain.Coordinates{Latitude: t.EndLatitude, Longitude: t.EndLongitude},
		t.Price, domain.Status(t.Status), t.StartedAt, t.FinishedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("restore trip: %w", err)
	}
	return value, nil
}

func (r *Repository) Fetch(ctx context.Context, id uuid.UUID) (*domain.Trip, error) {
	row, err := r.GetTrip(ctx, id)
	if err != nil {
		return nil, err
	}
	return restore(row)
}

// Complete uses a conditional UPDATE so only the winning request records history.
func (r *Repository) Complete(ctx context.Context, t *domain.Trip) error {
	if t.Status() != domain.StatusCompleted || t.FinishedAt() == nil {
		return domain.ErrInvalidTrip
	}
	_, err := r.FinishTrip(ctx, t.ID(), *t.FinishedAt())
	return err
}

func (r *Repository) AddStatusHistory(ctx context.Context, id uuid.UUID, from *domain.Status, to domain.Status, reason string) error {
	var previous *string
	if from != nil {
		s := string(*from)
		previous = &s
	}
	return r.insertStatusHistory(ctx, StatusHistory{TripID: id, FromStatus: previous, ToStatus: string(to), Reason: &reason})
}
