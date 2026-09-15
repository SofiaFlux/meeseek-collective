package resources

import (
	"errors"
	"sync"
	"testing"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
)

func TestConcurrentReservationsCannotOversubscribeEnvelope(t *testing.T) {
	svc, ctx, envelopeID := newLedger(t, 100)

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.Reserve(ctx, envelopeID, 60, hardCapped())
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var successes, budgetFailures int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrBudgetExceeded):
			budgetFailures++
		default:
			t.Fatalf("unexpected reservation error: %v", err)
		}
	}
	if successes != 1 || budgetFailures != 1 {
		t.Fatalf("successes=%d budgetFailures=%d, want 1/1", successes, budgetFailures)
	}
	available, err := svc.Available(ctx, envelopeID)
	if err != nil {
		t.Fatal(err)
	}
	if available != 40 {
		t.Fatalf("available = %d, want 40", available)
	}
}
