package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Gateway struct {
	client   *http.Client
	endpoint string
}

type requestPayload struct {
	RequestID   string            `json:"request_id"`
	RecipientID string            `json:"recipient_id"`
	Kind        string            `json:"kind"`
	Data        map[string]string `json:"data"`
}

// StatusError означает, что Push Service ответил
// HTTP-кодом, отличным от 202.
type StatusError struct {
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf(
		"push service returned HTTP %d",
		e.StatusCode,
	)
}

// TransportError означает ошибку сети, таймаут или отмену HTTP-запроса.
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("push transport: %v", e.Err)
}

func (e *TransportError) Unwrap() error {
	return e.Err
}

func New(baseURL string, timeout time.Duration) (*Gateway, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("push timeout must be positive")
	}

	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("parse push URL: %w", err)
	}

	if (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return nil, fmt.Errorf("invalid push HTTP URL")
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: timeout,
		}).DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		IdleConnTimeout:       90 * time.Second,
	}

	return &Gateway{
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
		endpoint: strings.TrimRight(
			parsed.String(), "/",
		) + "/api/v1/push",
	}, nil
}

func (g *Gateway) RequestPosition(
	ctx context.Context,
	driverID uuid.UUID,
	tripID uuid.UUID,
) error {
	payload := requestPayload{
		RequestID:   uuid.NewString(),
		RecipientID: driverID.String(),
		Kind:        "REQUEST_POSITION",
		Data: map[string]string{
			"trip_id": tripID.String(),
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode push request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		g.endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("create push request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return &TransportError{Err: err}
	}
	defer resp.Body.Close()

	// Ограниченно дочитываем ответ для повторного
	// использования HTTP-соединения.
	_, _ = io.Copy(
		io.Discard,
		io.LimitReader(resp.Body, 64<<10),
	)

	if resp.StatusCode != http.StatusAccepted {
		return &StatusError{
			StatusCode: resp.StatusCode,
		}
	}

	return nil
}

// Закрываем простаивающие HTTP-соединения.
func (g *Gateway) Close() {
	g.client.CloseIdleConnections()
}
