package verification

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/evidence"
	"github.com/SofiaFlux/meeseek-collective/internal/execution"
	"github.com/SofiaFlux/meeseek-collective/internal/purpose"
	state "github.com/SofiaFlux/meeseek-collective/internal/state/sqlite"
	"github.com/SofiaFlux/meeseek-collective/internal/testutil"
)

type fixture struct {
	ctx       context.Context
	dbPath    string
	root      string
	store     *state.Store
	clock     *testutil.Clock
	execution *execution.Service
	evidence  *evidence.Store
	verify    *Service
	missionID domain.ID
	task      domain.Task
	attempt   domain.Attempt
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")
	store, err := state.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DB().Close() })
	clk := testutil.NewClock(time.Date(2026, 9, 15, 14, 30, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	missionID, err := purposes.CreateMission(ctx, "Verify durable completion")
	if err != nil {
		t.Fatal(err)
	}
	execSvc := execution.New(store, clk, purposes)
	task, err := execSvc.CreateTask(ctx, execution.TaskRequest{
		Purpose:              domain.PurposeRef{Kind: domain.PurposeMission, ID: missionID},
		AcceptanceCriteria:   []string{"result is independently verified"},
		RequiredCapabilities: []string{"compute"},
		RequiredEnforcement:  domain.EnforcementPartial,
		ResourceEnvelopeID:   "envelope_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := execSvc.StartAttempt(ctx, task.ID, "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	evStore, err := evidence.New(store, filepath.Join(dir, "evidence"), clk)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{
		ctx: ctx, dbPath: dbPath, root: filepath.Join(dir, "evidence"), store: store, clock: clk,
		execution: execSvc, evidence: evStore, verify: New(store, clk, execSvc), missionID: missionID,
		task: task, attempt: attempt,
	}
}

func (f *fixture) put(t *testing.T, body string) evidence.EvidenceObject {
	t.Helper()
	ev, err := f.evidence.Put(f.ctx, strings.NewReader(body), evidence.Metadata{MediaType: "text/plain", Kind: "ATTEMPT_OUTPUT"})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestStagedBlobDoesNotImplyAttemptCompletion(t *testing.T) {
	f := newFixture(t)
	_ = f.put(t, "result")

	if err := f.store.DB().Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := state.Open(f.ctx, f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.DB().Close()

	var stateValue domain.TaskState
	if err := restarted.DB().QueryRowContext(f.ctx, `SELECT state FROM tasks WHERE task_id = ?`, f.task.ID).Scan(&stateValue); err != nil {
		t.Fatal(err)
	}
	if stateValue == domain.TaskAwaitingVerification {
		t.Fatal("orphan/staged evidence inferred Attempt completion")
	}
	if stateValue != domain.TaskExecuting {
		t.Fatalf("task state = %q, want EXECUTING", stateValue)
	}
}

func TestCompletionRecordResumesVerificationAfterRestart(t *testing.T) {
	f := newFixture(t)
	ev := f.put(t, "result")
	record, err := f.verify.CompleteAttempt(f.ctx, f.attempt.ID, CompletionManifest{EvidenceIDs: []domain.ID{ev.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if record.AttemptID != f.attempt.ID || record.ManifestHash == "" {
		t.Fatalf("unexpected completion record: %#v", record)
	}

	if err := f.store.DB().Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := state.Open(f.ctx, f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.DB().Close()

	var taskState domain.TaskState
	var attemptState domain.AttemptState
	var completionCount, verificationCount int
	if err := restarted.DB().QueryRowContext(f.ctx, `SELECT state FROM tasks WHERE task_id = ?`, f.task.ID).Scan(&taskState); err != nil {
		t.Fatal(err)
	}
	if err := restarted.DB().QueryRowContext(f.ctx, `SELECT state FROM attempts WHERE attempt_id = ?`, f.attempt.ID).Scan(&attemptState); err != nil {
		t.Fatal(err)
	}
	if err := restarted.DB().QueryRowContext(f.ctx, `SELECT count(*) FROM attempt_completion_records WHERE attempt_id = ?`, f.attempt.ID).Scan(&completionCount); err != nil {
		t.Fatal(err)
	}
	if err := restarted.DB().QueryRowContext(f.ctx, `SELECT count(*) FROM verification_work WHERE task_id = ? AND state = 'PENDING'`, f.task.ID).Scan(&verificationCount); err != nil {
		t.Fatal(err)
	}
	if taskState != domain.TaskAwaitingVerification || attemptState != domain.AttemptCompleted || completionCount != 1 || verificationCount != 1 {
		t.Fatalf("after restart task=%q attempt=%q completions=%d verification=%d", taskState, attemptState, completionCount, verificationCount)
	}
}

func TestCompleteAttemptUsesCanonicalLeaseGuard(t *testing.T) {
	f := newFixture(t)
	ev := f.put(t, "result")
	f.clock.Advance(2 * time.Minute)
	_, err := f.verify.CompleteAttempt(f.ctx, f.attempt.ID, CompletionManifest{EvidenceIDs: []domain.ID{ev.ID}})
	if !errors.Is(err, domain.ErrLeaseInactive) {
		t.Fatalf("got %v, want ErrLeaseInactive", err)
	}
}

func TestAcceptanceIsIndependentAndUnlocksSuccessDependencies(t *testing.T) {
	f := newFixture(t)
	ev := f.put(t, "result")
	if _, err := f.verify.CompleteAttempt(f.ctx, f.attempt.ID, CompletionManifest{EvidenceIDs: []domain.ID{ev.ID}}); err != nil {
		t.Fatal(err)
	}

	var before domain.TaskState
	if err := f.store.DB().QueryRowContext(f.ctx, `SELECT state FROM tasks WHERE task_id = ?`, f.task.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != domain.TaskAwaitingVerification {
		t.Fatalf("completion marked task %q, want AWAITING_VERIFICATION", before)
	}

	dependent, err := f.execution.CreateTask(f.ctx, execution.TaskRequest{
		Purpose:              domain.PurposeRef{Kind: domain.PurposeMission, ID: f.missionID},
		AcceptanceCriteria:   []string{"dependency succeeded"},
		RequiredCapabilities: []string{"compute"},
		RequiredEnforcement:  domain.EnforcementPartial,
		ResourceEnvelopeID:   "envelope_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB().ExecContext(f.ctx, `INSERT INTO task_dependencies(task_id, depends_on_task_id) VALUES (?, ?)`, dependent.ID, f.task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB().ExecContext(f.ctx, `UPDATE tasks SET state = ? WHERE task_id = ?`, domain.TaskCreated, dependent.ID); err != nil {
		t.Fatal(err)
	}

	acceptance, err := f.verify.AcceptTask(f.ctx, f.task.ID, AcceptanceRequest{
		VerifierID:   "verifier_independent",
		VerifierType: "INDEPENDENT_TEST",
		CriteriaMet:  true,
		EvidenceIDs:  []domain.ID{ev.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if acceptance.TaskID != f.task.ID || acceptance.VerifierID == "" {
		t.Fatalf("unexpected acceptance: %#v", acceptance)
	}

	var accepted, dependentState domain.TaskState
	if err := f.store.DB().QueryRowContext(f.ctx, `SELECT state FROM tasks WHERE task_id = ?`, f.task.ID).Scan(&accepted); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DB().QueryRowContext(f.ctx, `SELECT state FROM tasks WHERE task_id = ?`, dependent.ID).Scan(&dependentState); err != nil {
		t.Fatal(err)
	}
	if accepted != domain.TaskSucceeded || dependentState != domain.TaskEligible {
		t.Fatalf("accepted=%q dependent=%q", accepted, dependentState)
	}
}

func TestAcceptanceDoesNotEraseUnrelatedBlockedState(t *testing.T) {
	f := newFixture(t)
	ev := f.put(t, "result")
	if _, err := f.verify.CompleteAttempt(f.ctx, f.attempt.ID, CompletionManifest{EvidenceIDs: []domain.ID{ev.ID}}); err != nil {
		t.Fatal(err)
	}

	dependent, err := f.execution.CreateTask(f.ctx, execution.TaskRequest{
		Purpose:              domain.PurposeRef{Kind: domain.PurposeMission, ID: f.missionID},
		AcceptanceCriteria:   []string{"dependency succeeded"},
		RequiredCapabilities: []string{"compute"},
		RequiredEnforcement:  domain.EnforcementPartial,
		ResourceEnvelopeID:   "envelope_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB().ExecContext(f.ctx, `INSERT INTO task_dependencies(task_id, depends_on_task_id) VALUES (?, ?)`, dependent.ID, f.task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB().ExecContext(f.ctx, `UPDATE tasks SET state = ? WHERE task_id = ?`, domain.TaskBlocked, dependent.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := f.verify.AcceptTask(f.ctx, f.task.ID, AcceptanceRequest{
		VerifierID: "verifier_independent", VerifierType: "INDEPENDENT_TEST", CriteriaMet: true, EvidenceIDs: []domain.ID{ev.ID},
	}); err != nil {
		t.Fatal(err)
	}

	var dependentState domain.TaskState
	if err := f.store.DB().QueryRowContext(f.ctx, `SELECT state FROM tasks WHERE task_id = ?`, dependent.ID).Scan(&dependentState); err != nil {
		t.Fatal(err)
	}
	if dependentState != domain.TaskBlocked {
		t.Fatalf("unrelated BLOCKED task became %q", dependentState)
	}
}

func TestAcceptTaskRejectsBeforeCompletion(t *testing.T) {
	f := newFixture(t)
	_, err := f.verify.AcceptTask(f.ctx, f.task.ID, AcceptanceRequest{
		VerifierID: "verifier", VerifierType: "INDEPENDENT_TEST", CriteriaMet: true,
	})
	if err == nil {
		t.Fatal("task must not be accepted before durable completion")
	}
	var stateValue domain.TaskState
	if err := f.store.DB().QueryRowContext(f.ctx, `SELECT state FROM tasks WHERE task_id = ?`, f.task.ID).Scan(&stateValue); err != nil {
		t.Fatal(err)
	}
	if stateValue == domain.TaskSucceeded {
		t.Fatal("pre-completion acceptance marked Task SUCCEEDED")
	}
}
