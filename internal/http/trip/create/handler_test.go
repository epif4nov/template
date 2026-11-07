package create_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	handler "github.com/epif4nov/template/internal/http/trip/create"
	usecase "github.com/epif4nov/template/internal/usecase/trip/create"
)

func TestHandle(t *testing.T) {
	value, err := trip.New(uuid.New(), uuid.New(), trip.Coordinates{Latitude: 10, Longitude: 20}, trip.Coordinates{Latitude: 30, Longitude: 40}, 500, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{{"success", nil, 201, ""}, {"driver busy", trip.ErrDriverBusy, 409, "driver_busy"}, {"invalid data", trip.ErrInvalidTrip, 400, "invalid_request"}, {"database failure", errors.New("secret SQL password"), 500, "internal_error"}} {
		t.Run(tc.name, func(t *testing.T) {
			mock := NewMockuseCase(gomock.NewController(t))
			mock.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, input usecase.Input) (*trip.Trip, error) {
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("missing query timeout")
				}
				if input.UserID != value.UserID() || input.DriverID != value.DriverID() || input.Price != 500 || input.StartPoint.Latitude != 10 || input.EndPoint.Longitude != 40 {
					t.Fatalf("incorrect input: %+v", input)
				}
				return value, tc.err
			})
			h := handler.New(mock, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
			r := httptest.NewRequest("POST", "/api/v1/trips", strings.NewReader(fmt.Sprintf(`{"user_id":%q,"driver_id":%q,"start_point":{"latitude":10,"longitude":20},"end_point":{"latitude":30,"longitude":40},"price":500}`, value.UserID(), value.DriverID())))
			r.Header.Set("Content-Type", jsonContentType)
			w := httptest.NewRecorder()
			h.Handle(w, r, api.CreateTripParams{})
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
			if w.Header().Get("Location") != "/api/v1/trips/"+value.ID().String() {
				t.Fatal("missing Location")
			}
		})
	}
}

func TestHandleInvalidBody(t *testing.T) {
	valid := `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5","start_point":{"latitude":0,"longitude":0},"end_point":{"latitude":0,"longitude":0},"price":0}`
	for _, tc := range []struct{ name, body, contentType string }{
		{"malformed", "{", jsonContentType},
		{"missing fields", "{}", jsonContentType},
		{"null", "null", jsonContentType},
		{"null price", strings.Replace(valid, `"price":0`, `"price":null`, 1), jsonContentType},
		{"missing coordinate", strings.Replace(valid, `"latitude":0,`, "", 1), jsonContentType},
		{"invalid UUID", strings.Replace(valid, "5cb72c04-7650-45c9-a79b-bcdba0631e0c", "bad", 1), jsonContentType},
		{"unknown field", strings.Replace(valid, `"price":0`, `"price":0,"unknown":1`, 1), jsonContentType},
		{"extra JSON", valid + " {}", jsonContentType},
		{"content type", valid, "text/plain"},
		{"too large", strings.Repeat(" ", 1<<20) + valid, jsonContentType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := NewMockuseCase(gomock.NewController(t))
			h := handler.New(mock, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
			r := httptest.NewRequest("POST", "/api/v1/trips", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			h.Handle(w, r, api.CreateTripParams{})
			var problem api.Problem
			if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if w.Code != 400 || problem.Code != "invalid_request" {
				t.Fatalf("status %d body %s", w.Code, w.Body.String())
			}
		})
	}
}

const jsonContentType = "application/json"
