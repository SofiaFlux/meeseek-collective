package ghissue

import (
	"context"
	"encoding/json"
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

type observeFixture struct {
	ctx           context.Context
	store         *state.Store
	cases         *workflowcase.Service
	execSvc       *execution.Service
	evidenceStore *evidence.Store
	purposes      *purpose.Service
	cfg           ObserveConfig
}

func setupObserve(t *testing.T) observeFixture {
	t.Helper()
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	evidenceStore, err := evidence.New(store, t.TempDir(), clk)
	if err != nil {
		t.Fatal(err)
	}
	cases := workflowcase.New(store, clk, purposes)
	mission, err := purposes.CreateMission(ctx, "triage incoming issues")
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
	return observeFixture{
		ctx: ctx, store: store, cases: cases, execSvc: execSvc, evidenceStore: evidenceStore, purposes: purposes,
		cfg: ObserveConfig{
			MissionID: mission, Repository: "o/r", Maintainers: []string{"maint"},
			Grant:              workflow.Grant{Capabilities: []string{"github.issue.read"}},
			ResourceEnvelopeID: envelope, MaxSteps: 3, RemainingBudget: 10,
		},
	}
}

func snapshotCount(t *testing.T, store *state.Store, ctx context.Context) int {
	t.Helper()
	var n int
	if err := store.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM evidence_objects WHERE kind = 'github.issue.snapshot'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestObserveOnceRegistersMaintainerIssueAndClassifiesOthers(t *testing.T) {
	f := setupObserve(t)
	lister := &stubLister{name: "o/r", pages: []stubPage{{
		items: []any{
			wireIssue(),
			wireIssue(func(m map[string]any) { m["pull_request"] = map[string]any{"url": "x"} }),
			wireIssue(func(m map[string]any) { m["user"] = map[string]any{"login": "stranger"} }),
			wireIssue(func(m map[string]any) {
				m["number"] = float64(8)
				m["labels"] = []any{map[string]any{"name": "wontfix"}}
			}),
		},
		bad: []Unparseable{{Raw: map[string]any{"number": float64(99)}}},
	}}}
	result, err := ObserveOnce(f.ctx, lister, f.cases, f.execSvc, f.evidenceStore, f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Ensured) != 1 || len(result.Materialized) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.PullRequestsSkipped != 1 {
		t.Fatalf("skipped = %d, want 1", result.PullRequestsSkipped)
	}
	reasons := map[string]int{}
	for _, excluded := range result.Excluded {
		reasons[excluded.Reason]++
	}
	if reasons[ReasonUnparseable] != 1 || reasons[ReasonNotMaintainer] != 1 || reasons[ReasonHeldByLabel] != 1 {
		t.Fatalf("reasons = %v", reasons)
	}
	task, err := f.execSvc.Task(f.ctx, result.Materialized[0])
	if err != nil {
		t.Fatal(err)
	}
	if task.Objective != "Triage GitHub issue o/r#7" {
		t.Fatalf("objective = %q", task.Objective)
	}
	if task.State != domain.TaskEligible {
		t.Fatalf("state = %q, want ELIGIBLE (never leased)", task.State)
	}
	if task.TaskClass != "github.issue.triage" {
		t.Fatalf("class = %q", task.TaskClass)
	}
	stored, err := f.cases.Get(f.ctx, result.Ensured[0])
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(task.PayloadJSON, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["issueSnapshot"] != stored.ObservationEvidenceID {
		t.Fatalf("issueSnapshot = %v, want %s", payload["issueSnapshot"], stored.ObservationEvidenceID)
	}
	if payload["repo"] != "o/r" || payload["revision"] != "2026-09-24T10:00:00Z" || payload["triage"] != TriageUnclassified {
		t.Fatalf("payload = %v", payload)
	}
	if _, ok := payload["body"]; ok {
		t.Fatal("payload inlines body; the executor must resolve it through evidence")
	}
	if len(task.AcceptanceCriteria) != 1 || task.AcceptanceCriteria[0] != "triage decision recorded for 2026-09-24T10:00:00Z" {
		t.Fatalf("criteria = %v", task.AcceptanceCriteria)
	}
	_, snapshot, err := f.evidenceStore.Get(f.ctx, domain.ID(stored.ObservationEvidenceID))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(snapshot, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["updatedAt"] != stored.RevisionID {
		t.Fatalf("snapshot revision = %v, want %s", decoded["updatedAt"], stored.RevisionID)
	}
}

func TestObserveOnceRepollIsNoopAndNormalizesOffsets(t *testing.T) {
	f := setupObserve(t)
	page := func(updated string) *stubLister {
		return &stubLister{name: "o/r", pages: []stubPage{{items: []any{
			wireIssue(func(m map[string]any) { m["updated_at"] = updated }),
		}}}}
	}
	first, err := ObserveOnce(f.ctx, page("2026-09-24T10:00:00+00:00"), f.cases, f.execSvc, f.evidenceStore, f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotCount(t, f.store, f.ctx)
	sameInstant, err := ObserveOnce(f.ctx, page("2026-09-24T11:00:00+01:00"), f.cases, f.execSvc, f.evidenceStore, f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if sameInstant.Ensured[0] != first.Ensured[0] || sameInstant.Materialized[0] != first.Materialized[0] {
		t.Fatalf("offset-equivalent revision created new work: %+v vs %+v", sameInstant, first)
	}
	if after := snapshotCount(t, f.store, f.ctx); after != before {
		t.Fatalf("evidence snapshots grew %d -> %d", before, after)
	}
	next, err := ObserveOnce(f.ctx, page("2026-09-24T13:00:00+02:00"), f.cases, f.execSvc, f.evidenceStore, f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if next.Ensured[0] == first.Ensured[0] {
		t.Fatal("edited issue revision did not open a new case")
	}
}

func TestObserveOnceKeepsRepositoriesApart(t *testing.T) {
	f := setupObserve(t)
	firstCfg := f.cfg
	firstCfg.Repository = "owner-a/repo"
	secondCfg := f.cfg
	secondCfg.Repository = "owner-b/repo"
	first, err := ObserveOnce(f.ctx,
		&stubLister{name: "owner-a/repo", pages: []stubPage{{items: []any{wireIssue()}}}},
		f.cases, f.execSvc, f.evidenceStore, firstCfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ObserveOnce(f.ctx,
		&stubLister{name: "owner-b/repo", pages: []stubPage{{items: []any{wireIssue()}}}},
		f.cases, f.execSvc, f.evidenceStore, secondCfg)
	if err != nil {
		t.Fatal(err)
	}
	if first.Ensured[0] == second.Ensured[0] {
		t.Fatal("cross-repository issue #7 collided")
	}
	stored, err := f.cases.Get(f.ctx, second.Ensured[0])
	if err != nil || stored.ObjectID != "github:owner-b/repo#7" {
		t.Fatalf("stored = %+v err = %v", stored, err)
	}
}

func TestObserveOnceRejectsGrantViolations(t *testing.T) {
	f := setupObserve(t)
	lister := &stubLister{name: "o/r", pages: []stubPage{{items: []any{wireIssue()}}}}
	cases := map[string]func(ObserveConfig) ObserveConfig{
		"grant without read capability": func(c ObserveConfig) ObserveConfig {
			c.Grant = workflow.Grant{Capabilities: []string{"other.cap"}}
			return c
		},
		"empty grant": func(c ObserveConfig) ObserveConfig {
			c.Grant = workflow.Grant{}
			return c
		},
		"work capability outside grant": func(c ObserveConfig) ObserveConfig {
			c.WorkCapabilities = []string{"extra.cap"}
			return c
		},
		"missing maintainers": func(c ObserveConfig) ObserveConfig {
			c.Maintainers = nil
			return c
		},
		"blank maintainers": func(c ObserveConfig) ObserveConfig {
			c.Maintainers = []string{"  "}
			return c
		},
		"lister repository mismatch": func(c ObserveConfig) ObserveConfig {
			c.Repository = "o/other"
			return c
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ObserveOnce(f.ctx, lister, f.cases, f.execSvc, f.evidenceStore, mutate(f.cfg)); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if count := snapshotCount(t, f.store, f.ctx); count != 0 {
		t.Fatalf("snapshots = %d, want 0 (validation precedes any poll)", count)
	}
}

func TestObserveOnceNormalizesPaddedGrantTokens(t *testing.T) {
	f := setupObserve(t)
	lister := &stubLister{name: "o/r", pages: []stubPage{{items: []any{wireIssue()}}}}
	cfg := f.cfg
	cfg.Grant = workflow.Grant{Capabilities: []string{" github.issue.read ", "github.issue.read"}}
	cfg.Maintainers = []string{" Maint "}
	cfg.WorkCapabilities = []string{" "}
	result, err := ObserveOnce(f.ctx, lister, f.cases, f.execSvc, f.evidenceStore, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Ensured) != 1 {
		t.Fatalf("ensured = %d, want 1", len(result.Ensured))
	}
	task, err := f.execSvc.Task(f.ctx, result.Materialized[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(task.RequiredCapabilities) != 1 || task.RequiredCapabilities[0] != readCapability {
		t.Fatalf("required capabilities = %v", task.RequiredCapabilities)
	}
}

func TestObserveOnceSkipsInactiveCases(t *testing.T) {
	f := setupObserve(t)
	lister := func() *stubLister {
		return &stubLister{name: "o/r", pages: []stubPage{{items: []any{wireIssue()}}}}
	}
	first, err := ObserveOnce(f.ctx, lister(), f.cases, f.execSvc, f.evidenceStore, f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"BLOCKED", "READY_FOR_VERIFICATION", "CLOSED"} {
		if _, err := f.store.DB().ExecContext(f.ctx,
			`UPDATE workflow_cases SET state = ? WHERE case_id = ?`, state, first.Ensured[0]); err != nil {
			t.Fatal(err)
		}
		result, err := ObserveOnce(f.ctx, lister(), f.cases, f.execSvc, f.evidenceStore, f.cfg)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Materialized) != 0 || len(result.Ensured) != 0 {
			t.Fatalf("state %s: result = %+v", state, result)
		}
		if len(result.Excluded) != 1 || result.Excluded[0].Reason != ReasonCaseNotActive {
			t.Fatalf("state %s: excluded = %+v", state, result.Excluded)
		}
	}
}

func TestObserveOnceExcludesEnsureFailureWithoutCase(t *testing.T) {
	f := setupObserve(t)
	if err := f.purposes.DeactivateMission(f.ctx, f.cfg.MissionID); err != nil {
		t.Fatal(err)
	}
	lister := &stubLister{name: "o/r", pages: []stubPage{{items: []any{wireIssue()}}}}
	result, err := ObserveOnce(f.ctx, lister, f.cases, f.execSvc, f.evidenceStore, f.cfg)
	if err != nil {
		t.Fatalf("ObserveOnce = %v, want nil (ensure failure is an exclusion)", err)
	}
	if len(result.Excluded) != 1 || result.Excluded[0].Reason != ReasonEnsureFailed {
		t.Fatalf("excluded = %+v", result.Excluded)
	}
	if len(result.Failed) != 0 || len(result.Ensured) != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunReturnsNilOnCancellation(t *testing.T) {
	f := setupObserve(t)
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	lister := &stubLister{name: "o/r", pages: []stubPage{{items: []any{wireIssue()}}}}
	if err := Run(ctx, lister, f.cases, f.execSvc, f.evidenceStore, f.cfg, time.Millisecond); err != nil {
		t.Fatalf("Run = %v, want nil on cancelled context", err)
	}
}

func TestRunPropagatesTickError(t *testing.T) {
	f := setupObserve(t)
	lister := &stubLister{name: "not-a-repo", pages: nil}
	if err := Run(f.ctx, lister, f.cases, f.execSvc, f.evidenceStore, f.cfg, time.Millisecond); err == nil {
		t.Fatal("expected tick error")
	}
}
