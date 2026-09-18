package policy

import (
	"context"
	"testing"
	"time"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
)

func TestDefaultProfileCompilesAndAllowsOnlyAuthorizedLowRisk(t *testing.T) {
	profile, err := DefaultProfile()
	if err != nil {
		t.Fatal(err)
	}
	engine := NewOPAEngine(OPAConfig{
		ModuleName:    profile.ModuleName,
		Module:        profile.Module,
		PolicySetID:   profile.PolicySetID,
		PolicySetHash: profile.PolicyHash,
	})
	metadata, err := engine.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	if metadata.PolicySetHash != profile.PolicyHash || metadata.PolicyCapabilitiesHash != profile.CapabilitiesHash {
		t.Fatalf("metadata=%+v profile=%+v", metadata, profile)
	}
	now := time.Now().UTC()
	allowed, err := engine.Evaluate(context.Background(), PolicyInput{Risk: "LOW", AuthorityValid: true, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if allowed.Outcome != domain.PolicyAllow {
		t.Fatalf("authorized LOW risk outcome=%s", allowed.Outcome)
	}
	denied, err := engine.Evaluate(context.Background(), PolicyInput{Risk: "HIGH", AuthorityValid: true, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if denied.Outcome != domain.PolicyDeny {
		t.Fatalf("HIGH risk outcome=%s", denied.Outcome)
	}
}
