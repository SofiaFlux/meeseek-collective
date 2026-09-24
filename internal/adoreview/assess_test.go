package adoreview

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/evidence"
	"github.com/SofiaFlux/summa42/internal/executors"
	"github.com/SofiaFlux/summa42/internal/purpose"
	state "github.com/SofiaFlux/summa42/internal/state/sqlite"
	"github.com/SofiaFlux/summa42/internal/testutil"
	"github.com/SofiaFlux/summa42/internal/workflow"
	"github.com/SofiaFlux/summa42/internal/workflowcase"
)

func setupAssess(t *testing.T, grant workflow.Grant) (context.Context, *state.Store, *workflowcase.Service, *evidence.Store, domain.ID) {
	t.Helper()
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	cases := workflowcase.New(store, clk, purposes)
	evidenceStore, err := evidence.New(store, t.TempDir(), clk)
	if err != nil {
		t.Fatal(err)
	}
	mission, err := purposes.CreateMission(ctx, "ado review")
	if err != nil {
		t.Fatal(err)
	}
	return ctx, store, cases, evidenceStore, mission
}

func ensureReviewCase(t *testing.T, ctx context.Context, cases *workflowcase.Service, mission domain.ID, grant workflow.Grant) workflowcase.Case {
	t.Helper()
	c, err := cases.Ensure(ctx, workflowcase.Observation{
		MissionID: mission, Source: "ado", ObjectID: "shop#1", RevisionID: "a:b",
		EvidenceID: "ev-obs-1",
		FirstWork:  workflow.WorkProposal{Kind: "ado.pr.review", RequiredCapabilities: []string{"read"}, AuthorityCeiling: []string{"read"}},
		Grant:      grant, MaxSteps: 3, RemainingBudget: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type gateCaller struct {
	pr    map[string]any
	build map[string]any
	calls []string
}

func (g *gateCaller) Call(_ context.Context, capability string, _ any) (any, error) {
	g.calls = append(g.calls, capability)
	switch capability {
	case "ado.pr.get":
		return g.pr, nil
	case "ado.build.status":
		return g.build, nil
	default:
		return nil, errors.New("unexpected capability " + capability)
	}
}

func defaultReviewGrant() workflow.Grant {
	return workflow.Grant{
		Capabilities: []string{"read"},
		Actions:      []string{"ado.pr.approve", "ado.pr.comment"},
	}
}

func greenGateCaller() *gateCaller {
	return &gateCaller{
		pr:    map[string]any{"sourceCommit": "a", "targetCommit": "b"},
		build: map[string]any{"status": "succeeded"},
	}
}

func newReviewInput(c workflowcase.Case, caller PRCaller, review executors.ReviewResult) ReviewInput {
	return ReviewInput{
		Case: c, WorkID: c.CurrentWorkID, Review: review,
		ReviewEvidenceID: domain.ID("ev-review-1"),
		Repo:             "shop", PR: 1, SourceCommit: "a", TargetCommit: "b",
		Project: "proj", Caller: caller, RemainingBudget: 9,
	}
}

func latestDecision(t *testing.T, ctx context.Context, store *state.Store, evidenceStore *evidence.Store) (ReviewDecision, evidence.EvidenceObject) {
	t.Helper()
	var id string
	if err := store.DB().QueryRowContext(ctx,
		`SELECT evidence_id FROM evidence_objects WHERE kind = 'ado.review.decision' ORDER BY rowid DESC LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("latest decision: %v", err)
	}
	object, data, err := evidenceStore.Get(ctx, domain.ID(id))
	if err != nil {
		t.Fatal(err)
	}
	if object.Kind != "ado.review.decision" {
		t.Fatalf("decision kind = %q, want ado.review.decision", object.Kind)
	}
	if object.MediaType != "application/json" {
		t.Fatalf("decision media type = %q, want application/json", object.MediaType)
	}
	var decision ReviewDecision
	if err := json.Unmarshal(data, &decision); err != nil {
		t.Fatal(err)
	}
	return decision, object
}

func assertBlocked(t *testing.T, ctx context.Context, cases *workflowcase.Service, c workflowcase.Case, result workflowcase.AssessmentResult, reasonSubstr string) {
	t.Helper()
	if result.Decision.Outcome != workflow.OutcomeBlocked {
		t.Fatalf("outcome = %q, want BLOCKED", result.Decision.Outcome)
	}
	if !strings.Contains(result.Decision.Reason, reasonSubstr) {
		t.Fatalf("reason = %q, want substring %q", result.Decision.Reason, reasonSubstr)
	}
	stored, err := cases.Get(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != workflowcase.Blocked {
		t.Fatalf("case state = %q, want BLOCKED", stored.State)
	}
}

func TestAssessReviewCleanApproves(t *testing.T) {
	ctx, store, cases, evidenceStore, mission := setupAssess(t, defaultReviewGrant())
	c := ensureReviewCase(t, ctx, cases, mission, defaultReviewGrant())
	caller := greenGateCaller()
	review := executors.ReviewResult{
		Verdict: executors.ReviewClean, ReviewedCommits: []string{"a"}, ReviewedFiles: []string{"main.go"},
	}
	result, decision, err := AssessReview(ctx, cases, evidenceStore, newReviewInput(c, caller, review))
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.Outcome != workflow.OutcomeContinue {
		t.Fatalf("outcome = %q, want CONTINUE", result.Decision.Outcome)
	}
	if result.Decision.Next == nil || len(result.Decision.Next.ProposedActions) != 1 || result.Decision.Next.ProposedActions[0] != "ado.pr.approve" {
		t.Fatalf("proposed actions = %+v, want [ado.pr.approve]", result.Decision.Next)
	}
	if result.Decision.Next.Kind != "publish-decision" {
		t.Fatalf("next kind = %q, want publish-decision", result.Decision.Next.Kind)
	}
	stored, err := cases.Get(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != workflowcase.Active {
		t.Fatalf("case state = %q, want ACTIVE", stored.State)
	}
	if stored.CurrentWorkID == "" || stored.CurrentWorkID == c.CurrentWorkID {
		t.Fatalf("current work = %q, want a new work ID (was %q)", stored.CurrentWorkID, c.CurrentWorkID)
	}
	blob, _ := latestDecision(t, ctx, store, evidenceStore)
	if blob.Action != DecisionApproveAction {
		t.Fatalf("blob action = %q, want approve", blob.Action)
	}
	if blob.Vote != "approve" {
		t.Fatalf("blob vote = %q, want approve", blob.Vote)
	}
	if strings.TrimSpace(blob.Reason) == "" {
		t.Fatal("blob reason is blank")
	}
	if decision.Action != DecisionApproveAction {
		t.Fatalf("decision action = %q, want approve", decision.Action)
	}
}

func TestAssessReviewFindingsComment(t *testing.T) {
	ctx, store, cases, evidenceStore, mission := setupAssess(t, defaultReviewGrant())
	c := ensureReviewCase(t, ctx, cases, mission, defaultReviewGrant())
	caller := greenGateCaller()
	review := executors.ReviewResult{
		Verdict: executors.ReviewFindings, ReviewedCommits: []string{"a"},
		ReviewedFiles: []string{"main.go", "other.go"},
		Findings: []executors.ReviewFinding{
			{Path: "main.go", Line: 10, Explanation: "nil dereference", Evidence: "x := nil"},
			{Path: "other.go", Line: 0, Explanation: "dead code"},
		},
	}
	result, _, err := AssessReview(ctx, cases, evidenceStore, newReviewInput(c, caller, review))
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.Outcome != workflow.OutcomeContinue {
		t.Fatalf("outcome = %q, want CONTINUE", result.Decision.Outcome)
	}
	if result.Decision.Next == nil || len(result.Decision.Next.ProposedActions) != 1 || result.Decision.Next.ProposedActions[0] != "ado.pr.comment" {
		t.Fatalf("proposed actions = %+v, want [ado.pr.comment]", result.Decision.Next)
	}
	blob, _ := latestDecision(t, ctx, store, evidenceStore)
	if blob.Action != DecisionCommentAction {
		t.Fatalf("blob action = %q, want comment", blob.Action)
	}
	if len(blob.Comments) != 2 {
		t.Fatalf("blob comments = %+v, want 2", blob.Comments)
	}
	if want := "main.go:10: nil dereference\n\nEvidence: x := nil"; blob.Comments[0].Body != want {
		t.Fatalf("comment[0] body = %q, want %q", blob.Comments[0].Body, want)
	}
	if want := "other.go: dead code"; blob.Comments[1].Body != want {
		t.Fatalf("comment[1] body = %q, want %q", blob.Comments[1].Body, want)
	}
}

func TestAssessReviewUncertainHolds(t *testing.T) {
	ctx, store, cases, evidenceStore, mission := setupAssess(t, defaultReviewGrant())
	c := ensureReviewCase(t, ctx, cases, mission, defaultReviewGrant())
	caller := greenGateCaller()
	review := executors.ReviewResult{
		Verdict: executors.ReviewUncertain, ReviewedCommits: []string{"a"},
		ReviewedFiles: []string{"main.go"}, Reason: "cannot see the diff",
	}
	result, _, err := AssessReview(ctx, cases, evidenceStore, newReviewInput(c, caller, review))
	if err != nil {
		t.Fatal(err)
	}
	assertBlocked(t, ctx, cases, c, result, "uncertain-review")
	blob, _ := latestDecision(t, ctx, store, evidenceStore)
	if blob.Action != DecisionHoldAction {
		t.Fatalf("blob action = %q, want hold", blob.Action)
	}
}

func TestAssessReviewStaleHolds(t *testing.T) {
	ctx, store, cases, evidenceStore, mission := setupAssess(t, defaultReviewGrant())
	c := ensureReviewCase(t, ctx, cases, mission, defaultReviewGrant())
	caller := &gateCaller{
		pr:    map[string]any{"sourceCommit": "z", "targetCommit": "b"},
		build: map[string]any{"status": "succeeded"},
	}
	review := executors.ReviewResult{
		Verdict: executors.ReviewClean, ReviewedCommits: []string{"a"}, ReviewedFiles: []string{"main.go"},
	}
	result, _, err := AssessReview(ctx, cases, evidenceStore, newReviewInput(c, caller, review))
	if err != nil {
		t.Fatal(err)
	}
	assertBlocked(t, ctx, cases, c, result, "stale-review")
	blob, _ := latestDecision(t, ctx, store, evidenceStore)
	if blob.Action != DecisionHoldAction {
		t.Fatalf("blob action = %q, want hold", blob.Action)
	}
}

func TestAssessReviewStaleWhenReviewedCommitsMissSource(t *testing.T) {
	ctx, _, cases, evidenceStore, mission := setupAssess(t, defaultReviewGrant())
	c := ensureReviewCase(t, ctx, cases, mission, defaultReviewGrant())
	caller := greenGateCaller()
	review := executors.ReviewResult{
		Verdict: executors.ReviewClean, ReviewedCommits: []string{"old"}, ReviewedFiles: []string{"main.go"},
	}
	result, _, err := AssessReview(ctx, cases, evidenceStore, newReviewInput(c, caller, review))
	if err != nil {
		t.Fatal(err)
	}
	assertBlocked(t, ctx, cases, c, result, "stale-review")
}

func TestAssessReviewUncoveredFindingHolds(t *testing.T) {
	ctx, _, cases, evidenceStore, mission := setupAssess(t, defaultReviewGrant())
	c := ensureReviewCase(t, ctx, cases, mission, defaultReviewGrant())
	caller := greenGateCaller()
	review := executors.ReviewResult{
		Verdict: executors.ReviewFindings, ReviewedCommits: []string{"a"},
		ReviewedFiles: []string{"main.go"},
		Findings: []executors.ReviewFinding{
			{Path: "other.go", Line: 3, Explanation: "outside the reviewed set"},
		},
	}
	result, _, err := AssessReview(ctx, cases, evidenceStore, newReviewInput(c, caller, review))
	if err != nil {
		t.Fatal(err)
	}
	assertBlocked(t, ctx, cases, c, result, "uncovered-finding")
}

func TestAssessReviewEmptyFindingsHolds(t *testing.T) {
	ctx, _, cases, evidenceStore, mission := setupAssess(t, defaultReviewGrant())
	c := ensureReviewCase(t, ctx, cases, mission, defaultReviewGrant())
	caller := greenGateCaller()
	review := executors.ReviewResult{
		Verdict: executors.ReviewFindings, ReviewedCommits: []string{"a"},
		ReviewedFiles: []string{"main.go"},
	}
	result, _, err := AssessReview(ctx, cases, evidenceStore, newReviewInput(c, caller, review))
	if err != nil {
		t.Fatal(err)
	}
	assertBlocked(t, ctx, cases, c, result, "empty-findings")
}

func TestAssessReviewCleanWithFindingsMismatch(t *testing.T) {
	ctx, _, cases, evidenceStore, mission := setupAssess(t, defaultReviewGrant())
	c := ensureReviewCase(t, ctx, cases, mission, defaultReviewGrant())
	caller := greenGateCaller()
	review := executors.ReviewResult{
		Verdict: executors.ReviewClean, ReviewedCommits: []string{"a"},
		ReviewedFiles: []string{"main.go"},
		Findings: []executors.ReviewFinding{
			{Path: "main.go", Line: 1, Explanation: "contradicts the verdict"},
		},
	}
	result, _, err := AssessReview(ctx, cases, evidenceStore, newReviewInput(c, caller, review))
	if err != nil {
		t.Fatal(err)
	}
	assertBlocked(t, ctx, cases, c, result, "verdict-findings-mismatch")
}

func TestAssessReviewGrantDeniesApprove(t *testing.T) {
	denying := workflow.Grant{Capabilities: []string{"read"}, Actions: []string{}}
	ctx, _, cases, evidenceStore, mission := setupAssess(t, denying)
	c := ensureReviewCase(t, ctx, cases, mission, denying)
	caller := greenGateCaller()
	review := executors.ReviewResult{
		Verdict: executors.ReviewClean, ReviewedCommits: []string{"a"}, ReviewedFiles: []string{"main.go"},
	}
	result, _, err := AssessReview(ctx, cases, evidenceStore, newReviewInput(c, caller, review))
	if err != nil {
		t.Fatal(err)
	}
	assertBlocked(t, ctx, cases, c, result, "grant-denies-ado.pr.approve")
}

func TestAssessReviewGrantDeniesComment(t *testing.T) {
	denying := workflow.Grant{Capabilities: []string{"read"}, Actions: []string{"ado.pr.approve"}}
	ctx, _, cases, evidenceStore, mission := setupAssess(t, denying)
	c := ensureReviewCase(t, ctx, cases, mission, denying)
	caller := greenGateCaller()
	review := executors.ReviewResult{
		Verdict: executors.ReviewFindings, ReviewedCommits: []string{"a"},
		ReviewedFiles: []string{"main.go"},
		Findings: []executors.ReviewFinding{
			{Path: "main.go", Line: 2, Explanation: "needs a comment"},
		},
	}
	result, _, err := AssessReview(ctx, cases, evidenceStore, newReviewInput(c, caller, review))
	if err != nil {
		t.Fatal(err)
	}
	assertBlocked(t, ctx, cases, c, result, "grant-denies-ado.pr.comment")
}

func TestAssessReviewCIGreenContinues(t *testing.T) {
	ctx, _, cases, evidenceStore, mission := setupAssess(t, defaultReviewGrant())
	c := ensureReviewCase(t, ctx, cases, mission, defaultReviewGrant())
	caller := &gateCaller{
		pr:    map[string]any{"sourceCommit": "a", "targetCommit": "b", "mergeBuildId": "build-1"},
		build: map[string]any{"status": "succeeded"},
	}
	review := executors.ReviewResult{
		Verdict: executors.ReviewClean, ReviewedCommits: []string{"a"}, ReviewedFiles: []string{"main.go"},
	}
	input := newReviewInput(c, caller, review)
	input.RequireCI = true
	result, _, err := AssessReview(ctx, cases, evidenceStore, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.Outcome != workflow.OutcomeContinue {
		t.Fatalf("outcome = %q, want CONTINUE", result.Decision.Outcome)
	}
	if len(caller.calls) != 2 || caller.calls[0] != "ado.pr.get" || caller.calls[1] != "ado.build.status" {
		t.Fatalf("calls = %v, want [ado.pr.get ado.build.status]", caller.calls)
	}
}

func TestAssessReviewCIRedHolds(t *testing.T) {
	ctx, _, cases, evidenceStore, mission := setupAssess(t, defaultReviewGrant())
	c := ensureReviewCase(t, ctx, cases, mission, defaultReviewGrant())
	caller := &gateCaller{
		pr:    map[string]any{"sourceCommit": "a", "targetCommit": "b", "mergeBuildId": float64(42)},
		build: map[string]any{"status": "failed"},
	}
	review := executors.ReviewResult{
		Verdict: executors.ReviewClean, ReviewedCommits: []string{"a"}, ReviewedFiles: []string{"main.go"},
	}
	input := newReviewInput(c, caller, review)
	input.RequireCI = true
	result, _, err := AssessReview(ctx, cases, evidenceStore, input)
	if err != nil {
		t.Fatal(err)
	}
	assertBlocked(t, ctx, cases, c, result, "ci-failed")
}

func TestAssessReviewCIMissingStatusHolds(t *testing.T) {
	ctx, _, cases, evidenceStore, mission := setupAssess(t, defaultReviewGrant())
	c := ensureReviewCase(t, ctx, cases, mission, defaultReviewGrant())
	caller := &gateCaller{
		pr:    map[string]any{"sourceCommit": "a", "targetCommit": "b", "mergeBuildId": "build-1"},
		build: map[string]any{},
	}
	review := executors.ReviewResult{
		Verdict: executors.ReviewClean, ReviewedCommits: []string{"a"}, ReviewedFiles: []string{"main.go"},
	}
	input := newReviewInput(c, caller, review)
	input.RequireCI = true
	result, _, err := AssessReview(ctx, cases, evidenceStore, input)
	if err != nil {
		t.Fatal(err)
	}
	assertBlocked(t, ctx, cases, c, result, "ci-unknown")
}

func TestAssessReviewCIMissingBuildIDHolds(t *testing.T) {
	ctx, _, cases, evidenceStore, mission := setupAssess(t, defaultReviewGrant())
	c := ensureReviewCase(t, ctx, cases, mission, defaultReviewGrant())
	caller := greenGateCaller()
	review := executors.ReviewResult{
		Verdict: executors.ReviewClean, ReviewedCommits: []string{"a"}, ReviewedFiles: []string{"main.go"},
	}
	input := newReviewInput(c, caller, review)
	input.RequireCI = true
	result, _, err := AssessReview(ctx, cases, evidenceStore, input)
	if err != nil {
		t.Fatal(err)
	}
	assertBlocked(t, ctx, cases, c, result, "ci-unknown")
	if len(caller.calls) != 1 || caller.calls[0] != "ado.pr.get" {
		t.Fatalf("calls = %v, want only [ado.pr.get] (no build call without a build ID)", caller.calls)
	}
}
