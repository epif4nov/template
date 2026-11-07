//go:build integration

package trip_test

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "github.com/epif4nov/template/internal/domain/trip"
	"github.com/epif4nov/template/internal/infra/postgres/transaction"
	repository "github.com/epif4nov/template/internal/repository/trip"
	"github.com/epif4nov/template/internal/usecase/trip/create"
	"github.com/epif4nov/template/internal/usecase/trip/finish"
)

type application struct {
	create *create.UseCase
	finish *finish.UseCase
}

func newApplication(repo *repository.Repository, tx transaction.TxManager) *application {
	now := func() time.Time { return time.Now().UTC() }
	return &application{create: create.New(repo, tx, now), finish: finish.New(repo, tx, now)}
}

func record(t *domain.Trip) repository.Trip {
	return repository.Trip{ID: t.ID(), UserID: t.UserID(), DriverID: t.DriverID(), StartLatitude: t.StartPoint().Latitude, StartLongitude: t.StartPoint().Longitude, EndLatitude: t.EndPoint().Latitude, EndLongitude: t.EndPoint().Longitude, Price: t.Price(), Status: string(t.Status()), StartedAt: t.StartedAt(), FinishedAt: t.FinishedAt()}
}

func (a *application) CreateTrip(ctx context.Context, t repository.Trip) (repository.Trip, error) {
	value, err := a.create.Execute(ctx, create.Input{UserID: t.UserID, DriverID: t.DriverID, StartPoint: domain.Coordinates{Latitude: t.StartLatitude, Longitude: t.StartLongitude}, EndPoint: domain.Coordinates{Latitude: t.EndLatitude, Longitude: t.EndLongitude}, Price: t.Price})
	if err != nil {
		return repository.Trip{}, err
	}
	return record(value), nil
}

func (a *application) FinishTrip(ctx context.Context, id uuid.UUID) (repository.Trip, error) {
	value, err := a.finish.Execute(ctx, id)
	if err != nil {
		return repository.Trip{}, err
	}
	return record(value), nil
}
