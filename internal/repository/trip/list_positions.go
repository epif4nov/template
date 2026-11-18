package trip

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domain "github.com/epif4nov/template/internal/domain/trip"
	"github.com/epif4nov/template/internal/infra/postgres/transaction"
)

func (r *Repository) ListPositions(
	ctx context.Context,
	tripID uuid.UUID,
) ([]domain.Position, error) {
	// Проверяем существование поездки.
	if _, err := r.GetTrip(ctx, tripID); err != nil {
		return nil, err
	}

	rows, err := transaction.Executor(ctx, r.pool).Query(
		ctx,
		`SELECT id, latitude, longitude, recorded_at
		 FROM trip_positions
		 WHERE trip_id = $1
		 ORDER BY recorded_at ASC, id ASC`,
		tripID,
	)
	if err != nil {
		return nil, fmt.Errorf("query positions: %w", err)
	}
	defer rows.Close()

	positions := make([]domain.Position, 0)

	for rows.Next() {
		var p domain.Position
		p.TripID = tripID

		if err := rows.Scan(
			&p.ID,
			&p.Coordinates.Latitude,
			&p.Coordinates.Longitude,
			&p.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}

		positions = append(positions, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate positions: %w", err)
	}

	return positions, nil
}
