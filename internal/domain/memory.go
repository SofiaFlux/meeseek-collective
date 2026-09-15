package domain

import "time"

type EvidenceRef struct {
	ID          ID
	ContentHash string
	MediaType   string
	SizeBytes   int64
	CreatedAt   time.Time
}

type Event struct {
	ID        ID
	Kind      string
	SubjectID ID
	CreatedAt time.Time
}

type Claim struct {
	ID         ID
	Statement  string
	Status     ClaimStatus
	Confidence float64
	ValidFrom  time.Time
	ValidTo    time.Time
	CreatedAt  time.Time
}
