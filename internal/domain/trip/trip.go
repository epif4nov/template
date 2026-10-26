package trip

import (
	"time"

	"github.com/google/uuid"
)

type Coordinates struct {
	Latitude  float64
	Longitude float64
}

type Trip struct {
	id       uuid.UUID
	userID   uuid.UUID
	driverID uuid.UUID

	startPoint Coordinates
	endPoint   Coordinates

	price int64

	status     Status
	startedAt  time.Time
	finishedAt *time.Time
}

func New(
	userID uuid.UUID,
	driverID uuid.UUID,
	startPoint Coordinates,
	endPoint Coordinates,
	price int64,
	startedAt time.Time,
) (*Trip, error) {
	if userID == uuid.Nil ||
		driverID == uuid.Nil ||
		price < 0 ||
		!validCoordinates(startPoint) ||
		!validCoordinates(endPoint) {
		return nil, ErrInvalidTrip
	}

	return &Trip{
		id:         uuid.New(),
		userID:     userID,
		driverID:   driverID,
		startPoint: startPoint,
		endPoint:   endPoint,
		price:      price,
		status:     StatusActive,
		startedAt:  startedAt,
	}, nil
}

func (t *Trip) Finish(at time.Time) error {
	if t.status != StatusActive {
		return ErrTripCompleted
	}

	t.status = StatusCompleted
	t.finishedAt = &at

	return nil
}

func (t *Trip) Status() Status {
	return t.status
}

func (t *Trip) FinishedAt() *time.Time {
	if t.finishedAt == nil {
		return nil
	}

	value := *t.finishedAt
	return &value
}

func validCoordinates(point Coordinates) bool {
	return point.Latitude >= -90 &&
		point.Latitude <= 90 &&
		point.Longitude >= -180 &&
		point.Longitude <= 180
}

func (t *Trip) ID() uuid.UUID {
	return t.id
}
