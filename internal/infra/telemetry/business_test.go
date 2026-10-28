package telemetry

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	"github.com/epif4nov/template/internal/domain/trip"
	"github.com/epif4nov/template/internal/usecase/trip/create"
)

//go:generate go tool mockgen -source=business.go -destination=mocks_test.go -package=telemetry

func TestBusinessCountsOnlyCommittedSuccess(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() {
		if err := mp.Shutdown(t.Context()); err != nil {
			t.Error(err)
		}
	}()
	b, err := NewBusiness(mp, NewLogger(io.Discard, 0))
	if err != nil {
		t.Fatal(err)
	}
	value, err := trip.New(uuid.New(), uuid.New(), trip.Coordinates{Latitude: 1, Longitude: 2}, trip.Coordinates{Latitude: 3, Longitude: 4}, 100, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ctrl := gomock.NewController(t)
	c, f := NewMockCreateUseCase(ctrl), NewMockFinishUseCase(ctrl)
	failure := errors.New("commit failed")
	gomock.InOrder(
		c.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, failure),
		f.EXPECT().Execute(gomock.Any(), value.ID()).Return(nil, failure),
		c.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(value, nil),
		f.EXPECT().Execute(gomock.Any(), value.ID()).Return(value, nil),
	)
	for i := range 2 {
		_, createErr := (Create{c, b}).Execute(t.Context(), create.Input{})
		_, finishErr := (Finish{f, b}).Execute(t.Context(), value.ID())
		if i == 0 && (createErr == nil || finishErr == nil) {
			t.Fatal("errors lost")
		}
		if i == 1 && (createErr != nil || finishErr != nil) {
			t.Fatal(createErr, finishErr)
		}
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok || !sum.IsMonotonic || len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 1 || sum.DataPoints[0].Attributes.Len() != 0 {
				t.Fatal(m)
			}
			count++
		}
	}
	if count != 2 {
		t.Fatal(count)
	}
}
