package container

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/epif4nov/template/internal/config"
	pushgateway "github.com/epif4nov/template/internal/gateway/push"
	api "github.com/epif4nov/template/internal/generated"
	"github.com/epif4nov/template/internal/http/middleware"
	"github.com/epif4nov/template/internal/http/response"
	createhandler "github.com/epif4nov/template/internal/http/trip/create"
	finishhandler "github.com/epif4nov/template/internal/http/trip/finish"
	gethandler "github.com/epif4nov/template/internal/http/trip/get"
	listpositionhandler "github.com/epif4nov/template/internal/http/trip/positions/list"
	savepositionhandler "github.com/epif4nov/template/internal/http/trip/positions/save"
	"github.com/epif4nov/template/internal/infra/postgres/transaction"
	"github.com/epif4nov/template/internal/infra/telemetry"
	repository "github.com/epif4nov/template/internal/repository/trip"
	"github.com/epif4nov/template/internal/usecase/trip/create"
	"github.com/epif4nov/template/internal/usecase/trip/finish"
	"github.com/epif4nov/template/internal/usecase/trip/get"
	listposition "github.com/epif4nov/template/internal/usecase/trip/positions/list"
	saveposition "github.com/epif4nov/template/internal/usecase/trip/positions/save"
	workerpositions "github.com/epif4nov/template/internal/worker/positions"
)

type Container struct {
	pool            *pgxpool.Pool
	business        *telemetry.Business
	Handler         http.Handler
	positionService *workerpositions.Service
	pushGateway     *pushgateway.Gateway
}

func (c *Container) Close() {
	if c.business != nil {
		c.business.UnregisterActiveTrips()
	}

	if c.pushGateway != nil {
		c.pushGateway.Close()
	}
	c.pool.Close()
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger, signals *telemetry.Providers) (*Container, error) {
	pgTracer, err := telemetry.NewPostgres(signals.Traces, signals.Metrics)
	if err != nil {
		return nil, err
	}
	business, err := telemetry.NewBusiness(signals.Metrics, logger)
	if err != nil {
		return nil, err
	}
	mw, err := middleware.New(logger, signals.Traces, signals.Metrics)
	if err != nil {
		return nil, err
	}
	// Настраиваем пул соединений PostgreSQL.
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}

	poolCfg.ConnConfig.Tracer = pgTracer
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

	if err := business.RegisterActiveTrips(
		signals.Metrics,
		func(ctx context.Context) (int64, error) {
			queryCtx, cancel := context.WithTimeout(
				ctx,
				cfg.DatabaseQueryTimeout,
			)
			defer cancel()

			var count int64

			err := pool.QueryRow(
				queryCtx,
				`SELECT COUNT(*)
				FROM trips
				WHERE status = 'active'`,
			).Scan(&count)

			return count, err
		},
	); err != nil {
		pool.Close()
		return nil, fmt.Errorf(
			"register active trips metric: %w", err,
		)
	}

	repo := repository.New(pool)
	tx := transaction.NewManager(pool, cfg.DatabaseQueryTimeout)
	now := func() time.Time { return time.Now().UTC() }

	positionQueue, err := workerpositions.NewQueue(
		cfg.PositionQueueSize,
	)
	if err != nil {
		business.UnregisterActiveTrips()
		pool.Close()
		return nil, fmt.Errorf("create position queue: %w", err)
	}

	pushGateway, err := pushgateway.New(
		cfg.PushHTTPURL,
		cfg.PushRequestTimeout,
	)
	if err != nil {
		business.UnregisterActiveTrips()
		pool.Close()
		return nil, fmt.Errorf("create push gateway: %w", err)
	}

	workerPool, err := workerpositions.NewPool(
		positionQueue,
		pushGateway,
		logger,
		cfg.PositionWorkers,
		cfg.PushRequestTimeout,
	)
	if err != nil {
		pushGateway.Close()
		business.UnregisterActiveTrips()
		pool.Close()
		return nil, fmt.Errorf("create worker pool: %w", err)
	}

	scanner, err := workerpositions.NewScanner(
		repo,
		positionQueue,
		logger,
		cfg.PositionCheckInterval,
		cfg.PositionStaleAfter,
		cfg.DatabaseQueryTimeout,
	)
	if err != nil {
		pushGateway.Close()
		business.UnregisterActiveTrips()
		pool.Close()
		return nil, fmt.Errorf("create scanner: %w", err)
	}

	positionSignals, err := telemetry.NewPositions(signals.Metrics, signals.Traces, positionQueue.Len)
	if err != nil {
		pushGateway.Close()
		business.UnregisterActiveTrips()
		pool.Close()
		return nil, fmt.Errorf("create position telemetry: %w", err)
	}
	workerPool.SetSignals(positionSignals)
	scanner.SetSignals(positionSignals)

	positionService := workerpositions.NewService(
		scanner,
		positionQueue,
		workerPool,
	)

	savePositionUseCase := saveposition.New(repo, tx, now)
	savePositionUseCase.SetOnSaved(func(ctx context.Context, id uuid.UUID) {
		positionQueue.Cancel(id)
		positionSignals.Saved(ctx)
	})

	handler := &handlers{
		create: createhandler.New(telemetry.Create{Next: create.New(repo, tx, now), Signals: business}, cfg.DatabaseQueryTimeout, logger),
		get:    gethandler.New(get.New(repo), cfg.DatabaseQueryTimeout, logger),
		finish: finishhandler.New(telemetry.Finish{Next: finish.New(repo, tx, now), Signals: business}, cfg.DatabaseQueryTimeout, logger),
		pool:   pool, timeout: cfg.DatabaseQueryTimeout,
		savePosition: savepositionhandler.New(
			savePositionUseCase,
			cfg.DatabaseQueryTimeout,
			logger,
		),

		listPositions: listpositionhandler.New(
			listposition.New(repo),
			cfg.DatabaseQueryTimeout,
			logger,
		),
	}
	router := chi.NewRouter()
	api.HandlerWithOptions(handler, api.ChiServerOptions{BaseRouter: router, ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
		response.Problem(w, 400, "invalid_request", "Invalid request", err.Error())
	}})
	return &Container{
		pool:            pool,
		business:        business,
		Handler:         mw.Wrap(router),
		positionService: positionService,
		pushGateway:     pushGateway,
	}, nil
}

type handlers struct {
	api.Unimplemented
	create        *createhandler.Handler
	get           *gethandler.Handler
	finish        *finishhandler.Handler
	pool          interface{ Ping(context.Context) error }
	timeout       time.Duration
	savePosition  *savepositionhandler.Handler
	listPositions *listpositionhandler.Handler
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

func (h *handlers) CreateTripPosition(
	w http.ResponseWriter,
	r *http.Request,
	id api.TripId,
) {
	h.savePosition.Handle(w, r, id)
}

func (h *handlers) ListTripPositions(
	w http.ResponseWriter,
	r *http.Request,
	id api.TripId,
) {
	h.listPositions.Handle(w, r, id)
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

func (c *Container) Start(ctx context.Context) {
	c.positionService.Start(ctx)
}

func (c *Container) Shutdown(ctx context.Context) error {
	return c.positionService.Shutdown(ctx)
}
