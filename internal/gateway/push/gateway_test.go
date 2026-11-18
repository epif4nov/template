package push

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRequestPosition(t *testing.T) {
	received := make(chan requestPayload, 1)

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/push" {
				http.Error(w, "wrong path", 404)
				return
			}
			if r.Method != http.MethodPost {
				http.Error(w, "wrong method", http.StatusMethodNotAllowed)
				return
			}
			if r.Header.Get("Content-Type") != "application/json" {
				http.Error(w, "wrong content type", 400)
				return
			}

			var payload requestPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "invalid json", 400)
				return
			}

			received <- payload
			w.WriteHeader(http.StatusAccepted)
		},
	))
	defer server.Close()

	gateway, err := New(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer gateway.Close()

	driverID := uuid.New()
	tripID := uuid.New()

	if err := gateway.RequestPosition(
		context.Background(), driverID, tripID,
	); err != nil {
		t.Fatal(err)
	}

	payload := <-received

	if payload.RequestID == "" {
		t.Fatal("request ID is empty")
	}
	if payload.RecipientID != driverID.String() {
		t.Fatal("incorrect recipient ID")
	}
	if payload.Kind != "REQUEST_POSITION" {
		t.Fatal("incorrect push kind")
	}
	if payload.Data["trip_id"] != tripID.String() {
		t.Fatal("incorrect trip ID")
	}
}

func TestPushHTTPStatusErrors(t *testing.T) {
	statuses := []int{400, 429, 503}

	for _, status := range statuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(status)
				},
			))
			defer server.Close()

			gateway, err := New(server.URL, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer gateway.Close()

			err = gateway.RequestPosition(
				context.Background(), uuid.New(), uuid.New(),
			)

			var statusErr *StatusError
			if !errors.As(err, &statusErr) {
				t.Fatalf("expected StatusError, got %v", err)
			}
			if statusErr.StatusCode != status {
				t.Fatalf(
					"status = %d, want %d",
					statusErr.StatusCode, status,
				)
			}
		})
	}
}

func TestPushTransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		},
	))

	gateway, err := New(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer gateway.Close()

	server.Close()

	err = gateway.RequestPosition(
		context.Background(), uuid.New(), uuid.New(),
	)

	var transportErr *TransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("expected TransportError, got %v", err)
	}
}

func TestPushRespectsContextDeadline(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	gateway, err := New(server.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer gateway.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err = gateway.RequestPosition(ctx, uuid.New(), uuid.New())
	var transportErr *TransportError
	if !errors.As(err, &transportErr) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline transport error, got %v", err)
	}
}
