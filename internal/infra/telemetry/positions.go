package telemetry

import (
	"context"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Positions records the bounded set of signals required by homework 03.
type Positions struct {
	stale    metric.Int64Counter
	requests metric.Int64Counter
	duration metric.Float64Histogram
	saved    metric.Int64Counter
	active   atomic.Int64
	tracer   trace.Tracer
}

func NewPositions(mp metric.MeterProvider, tp trace.TracerProvider, queueLen func() int) (*Positions, error) {
	m := mp.Meter("tripgo/positions")
	stale, err := m.Int64Counter("tripgo.stale_checks", metric.WithUnit("{check}"))
	if err != nil {
		return nil, err
	}
	requests, err := m.Int64Counter("tripgo.position_requests", metric.WithUnit("{request}"))
	if err != nil {
		return nil, err
	}
	duration, err := m.Float64Histogram("tripgo.position_requests.duration", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	saved, err := m.Int64Counter("tripgo.positions.saved", metric.WithUnit("{position}"))
	if err != nil {
		return nil, err
	}
	queueSize, err := m.Int64ObservableGauge("tripgo.position_requests.queue.size", metric.WithUnit("{task}"))
	if err != nil {
		return nil, err
	}
	workersActive, err := m.Int64ObservableGauge("tripgo.position_requests.workers.active", metric.WithUnit("{worker}"))
	if err != nil {
		return nil, err
	}
	p := &Positions{stale: stale, requests: requests, duration: duration, saved: saved, tracer: tp.Tracer("tripgo/positions")}
	_, err = m.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		observer.ObserveInt64(queueSize, int64(queueLen()))
		observer.ObserveInt64(workersActive, p.active.Load())
		return nil
	}, queueSize, workersActive)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Positions) Saved(ctx context.Context) { p.saved.Add(ctx, 1) }

func (p *Positions) StaleCheck(ctx context.Context, result string) {
	p.stale.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func (p *Positions) WorkerActive(_ context.Context, delta int64) { p.active.Add(delta) }

func (p *Positions) RequestDropped(ctx context.Context) {
	p.requests.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "dropped")))
}

func (p *Positions) RequestResult(ctx context.Context, requestErr error, elapsed time.Duration) {
	result := "success"
	if requestErr != nil {
		result = "error"
	}
	opts := metric.WithAttributes(attribute.String("result", result))
	p.requests.Add(ctx, 1, opts)
	p.duration.Record(ctx, elapsed.Seconds(), opts)
}

func (p *Positions) StartScan(ctx context.Context) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, "positions.scan")
}

func (p *Positions) StartTask(ctx context.Context, link trace.SpanContext) (context.Context, trace.Span) {
	// A queued task is a new root. Its causal scan is represented by a link.
	if link.IsValid() {
		return p.tracer.Start(ctx, "positions.worker", trace.WithNewRoot(), trace.WithLinks(trace.Link{SpanContext: link}))
	}
	return p.tracer.Start(ctx, "positions.worker", trace.WithNewRoot())
}

func (p *Positions) StartPush(ctx context.Context) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, "push.request_position", trace.WithSpanKind(trace.SpanKindClient))
}
