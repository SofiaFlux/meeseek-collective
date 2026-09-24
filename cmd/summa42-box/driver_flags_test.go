package main

import (
	"strings"
	"testing"
	"time"
)

func requiredDriverArgs() []string {
	return []string{
		"--mission=m1",
		"--envelope=env1",
	}
}

func TestParseDriverFlagsDefaults(t *testing.T) {
	cfg, interval, err := parseDriverFlags(requiredDriverArgs())
	if err != nil {
		t.Fatal(err)
	}
	if interval != 30*time.Second {
		t.Fatalf("interval = %v, want 30s", interval)
	}
	if cfg.MissionID != "m1" || cfg.ResourceEnvelopeID != "env1" {
		t.Fatalf("identity = %q %q, want m1 env1", cfg.MissionID, cfg.ResourceEnvelopeID)
	}
	if cfg.Project != "" {
		t.Fatalf("project = %q, want empty", cfg.Project)
	}
}

func TestParseDriverFlagsMissingMission(t *testing.T) {
	if _, _, err := parseDriverFlags([]string{"--envelope=env1"}); err == nil {
		t.Fatal("expected error for missing --mission")
	} else if !strings.Contains(err.Error(), "--mission") {
		t.Fatalf("error = %q, want explicit --mission message", err)
	}
}

func TestParseDriverFlagsMissingEnvelope(t *testing.T) {
	if _, _, err := parseDriverFlags([]string{"--mission=m1"}); err == nil {
		t.Fatal("expected error for missing --envelope")
	} else if !strings.Contains(err.Error(), "--envelope") {
		t.Fatalf("error = %q, want explicit --envelope message", err)
	}
}

func TestParseDriverFlagsAcceptsOverrides(t *testing.T) {
	cfg, interval, err := parseDriverFlags([]string{
		"--mission=mission",
		"--envelope=envelope",
		"--project=shop",
		"--poll-interval=1m",
	})
	if err != nil {
		t.Fatal(err)
	}
	if interval != time.Minute {
		t.Fatalf("interval = %v, want 1m", interval)
	}
	if cfg.MissionID != "mission" || cfg.ResourceEnvelopeID != "envelope" || cfg.Project != "shop" {
		t.Fatalf("config = %q %q %q, want mission envelope shop", cfg.MissionID, cfg.ResourceEnvelopeID, cfg.Project)
	}
}

func TestParseDriverFlagsRejectsNonPositivePoll(t *testing.T) {
	if _, _, err := parseDriverFlags(append(requiredDriverArgs(), "--poll-interval=0")); err == nil {
		t.Fatal("expected error for zero poll interval")
	}
}

func TestParseDriverFlagsRejectsUnknownFlag(t *testing.T) {
	if _, _, err := parseDriverFlags(append(requiredDriverArgs(), "--unknown")); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}
