package trip

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRestore(t *testing.T) {
	start := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	id, user, driver := uuid.New(), uuid.New(), uuid.New()
	for _, tc := range []struct {
		name     string
		id       uuid.UUID
		status   Status
		finished *time.Time
		invalid  bool
	}{
		{"active", id, StatusActive, nil, false}, {"completed", id, StatusCompleted, &end, false},
		{"nil ID", uuid.Nil, StatusActive, nil, true}, {"unknown status", id, "invalid", nil, true},
		{"active with finish", id, StatusActive, &end, true}, {"completed without finish", id, StatusCompleted, nil, true},
		{"finish before start", id, StatusCompleted, func() *time.Time { v := start.Add(-time.Minute); return &v }(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Restore(tc.id, user, driver, Coordinates{Latitude: 50}, Coordinates{Longitude: 60}, 150, tc.status, start, tc.finished)
			if tc.invalid {
				if !errors.Is(err, ErrInvalidTrip) {
					t.Fatalf("got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ID() != id || got.UserID() != user || got.DriverID() != driver || got.StartPoint().Latitude != 50 || got.EndPoint().Longitude != 60 || got.Price() != 150 || !got.StartedAt().Equal(start) || got.Status() != tc.status {
				t.Fatal("fields lost during restoration")
			}
			if tc.finished != nil {
				saved := *tc.finished
				*got.FinishedAt() = start
				*tc.finished = start
				if !got.FinishedAt().Equal(saved) {
					t.Fatal("finish time is mutable through pointer")
				}
				*tc.finished = saved
			}
		})
	}
}

func TestNewValidation(t *testing.T) {
	at := time.Now()
	id := uuid.New()
	for _, tc := range []struct {
		name         string
		user, driver uuid.UUID
		start, end   Coordinates
		price        int64
		at           time.Time
	}{
		{name: "user", driver: id, at: at}, {name: "driver", user: id, at: at},
		{name: "price", user: id, driver: id, price: -1, at: at},
		{name: "latitude", user: id, driver: id, start: Coordinates{Latitude: 91}, at: at},
		{name: "longitude", user: id, driver: id, end: Coordinates{Longitude: -181}, at: at},
		{name: "NaN", user: id, driver: id, start: Coordinates{Latitude: math.NaN()}, at: at},
		{name: "infinity", user: id, driver: id, end: Coordinates{Longitude: math.Inf(1)}, at: at},
		{name: "zero time", user: id, driver: id},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.user, tc.driver, tc.start, tc.end, tc.price, tc.at); !errors.Is(err, ErrInvalidTrip) {
				t.Fatalf("got %v", err)
			}
		})
	}
	value, err := New(id, id, Coordinates{Latitude: -90, Longitude: 180}, Coordinates{Latitude: 90, Longitude: -180}, 0, at)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []time.Time{{}, at.Add(-time.Second)} {
		if err := value.Finish(invalid); !errors.Is(err, ErrInvalidTrip) {
			t.Fatalf("got %v", err)
		}
		if value.Status() != StatusActive || value.FinishedAt() != nil {
			t.Fatal("invalid finish changed state")
		}
	}
}
