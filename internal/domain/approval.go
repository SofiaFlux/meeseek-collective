package domain

import "time"

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
	CreatedAt         time.Time
}
