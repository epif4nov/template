package trip

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/infra/postgres/transaction"
)

type StatusHistory struct {
	TripID uuid.UUID

	FromStatus *string
	ToStatus   string

	Reason *string
}

func (r *Repository) insertStatusHistory(
	ctx context.Context,
	history StatusHistory,
) error {
	query, args, err := sq.
		Insert("trip_status_history").
		Columns(
			"trip_id",
			"from_status",
			"to_status",
			"reason",
		).
		Values(
			history.TripID,
			history.FromStatus,
			history.ToStatus,
			history.Reason,
		).
		PlaceholderFormat(sq.Dollar).
		ToSql()

	if err != nil {
		return fmt.Errorf("build status history query: %w", err)
	}

	executor := transaction.Executor(ctx, r.pool)

	_, err = executor.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("add status history: %w", err)
	}

	return nil
}
