package domain

import "time"

type Attempt struct {
	ID              ID
	TaskID          ID
	State           AttemptState
	FenceGeneration int64
	LeaseState      LeaseState
	LeaseExpiresAt  time.Time
	ExecutorKind    string
	StartedAt       time.Time
	CompletedAt     time.Time
}
