package domain

import "time"

type ResourceEnvelope struct {
	ID        ID
	ParentID  ID
	HardLimit int64
	CreatedAt time.Time
}

type Reservation struct {
	ID         ID
	EnvelopeID ID
	State      ReservationState
	Amount     int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
