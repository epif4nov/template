package telemetry

import (
	"context"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestPositionMetricsAndSpanLink(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder), sdktrace.WithSampler(sdktrace.AlwaysSample()))
	defer func() { _ = tp.Shutdown(context.Background()); _ = mp.Shutdown(context.Background()) }()
	p, err := NewPositions(mp, tp, func() int { return 3 })
	if err != nil {
		t.Fatal(err)
	}
	ctx, scan := p.StartScan(context.Background())
	link := scan.SpanContext()
	p.StaleCheck(ctx, "queued")
	p.StaleCheck(ctx, "dropped")
	scan.End()
	workerCtx, worker := p.StartTask(ctx, link)
	p.WorkerActive(workerCtx, 1)
	p.RequestResult(workerCtx, nil, 10*time.Millisecond)
	p.RequestDropped(workerCtx)
	p.Saved(workerCtx)
	_, push := p.StartPush(workerCtx)
	push.End()
	worker.End()

	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			names[metric.Name] = true
		}
	}
	for _, name := range []string{
		"tripgo.stale_checks", "tripgo.position_requests.queue.size", "tripgo.position_requests.workers.active",
		"tripgo.position_requests", "tripgo.position_requests.duration", "tripgo.positions.saved",
	} {
		if !names[name] {
			t.Errorf("missing metric %s", name)
		}
	}

	spans := map[string]sdktrace.ReadOnlySpan{}
	for _, span := range recorder.Ended() {
		spans[span.Name()] = span
	}
	if spans["positions.worker"] == nil || spans["push.request_position"] == nil {
		t.Fatal("worker or push span missing")
	}
	if spans["positions.worker"].Parent().IsValid() {
		t.Fatal("worker span must be a root")
	}
	links := spans["positions.worker"].Links()
	if len(links) != 1 || links[0].SpanContext.SpanID() != link.SpanID() {
		t.Fatal("worker span link missing")
	}
	if spans["push.request_position"].Parent().SpanID() != spans["positions.worker"].SpanContext().SpanID() {
		t.Fatal("push span is not child of worker")
	}
}
