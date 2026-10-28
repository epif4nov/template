package create

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/epif4nov/template/internal/domain/trip"
	api "github.com/epif4nov/template/internal/generated"
	"github.com/epif4nov/template/internal/http/domainerror"
	"github.com/epif4nov/template/internal/http/response"
	usecase "github.com/epif4nov/template/internal/usecase/trip/create"
)

type Handler struct {
	uc      useCase
	timeout time.Duration
	logger  *slog.Logger
}

func New(uc useCase, timeout time.Duration, logger *slog.Logger) *Handler {
	return &Handler{uc: uc, timeout: timeout, logger: logger}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	input, err := response.DecodeTripData(w, r)
	if err != nil {
		response.Problem(w, 400, "invalid_request", "Invalid request", err.Error())
		return
	}
	value := usecase.Input{UserID: input.UserId, DriverID: input.DriverId, StartPoint: trip.Coordinates{Latitude: input.StartPoint.Latitude, Longitude: input.StartPoint.Longitude}, EndPoint: trip.Coordinates{Latitude: input.EndPoint.Latitude, Longitude: input.EndPoint.Longitude}, Price: input.Price}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	result, err := h.uc.Execute(ctx, value)
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}
	w.Header().Set("Location", "/api/v1/trips/"+result.ID().String())
	response.JSON(w, 201, response.Trip(result))
}
