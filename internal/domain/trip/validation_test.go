package trip

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidationErrorDetails(t *testing.T) {
	id := uuid.New()
	at := time.Date(2026, 10, 28, 6, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		field        string
		user, driver uuid.UUID
		start, end   Coordinates
		price        int64
		at           time.Time
	}{
		{field: "user_id", driver: id, at: at},
		{field: "driver_id", user: id, at: at},
		{field: "price", user: id, driver: id, price: -1, at: at},
		{field: "started_at", user: id, driver: id},
		{field: "start_point", user: id, driver: id, start: Coordinates{Latitude: 91}, at: at},
		{field: "end_point", user: id, driver: id, end: Coordinates{Longitude: 181}, at: at},
	} {
		t.Run(tc.field, func(t *testing.T) {
			_, err := New(tc.user, tc.driver, tc.start, tc.end, tc.price, tc.at)
			wrapped := fmt.Errorf("create trip: %w", err)
			var validation *ValidationError
			if !errors.Is(wrapped, ErrInvalidTrip) || !errors.As(wrapped, &validation) {
				t.Fatalf("validation identity lost: %v", wrapped)
			}
			if validation.Field != tc.field || validation.Reason == "" || validation.Error() == "" {
				t.Fatalf("incorrect validation detail: %+v", validation)
			}
		})
	}
	value, err := New(id, id, Coordinates{}, Coordinates{}, 0, at)
	if err != nil {
		t.Fatal(err)
	}
	err = value.Finish(at.Add(-time.Second))
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Field != "finished_at" {
		t.Fatalf("finish validation detail: %v", err)
	}
	if value.Status() != StatusActive || value.FinishedAt() != nil {
		t.Fatal("invalid finish mutated trip")
	}
}
