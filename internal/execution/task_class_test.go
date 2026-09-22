package execution_test

import (
	"context"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/purpose"
	"github.com/SofiaFlux/summa42/internal/testutil"
)

func TestTaskClassRoundTripsAndChildInherits(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	svc := execution.New(store, clk, purposes)

	envelopeID := domain.ID("envelope_task_class")
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, 100, ?)`,
		envelopeID, clk.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}

	root, err := svc.CreateTask(ctx, execution.TaskRequest{
		Purpose:              domain.PurposeRef{Kind: domain.PurposeOwnerDirective, ID: "owner-task-class"},
		TaskClass:            "  repo.review  ",
		AcceptanceCriteria:   []string{"done"},
		RequiredCapabilities: []string{"repo.read"},
		RequiredEnforcement:  domain.EnforcementEnforced,
		AuthorityCeiling:     []string{"repo.read"},
		ResourceEnvelopeID:   envelopeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if root.TaskClass != "repo.review" {
		t.Fatalf("root task class=%q, want repo.review", root.TaskClass)
	}

	loaded, err := svc.Task(ctx, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TaskClass != "repo.review" {
		t.Fatalf("loaded task class=%q, want repo.review", loaded.TaskClass)
	}

	child, err := svc.CreateChildTask(ctx, root.ID, execution.TaskRequest{
		AcceptanceCriteria:   []string{"child done"},
		RequiredCapabilities: []string{"repo.read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.TaskClass != "repo.review" {
		t.Fatalf("child task class=%q, want inherited repo.review", child.TaskClass)
	}
}
