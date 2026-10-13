package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	api "github.com/epif4nov/template/internal/generated"
	"github.com/epif4nov/template/internal/repository"
	"github.com/epif4nov/template/internal/service"
)

type Handler struct {
	api.Unimplemented

	service      *service.Service
	pool         *pgxpool.Pool
	queryTimeout time.Duration
	logger       *slog.Logger
}

func New(
	svc *service.Service,
	pool *pgxpool.Pool,
	queryTimeout time.Duration,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		service:      svc,
		pool:         pool,
		queryTimeout: queryTimeout,
		logger:       logger,
	}
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.HealthResponse{
		Status: api.Ok,
	})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{
			Status: api.Unavailable,
		})
		return
	}

	writeJSON(w, http.StatusOK, api.HealthResponse{
		Status: api.Ok,
	})
}

func (h *Handler) CreateTrip(
	w http.ResponseWriter,
	r *http.Request,
	_ api.CreateTripParams,
) {
	input, err := decodeTripData(w, r)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest,
			"invalid_request", "Invalid request", err.Error())
		return
	}

	trip := repository.Trip{
		UserID:         uuid.UUID(input.UserId),
		DriverID:       uuid.UUID(input.DriverId),
		StartLatitude:  input.StartPoint.Latitude,
		StartLongitude: input.StartPoint.Longitude,
		EndLatitude:    input.EndPoint.Latitude,
		EndLongitude:   input.EndPoint.Longitude,
		Price:          input.Price,
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	created, err := h.service.CreateTrip(ctx, trip)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/v1/trips/"+created.ID.String())
	writeJSON(w, http.StatusCreated, toAPITrip(created))
}

func (h *Handler) GetTrip(
	w http.ResponseWriter,
	r *http.Request,
	tripID api.TripId,
) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	trip, err := h.service.GetTrip(ctx, uuid.UUID(tripID))
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toAPITrip(trip))
}

func (h *Handler) FinishTrip(
	w http.ResponseWriter,
	r *http.Request,
	tripID api.TripId,
) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	trip, err := h.service.FinishTrip(ctx, uuid.UUID(tripID))
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toAPITrip(trip))
}

func (h *Handler) InvalidParameter(
	w http.ResponseWriter,
	r *http.Request,
	err error,
) {
	writeProblem(w, r, http.StatusBadRequest,
		"invalid_request", "Invalid request", err.Error())
}

func (h *Handler) handleError(
	w http.ResponseWriter,
	r *http.Request,
	err error,
) {
	switch {
	case errors.Is(err, service.ErrInvalidTrip):
		writeProblem(w, r, http.StatusBadRequest,
			"invalid_request", "Invalid request", "Invalid trip data")

	case errors.Is(err, repository.ErrTripNotFound):
		writeProblem(w, r, http.StatusNotFound,
			"trip_not_found", "Trip not found", "Trip was not found")

	case errors.Is(err, repository.ErrDriverBusy):
		writeProblem(w, r, http.StatusConflict,
			"driver_busy", "Driver busy",
			"Driver already has an active trip")

	case errors.Is(err, repository.ErrTripCompleted):
		writeProblem(w, r, http.StatusConflict,
			"trip_completed", "Trip completed",
			"Operation is not allowed for a completed trip")

	default:
		h.logger.Error("trip request failed", "error", err)
		writeProblem(w, r, http.StatusInternalServerError,
			"internal_error", "Internal Server Error",
			"Internal server error")
	}
}

func toAPITrip(trip repository.Trip) api.Trip {
	return api.Trip{
		Id:       trip.ID,
		UserId:   trip.UserID,
		DriverId: trip.DriverID,
		StartPoint: api.Coordinates{
			Latitude:  trip.StartLatitude,
			Longitude: trip.StartLongitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  trip.EndLatitude,
			Longitude: trip.EndLongitude,
		},
		Price:      trip.Price,
		Status:     api.TripStatus(trip.Status),
		StartedAt:  trip.StartedAt,
		FinishedAt: trip.FinishedAt,
	}
}
