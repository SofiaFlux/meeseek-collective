package domain

import "time"

type ApprovalDecisionRecord struct {
	ApproverID     ID
	RequestDigest  string
	DecisionAction string
	DecidedAt      time.Time
}

type ApprovalRequestRecord struct {
	ID                ID
	SubjectKind       string
	SubjectID         ID
	RequestDigest     string
	PolicyDecisionID  ID
	RequiredApprovers []ID
	RequestedBy       ID
	State             ApprovalState
	ExpiresAt         time.Time
	DecidedAt         time.Time
	ApproverID        ID
	DecisionAction    string
	Decisions         []ApprovalDecisionRecord
	CreatedAt         time.Time
}
