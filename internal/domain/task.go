package domain

import "time"

type Task struct {
	ID                  ID
	ParentTaskID        ID
	PurposeID           ID
	State               TaskState
	CurrentAttemptID    ID
	CurrentFence        int64
	RequiredEnforcement EnforcementLevel
	ResourceEnvelopeID  ID
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
