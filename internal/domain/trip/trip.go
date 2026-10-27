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
		startedAt.IsZero() ||
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

	if at.IsZero() || at.Before(t.startedAt) {
		return ErrInvalidTrip
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

// Restore validates persisted state without generating a new identity.
func Restore(id, userID, driverID uuid.UUID, start, end Coordinates, price int64, status Status, startedAt time.Time, finishedAt *time.Time) (*Trip, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidTrip
	}
	value, err := New(userID, driverID, start, end, price, startedAt)
	if err != nil {
		return nil, err
	}
	value.id = id
	switch status {
	case StatusActive:
		if finishedAt != nil {
			return nil, ErrInvalidTrip
		}
	case StatusCompleted:
		if finishedAt == nil {
			return nil, ErrInvalidTrip
		}
		if err := value.Finish(*finishedAt); err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalidTrip
	}
	return value, nil
}

func (t *Trip) UserID() uuid.UUID { return t.userID }

func (t *Trip) DriverID() uuid.UUID { return t.driverID }

func (t *Trip) StartPoint() Coordinates { return t.startPoint }

func (t *Trip) EndPoint() Coordinates { return t.endPoint }

func (t *Trip) Price() int64 { return t.price }

func (t *Trip) StartedAt() time.Time { return t.startedAt }
