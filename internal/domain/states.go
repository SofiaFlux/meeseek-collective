package domain

type TaskState string

const (
	TaskCreated              TaskState = "CREATED"
	TaskEligible             TaskState = "ELIGIBLE"
	TaskExecuting            TaskState = "EXECUTING"
	TaskAwaitingVerification TaskState = "AWAITING_VERIFICATION"
	TaskSucceeded            TaskState = "SUCCEEDED"
	TaskFailed               TaskState = "FAILED"
	TaskBlocked              TaskState = "BLOCKED"
	TaskCancelled            TaskState = "CANCELLED"
	TaskChallenged           TaskState = "CHALLENGED"
	TaskExpired              TaskState = "EXPIRED"
)

type AttemptState string

const (
	AttemptLeased    AttemptState = "LEASED"
	AttemptRunning   AttemptState = "RUNNING"
	AttemptCompleted AttemptState = "COMPLETED"
	AttemptFailed    AttemptState = "FAILED"
	AttemptCancelled AttemptState = "CANCELLED"
	AttemptExpired   AttemptState = "EXPIRED"
)

type LeaseState string

const (
	LeaseActive  LeaseState = "ACTIVE"
	LeaseRevoked LeaseState = "REVOKED"
	LeaseExpired LeaseState = "EXPIRED"
)

type OperationState string

const (
	OperationPrepared          OperationState = "PREPARED"
	OperationDispatched        OperationState = "DISPATCHED"
	OperationConfirmedEffect   OperationState = "CONFIRMED_EFFECT"
	OperationConfirmedNoEffect OperationState = "CONFIRMED_NO_EFFECT"
	OperationOutcomeUnknown    OperationState = "OUTCOME_UNKNOWN"
	OperationCancelled         OperationState = "CANCELLED"
)

func (s OperationState) Terminal() bool {
	switch s {
	case OperationConfirmedEffect, OperationConfirmedNoEffect, OperationCancelled:
		return true
	default:
		return false
	}
}

type EnforcementLevel string

const (
	EnforcementEnforced   EnforcementLevel = "ENFORCED"
	EnforcementPartial    EnforcementLevel = "PARTIAL"
	EnforcementUnenforced EnforcementLevel = "UNENFORCED"
)

type PolicyOutcome string

const (
	PolicyAllow           PolicyOutcome = "ALLOW"
	PolicyAllowWithLimit  PolicyOutcome = "ALLOW_WITH_LIMIT"
	PolicyRequireApproval PolicyOutcome = "REQUIRE_APPROVAL"
	PolicyDeny            PolicyOutcome = "DENY"
)

type ReservationState string

const (
	ReservationHeld       ReservationState = "HELD"
	ReservationSettled    ReservationState = "SETTLED"
	ReservationReleased   ReservationState = "RELEASED"
	ReservationUnresolved ReservationState = "UNRESOLVED"
)

type ClaimStatus string

const (
	ClaimSpeculation ClaimStatus = "SPECULATION"
	ClaimHypothesis  ClaimStatus = "HYPOTHESIS"
	ClaimSupported   ClaimStatus = "SUPPORTED"
	ClaimVerified    ClaimStatus = "VERIFIED"
)

type ChallengeScope string

const (
	ChallengeTask              ChallengeScope = "TASK"
	ChallengeParent            ChallengeScope = "PARENT"
	ChallengeGoal              ChallengeScope = "GOAL"
	ChallengeMissionAssumption ChallengeScope = "MISSION_ASSUMPTION"
)

type FailureClass string

const (
	FailureTransient           FailureClass = "TRANSIENT"
	FailureCapability          FailureClass = "CAPABILITY"
	FailureEpistemic           FailureClass = "EPISTEMIC"
	FailurePlanning            FailureClass = "PLANNING"
	FailureResource            FailureClass = "RESOURCE"
	FailureAuthority           FailureClass = "AUTHORITY"
	FailureObjectiveImpossible FailureClass = "OBJECTIVE_IMPOSSIBLE"
	FailureExecution           FailureClass = "EXECUTION"
)
