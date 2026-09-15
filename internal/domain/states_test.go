package domain

import "testing"

func TestOperationStateTerminality(t *testing.T) {
	if !OperationConfirmedEffect.Terminal() {
		t.Fatal("confirmed effect must be terminal")
	}
	if OperationPrepared.Terminal() {
		t.Fatal("prepared is not terminal")
	}
}

func TestAddendumTaskStatesAreDefined(t *testing.T) {
	if TaskChallenged != "CHALLENGED" || TaskExpired != "EXPIRED" {
		t.Fatalf("unexpected addendum states: %q %q", TaskChallenged, TaskExpired)
	}
}
