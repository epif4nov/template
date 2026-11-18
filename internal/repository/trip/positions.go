package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/epif4nov/template/internal/domain/trip"
	"github.com/epif4nov/template/internal/infra/postgres/transaction"
)

// LockActiveTrip проверяет статус поездки и блокирует строку
// до завершения транзакции.
func (r *Repository) LockActiveTrip(
	ctx context.Context,
	id uuid.UUID,
) error {
	var status string

	err := transaction.Executor(ctx, r.pool).QueryRow(
		ctx,
		`SELECT status FROM trips WHERE id = $1 FOR UPDATE`,
		id,
	).Scan(&status)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrTripNotFound
	}
	if err != nil {
		return fmt.Errorf("lock trip: %w", err)
	}
	if status != "active" {
		return domain.ErrTripCompleted
	}

	return nil
}

// InsertPosition добавляет координату и возвращает её ID.
func (r *Repository) InsertPosition(
	ctx context.Context,
	tripID uuid.UUID,
	latitude, longitude float64,
	recordedAt time.Time,
) (int64, error) {
	var id int64

	err := transaction.Executor(ctx, r.pool).QueryRow(
		ctx,
		`INSERT INTO trip_positions
			(trip_id, latitude, longitude, recorded_at)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		tripID, latitude, longitude, recordedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert position: %w", err)
	}

	return id, nil
}

// UpdateLastPositionAt сохраняет самое позднее время координаты.
func (r *Repository) UpdateLastPositionAt(
	ctx context.Context,
	tripID uuid.UUID,
	recordedAt time.Time,
) error {
	_, err := transaction.Executor(ctx, r.pool).Exec(
		ctx,
		`UPDATE trips
		 SET last_position_at =
		     GREATEST(COALESCE(last_position_at, $2), $2),
		     updated_at = now()
		 WHERE id = $1`,
		tripID, recordedAt,
	)
	if err != nil {
		return fmt.Errorf("update last position: %w", err)
	}

	return nil
}
