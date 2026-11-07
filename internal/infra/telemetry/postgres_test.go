package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestPostgresSignals(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
		if err := mp.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	tracer, err := NewPostgres(tp, mp)
	if err != nil {
		t.Fatal(err)
	}
	ctx, parent := tp.Tracer("test").Start(t.Context(), "HTTP")
	for _, failure := range []error{nil, &pgconn.PgError{Code: "23505", Message: "private value"}, errors.New("connection failed")} {
		queryCtx := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "INSERT INTO trips (id) VALUES ($1)", Args: []any{"private uuid"}})
		tracer.TraceQueryEnd(queryCtx, nil, pgx.TraceQueryEndData{Err: failure})
	}
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 4 {
		t.Fatal(len(spans))
	}
	for i, span := range spans[:3] {
		if span.Parent().SpanID() != parent.SpanContext().SpanID() || span.SpanContext().TraceID() != parent.SpanContext().TraceID() {
			t.Fatal("database span is not a child")
		}
		attrs := attribute.NewSet(span.Attributes()...)
		op, _ := attrs.Value("db.operation.name")
		table, _ := attrs.Value("db.collection.name")
		if span.Name() != "INSERT" || op.AsString() != "INSERT" || table.AsString() != "trips" {
			t.Fatal(span)
		}
		if i > 0 && span.Status().Code != codes.Error {
			t.Fatal("missing error")
		}
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	instruments := 0
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			instruments++
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if m.Name != "db.client.operation.duration" || m.Unit != "s" || !ok || len(hist.DataPoints) != 3 {
				t.Fatal(m)
			}
			for _, p := range hist.DataPoints {
				if p.Attributes.Len() != 3 || p.Count != 1 {
					t.Fatal(p)
				}
				if _, ok := p.Attributes.Value("error.type"); !ok {
					t.Fatal("missing error.type")
				}
			}
		}
	}
	if instruments != 1 {
		t.Fatal(instruments)
	}
}

func TestLoggerOutsideSpan(t *testing.T) {
	var out bytes.Buffer
	logger := NewLogger(&out, 0)
	logger.InfoContext(t.Context(), "starting")
	var record map[string]any
	if err := json.Unmarshal(out.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"time", "level", "msg", "service", "trace_id", "span_id"} {
		if _, ok := record[key]; !ok {
			t.Fatalf("missing %s", key)
		}
	}
	if record["trace_id"] != "" || record["span_id"] != "" {
		t.Fatal(record)
	}
}
