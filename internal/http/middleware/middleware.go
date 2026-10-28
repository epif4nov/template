// Package middleware implements the request pipeline: Recover, Tracing, Logging,
// Metrics, then the chi router.
package middleware

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/epif4nov/template/internal/http/response"
)

type Middleware struct {
	logger   *slog.Logger
	traces   trace.TracerProvider
	duration metric.Float64Histogram
	active   metric.Int64UpDownCounter
}

func New(logger *slog.Logger, tp trace.TracerProvider, mp metric.MeterProvider) (*Middleware, error) {
	meter := mp.Meter("tripgo/http")
	duration, err := meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10))
	if err != nil {
		return nil, err
	}
	active, err := meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}"))
	if err != nil {
		return nil, err
	}
	return &Middleware{logger, tp, duration, active}, nil
}

// responseBuffer lets Recover replace a partial response without writing twice.
// TripGo has bounded JSON responses and no streaming endpoints.
type responseBuffer struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *responseBuffer) Header() http.Header { return w.header }
func (w *responseBuffer) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}

func (w *responseBuffer) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(b)
}

func (w *responseBuffer) code() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

type requestState struct {
	response *responseBuffer
	route    string
	ctx      context.Context
	stack    string
}
type stateKey struct{}

func state(r *http.Request) *requestState { return r.Context().Value(stateKey{}).(*requestState) }

func (m *Middleware) Wrap(router chi.Router) http.Handler {
	return m.Recover(router, m.Tracing(m.Logging(m.Metrics(router))))
}

func (m *Middleware) Recover(router chi.Router, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Match in a separate context so active_requests has a template before the
		// handler starts. The real routing context is updated by chi during dispatch.
		match := chi.NewRouteContext()
		route := "unmatched"
		if router.Match(match, r.Method, r.URL.Path) {
			route = match.RoutePattern()
		}
		out := &responseBuffer{header: make(http.Header)}
		s := &requestState{response: out, route: route, ctx: r.Context()}
		ctx := context.WithValue(r.Context(), chi.RouteCtxKey, chi.NewRouteContext())
		r = r.WithContext(context.WithValue(ctx, stateKey{}, s))
		defer func() {
			if value := recover(); value != nil {
				m.logger.ErrorContext(s.ctx, "request panic", "error", fmt.Sprint(value), "stack", s.stack)
				out.header = make(http.Header)
				out.body.Reset()
				out.status = 0
				response.Problem(out, 500, "internal_error", "Internal Server Error", "Internal server error")
			}
			for key, values := range out.header {
				w.Header()[key] = values
			}
			w.WriteHeader(out.code())
			if _, err := w.Write(out.body.Bytes()); err != nil {
				m.logger.ErrorContext(s.ctx, "write response failed", "error", err)
			}
		}()
		next.ServeHTTP(out, r)
	})
}

func (m *Middleware) Tracing(next http.Handler) http.Handler {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := state(r)
		s.ctx = r.Context()
		defer func() {
			span := trace.SpanFromContext(r.Context())
			// RoutePattern is complete only after chi has dispatched the request.
			if route := chi.RouteContext(r.Context()).RoutePattern(); route != "" {
				s.route = route
			}
			span.SetName(requestMethod(r.Method) + " " + s.route)
			span.SetAttributes(attribute.String("http.route", s.route), attribute.Int("http.response.status_code", s.response.code()))
			if s.response.code() >= 400 {
				span.SetStatus(codes.Error, http.StatusText(s.response.code()))
				span.SetAttributes(attribute.String("error.type", strconv.Itoa(s.response.code())))
			}
		}()
		next.ServeHTTP(w, r)
	})
	return otelhttp.NewHandler(handler, "HTTP",
		otelhttp.WithTracerProvider(m.traces),
		// Own HTTP instruments guarantee the exact lab labels and avoid duplicates.
		otelhttp.WithMeterProvider(noop.NewMeterProvider()),
		otelhttp.WithPropagators(propagation.TraceContext{}),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return requestMethod(r.Method) + " " + state(r).route
		}),
	)
}

func (m *Middleware) Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			s := state(r)
			level := slog.LevelInfo
			if s.response.code() >= 500 {
				level = slog.LevelError
			}
			m.logger.Log(r.Context(), level, "request handled",
				"method", r.Method, "route", s.route, "status", s.response.code(),
				"duration_ms", float64(time.Since(start).Microseconds())/1000)
		}()
		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := state(r)
		attrs := []attribute.KeyValue{attribute.String("http.request.method", requestMethod(r.Method)), attribute.String("http.route", s.route)}
		opts := metric.WithAttributes(attrs...)
		m.active.Add(r.Context(), 1, opts)
		start := time.Now()
		defer func() {
			value := recover()
			if value != nil {
				s.stack = string(debug.Stack())
				s.response.status = http.StatusInternalServerError
			}
			m.active.Add(r.Context(), -1, opts)
			attrs = append(attrs, attribute.Int("http.response.status_code", s.response.code()))
			m.duration.Record(r.Context(), time.Since(start).Seconds(), metric.WithAttributes(attrs...))
			if value != nil {
				panic(value)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func requestMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodHead, http.MethodOptions, http.MethodConnect, http.MethodTrace:
		return method
	default:
		return "_OTHER"
	}
}
