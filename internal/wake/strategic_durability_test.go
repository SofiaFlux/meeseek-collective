package wake_test

import (
	"context"
	"testing"
	"time"

	"github.com/SofiaFlux/meeseek-collective/internal/execution"
	"github.com/SofiaFlux/meeseek-collective/internal/purpose"
	"github.com/SofiaFlux/meeseek-collective/internal/resources"
	"github.com/SofiaFlux/meeseek-collective/internal/scheduler"
	"github.com/SofiaFlux/meeseek-collective/internal/testutil"
	"github.com/SofiaFlux/meeseek-collective/internal/wake"
)

func TestStrategicPulseSchedulesSuccessorWithoutConsumingCurrentWakeup(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 15, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	resourceSvc := resources.New(store, clk)
	schedulerSvc := scheduler.New(store, clk, purposes, execSvc, resourceSvc, time.Minute)
	wakeSvc := wake.New(store, clk, schedulerSvc)

	current, err := wakeSvc.Schedule(ctx, wake.KindStrategicPulse, "", clk.Now())
	if err != nil {
		t.Fatal(err)
	}
	if due, err := wakeSvc.Due(ctx); err != nil {
		t.Fatal(err)
	} else if len(due) != 1 || due[0].ID != current.ID {
		t.Fatalf("initial due = %#v, want current strategic pulse %s", due, current.ID)
	}

	interval := 15 * time.Minute
	pulse, err := wakeSvc.StrategicPulse(ctx, scheduler.CapacitySnapshot{}, interval)
	if err != nil {
		t.Fatal(err)
	}
	if !pulse.NextPulseAt.Equal(clk.Now().Add(interval)) {
		t.Fatalf("next pulse = %v, want %v", pulse.NextPulseAt, clk.Now().Add(interval))
	}

	// The current delivery is still unacknowledged, so it must remain due even
	// though its handler durably scheduled the successor.
	if due, err := wakeSvc.Due(ctx); err != nil {
		t.Fatal(err)
	} else if len(due) != 1 || due[0].ID != current.ID {
		t.Fatalf("current pulse disappeared before acknowledgement: %#v", due)
	}

	if err := wakeSvc.Acknowledge(ctx, current.ID); err != nil {
		t.Fatal(err)
	}
	next, err := wakeSvc.NextDeadline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || !next.Equal(pulse.NextPulseAt) {
		t.Fatalf("next deadline after acknowledging current pulse = %v, want %v", next, pulse.NextPulseAt)
	}
}
