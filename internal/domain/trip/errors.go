package trip

import "errors"

var (
	ErrInvalidTrip   = errors.New("invalid trip data")
	ErrTripNotFound  = errors.New("trip not found")
	ErrTripCompleted = errors.New("trip already completed")
	ErrDriverBusy    = errors.New("driver already has an active trip")
)

// ValidationError identifies an invalid field without retaining input values.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Reason }

func (e *ValidationError) Unwrap() error { return ErrInvalidTrip }
