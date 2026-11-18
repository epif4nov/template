package save

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	api "github.com/epif4nov/template/internal/generated"
	"github.com/epif4nov/template/internal/http/domainerror"
	"github.com/epif4nov/template/internal/http/response"
	usecase "github.com/epif4nov/template/internal/usecase/trip/positions/save"
)

type Handler struct {
	uc      *usecase.UseCase
	timeout time.Duration
	logger  *slog.Logger
}

func New(
	uc *usecase.UseCase,
	timeout time.Duration,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		uc:      uc,
		timeout: timeout,
		logger:  logger,
	}
}

func (h *Handler) Handle(
	w http.ResponseWriter,
	r *http.Request,
	tripID api.TripId,
) {
	input, err := response.DecodePositionData(w, r)
	if err != nil {
		response.Problem(
			w, 400, "invalid_request",
			"Invalid request", err.Error(),
		)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(), h.timeout,
	)
	defer cancel()

	position, err := h.uc.Execute(ctx, usecase.Input{
		TripID:     tripID,
		Latitude:   input.Coordinates.Latitude,
		Longitude:  input.Coordinates.Longitude,
		RecordedAt: input.RecordedAt,
	})
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}

	response.JSON(w, http.StatusCreated, api.TripPosition{
		Id:     position.ID,
		TripId: position.TripID,
		Coordinates: api.Coordinates{
			Latitude:  position.Latitude,
			Longitude: position.Longitude,
		},
		RecordedAt: position.RecordedAt,
	})
}
