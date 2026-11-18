package trip

import (
	"time"

	"github.com/google/uuid"
)

type StaleTrip struct {
	ID         uuid.UUID
	DriverID   uuid.UUID
	LastSeenAt time.Time
}
