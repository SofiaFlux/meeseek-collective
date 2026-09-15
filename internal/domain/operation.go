package domain

import "time"

type ExternalOperation struct {
	ID                ID
	TaskID            ID
	AttemptID         ID
	EffectSlotID      ID
	State             OperationState
	IntentFingerprint string
	IntentRevision    int64
	Provider          string
	ProviderReference string
	ReservationID     ID
	CreatedAt         time.Time
	DispatchedAt      time.Time
	SettledAt         time.Time
}
