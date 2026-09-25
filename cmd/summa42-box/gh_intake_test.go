package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/capabilities"
	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/executors"
	"github.com/SofiaFlux/summa42/internal/fieldfeedback"
	"github.com/SofiaFlux/summa42/internal/localconfig"
	"github.com/SofiaFlux/summa42/internal/policy"
	summa42runtime "github.com/SofiaFlux/summa42/internal/runtime"
)

type intakePolicy struct{}

func (intakePolicy) Evaluate(_ context.Context, in policy.PolicyInput) (domain.PolicyDecision, error) {
	return domain.PolicyDecision{
		ID: domain.NewID("policy-decision"), Outcome: domain.PolicyAllow,
		PolicySetID: "policy-test", PolicySetHash: "hash", PolicyCapabilitiesHash: "caps",
		InputDigest: "input", EvaluatedAt: in.Now,
	}, nil
}

type intakeCapabilityProvider struct{}

func (intakeCapabilityProvider) Name() string { return "ado" }
func (intakeCapabilityProvider) Call(context.Context, string, any) (any, error) {
	return nil, nil
}

type intakeSink struct{}

func (intakeSink) Create(context.Context, fieldfeedback.IssuePayload) (string, int64, error) {
	return "issue-1", 0, nil
}
func (intakeSink) FindByMarker(context.Context, string) (string, bool, error) { return "", false, nil }

func validGHIntakeArgs() []string {
	return []string{
		"--mission", "mission-1", "--repo", "o/r", "--maintainer", "maint",
		"--grant-capability", "github.issue.read", "--envelope", "envelope-1",
		"--max-steps", "3", "--remaining-budget", "10", "--poll-interval", "1s",
	}
}

func withoutFlag(args []string, flag, value string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == flag && i+1 < len(args) && args[i+1] == value {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func replaceFlag(args []string, flag, old, value string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == flag && i+1 < len(args) && args[i+1] == old {
			out = append(out, flag, value)
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func TestParseGHIntakeFlagsAcceptsValidInput(t *testing.T) {
	t.Setenv("SUMMA42_GITHUB_REPOSITORY", "env/repo")
	cfg, interval, err := parseGHIntakeFlags(validGHIntakeArgs())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MissionID != "mission-1" || cfg.Repository != "o/r" {
		t.Fatalf("mission=%q repo=%q (--repo must win over env)", cfg.MissionID, cfg.Repository)
	}
	if len(cfg.Maintainers) != 1 || cfg.Maintainers[0] != "maint" {
		t.Fatalf("maintainers = %v", cfg.Maintainers)
	}
	if len(cfg.Grant.Capabilities) != 1 || cfg.Grant.Capabilities[0] != "github.issue.read" {
		t.Fatalf("grant = %v", cfg.Grant.Capabilities)
	}
	if cfg.ResourceEnvelopeID != "envelope-1" || cfg.MaxSteps != 3 || cfg.RemainingBudget != 10 {
		t.Fatalf("cfg = %+v", cfg)
	}
	if interval != time.Second {
		t.Fatalf("interval = %v", interval)
	}
}

func TestParseGHIntakeFlagsFallsBackToRepositoryEnv(t *testing.T) {
	t.Setenv("SUMMA42_GITHUB_REPOSITORY", "env/repo")
	args := withoutFlag(validGHIntakeArgs(), "--repo", "o/r")
	cfg, _, err := parseGHIntakeFlags(args)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repository != "env/repo" {
		t.Fatalf("repo = %q, want env fallback", cfg.Repository)
	}
}

func TestParseGHIntakeFlagsRejectsInvalidInput(t *testing.T) {
	valid := validGHIntakeArgs()
	cases := map[string][]string{
		"missing mission":               withoutFlag(valid, "--mission", "mission-1"),
		"missing repository":            withoutFlag(valid, "--repo", "o/r"),
		"missing maintainer":            withoutFlag(valid, "--maintainer", "maint"),
		"missing envelope":              withoutFlag(valid, "--envelope", "envelope-1"),
		"empty grant":                   withoutFlag(valid, "--grant-capability", "github.issue.read"),
		"grant without read capability": replaceFlag(valid, "--grant-capability", "github.issue.read", "other.cap"),
		"non-positive max steps":        replaceFlag(valid, "--max-steps", "3", "0"),
		"non-positive budget":           replaceFlag(valid, "--remaining-budget", "10", "0"),
		"non-positive interval":         replaceFlag(valid, "--poll-interval", "1s", "0s"),
		"work capability outside grant": append(append([]string{}, valid...), "--work-capability", "extra.cap"),
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("SUMMA42_GITHUB_REPOSITORY", "")
			if _, _, err := parseGHIntakeFlags(args); err == nil {
				t.Fatal("expected startup error")
			}
		})
	}
}

func TestOpenGHIntakeBoxStripsWritableComposition(t *testing.T) {
	root := t.TempDir()
	box, err := openGHIntakeBox(context.Background(), summa42runtime.Config{
		StatePath: filepath.Join(root, "state.db"), EvidencePath: filepath.Join(root, "evidence"),
		CollectiveID: "collective-1", OwnerPrincipalID: "owner-1", PolicyEngine: intakePolicy{},
		FieldFeedback: localconfig.FieldFeedbackConfig{
			Enabled: true, Mode: localconfig.FeedbackModeAutoIfAllowed, Provider: "github",
			Destination: "o/r", MaintenanceEnvelopeID: "envelope-1", RequiredEnforcement: domain.EnforcementEnforced,
		},
		FeedbackSink:        intakeSink{},
		OperationProviders:  nil,
		CapabilityProviders: []capabilities.Provider{intakeCapabilityProvider{}},
		Executors:           map[string]executors.Executor{"ado-publish": publishCapacityExecutor{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	if len(box.Executors) != 0 {
		t.Fatalf("executors = %v, want none", box.Executors)
	}
	if _, err := box.Capabilities.AssessProvider(context.Background(), "ado"); err == nil {
		t.Fatal("capability provider registered in read-only intake Box")
	}
}
