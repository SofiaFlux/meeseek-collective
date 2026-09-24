package main

import (
	"strings"
	"testing"
	"time"
)

func requiredObserverArgs() []string {
	return []string{
		"--mission=m1",
		"--reviewer-id=me",
		"--grant-capability=read",
		"--envelope=env1",
		"--max-steps=3",
		"--remaining-budget=10",
	}
}

func TestParseObserverFlagsDefaults(t *testing.T) {
	cfg, interval, err := parseObserverFlags(requiredObserverArgs())
	if err != nil {
		t.Fatal(err)
	}
	if interval != 5*time.Minute {
		t.Fatalf("interval = %v, want 5m", interval)
	}
	if cfg.MissionID != "m1" || cfg.ReviewerID != "me" {
		t.Fatalf("identity = %q %q, want m1 me", cfg.MissionID, cfg.ReviewerID)
	}
	if len(cfg.Grant.Capabilities) != 1 || cfg.Grant.Capabilities[0] != "read" {
		t.Fatalf("grant capabilities = %v, want [read]", cfg.Grant.Capabilities)
	}
	if len(cfg.Grant.Actions) != 0 || len(cfg.WorkCapabilities) != 0 {
		t.Fatalf("actions/work = %v %v, want both empty", cfg.Grant.Actions, cfg.WorkCapabilities)
	}
	if cfg.ResourceEnvelopeID != "env1" || cfg.MaxSteps != 3 || cfg.RemainingBudget != 10 {
		t.Fatalf("limits = %q %d %d, want env1 3 10", cfg.ResourceEnvelopeID, cfg.MaxSteps, cfg.RemainingBudget)
	}
	if cfg.Project != "" || cfg.Repository != "" {
		t.Fatalf("scope = %q %q, want empty empty", cfg.Project, cfg.Repository)
	}
}

func TestParseObserverFlagsMissingMission(t *testing.T) {
	args := []string{
		"--reviewer-id=me",
		"--grant-capability=read",
		"--envelope=env1",
		"--max-steps=3",
		"--remaining-budget=10",
	}
	if _, _, err := parseObserverFlags(args); err == nil {
		t.Fatal("expected error for missing --mission")
	}
}

func TestParseObserverFlagsRejectsNonPositivePoll(t *testing.T) {
	args := append(requiredObserverArgs(), "--poll-interval=0")
	if _, _, err := parseObserverFlags(args); err == nil {
		t.Fatal("expected error for zero poll interval")
	}
}

func TestParseObserverFlagsRejectsEmptyGrantCapabilities(t *testing.T) {
	args := []string{
		"--mission=m1",
		"--reviewer-id=me",
		"--envelope=env1",
		"--max-steps=3",
		"--remaining-budget=10",
	}
	if _, _, err := parseObserverFlags(args); err == nil {
		t.Fatal("expected explicit error for empty grant capabilities")
	} else if !strings.Contains(err.Error(), "grant-capability") {
		t.Fatalf("error = %q, want explicit --grant-capability message", err)
	}
}

func TestParseObserverFlagsAcceptsRepeatableCapabilities(t *testing.T) {
	args := append(requiredObserverArgs(),
		"--grant-capability=write",
		"--grant-action=review",
		"--work-capability=read",
		"--project=shop",
		"--repository=web",
		"--poll-interval=1m",
	)
	cfg, interval, err := parseObserverFlags(args)
	if err != nil {
		t.Fatal(err)
	}
	if interval != time.Minute {
		t.Fatalf("interval = %v, want 1m", interval)
	}
	if len(cfg.Grant.Capabilities) != 2 || cfg.Grant.Capabilities[1] != "write" {
		t.Fatalf("grant capabilities = %v, want [read write]", cfg.Grant.Capabilities)
	}
	if len(cfg.Grant.Actions) != 1 || cfg.Grant.Actions[0] != "review" {
		t.Fatalf("grant actions = %v, want [review]", cfg.Grant.Actions)
	}
	if len(cfg.WorkCapabilities) != 1 || cfg.WorkCapabilities[0] != "read" {
		t.Fatalf("work capabilities = %v, want [read]", cfg.WorkCapabilities)
	}
	if cfg.Project != "shop" || cfg.Repository != "web" {
		t.Fatalf("scope = %q %q, want shop web", cfg.Project, cfg.Repository)
	}
}
