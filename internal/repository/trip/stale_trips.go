package trip

import (
	"context"
	"fmt"
	"time"

	domain "github.com/epif4nov/template/internal/domain/trip"
	"github.com/epif4nov/template/internal/infra/postgres/transaction"
)

func (r *Repository) FindStaleTrips(
	ctx context.Context,
	staleAfter time.Duration,
) ([]domain.StaleTrip, error) {
	if staleAfter <= 0 {
		return nil, fmt.Errorf("staleAfter must be positive")
	}

	const query = `
		SELECT
			id,
			driver_id,
			COALESCE(last_position_at, started_at)
		FROM trips
		WHERE status = 'active'
		  AND COALESCE(last_position_at, started_at)
		      < NOW() - ($1::double precision * INTERVAL '1 second')
		ORDER BY
			COALESCE(last_position_at, started_at) ASC,
			id ASC
	`

	executor := transaction.Executor(ctx, r.pool)

	rows, err := executor.Query(
		ctx,
		query,
		staleAfter.Seconds(),
	)
	if err != nil {
		return nil, fmt.Errorf("find stale trips: %w", err)
	}
	defer rows.Close()

	trips := make([]domain.StaleTrip, 0)

	for rows.Next() {
		var t domain.StaleTrip

		if err := rows.Scan(
			&t.ID,
			&t.DriverID,
			&t.LastSeenAt,
		); err != nil {
			return nil, fmt.Errorf("scan stale trip: %w", err)
		}

		trips = append(trips, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stale trips: %w", err)
	}

	return trips, nil
}
