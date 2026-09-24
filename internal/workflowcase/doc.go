// Package workflowcase provides a durable ledger for workflow observations
// and assessments. Ensure deduplicates observations, and Assess applies and
// persists the pure workflow decision for the active work. MaterializeTask
// creates or reuses a durable Task keyed by the active case Work ID. The case
// supplies mission purpose, work class, capabilities, and authority; the recipe
// supplies objective, payload, acceptance criteria, enforcement, and resource
// envelope. The case is rechecked in the Task creation transaction.
//
// Materialization does not dispatch work. The scheduler worker, workflow
// observers, ADO observation/review/protected-effect adapters, publisher, and
// independent final verifier operate around this durable ledger. This service
// does not create Attempt records, verify results, create or validate signed
// grants or evidence objects, or perform external effects. Callers must bind
// case work IDs to real Attempt IDs and revalidate the current Owner grant
// before performing any effect.
package workflowcase
