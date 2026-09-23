// Package workflowcase provides a durable ledger for workflow observations
// and assessments. Ensure deduplicates observations, and Assess applies and
// persists the pure workflow decision for the active work. MaterializeTask
// creates or reuses a durable Task keyed by the active case Work ID. The case
// supplies mission purpose, work class, capabilities, and authority; the recipe
// supplies objective, payload, acceptance criteria, enforcement, and resource
// envelope. The case is rechecked in the Task creation transaction.
//
// Materialization does not dispatch work. The Scheduler has no production
// worker loop, and ADO observation, review, and protected-effect adapters are
// follow-on work. This service does not create Attempt records, verify results,
// create or validate signed grants or evidence objects, or perform external
// effects. Callers must bind case work IDs to real Attempt IDs and revalidate
// the current Owner grant before performing any effect.
package workflowcase
