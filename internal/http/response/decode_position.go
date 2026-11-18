package response

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	api "github.com/epif4nov/template/internal/generated"
)

func DecodePositionData(
	w http.ResponseWriter,
	r *http.Request,
) (api.PositionData, error) {
	var input api.PositionData

	contentType, _, err := mime.ParseMediaType(
		r.Header.Get("Content-Type"),
	)
	if err != nil || contentType != "application/json" {
		return input, errors.New(
			"Content-Type must be application/json",
		)
	}

	body, err := io.ReadAll(
		http.MaxBytesReader(w, r.Body, 1<<20),
	)
	if err != nil {
		return input, fmt.Errorf("read body: %w", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return input, fmt.Errorf("invalid JSON: %w", err)
	}
	if fields == nil {
		return input, errors.New("body must be an object")
	}

	for _, field := range []string{
		"coordinates", "recorded_at",
	} {
		if err := require(fields, field); err != nil {
			return input, err
		}
	}

	var coordinates map[string]json.RawMessage
	if err := json.Unmarshal(
		fields["coordinates"], &coordinates,
	); err != nil {
		return input, fmt.Errorf("invalid coordinates: %w", err)
	}
	if coordinates == nil {
		return input, errors.New("coordinates must be an object")
	}

	for _, field := range []string{
		"latitude", "longitude",
	} {
		if err := require(coordinates, field); err != nil {
			return input, err
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		return input, fmt.Errorf("decode position: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return input, errors.New("expected one JSON object")
	}

	return input, nil
}
