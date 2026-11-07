//go:build integration

package trip_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/epif4nov/template/internal/config"
	"github.com/epif4nov/template/internal/container"
	"github.com/epif4nov/template/internal/infra/telemetry"
)

func TestHTTPPostgresTraceIncludesTransaction(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder), sdktrace.WithSampler(sdktrace.AlwaysSample()))
	mp := sdkmetric.NewMeterProvider()
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
		if err := mp.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	cfg := config.Config{
		DatabaseURL: os.Getenv("DATABASE_URL"), DatabaseMaxConns: 2,
		DatabaseConnectTimeout: 5 * time.Second, DatabaseQueryTimeout: 5 * time.Second, DatabaseMaxConnLifetime: time.Minute,
	}
	app, err := container.New(t.Context(), cfg, telemetry.NewLogger(io.Discard, 0), &telemetry.Providers{Traces: tp, Metrics: mp})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	driver := uuid.New()
	cleanupPool := openTestPool(t)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := cleanupPool.Exec(ctx, "DELETE FROM trips WHERE driver_id = $1", driver); err != nil {
			t.Error(err)
		}
	}()
	body := fmt.Sprintf(`{"user_id":%q,"driver_id":%q,"start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":100}`, uuid.New(), driver)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	out := httptest.NewRecorder()
	app.Handler.ServeHTTP(out, req)
	if out.Code != 201 {
		t.Fatalf("create: %d %s", out.Code, out.Body)
	}
	spans := recorder.Ended()
	var httpSpan sdktrace.ReadOnlySpan
	for _, s := range spans {
		if s.Name() == "POST /api/v1/trips" {
			httpSpan = s
		}
	}
	if httpSpan == nil {
		t.Fatal("HTTP span missing")
	}
	operations := map[string]bool{}
	tables := map[string]bool{}
	for _, s := range spans {
		if s.Parent().SpanID() != httpSpan.SpanContext().SpanID() {
			continue
		}
		attrs := attribute.NewSet(s.Attributes()...)
		op, _ := attrs.Value("db.operation.name")
		table, _ := attrs.Value("db.collection.name")
		operations[op.AsString()] = true
		tables[table.AsString()] = true
	}
	for _, op := range []string{"BEGIN", "INSERT", "COMMIT"} {
		if !operations[op] {
			t.Fatalf("missing %s: %v", op, operations)
		}
	}
	for _, table := range []string{"trips", "trip_status_history"} {
		if !tables[table] {
			t.Fatalf("missing %s: %v", table, tables)
		}
	}
	finish := httptest.NewRecorder()
	app.Handler.ServeHTTP(finish, httptest.NewRequest(http.MethodPost, out.Header().Get("Location")+"/finish", nil))
	if finish.Code != 200 {
		t.Fatalf("finish: %d %s", finish.Code, finish.Body)
	}
}
