package domain

import "time"

type AdaptationGrant struct {
	ID                         ID
	RequestID                  ID
	Kind                       string
	ScopeKey                   string
	AllowedExecutors           []string
	MinVerifiedSamples         int
	MaxAcceptanceRegressionBps int
	MaxCostRegressionBps       int
	OwnerPrincipalID           ID
	ExpiresAt                  time.Time
	ActivatedAt                time.Time
}

type ExperienceProposal struct {
	ID                     ID
	GrantID                ID
	GenericTaskClass       GenericTaskClass
	ScopeKey               string
	PreferredExecutor      string
	EvidenceObservationIDs []ID
	State                  ExperienceRuleState
	CreatedAt              time.Time
}

type ExperienceRule struct {
	ID                ID
	ProposalID        ID
	GrantID           ID
	Version           int
	AdaptationKind    string
	ScopeKey          string
	GenericTaskClass  GenericTaskClass
	PreferredExecutor string
	State             ExperienceRuleState
	VerifiedSamples   int
	ActivatedAt       time.Time
	RolledBackAt      time.Time
	CreatedAt         time.Time
}
