package finish_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/epif4nov/template/internal/domain/trip"
	api "github.com/epif4nov/template/internal/generated"
	handler "github.com/epif4nov/template/internal/http/trip/finish"
)

func TestHandle(t *testing.T) {
	value, err := trip.New(uuid.New(), uuid.New(), trip.Coordinates{Latitude: 10, Longitude: 20}, trip.Coordinates{Latitude: 30, Longitude: 40}, 500, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Finish(value.StartedAt().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{{"success", nil, 200, ""}, {"not found", trip.ErrTripNotFound, 404, "trip_not_found"}, {"invalid ID", trip.ErrInvalidTrip, 400, "invalid_request"}, {"database failure", errors.New("secret SQL password"), 500, "internal_error"}, {"already completed", trip.ErrTripCompleted, 409, "trip_completed"}} {
		t.Run(tc.name, func(t *testing.T) {
			mock := NewMockuseCase(gomock.NewController(t))
			mock.EXPECT().Execute(gomock.Any(), value.ID()).DoAndReturn(func(ctx context.Context, _ uuid.UUID) (*trip.Trip, error) {
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("missing query timeout")
				}
				return value, tc.err
			})
			h := handler.New(mock, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
			r := httptest.NewRequest("GET", "/api/v1/trips/"+value.ID().String(), nil)
			w := httptest.NewRecorder()
			h.Handle(w, r, value.ID())
			if w.Code != tc.status {
				t.Fatalf("status %d, body %s", w.Code, w.Body.String())
			}
			if tc.code != "" {
				var problem api.Problem
				if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
					t.Fatal(err)
				}
				if problem.Code != tc.code || w.Header().Get("Content-Type") != "application/problem+json" {
					t.Fatalf("unexpected problem: %+v", problem)
				}
				if strings.Contains(w.Body.String(), "secret") {
					t.Fatal("internal details leaked")
				}
				return
			}
			var got api.Trip
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Id != value.ID() || got.UserId != value.UserID() || got.DriverId != value.DriverID() || got.Price != value.Price() || got.StartPoint.Latitude != 10 || got.EndPoint.Longitude != 40 || got.Status != api.TripStatus(value.Status()) || !got.StartedAt.Equal(value.StartedAt()) {
				t.Fatalf("incorrect response: %+v", got)
			}
			if got.FinishedAt == nil || !got.FinishedAt.Equal(*value.FinishedAt()) {
				t.Fatal("lost finish time")
			}
		})
	}
}
