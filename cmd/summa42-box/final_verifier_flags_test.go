package main

import (
	"strings"
	"testing"
	"time"
)

func requiredFinalVerifierArgs() []string {
	return []string{"--mission=m1"}
}

func TestParseFinalVerifierFlagsDefaults(t *testing.T) {
	cfg, interval, err := parseFinalVerifierFlags(requiredFinalVerifierArgs())
	if err != nil {
		t.Fatal(err)
	}
	if interval != 30*time.Second {
		t.Fatalf("interval = %v, want 30s", interval)
	}
	if cfg.MissionID != "m1" || cfg.Project != "" {
		t.Fatalf("scope = %q %q, want m1 and empty project", cfg.MissionID, cfg.Project)
	}
	if cfg.VerifierID != "final-verifier" || cfg.VerifierType != "AUTOMATED" {
		t.Fatalf("verifier = %q/%q, want final-verifier/AUTOMATED", cfg.VerifierID, cfg.VerifierType)
	}
}

func TestParseFinalVerifierFlagsMissingMission(t *testing.T) {
	if _, _, err := parseFinalVerifierFlags(nil); err == nil {
		t.Fatal("expected error for missing --mission")
	} else if !strings.Contains(err.Error(), "--mission") {
		t.Fatalf("error = %q, want explicit --mission message", err)
	}
}

func TestParseFinalVerifierFlagsAcceptsOverrides(t *testing.T) {
	cfg, interval, err := parseFinalVerifierFlags([]string{
		"--mission=mission",
		"--project=shop",
		"--verifier-id=independent-reviewer",
		"--verifier-type=INDEPENDENT",
		"--poll-interval=1m",
	})
	if err != nil {
		t.Fatal(err)
	}
	if interval != time.Minute {
		t.Fatalf("interval = %v, want 1m", interval)
	}
	if cfg.MissionID != "mission" || cfg.Project != "shop" || cfg.VerifierID != "independent-reviewer" || cfg.VerifierType != "INDEPENDENT" {
		t.Fatalf("config = %q %q %q %q", cfg.MissionID, cfg.Project, cfg.VerifierID, cfg.VerifierType)
	}
}

func TestParseFinalVerifierFlagsRejectsNonPositivePoll(t *testing.T) {
	if _, _, err := parseFinalVerifierFlags(append(requiredFinalVerifierArgs(), "--poll-interval=0")); err == nil {
		t.Fatal("expected error for zero poll interval")
	}
}

func TestParseFinalVerifierFlagsRejectsBlankVerifierIdentity(t *testing.T) {
	if _, _, err := parseFinalVerifierFlags(append(requiredFinalVerifierArgs(), "--verifier-id=")); err == nil {
		t.Fatal("expected error for blank verifier ID")
	}
	if _, _, err := parseFinalVerifierFlags(append(requiredFinalVerifierArgs(), "--verifier-type=")); err == nil {
		t.Fatal("expected error for blank verifier type")
	}
}

func TestParseFinalVerifierFlagsRejectsUnknownFlag(t *testing.T) {
	if _, _, err := parseFinalVerifierFlags(append(requiredFinalVerifierArgs(), "--unknown")); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}
