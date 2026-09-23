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
		{name: "continue with blank kind", assessment: Assessment{Verdict: Continue, EvidenceIDs: []string{"evidence-1"}, Next: &WorkProposal{Kind: " \t "}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Decide(testInput(tt.assessment)); err == nil {
				t.Fatal("Decide() error = nil, want validation error")
			}
		})
	}
}

func TestDecideRejectsAuthorityGrowth(t *testing.T) {
	tests := []struct {
		name     string
		proposal WorkProposal
	}{
		{
			name: "ceiling exceeds grant",
			proposal: WorkProposal{
				Kind: "second-step", AuthorityCeiling: []string{"admin"},
			},
		},
		{
			name: "required capability exceeds child ceiling",
			proposal: WorkProposal{
				Kind: "second-step", RequiredCapabilities: []string{"write"}, AuthorityCeiling: []string{"read"},
			},
		},
		{
			name: "action exceeds grant",
			proposal: WorkProposal{
				Kind: "second-step", ProposedActions: []string{"delete"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := testInput(Assessment{
				Verdict: Continue, EvidenceIDs: []string{"evidence-1"}, Next: &tt.proposal,
			})
			decision, err := Decide(input)
			if err == nil {
				t.Fatalf("Decide() = %#v, nil error; want authority validation error", decision)
			}
			if decision.Next != nil {
				t.Errorf("Decide() next = %#v, want nil for invalid authority", decision.Next)
			}
		})
	}
}

func TestDecideBlocksExhaustion(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Input)
		want  string
	}{
		{
			name: "step limit",
			setup: func(input *Input) {
				input.CompletedSteps = input.Limits.MaxSteps
			},
			want: "maximum steps exhausted",
		},
		{
			name: "remaining budget",
			setup: func(input *Input) {
				input.Limits.RemainingBudget = 0
			},
			want: "remaining budget exhausted",
		},
		{
			name: "no progress",
			setup: func(input *Input) {
				input.ProgressSignature = "same-state"
				input.PreviousProgressSignature = "same-state"
			},
			want: "no progress detected",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := testInput(Assessment{
				Verdict:     Continue,
				EvidenceIDs: []string{"evidence-1"},
				Next: &WorkProposal{
					Kind: "second-step", RequiredCapabilities: []string{"read"}, AuthorityCeiling: []string{"read"},
				},
			})
			tt.setup(&input)
			decision, err := Decide(input)
			if err != nil {
				t.Fatalf("Decide() error = %v", err)
			}
			if decision.Outcome != OutcomeBlocked {
				t.Errorf("Decide() outcome = %q, want %q", decision.Outcome, OutcomeBlocked)
			}
			if decision.Next != nil {
				t.Errorf("Decide() next = %#v, want nil", decision.Next)
			}
			if decision.Reason != tt.want {
				t.Errorf("Decide() reason = %q, want %q", decision.Reason, tt.want)
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
