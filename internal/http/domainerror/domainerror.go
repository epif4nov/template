package domainerror

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/epif4nov/template/internal/domain/trip"
	api "github.com/epif4nov/template/internal/generated"
)

func Problem(err error) (int, api.Problem) {
	status, code, title, detail := 500, "internal_error", "Internal Server Error", "Internal server error"
	var validation *trip.ValidationError
	switch {
	case errors.As(err, &validation):
		status, code, title, detail = 400, "invalid_request", "Invalid request", validation.Error()
	case errors.Is(err, trip.ErrInvalidTrip):
		status, code, title, detail = 400, "invalid_request", "Invalid request", "Invalid trip data"
	case errors.Is(err, trip.ErrTripNotFound):
		status, code, title, detail = 404, "trip_not_found", "Trip not found", "Trip was not found"
	case errors.Is(err, trip.ErrDriverBusy):
		status, code, title, detail = 409, "driver_busy", "Driver busy", "Driver already has an active trip"
	case errors.Is(err, trip.ErrTripCompleted):
		status, code, title, detail = 409, "trip_completed", "Trip completed", "Operation is not allowed for a completed trip"
	}
	return status, api.Problem{Type: "https://tripgo.example/problems/" + strings.ReplaceAll(code, "_", "-"), Title: title, Status: int32(status), Code: code, Detail: &detail}
}

func Write(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	status, body := Problem(err)
	if status == 500 {
		logger.ErrorContext(r.Context(), "trip request failed", "error", err)
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
