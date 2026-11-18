package container_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/epif4nov/template/internal/http/middleware"
	"github.com/epif4nov/template/internal/infra/telemetry"
)

func TestHTTPWithUnavailableCollector(t *testing.T) {
	for _, protocol := range []string{"http/protobuf", "grpc"} {
		t.Run(protocol, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			endpoint := "http://" + listener.Addr().String()
			if err := listener.Close(); err != nil {
				t.Fatal(err)
			}
			logger := telemetry.NewLogger(io.Discard, 0)
			providers, err := telemetry.New(t.Context(), endpoint, protocol, "test", logger)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				_ = providers.Shutdown(ctx) // A disconnected exporter can fail to flush.
			}()
			mw, err := middleware.New(logger, providers.Traces, providers.Metrics)
			if err != nil {
				t.Fatal(err)
			}
			router := chi.NewRouter()
			router.Get("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
			h := mw.Wrap(router)
			check := func() {
				out := httptest.NewRecorder()
				h.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/health", nil))
				if out.Code != http.StatusOK {
					t.Fatal(out.Code)
				}
			}
			check()
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			if err := providers.Traces.ForceFlush(ctx); err == nil {
				t.Fatal("expected unavailable collector")
			}
			check()
		})
	}
}
