package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr              string
	HTTPReadTimeout       time.Duration
	HTTPReadHeaderTimeout time.Duration
	HTTPWriteTimeout      time.Duration
	HTTPIdleTimeout       time.Duration
	OTLPEndpoint          string
	OTLPProtocol          string
	TracesSampler         string
	LogLevel              string
	ShutdownTimeout       time.Duration

	DatabaseURL             string
	DatabaseMaxConns        int32
	DatabaseMinConns        int32
	DatabaseMaxConnLifetime time.Duration
	DatabaseConnectTimeout  time.Duration
	DatabaseQueryTimeout    time.Duration
}

func Load() (Config, error) {
	var cfg Config
	var err error

	if cfg.HTTPAddr, err = required("HTTP_ADDR"); err != nil {
		return cfg, err
	}

	if cfg.LogLevel, err = required("LOG_LEVEL"); err != nil {
		return cfg, err
	}

	if cfg.ShutdownTimeout, err = duration("SHUTDOWN_TIMEOUT"); err != nil {
		return cfg, err
	}

	if cfg.HTTPReadTimeout, err = duration("HTTP_READ_TIMEOUT"); err != nil {
		return cfg, err
	}

	if cfg.HTTPReadHeaderTimeout, err = duration("HTTP_READ_HEADER_TIMEOUT"); err != nil {
		return cfg, err
	}

	if cfg.HTTPWriteTimeout, err = duration("HTTP_WRITE_TIMEOUT"); err != nil {
		return cfg, err
	}

	if cfg.HTTPIdleTimeout, err = duration("HTTP_IDLE_TIMEOUT"); err != nil {
		return cfg, err
	}

	if cfg.DatabaseURL, err = required("DATABASE_URL"); err != nil {
		return cfg, err
	}

	if cfg.DatabaseMaxConns, err = integer("DATABASE_MAX_CONNS"); err != nil {
		return cfg, err
	}

	if cfg.DatabaseMinConns, err = integer("DATABASE_MIN_CONNS"); err != nil {
		return cfg, err
	}

	if cfg.DatabaseMaxConnLifetime, err = duration("DATABASE_MAX_CONN_LIFETIME"); err != nil {
		return cfg, err
	}

	if cfg.DatabaseConnectTimeout, err = duration("DATABASE_CONNECT_TIMEOUT"); err != nil {
		return cfg, err
	}

	if cfg.DatabaseQueryTimeout, err = duration("DATABASE_QUERY_TIMEOUT"); err != nil {
		return cfg, err
	}

	if cfg.DatabaseMaxConns < 1 ||
		cfg.DatabaseMinConns < 0 ||
		cfg.DatabaseMinConns > cfg.DatabaseMaxConns {
		return cfg, fmt.Errorf("invalid database pool settings")
	}

	if cfg.OTLPEndpoint, err = required("OTEL_EXPORTER_OTLP_ENDPOINT"); err != nil {
		return cfg, err
	}
	if cfg.OTLPProtocol, err = required("OTEL_EXPORTER_OTLP_PROTOCOL"); err != nil {
		return cfg, err
	}
	if cfg.TracesSampler, err = required("OTEL_TRACES_SAMPLER"); err != nil {
		return cfg, err
	}
	switch cfg.TracesSampler {
	case "always_on", "always_off", "traceidratio", "parentbased_always_on", "parentbased_always_off", "parentbased_traceidratio":
	default:
		return cfg, fmt.Errorf("unsupported OTEL_TRACES_SAMPLER: %s", cfg.TracesSampler)
	}
	return cfg, nil
}

func required(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("missing environment variable: %s", key)
	}
	return value, nil
}

func duration(key string) (time.Duration, error) {
	value, err := required(key)
	if err != nil {
		return 0, err
	}

	result, err := time.ParseDuration(value)
	if err != nil || result <= 0 {
		return 0, fmt.Errorf("invalid duration for %s", key)
	}

	return result, nil
}

func integer(key string) (int32, error) {
	value, err := required(key)
	if err != nil {
		return 0, err
	}

	result, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %w", key, err)
	}

	return int32(result), nil
}
