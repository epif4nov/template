package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/epif4nov/template/internal/repository"
	"github.com/epif4nov/template/internal/transaction"
)

var ErrInvalidTrip = errors.New("invalid trip data")

type Service struct {
	repo      *repository.Repository
	txManager transaction.TxManager
}

func New(
	repo *repository.Repository,
	txManager transaction.TxManager,
) *Service {
	return &Service{
		repo:      repo,
		txManager: txManager,
	}
}

func (s *Service) CreateTrip(
	ctx context.Context,
	trip repository.Trip,
) (repository.Trip, error) {

	// Проверяем входные данные.
	if trip.UserID == uuid.Nil ||
		trip.DriverID == uuid.Nil ||
		trip.Price < 0 ||
		!inRange(trip.StartLatitude, -90, 90) ||
		!inRange(trip.StartLongitude, -180, 180) ||
		!inRange(trip.EndLatitude, -90, 90) ||
		!inRange(trip.EndLongitude, -180, 180) {
		return repository.Trip{}, ErrInvalidTrip
	}

	// Заполняем данные новой поездки.
	trip.ID = uuid.New()
	trip.Status = "active"
	trip.StartedAt = time.Now().UTC()
	trip.FinishedAt = nil

	// Объединяем две операции в одну транзакцию.
	err := s.txManager.Do(ctx, func(txCtx context.Context) error {

		// Создаём поездку.
		if err := s.repo.CreateTrip(txCtx, trip); err != nil {
			return err
		}

		// Записываем историю создания.
		reason := "trip created"

		history := repository.StatusHistory{
			TripID:     trip.ID,
			FromStatus: nil,
			ToStatus:   "active",
			Reason:     &reason,
		}

		if err := s.repo.AddStatusHistory(txCtx, history); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return repository.Trip{}, fmt.Errorf("create trip: %w", err)
	}

	return trip, nil
}

func inRange(value, min, max float64) bool {
	return value >= min && value <= max
}

func (s *Service) GetTrip(
	ctx context.Context,
	id uuid.UUID,
) (repository.Trip, error) {
	if id == uuid.Nil {
		return repository.Trip{}, ErrInvalidTrip
	}

	trip, err := s.repo.GetTrip(ctx, id)
	if err != nil {
		return repository.Trip{}, fmt.Errorf("get trip: %w", err)
	}

	return trip, nil
}

func (s *Service) FinishTrip(
	ctx context.Context,
	id uuid.UUID,
) (repository.Trip, error) {
	if id == uuid.Nil {
		return repository.Trip{}, ErrInvalidTrip
	}

	var completedTrip repository.Trip

	err := s.txManager.Do(ctx, func(txCtx context.Context) error {
		trip, err := s.repo.FinishTrip(
			txCtx,
			id,
			time.Now().UTC(),
		)
		if err != nil {
			return err
		}

		previousStatus := "active"
		reason := "trip finished"

		history := repository.StatusHistory{
			TripID:     id,
			FromStatus: &previousStatus,
			ToStatus:   "completed",
			Reason:     &reason,
		}

		if err := s.repo.AddStatusHistory(txCtx, history); err != nil {
			return err
		}

		completedTrip = trip
		return nil
	})

	if err != nil {
		return repository.Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	return completedTrip, nil
}
