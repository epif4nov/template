package telemetry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

// Postgres enriches otelpgx at the pool boundary. Its built-in metric is disabled
// because it lacks collection and error attributes required by the lab.
type Postgres struct {
	*otelpgx.Tracer
	duration metric.Float64Histogram
}
type (
	queryKey  struct{}
	queryData struct {
		start                 time.Time
		operation, collection string
	}
)

func NewPostgres(tp trace.TracerProvider, mp metric.MeterProvider) (*Postgres, error) {
	h, err := mp.Meter("tripgo/postgres").Float64Histogram("db.client.operation.duration", metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(.001, .005, .01, .05, .1, .5, 1, 5, 10))
	if err != nil {
		return nil, err
	}
	return &Postgres{Tracer: otelpgx.NewTracer(
		otelpgx.WithTracerProvider(tp), otelpgx.WithMeterProvider(noop.NewMeterProvider()),
		otelpgx.WithDisableSQLStatementInAttributes(), otelpgx.WithDisableConnectionDetailsInAttributes(),
		otelpgx.WithDisableAcquireTracer(),
	), duration: h}, nil
}

func queryLabels(sql string) (string, string) {
	fields := strings.Fields(strings.NewReplacer("(", " ", ")", " ", "\"", "").Replace(strings.ToLower(sql)))
	op := "OTHER"
	if len(fields) > 0 {
		switch fields[0] {
		case "select", "insert", "update", "delete", "begin", "commit", "rollback":
			op = strings.ToUpper(fields[0])
		}
	}
	// Only known schema identifiers are allowed; SQL values never become labels.
	collection := ""
	for _, f := range fields {
		if f == "trips" || f == "trip_status_history" {
			collection = f
			break
		}
	}
	return op, collection
}

func (p *Postgres) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = p.Tracer.TraceQueryStart(ctx, conn, data)
	operation, collection := queryLabels(data.SQL)
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("db.operation.name", operation), attribute.String("db.collection.name", collection))
	return context.WithValue(ctx, queryKey{}, queryData{time.Now(), operation, collection})
}

func (p *Postgres) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	q, _ := ctx.Value(queryKey{}).(queryData)
	errorType := ""
	if data.Err != nil {
		errorType = fmt.Sprintf("%T", data.Err)
		var pgErr *pgconn.PgError
		if errors.As(data.Err, &pgErr) {
			errorType = pgErr.Code
		}
		span := trace.SpanFromContext(ctx)
		span.SetStatus(codes.Error, "database operation failed")
		span.SetAttributes(attribute.String("error.type", errorType))
	}
	p.duration.Record(ctx, time.Since(q.start).Seconds(), metric.WithAttributes(
		attribute.String("db.operation.name", q.operation),
		attribute.String("db.collection.name", q.collection),
		attribute.String("error.type", errorType),
	))
	p.Tracer.TraceQueryEnd(ctx, conn, data)
}
