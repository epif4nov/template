package container

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/epif4nov/template/internal/config"
	api "github.com/epif4nov/template/internal/generated"
	"github.com/epif4nov/template/internal/http/response"
	createhandler "github.com/epif4nov/template/internal/http/trip/create"
	finishhandler "github.com/epif4nov/template/internal/http/trip/finish"
	gethandler "github.com/epif4nov/template/internal/http/trip/get"
	"github.com/epif4nov/template/internal/infra/postgres/transaction"
	repository "github.com/epif4nov/template/internal/repository/trip"
	"github.com/epif4nov/template/internal/usecase/trip/create"
	"github.com/epif4nov/template/internal/usecase/trip/finish"
	"github.com/epif4nov/template/internal/usecase/trip/get"
)

type Container struct {
	pool    *pgxpool.Pool
	Handler http.Handler
}

func (c *Container) Close() { c.pool.Close() }

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Container, error) {
	// Настраиваем пул соединений PostgreSQL.
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}

	poolCfg.MaxConns = cfg.DatabaseMaxConns
	poolCfg.MinConns = cfg.DatabaseMinConns
	poolCfg.MaxConnLifetime = cfg.DatabaseMaxConnLifetime
	poolCfg.ConnConfig.ConnectTimeout = cfg.DatabaseConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}

	// Проверяем доступность базы при запуске.
	pingCtx, cancelPing := context.WithTimeout(
		ctx,
		cfg.DatabaseConnectTimeout,
	)
	err = pool.Ping(pingCtx)
	cancelPing()

	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping: %w", err)
	}

	logger.Info("database connected")

	repo := repository.New(pool)
	tx := transaction.NewManager(pool, cfg.DatabaseQueryTimeout)
	now := func() time.Time { return time.Now().UTC() }
	handler := &handlers{
		create: createhandler.New(create.New(repo, tx, now), cfg.DatabaseQueryTimeout, logger),
		get:    gethandler.New(get.New(repo), cfg.DatabaseQueryTimeout, logger),
		finish: finishhandler.New(finish.New(repo, tx, now), cfg.DatabaseQueryTimeout, logger),
		pool:   pool, timeout: cfg.DatabaseQueryTimeout,
	}
	router := chi.NewRouter()
	api.HandlerWithOptions(handler, api.ChiServerOptions{BaseRouter: router, ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
		response.Problem(w, r, 400, "invalid_request", "Invalid request", err.Error())
	}})
	return &Container{pool: pool, Handler: router}, nil
}

type handlers struct {
	api.Unimplemented
	create  *createhandler.Handler
	get     *gethandler.Handler
	finish  *finishhandler.Handler
	pool    interface{ Ping(context.Context) error }
	timeout time.Duration
}

func (h *handlers) CreateTrip(w http.ResponseWriter, r *http.Request, p api.CreateTripParams) {
	h.create.Handle(w, r, p)
}

func (h *handlers) GetTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	h.get.Handle(w, r, id)
}

func (h *handlers) FinishTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	h.finish.Handle(w, r, id)
}

func (h *handlers) Health(w http.ResponseWriter, _ *http.Request) {
	response.JSON(w, 200, api.HealthResponse{Status: api.Ok})
}

func (h *handlers) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		response.JSON(w, 503, api.HealthResponse{Status: api.Unavailable})
		return
	}
	response.JSON(w, 200, api.HealthResponse{Status: api.Ok})
}
