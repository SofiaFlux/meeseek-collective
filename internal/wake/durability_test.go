package wake_test

import (
	"context"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/testutil"
	"github.com/SofiaFlux/summa42/internal/wake"
)

func TestDueWakeupSurvivesCrashUntilAcknowledged(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 14, 0, 0, 0, time.UTC))
	created, err := wake.New(store, clk, nil).Schedule(ctx, wake.KindTaskReevaluation, "", clk.Now())
	if err != nil {
		t.Fatal(err)
	}

	firstRuntime := wake.New(store, clk, nil)
	due, err := firstRuntime.Due(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != created.ID {
		t.Fatalf("first Due = %#v, want %s", due, created.ID)
	}

	// Simulate a crash before the handler durably commits its work. A new Box
	// instance must see the same wakeup again rather than losing it as FIRED.
	restarted := wake.New(store, clk, nil)
	due, err = restarted.Due(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != created.ID {
		t.Fatalf("Due after restart = %#v, want durable wakeup %s", due, created.ID)
	}

	if err := restarted.Acknowledge(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	due, err = restarted.Due(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("acknowledged wakeup fired again: %#v", due)
	}
}
