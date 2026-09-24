package adoreview

import (
	"context"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/evidence"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/purpose"
	state "github.com/SofiaFlux/summa42/internal/state/sqlite"
	"github.com/SofiaFlux/summa42/internal/testutil"
	"github.com/SofiaFlux/summa42/internal/workflow"
	"github.com/SofiaFlux/summa42/internal/workflowcase"
)

func setupObserve(t *testing.T) (context.Context, *state.Store, *workflowcase.Service, *execution.Service, *evidence.Store, Config) {
	t.Helper()
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	evidenceStore, err := evidence.New(store, t.TempDir(), clk)
	if err != nil {
		t.Fatal(err)
	}
	cases := workflowcase.New(store, clk, purposes)
	mission, err := purposes.CreateMission(ctx, "ado review")
	if err != nil {
		t.Fatal(err)
	}
	envelope := domain.NewID("envelope")
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, ?, ?)`,
		envelope, 100, clk.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}
	cfg := Config{MissionID: mission, ReviewerID: "me",
		Grant:              workflow.Grant{Capabilities: []string{"read"}},
		ResourceEnvelopeID: envelope, MaxSteps: 3, RemainingBudget: 10}
	return ctx, store, cases, execSvc, evidenceStore, cfg
}

func goodPRItem() map[string]any {
	return map[string]any{
		"repository": "shop", "number": float64(1),
		"sourceCommit": "a", "targetCommit": "b",
		"author":    "u9",
		"reviewers": []any{map[string]any{"id": "me"}},
	}
}

func draftPRItem() map[string]any {
	return map[string]any{
		"repository": "shop", "number": float64(2),
		"sourceCommit": "c", "targetCommit": "d",
		"author":    "u9",
		"reviewers": []any{map[string]any{"id": "me"}},
		"isDraft":   true,
	}
}

func snapshotCount(t *testing.T, store *state.Store, ctx context.Context) int {
	t.Helper()
	var n int
	if err := store.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM evidence_objects WHERE kind = 'ado.pr.snapshot'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestObserveOnceMaterializesNewRevision(t *testing.T) {
	ctx, _, cases, execSvc, evidenceStore, cfg := setupObserve(t)
	caller := &fakeCaller{pages: []any{map[string]any{"prs": []any{goodPRItem()}}}}
	result, err := ObserveOnce(ctx, caller, cases, execSvc, evidenceStore, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Ensured) != 1 || len(result.Materialized) != 1 {
		t.Fatalf("result = %+v, want 1 ensured 1 materialized", result)
	}
	if len(result.Excluded) != 0 || len(result.Failed) != 0 {
		t.Fatalf("result = %+v, want 0 excluded 0 failed", result)
	}
	task, err := execSvc.Task(ctx, result.Materialized[0])
	if err != nil {
		t.Fatal(err)
	}
	if task.Objective != "Review ADO PR shop#1" {
		t.Fatalf("objective = %q, want %q", task.Objective, "Review ADO PR shop#1")
	}
	if task.ResourceEnvelopeID != cfg.ResourceEnvelopeID {
		t.Fatalf("envelope = %q, want %q", task.ResourceEnvelopeID, cfg.ResourceEnvelopeID)
	}
}

func TestObserveOnceReusesCaseWithoutNewEvidence(t *testing.T) {
	ctx, store, cases, execSvc, evidenceStore, cfg := setupObserve(t)
	caller := &fakeCaller{pages: []any{map[string]any{"prs": []any{goodPRItem()}}}}
	first, err := ObserveOnce(ctx, caller, cases, execSvc, evidenceStore, cfg)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotCount(t, store, ctx)
	caller.pages = []any{map[string]any{"prs": []any{goodPRItem()}}}
	second, err := ObserveOnce(ctx, caller, cases, execSvc, evidenceStore, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Ensured) != 1 || second.Ensured[0] != first.Ensured[0] {
		t.Fatalf("second ensured = %v, want [%s]", second.Ensured, first.Ensured[0])
	}
	if len(second.Materialized) != 1 || second.Materialized[0] != first.Materialized[0] {
		t.Fatalf("second materialized = %v, want same task %v", second.Materialized, first.Materialized)
	}
	if after := snapshotCount(t, store, ctx); after != before {
		t.Fatalf("evidence snapshots grew %d -> %d, want unchanged", before, after)
	}
}

func TestObserveOnceExcludesAndRepairsPartialCase(t *testing.T) {
	ctx, _, cases, execSvc, evidenceStore, cfg := setupObserve(t)
	// Partial case: Ensure-without-Task for the good PR revision (a:b).
	partial, err := cases.Ensure(ctx, workflowcase.Observation{
		MissionID: cfg.MissionID, Source: "ado", ObjectID: "shop#1", RevisionID: "a:b",
		EvidenceID: "ev-partial",
		FirstWork:  workflow.WorkProposal{Kind: "ado.pr.review", RequiredCapabilities: []string{"read"}, AuthorityCeiling: []string{"read"}},
		Grant:      cfg.Grant, MaxSteps: cfg.MaxSteps, RemainingBudget: cfg.RemainingBudget,
	})
	if err != nil {
		t.Fatal(err)
	}
	caller := &fakeCaller{pages: []any{map[string]any{"prs": []any{draftPRItem(), goodPRItem()}}}}
	result, err := ObserveOnce(ctx, caller, cases, execSvc, evidenceStore, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Excluded) != 1 || result.Excluded[0].Reason != ReasonDraft {
		t.Fatalf("excluded = %+v, want one draft", result.Excluded)
	}
	if len(result.Ensured) != 1 || result.Ensured[0] != partial.ID {
		t.Fatalf("ensured = %v, want [%s] (no new case)", result.Ensured, partial.ID)
	}
	if len(result.Materialized) != 1 {
		t.Fatalf("materialized = %v, want 1 repaired task", result.Materialized)
	}
	if len(result.Failed) != 0 {
		t.Fatalf("failed = %+v, want empty", result.Failed)
	}
}
