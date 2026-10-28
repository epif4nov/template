package trip

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/epif4nov/template/internal/domain/trip"
	"github.com/epif4nov/template/internal/infra/postgres/transaction"
)

var ErrTripNotFound = domain.ErrTripNotFound

func (r *Repository) GetTrip(
	ctx context.Context,
	id uuid.UUID,
) (Trip, error) {
	query, args, err := sq.
		Select(
			"id",
			"user_id",
			"driver_id",
			"start_latitude",
			"start_longitude",
			"end_latitude",
			"end_longitude",
			"price",
			"status",
			"started_at",
			"finished_at",
		).
		From("trips").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return Trip{}, fmt.Errorf("build get trip query: %w", err)
	}

	executor := transaction.Executor(ctx, r.pool)

	var trip Trip

	err = executor.QueryRow(ctx, query, args...).Scan(
		&trip.ID,
		&trip.UserID,
		&trip.DriverID,
		&trip.StartLatitude,
		&trip.StartLongitude,
		&trip.EndLatitude,
		&trip.EndLongitude,
		&trip.Price,
		&trip.Status,
		&trip.StartedAt,
		&trip.FinishedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Trip{}, ErrTripNotFound
	}

	if err != nil {
		return Trip{}, fmt.Errorf("get trip: %w", err)
	}

	return trip, nil
}
