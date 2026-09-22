package domain

import "time"

type GenericTaskClass string

const (
	GenericTaskDebugging     GenericTaskClass = "DEBUGGING"
	GenericTaskReview        GenericTaskClass = "REVIEW"
	GenericTaskRefactor      GenericTaskClass = "REFACTOR"
	GenericTaskDocumentation GenericTaskClass = "DOCUMENTATION"
	GenericTaskMigration     GenericTaskClass = "MIGRATION"
	GenericTaskTesting       GenericTaskClass = "TESTING"
	GenericTaskMaintenance   GenericTaskClass = "MAINTENANCE"
	GenericTaskOther         GenericTaskClass = "OTHER"
)

func (c GenericTaskClass) Valid() bool {
	switch c {
	case GenericTaskDebugging, GenericTaskReview, GenericTaskRefactor, GenericTaskDocumentation,
		GenericTaskMigration, GenericTaskTesting, GenericTaskMaintenance, GenericTaskOther:
		return true
	default:
		return false
	}
}

type FieldObservation struct {
	ID              ID
	CollectiveID    ID
	TaskID          ID
	AttemptID       ID
	OperationID     ID
	Category        string
	BasisClass      string
	SourceKind      string
	SummaryLocal    string
	MetricsJSON     string
	RuntimeVersion  string
	ExecutorKind    string
	ExecutorVersion string
	Enforcement     EnforcementLevel
	CreatedAt       time.Time
}

type FeedbackCandidate struct {
	ID                  ID
	State               FeedbackCandidateState
	GenericTaskClass    GenericTaskClass
	Category            string
	ExpectedBehavior    string
	ObservedBehavior    string
	StateTransitionJSON string
	MetricsJSON         string
	HumanIntervention   bool
	RecoveryResult      string
	RuntimeVersion      string
	ExecutorKind        string
	ExecutorVersion     string
	Enforcement         EnforcementLevel
	CorrelationKey      string
	CreatedAt           time.Time
}

type SanitizationResult struct {
	ID               ID
	CandidateID      ID
	SanitizerVersion string
	RulesetHash      string
	InputDigest      string
	Outcome          SanitizationOutcome
	ReasonCodesJSON  string
	CreatedAt        time.Time
}

type SanitizedFeedback struct {
	ID                   ID
	CandidateID          ID
	SchemaVersion        int
	ContentJSON          string
	ContentHash          string
	SanitizationResultID ID
	CorrelationKey       string
	Fingerprint          string
	CreatedAt            time.Time
}
