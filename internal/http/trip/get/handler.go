package get

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	api "github.com/epif4nov/template/internal/generated"
	"github.com/epif4nov/template/internal/http/domainerror"
	"github.com/epif4nov/template/internal/http/response"
)

type Handler struct {
	uc      useCase
	timeout time.Duration
	logger  *slog.Logger
}

func New(uc useCase, timeout time.Duration, logger *slog.Logger) *Handler {
	return &Handler{uc: uc, timeout: timeout, logger: logger}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, id api.TripId) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	result, err := h.uc.Execute(ctx, id)
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}
	response.JSON(w, 200, response.Trip(result))
}
