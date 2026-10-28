package telemetry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOTLPHTTPUsesSignalPaths(t *testing.T) {
	paths := make(chan string, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	providers, err := New(t.Context(), server.URL+"/otel", "http/protobuf", "test", NewLogger(io.Discard, 0))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := providers.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()
	_, span := providers.Traces.Tracer("test").Start(t.Context(), "request")
	span.End()
	counter, err := providers.Metrics.Meter("test").Int64Counter("test.count")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(t.Context(), 1)
	if err := providers.Traces.ForceFlush(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := providers.Metrics.ForceFlush(t.Context()); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for range 2 {
		select {
		case path := <-paths:
			seen[path] = true
		case <-t.Context().Done():
			t.Fatal("no OTLP request")
		}
	}
	if !seen["/otel/v1/traces"] || !seen["/otel/v1/metrics"] {
		t.Fatal(seen)
	}
}
