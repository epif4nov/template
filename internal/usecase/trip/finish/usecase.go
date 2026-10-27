package finish

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/domain/trip"
)

type UseCase struct {
	repo tripRepository
	tx   txManager
	now  func() time.Time
}

func New(repo tripRepository, tx txManager, now func() time.Time) *UseCase {
	return &UseCase{repo: repo, tx: tx, now: now}
}

func (u *UseCase) Execute(ctx context.Context, id uuid.UUID) (*trip.Trip, error) {
	if id == uuid.Nil {
		return nil, trip.ErrInvalidTrip
	}
	var result *trip.Trip
	err := u.tx.Do(ctx, func(txCtx context.Context) error {
		value, err := u.repo.Fetch(txCtx, id)
		if err != nil {
			return err
		}
		previous := value.Status()
		if err := value.Finish(u.now()); err != nil {
			return err
		}
		if err := u.repo.Complete(txCtx, value); err != nil {
			return err
		}
		if err := u.repo.AddStatusHistory(txCtx, id, &previous, value.Status(), "trip finished"); err != nil {
			return err
		}
		result = value
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("finish trip: %w", err)
	}
	return result, nil
}
