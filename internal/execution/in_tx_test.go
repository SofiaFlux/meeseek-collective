package execution_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/purpose"
	state "github.com/SofiaFlux/summa42/internal/state/sqlite"
	"github.com/SofiaFlux/summa42/internal/testutil"
)

func intakeTaskFixture(t *testing.T) (context.Context, *state.Store, *execution.Service, domain.ID) {
	t.Helper()
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	svc := execution.New(store, clk, purposes)
	envelope := domain.NewID("envelope")
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, ?, ?)`,
		envelope, 100, clk.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}
	return ctx, store, svc, envelope
}

func intakeTaskRequest(envelope domain.ID) execution.TaskRequest {
	return execution.TaskRequest{
		Purpose:            domain.PurposeRef{Kind: domain.PurposeOwnerDirective, ID: "owner-intake"},
		Objective:          "Triage GitHub issue o/r#7",
		AcceptanceCriteria: []string{"triage decision recorded for 2026-09-24T10:00:00Z"},
		ResourceEnvelopeID: envelope,
		IdempotencyKey:     "work-1",
	}
}

func TestCreateTaskWithGuardInTxRollsBackWithEnclosingTransaction(t *testing.T) {
	ctx, store, svc, envelope := intakeTaskFixture(t)
	failure := errors.New("rollback")
	err := store.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := svc.CreateTaskWithGuardInTx(ctx, tx, intakeTaskRequest(envelope), nil); err != nil {
			t.Fatal(err)
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want rollback sentinel", err)
	}
	if _, found, err := svc.FindByIdempotencyKey(ctx, "work-1"); err != nil || found {
		t.Fatalf("task persisted after rollback: found=%v err=%v", found, err)
	}
}

func TestCreateTaskWithGuardInTxPersistsOnCommitAndReplays(t *testing.T) {
	ctx, store, svc, envelope := intakeTaskFixture(t)
	create := func() domain.Task {
		t.Helper()
		var task domain.Task
		if err := store.WithTx(ctx, func(tx *sql.Tx) error {
			created, err := svc.CreateTaskWithGuardInTx(ctx, tx, intakeTaskRequest(envelope), nil)
			if err != nil {
				return err
			}
			task = created
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return task
	}
	first := create()
	second := create()
	if first.ID != second.ID {
		t.Fatalf("replay created a new task: %s vs %s", first.ID, second.ID)
	}
	if _, found, err := svc.FindByIdempotencyKey(ctx, "work-1"); err != nil || !found {
		t.Fatalf("committed task not found: found=%v err=%v", found, err)
	}
}

func TestCreateTaskWithGuardInTxAppliesGuard(t *testing.T) {
	ctx, store, svc, envelope := intakeTaskFixture(t)
	failure := errors.New("guard rejected")
	err := store.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := svc.CreateTaskWithGuardInTx(ctx, tx, intakeTaskRequest(envelope), func(context.Context, *sql.Tx) error {
			return failure
		})
		return err
	})
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want guard failure", err)
	}
	if _, found, err := svc.FindByIdempotencyKey(ctx, "work-1"); err != nil || found {
		t.Fatalf("guarded task persisted: found=%v err=%v", found, err)
	}
}
