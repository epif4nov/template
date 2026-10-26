package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/transaction"
)

var ErrTripCompleted = errors.New("trip already completed")

func (r *Repository) FinishTrip(
	ctx context.Context,
	id uuid.UUID,
	finishedAt time.Time,
) (Trip, error) {
	query, args, err := sq.
		Update("trips").
		Set("status", "completed").
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(sq.Eq{
			"id":     id,
			"status": "active",
		}).
		PlaceholderFormat(sq.Dollar).
		ToSql()

	if err != nil {
		return Trip{}, fmt.Errorf("build finish trip query: %w", err)
	}

	executor := transaction.Executor(ctx, r.pool)

	result, err := executor.Exec(ctx, query, args...)
	if err != nil {
		return Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	if result.RowsAffected() == 0 {
		_, err := r.GetTrip(ctx, id)

		if err != nil {
			return Trip{}, err
		}

		return Trip{}, ErrTripCompleted
	}

	trip, err := r.GetTrip(ctx, id)
	if err != nil {
		return Trip{}, err
	}

	return trip, nil
}
