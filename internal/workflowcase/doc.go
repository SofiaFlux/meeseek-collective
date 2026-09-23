// Package workflowcase provides a durable ledger for workflow observations
// and assessments. Ensure deduplicates observations, and Assess applies and
// persists the pure workflow decision for the active work.
//
// This service does not create or validate Task or Attempt records, verify
// results, create or validate signed grants or evidence objects, or perform
// external effects. Callers must bind case work IDs to real Task and Attempt
// IDs and revalidate the current Owner grant before performing any effect.
package workflowcase
