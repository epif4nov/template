package telemetry

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// LogHandler adds correlation fields to every record.
type LogHandler struct{ slog.Handler }

func NewLogger(out io.Writer, level slog.Level) *slog.Logger {
	return slog.New(LogHandler{slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level})}).With("service", "trip-service")
}

func (h LogHandler) Handle(ctx context.Context, r slog.Record) error {
	sc := trace.SpanContextFromContext(ctx)
	traceID, spanID := "", ""
	if sc.IsValid() {
		traceID, spanID = sc.TraceID().String(), sc.SpanID().String()
	}
	r.AddAttrs(slog.String("trace_id", traceID), slog.String("span_id", spanID))
	return h.Handler.Handle(ctx, r)
}

func (h LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return LogHandler{h.Handler.WithAttrs(attrs)}
}

func (h LogHandler) WithGroup(name string) slog.Handler { return LogHandler{h.Handler.WithGroup(name)} }
