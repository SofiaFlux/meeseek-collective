package adoreview

import (
	"testing"
)

// The miss path now commits case + Task in one transaction; the task is keyed
// by the case's work ID, so the linkage must hold for every fresh observation.
func TestObserveOnceMissPathLinksTaskToCaseWork(t *testing.T) {
	ctx, store, cases, execSvc, evidenceStore, cfg := setupObserve(t)
	caller := &fakeCaller{pages: []any{map[string]any{"prs": []any{goodPRItem()}}}}
	result, err := ObserveOnce(ctx, caller, cases, execSvc, evidenceStore, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Ensured) != 1 || len(result.Materialized) != 1 || len(result.Failed) != 0 {
		t.Fatalf("result = %+v", result)
	}
	var workID string
	if err := store.DB().QueryRowContext(ctx,
		`SELECT current_work_id FROM workflow_cases WHERE case_id = ?`, result.Ensured[0]).Scan(&workID); err != nil {
		t.Fatal(err)
	}
	task, err := execSvc.Task(ctx, result.Materialized[0])
	if err != nil {
		t.Fatal(err)
	}
	if task.IdempotencyKey != workID {
		t.Fatalf("task idempotency key = %q, want case work %q", task.IdempotencyKey, workID)
	}
	if task.TaskClass != "ado.pr.review" {
		t.Fatalf("task class = %q", task.TaskClass)
	}
	var count int
	if err := store.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workflow_cases WHERE case_id = ?`, result.Ensured[0]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("case rows = %d, want 1", count)
	}
}
