package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/epif4nov/template/internal/repository"
)

// создаёт поездку один раз для каждой пары: ключ + содержимое запроса
func (s *Service) CreateTripIdempotent(
	ctx context.Context,
	trip repository.Trip,
	key string,
	hash [32]byte,
) (repository.Trip, error) {
	var result repository.Trip

	err := s.txManager.Do(ctx, func(txCtx context.Context) error {
		isNew, savedBody, err := s.repo.ClaimCreateTrip(
			txCtx, key, hash,
		)
		if err != nil {
			return err
		}

		if !isNew {
			if err := json.Unmarshal(savedBody, &result); err != nil {
				return fmt.Errorf(
					"decode saved trip: %w", err,
				)
			}
			return nil
		}

		// использует текущую транзакцию из контекста, не открывая новую
		result, err = s.CreateTrip(txCtx, trip)
		if err != nil {
			return err
		}

		return s.repo.CompleteCreateTrip(
			txCtx, key, result,
		)
	})
	if err != nil {
		return repository.Trip{}, fmt.Errorf(
			"idempotent create trip: %w", err,
		)
	}

	return result, nil
}
