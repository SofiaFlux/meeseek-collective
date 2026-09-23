package workflow

import (
	"reflect"
	"testing"
)

func TestDecideReady(t *testing.T) {
	decision, err := Decide(testInput(Assessment{
		Verdict:     Ready,
		EvidenceIDs: []string{"evidence-1"},
	}))
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.Outcome != OutcomeReady {
		t.Fatalf("Decide() outcome = %q, want %q", decision.Outcome, OutcomeReady)
	}
}

func TestDecideContinueNarrowsAuthority(t *testing.T) {
	proposal := WorkProposal{
		Kind:                 "second-step",
		RequiredCapabilities: []string{"read"},
		AuthorityCeiling:     []string{"read"},
	}
	decision, err := Decide(testInput(Assessment{
		Verdict:     Continue,
		EvidenceIDs: []string{"evidence-1"},
		Next:        &proposal,
	}))
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.Outcome != OutcomeContinue {
		t.Fatalf("Decide() outcome = %q, want %q", decision.Outcome, OutcomeContinue)
	}
	if !reflect.DeepEqual(decision.Next, &proposal) {
		t.Fatalf("Decide() next = %#v, want %#v", decision.Next, &proposal)
	}
	decision.Next.RequiredCapabilities[0] = "changed"
	if proposal.RequiredCapabilities[0] != "read" {
		t.Fatal("Decide() returned a proposal that aliases the input slices")
	}
}

func TestDecideBlocksOnUnknown(t *testing.T) {
	decision, err := Decide(testInput(Assessment{
		Verdict:     Unknown,
		Reason:      "review incomplete",
		EvidenceIDs: []string{"evidence-1"},
	}))
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.Outcome != OutcomeBlocked {
		t.Fatalf("Decide() outcome = %q, want %q", decision.Outcome, OutcomeBlocked)
	}
	if decision.Reason != "review incomplete" {
		t.Fatalf("Decide() reason = %q, want %q", decision.Reason, "review incomplete")
	}
}

func TestDecideRejectsInvalidAssessments(t *testing.T) {
	tests := []struct {
		name       string
		assessment Assessment
	}{
		{name: "missing evidence", assessment: Assessment{Verdict: Ready}},
		{name: "empty evidence ID", assessment: Assessment{Verdict: Ready, EvidenceIDs: []string{""}}},
		{name: "whitespace evidence ID among valid IDs", assessment: Assessment{Verdict: Ready, EvidenceIDs: []string{"evidence-1", " \t "}}},
		{name: "invalid verdict", assessment: Assessment{Verdict: "MAYBE", EvidenceIDs: []string{"evidence-1"}}},
		{name: "ready with next work", assessment: Assessment{Verdict: Ready, EvidenceIDs: []string{"evidence-1"}, Next: &WorkProposal{Kind: "extra"}}},
		{name: "continue without next work", assessment: Assessment{Verdict: Continue, EvidenceIDs: []string{"evidence-1"}}},
		{name: "continue without kind", assessment: Assessment{Verdict: Continue, EvidenceIDs: []string{"evidence-1"}, Next: &WorkProposal{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Decide(testInput(tt.assessment)); err == nil {
				t.Fatal("Decide() error = nil, want validation error")
			}
		})
	}
}

func testInput(assessment Assessment) Input {
	return Input{
		Assessment:     assessment,
		Grant:          Grant{Capabilities: []string{"read", "write"}, Actions: []string{"comment"}},
		Limits:         Limits{MaxSteps: 3, RemainingBudget: 2},
		CompletedSteps: 1,
	}
}
