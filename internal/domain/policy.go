package domain

import "time"

type PolicyDecision struct {
	ID                     ID
	Outcome                PolicyOutcome
	Limits                 map[string]any
	RequiredApprovals      []ID
	ReasonCodes            []string
	PolicySetID            ID
	PolicySetHash          string
	PolicyCapabilitiesHash string
	InputDigest            string
	EvaluatedAt            time.Time
}
