package list

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	api "github.com/epif4nov/template/internal/generated"
	"github.com/epif4nov/template/internal/http/domainerror"
	"github.com/epif4nov/template/internal/http/response"
	usecase "github.com/epif4nov/template/internal/usecase/trip/positions/list"
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
	return &Handler{uc: uc, timeout: timeout, logger: logger}
}

func (h *Handler) Handle(
	w http.ResponseWriter,
	r *http.Request,
	tripID api.TripId,
) {
	ctx, cancel := context.WithTimeout(
		r.Context(), h.timeout,
	)
	defer cancel()

	positions, err := h.uc.Execute(ctx, tripID)
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}

	result := make([]api.TripPosition, 0, len(positions))

	for _, p := range positions {
		result = append(result, api.TripPosition{
			Id:     p.ID,
			TripId: p.TripID,
			Coordinates: api.Coordinates{
				Latitude:  p.Coordinates.Latitude,
				Longitude: p.Coordinates.Longitude,
			},
			RecordedAt: p.RecordedAt,
		})
	}

	response.JSON(w, http.StatusOK,
		api.TripPositionsResponse{Positions: result},
	)
}
