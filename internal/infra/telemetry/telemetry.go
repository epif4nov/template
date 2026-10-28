package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Providers struct {
	Traces  *sdktrace.TracerProvider
	Metrics *sdkmetric.MeterProvider
}

// New uses the standard OTLP environment configuration. Export is asynchronous;
// no connection to the collector is required to serve business requests.
func New(ctx context.Context, endpoint, protocol, version string, logger *slog.Logger) (*Providers, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid OTEL_EXPORTER_OTLP_ENDPOINT")
	}
	res := resource.NewSchemaless(
		attribute.String("service.name", "trip-service"),
		attribute.String("service.namespace", "tripgo"),
		attribute.String("service.version", version),
		attribute.String("deployment.environment", "local"),
	)
	var te sdktrace.SpanExporter
	var me sdkmetric.Exporter
	switch protocol {
	case "http/protobuf":
		te, err = otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint), otlptracehttp.WithURLPath(strings.TrimRight(u.Path, "/")+"/v1/traces"))
		if err == nil {
			me, err = otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint), otlpmetrichttp.WithURLPath(strings.TrimRight(u.Path, "/")+"/v1/metrics"))
		}
	case "grpc":
		te, err = otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(endpoint))
		if err == nil {
			me, err = otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpointURL(endpoint))
		}
	default:
		return nil, fmt.Errorf("unsupported OTEL_EXPORTER_OTLP_PROTOCOL: %s", protocol)
	}
	if err != nil {
		if te != nil {
			err = errors.Join(err, te.Shutdown(ctx))
		}
		return nil, fmt.Errorf("initialize OTLP: %w", err)
	}
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.ErrorContext(ctx, "telemetry export failed", "error", err)
	}))
	// The SDK reads OTEL_TRACES_SAMPLER and OTEL_TRACES_SAMPLER_ARG.
	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(te))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(me)))
	return &Providers{Traces: tp, Metrics: mp}, nil
}

func (p *Providers) Shutdown(ctx context.Context) error {
	return errors.Join(p.Traces.Shutdown(ctx), p.Metrics.Shutdown(ctx))
}
