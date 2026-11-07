package response

import (
	domain "github.com/epif4nov/template/internal/domain/trip"
	api "github.com/epif4nov/template/internal/generated"
)

func Trip(t *domain.Trip) api.Trip {
	return api.Trip{Id: t.ID(), UserId: t.UserID(), DriverId: t.DriverID(), StartPoint: api.Coordinates{Latitude: t.StartPoint().Latitude, Longitude: t.StartPoint().Longitude}, EndPoint: api.Coordinates{Latitude: t.EndPoint().Latitude, Longitude: t.EndPoint().Longitude}, Price: t.Price(), Status: api.TripStatus(t.Status()), StartedAt: t.StartedAt(), FinishedAt: t.FinishedAt()}
}
