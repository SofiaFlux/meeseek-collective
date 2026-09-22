package verification

import (
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
)

func TestRejectedVerificationDoesNotBlockLaterAttemptCompletion(t *testing.T) {
	f := newFixture(t)
	firstEvidence := f.put(t, "first result")
	if _, err := f.verify.CompleteAttempt(f.ctx, f.attempt.ID, CompletionManifest{EvidenceIDs: []domain.ID{firstEvidence.ID}}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.store.DB().ExecContext(f.ctx,
		`UPDATE verification_work SET state = 'REJECTED' WHERE task_id = ? AND attempt_id = ?`,
		f.task.ID, f.attempt.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB().ExecContext(f.ctx,
		`UPDATE tasks SET state = ?, current_attempt_id = NULL WHERE task_id = ?`,
		domain.TaskEligible, f.task.ID,
	); err != nil {
		t.Fatal(err)
	}

	secondAttempt, err := f.execution.StartAttempt(f.ctx, f.task.ID, "retry", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	secondEvidence := f.put(t, "second result")
	if _, err := f.verify.CompleteAttempt(f.ctx, secondAttempt.ID, CompletionManifest{EvidenceIDs: []domain.ID{secondEvidence.ID}}); err != nil {
		t.Fatalf("second completion after rejected verification: %v", err)
	}

	var count int
	if err := f.store.DB().QueryRowContext(f.ctx,
		`SELECT count(*) FROM verification_work WHERE task_id = ?`, f.task.ID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("verification work rows = %d, want 2", count)
	}
}
