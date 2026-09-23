// Package workflow validates proposed steps in a bounded work and assessment
// workflow.
//
// Decide is a pure proposal validator, not an authorization check or an
// executor. Callers must durably record each Assessment and the associated
// case state. Before any external operation, callers must recheck the current
// lease and fence, inherited grant, resource revision, and applicable policy.
// Callers must reconcile dispatches whose outcomes are unknown before retrying
// or advancing the case. A separate, independent verifier must determine the
// final case outcome.
//
// This package does not persist state, acquire authority, perform external
// operations, reconcile their results, or verify the completed case.
package workflow
