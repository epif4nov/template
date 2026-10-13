package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	api "github.com/epif4nov/template/internal/generated"
	"github.com/epif4nov/template/internal/httpapi"
	"github.com/epif4nov/template/internal/repository"
	"github.com/epif4nov/template/internal/service"
	"github.com/epif4nov/template/internal/transaction"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/epif4nov/template/internal/config"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// 1. Загружаем конфигурацию.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// 2. Настраиваем логирование.
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(
		os.Stdout,
		&slog.HandlerOptions{Level: level},
	)).With("service", "trip-service")

	// 3. Настраиваем пул соединений PostgreSQL.
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse database config: %w", err)
	}

	poolCfg.MaxConns = cfg.DatabaseMaxConns
	poolCfg.MinConns = cfg.DatabaseMinConns
	poolCfg.MaxConnLifetime = cfg.DatabaseMaxConnLifetime
	poolCfg.ConnConfig.ConnectTimeout = cfg.DatabaseConnectTimeout

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return fmt.Errorf("create database pool: %w", err)
	}
	defer pool.Close()

	// Проверяем доступность базы при запуске.
	pingCtx, cancelPing := context.WithTimeout(
		context.Background(),
		cfg.DatabaseConnectTimeout,
	)
	err = pool.Ping(pingCtx)
	cancelPing()

	if err != nil {
		return fmt.Errorf("database ping: %w", err)
	}

	logger.Info("database connected")

	// 4. Создаём сервис и подключаем HTTP-обработчики.
	repo := repository.New(pool)
	txManager := transaction.NewManager(pool, cfg.DatabaseQueryTimeout)
	tripService := service.New(repo, txManager)

	handler := httpapi.New(
		tripService,
		pool,
		cfg.DatabaseQueryTimeout,
		logger,
	)

	router := chi.NewRouter()

	api.HandlerWithOptions(handler, api.ChiServerOptions{
		BaseRouter: router,
		ErrorHandlerFunc: func(
			w http.ResponseWriter,
			r *http.Request,
			err error,
		) {
			handler.InvalidParameter(w, r, err)
		},
	})

	// 5. Настраиваем HTTP-сервер.
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

	// 6. Обрабатываем сигналы остановки.
	signalCtx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	serverErrors := make(chan error, 1)

	go func() {
		logger.Info("HTTP server starting", "address", cfg.HTTPAddr)

		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	select {
	case <-signalCtx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErrors:
		return fmt.Errorf("HTTP server: %w", err)
	}

	// 7. Graceful shutdown.
	shutdownCtx, cancelShutdown := context.WithTimeout(
		context.Background(),
		cfg.ShutdownTimeout,
	)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("server shutdown: %w", err)
	}

	logger.Info("HTTP server stopped")
	return nil
}
