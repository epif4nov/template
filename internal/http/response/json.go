package response

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	api "github.com/epif4nov/template/internal/generated"
)

func DecodeTripData(w http.ResponseWriter, r *http.Request) (api.TripData, error) {
	var input api.TripData

	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return input, errors.New("Content-Type must be application/json")
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return input, fmt.Errorf("read request body: %w", err)
	}

	if err := validateRequiredFields(body); err != nil {
		return input, err
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		return input, fmt.Errorf("invalid JSON: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return input, errors.New("request body must contain one JSON object")
	}

	return input, nil
}

func validateRequiredFields(body []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if fields == nil {
		return errors.New("request body must be an object")
	}

	for _, name := range []string{
		"user_id", "driver_id", "start_point", "end_point", "price",
	} {
		if err := require(fields, name); err != nil {
			return err
		}
	}

	for _, name := range []string{"start_point", "end_point"} {
		var coordinates map[string]json.RawMessage
		if err := json.Unmarshal(fields[name], &coordinates); err != nil {
			return fmt.Errorf("%s must be an object: %w", name, err)
		}
		if coordinates == nil {
			return fmt.Errorf("%s must be an object", name)
		}

		for _, coordinate := range []string{"latitude", "longitude"} {
			if err := require(coordinates, coordinate); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}

	return nil
}

func require(fields map[string]json.RawMessage, name string) error {
	value, ok := fields[name]
	if !ok || strings.TrimSpace(string(value)) == "null" {
		return fmt.Errorf("required field %s is missing or null", name)
	}
	return nil
}

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func Problem(
	w http.ResponseWriter,
	status int32,
	code, title, detail string,
) {
	problemType := "https://tripgo.example/problems/" +
		strings.ReplaceAll(code, "_", "-")

	problem := api.Problem{
		Type:   problemType,
		Title:  title,
		Status: status,
		Code:   code,
		Detail: &detail,
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(int(status))
	_ = json.NewEncoder(w).Encode(problem)
}
