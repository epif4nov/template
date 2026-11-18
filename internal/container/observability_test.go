package container_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/epif4nov/template/internal/http/middleware"
	"github.com/epif4nov/template/internal/infra/telemetry"
)

const tripRoute = "/api/v1/trips/{tripId}"

func TestRequestObservability(t *testing.T) {
	for _, status := range []int{200, 400, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var logs bytes.Buffer
			logger := telemetry.NewLogger(&logs, 0)
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			defer func() {
				if err := mp.Shutdown(t.Context()); err != nil {
					t.Error(err)
				}
			}()
			recorder := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			defer func() {
				if err := tp.Shutdown(t.Context()); err != nil {
					t.Error(err)
				}
			}()
			mw, err := middleware.New(logger, tp, mp)
			if err != nil {
				t.Fatal(err)
			}
			router := chi.NewRouter()
			router.Get(tripRoute, func(w http.ResponseWriter, r *http.Request) {
				var current metricdata.ResourceMetrics
				if err := reader.Collect(r.Context(), &current); err != nil {
					t.Fatal(err)
				}
				assertActive(t, current, 1)
				if status == 500 {
					_, _ = w.Write([]byte("secret partial response"))
					panic("private database failure")
				}
				w.WriteHeader(status)
			})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/5cb72c04-1234-4321-8123-123456789012", nil)
			req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
			out := httptest.NewRecorder()
			mw.Wrap(router).ServeHTTP(out, req)
			if out.Code != status {
				t.Fatalf("status = %d", out.Code)
			}
			if status == 500 {
				if out.Header().Get("Content-Type") != "application/problem+json" ||
					!strings.Contains(out.Body.String(), "internal_error") ||
					strings.Contains(out.Body.String(), "private") ||
					strings.Contains(out.Body.String(), "secret") {
					t.Fatalf("unsafe response: %s", out.Body.String())
				}
			}
			spans := recorder.Ended()
			if len(spans) != 1 {
				t.Fatalf("spans = %d", len(spans))
			}
			span := spans[0]
			if span.Name() != "GET /api/v1/trips/{tripId}" {
				t.Fatal(span.Name())
			}
			if span.Parent().SpanID().String() != "00f067aa0ba902b7" {
				t.Fatal("W3C parent lost")
			}
			if status >= 400 && span.Status().Code != codes.Error {
				t.Fatal("error status lost")
			}
			if getAttribute(span.Attributes(), "http.route").AsString() != tripRoute {
				t.Fatal(span.Attributes())
			}
			if status >= 400 && getAttribute(span.Attributes(), "error.type").AsString() == "" {
				t.Fatal("error.type missing")
			}
			assertLogs(t, &logs, span, status)
			var data metricdata.ResourceMetrics
			if err := reader.Collect(context.Background(), &data); err != nil {
				t.Fatal(err)
			}
			assertActive(t, data, 0)
			assertDuration(t, data, status)
		})
	}
}

func getAttribute(attrs []attribute.KeyValue, name string) attribute.Value {
	for _, a := range attrs {
		if string(a.Key) == name {
			return a.Value
		}
	}
	return attribute.Value{}
}

func assertActive(t *testing.T, data metricdata.ResourceMetrics, want int64) {
	t.Helper()
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "http.server.active_requests" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok || sum.IsMonotonic || len(sum.DataPoints) != 1 {
				t.Fatal(m)
			}
			p := sum.DataPoints[0]
			route, _ := p.Attributes.Value(attribute.Key("http.route"))
			if p.Value != want || route.AsString() != tripRoute || p.Attributes.Len() != 2 {
				t.Fatal(p)
			}
			return
		}
	}
	t.Fatal("active_requests missing")
}

func assertLogs(t *testing.T, logs *bytes.Buffer, span sdktrace.ReadOnlySpan, status int) {
	count := 0
	dec := json.NewDecoder(logs)
	for dec.More() {
		var record map[string]any
		if err := dec.Decode(&record); err != nil {
			t.Fatal(err)
		}
		if record["trace_id"] != span.SpanContext().TraceID().String() || record["span_id"] != span.SpanContext().SpanID().String() {
			t.Fatal(record)
		}
		if record["msg"] == "request handled" {
			count++
			if record["route"] != tripRoute || record["status"] != float64(status) {
				t.Fatal(record)
			}
			expectedLevel := "INFO"
			if status == 500 {
				expectedLevel = "ERROR"
			}
			if record["level"] != expectedLevel {
				t.Fatal(record)
			}
		}
	}
	if count != 1 {
		t.Fatalf("request logs = %d", count)
	}
}

func assertDuration(t *testing.T, data metricdata.ResourceMetrics, status int) {
	histograms := 0
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "http.server.request.duration" {
				continue
			}
			histograms++
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if !ok || m.Unit != "s" || len(hist.DataPoints) != 1 {
				t.Fatal(m)
			}
			point := hist.DataPoints[0]
			route, _ := point.Attributes.Value(attribute.Key("http.route"))
			code, _ := point.Attributes.Value(attribute.Key("http.response.status_code"))
			if point.Count != 1 || route.AsString() != tripRoute || code.AsInt64() != int64(status) || point.Attributes.Len() != 3 {
				t.Fatal(point)
			}
		}
	}
	if histograms != 1 {
		t.Fatalf("duration instruments = %d", histograms)
	}
}
