package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/epif4nov/template/internal/config"
	"github.com/epif4nov/template/internal/container"
	"github.com/epif4nov/template/internal/infra/telemetry"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		telemetry.NewLogger(os.Stdout, slog.LevelError).Error("service failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Загружаем конфигурацию.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Настраиваем логирование.
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}

	logger := telemetry.NewLogger(os.Stdout, level)
	signals, err := telemetry.New(context.Background(), cfg.OTLPEndpoint, cfg.OTLPProtocol, version, logger)
	if err != nil {
		return err
	}
	// Reuse one shutdown deadline for HTTP and telemetry.
	var shutdownDeadline time.Time
	defer func() {
		if shutdownDeadline.IsZero() {
			shutdownDeadline = time.Now().Add(cfg.ShutdownTimeout)
		}
		ctx, cancel := context.WithDeadline(context.Background(), shutdownDeadline)
		defer cancel()
		if err := signals.Shutdown(ctx); err != nil {
			logger.Error("telemetry shutdown failed", "error", err)
		}
	}()

	app, err := container.New(context.Background(), cfg, logger, signals)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	defer app.Close()
	// Настраиваем HTTP-сервер.
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           app.Handler,
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

	// Обрабатываем сигналы остановки.
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

	// Graceful shutdown.
	shutdownDeadline = time.Now().Add(cfg.ShutdownTimeout)
	shutdownCtx, cancelShutdown := context.WithDeadline(context.Background(), shutdownDeadline)

	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("server shutdown: %w", err)
	}

	logger.Info("HTTP server stopped")
	return nil
}
