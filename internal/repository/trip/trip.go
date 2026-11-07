package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "github.com/epif4nov/template/internal/domain/trip"
	"github.com/epif4nov/template/internal/infra/postgres/transaction"
)

var ErrDriverBusy = domain.ErrDriverBusy

type Trip struct {
	ID uuid.UUID

	UserID   uuid.UUID
	DriverID uuid.UUID

	StartLatitude  float64
	StartLongitude float64
	EndLatitude    float64
	EndLongitude   float64

	Price int64

	Status string

	StartedAt  time.Time
	FinishedAt *time.Time
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool: pool,
	}
}

func (r *Repository) CreateTrip(
	ctx context.Context,
	trip Trip,
) error {
	query, args, err := sq.
		Insert("trips").
		Columns(
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
		Values(
			trip.ID,
			trip.UserID,
			trip.DriverID,
			trip.StartLatitude,
			trip.StartLongitude,
			trip.EndLatitude,
			trip.EndLongitude,
			trip.Price,
			trip.Status,
			trip.StartedAt,
			trip.FinishedAt,
		).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build create trip query: %w", err)
	}

	executor := transaction.Executor(ctx, r.pool)

	_, err = executor.Exec(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) &&
			pgErr.Code == "23505" &&
			pgErr.ConstraintName == "trips_active_driver_idx" {
			return ErrDriverBusy
		}

		return fmt.Errorf("create trip: %w", err)
	}

	return nil
}
