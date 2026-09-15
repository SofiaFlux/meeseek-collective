package domain

import "errors"

var (
	ErrStaleAttempt   = errors.New("stale attempt")
	ErrLeaseInactive  = errors.New("lease inactive")
	ErrPolicyDenied   = errors.New("policy denied")
	ErrIntentConflict = errors.New("intent conflict")
	ErrBudgetExceeded = errors.New("budget exceeded")
	ErrOutcomeUnknown = errors.New("external operation outcome unknown")
)
