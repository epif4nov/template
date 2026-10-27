package domainerror_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/epif4nov/template/internal/domain/trip"
	"github.com/epif4nov/template/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{trip.ErrInvalidTrip, 400, "invalid_request"}, {trip.ErrTripNotFound, 404, "trip_not_found"}, {trip.ErrDriverBusy, 409, "driver_busy"}, {trip.ErrTripCompleted, 409, "trip_completed"}, {errors.New("secret SQL password"), 500, "internal_error"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			status, body := domainerror.Problem(fmt.Errorf("wrapped: %w", tc.err))
			if status != tc.status || body.Status != int32(tc.status) || body.Code != tc.code || body.Title == "" || body.Type == "" || body.Detail == nil {
				t.Fatalf("unexpected problem: %d %+v", status, body)
			}
			if strings.Contains(*body.Detail, "secret") {
				t.Fatal("internal details leaked")
			}
		})
	}
}
