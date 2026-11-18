package domainerror_test

import (
	"fmt"
	"testing"

	"github.com/epif4nov/template/internal/domain/trip"
	"github.com/epif4nov/template/internal/http/domainerror"
)

func TestProblemValidationDetails(t *testing.T) {
	err := fmt.Errorf("create trip: %w", &trip.ValidationError{Field: "price", Reason: "must be non-negative"})
	status, body := domainerror.Problem(err)
	if status != 400 || body.Code != "invalid_request" || body.Detail == nil || *body.Detail != "price: must be non-negative" {
		t.Fatalf("validation details lost: %d %+v", status, body)
	}
}
