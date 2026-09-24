package main

import (
	"testing"
	"time"
)

func TestParseWorkerFlagsDefaults(t *testing.T) {
	poll, lease, err := parseWorkerFlags([]string{})
	if err != nil {
		t.Fatal(err)
	}
	if poll != 30*time.Second || lease != 0 {
		t.Fatalf("flags = %v %v, want 30s 0", poll, lease)
	}
}

func TestParseWorkerFlagsRejectsNonPositivePoll(t *testing.T) {
	if _, _, err := parseWorkerFlags([]string{"--poll-interval=0"}); err == nil {
		t.Fatal("expected error for zero poll interval")
	}
}

func TestParseWorkerFlagsAcceptsOverrides(t *testing.T) {
	poll, lease, err := parseWorkerFlags([]string{"--poll-interval=5s", "--lease-duration=2m"})
	if err != nil {
		t.Fatal(err)
	}
	if poll != 5*time.Second || lease != 2*time.Minute {
		t.Fatalf("flags = %v %v, want 5s 2m", poll, lease)
	}
}
