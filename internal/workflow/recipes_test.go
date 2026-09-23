package workflow

import (
	"os"
	"strings"
	"testing"
)

func TestReviewThenPublishThenVerify(t *testing.T) {
	grant := Grant{
		Capabilities: []string{"ado.read", "ado.write"},
		Actions:      []string{"ado.pr.comment", "ado.pr.approve"},
	}
	publication := WorkProposal{
		Kind:                 "publish-decision",
		RequiredCapabilities: []string{"ado.write"},
		AuthorityCeiling:     []string{"ado.write"},
		ProposedActions:      []string{"ado.pr.comment"},
	}

	first, err := Decide(Input{
		Assessment: Assessment{
			Verdict:     Continue,
			EvidenceIDs: []string{"review-evidence"},
			Next:        &publication,
		},
		Grant:          grant,
		Limits:         Limits{MaxSteps: 3, RemainingBudget: 2},
		CompletedSteps: 1,
	})
	if err != nil {
		t.Fatalf("Decide(review) error = %v", err)
	}
	if first.Outcome != OutcomeContinue {
		t.Fatalf("Decide(review) outcome = %q, want %q", first.Outcome, OutcomeContinue)
	}
	if first.Next == nil || first.Next.Kind != "publish-decision" {
		t.Fatalf("Decide(review) next = %#v, want publish-decision", first.Next)
	}
	if len(first.Next.RequiredCapabilities) != 1 || first.Next.RequiredCapabilities[0] != "ado.write" {
		t.Fatalf("Decide(review) next capabilities = %#v, want only ado.write", first.Next.RequiredCapabilities)
	}
	if len(first.Next.ProposedActions) != 1 || first.Next.ProposedActions[0] != "ado.pr.comment" {
		t.Fatalf("Decide(review) next actions = %#v, want only ado.pr.comment", first.Next.ProposedActions)
	}

	second, err := Decide(Input{
		Assessment: Assessment{
			Verdict:     Ready,
			EvidenceIDs: []string{"publication-evidence"},
		},
		Grant:          grant,
		Limits:         Limits{MaxSteps: 3, RemainingBudget: 1},
		CompletedSteps: 2,
	})
	if err != nil {
		t.Fatalf("Decide(publication) error = %v", err)
	}
	if second.Outcome != OutcomeReady {
		t.Fatalf("Decide(publication) outcome = %q, want %q", second.Outcome, OutcomeReady)
	}
	if second.Next != nil {
		t.Fatalf("Decide(publication) next = %#v, want nil", second.Next)
	}
}

func TestDocumentReviewThenArchiveThenVerify(t *testing.T) {
	grant := Grant{
		Capabilities: []string{"document.read", "archive.write"},
		Actions:      []string{"report.archive"},
	}
	archive := WorkProposal{
		Kind:                 "archive-report",
		RequiredCapabilities: []string{"archive.write"},
		AuthorityCeiling:     []string{"archive.write"},
		ProposedActions:      []string{"report.archive"},
	}

	first, err := Decide(Input{
		Assessment: Assessment{
			Verdict:     Continue,
			EvidenceIDs: []string{"document-review-evidence"},
			Next:        &archive,
		},
		Grant:          grant,
		Limits:         Limits{MaxSteps: 3, RemainingBudget: 2},
		CompletedSteps: 1,
	})
	if err != nil {
		t.Fatalf("Decide(document review) error = %v", err)
	}
	if first.Outcome != OutcomeContinue {
		t.Fatalf("Decide(document review) outcome = %q, want %q", first.Outcome, OutcomeContinue)
	}
	if first.Next == nil || first.Next.Kind != "archive-report" {
		t.Fatalf("Decide(document review) next = %#v, want archive-report", first.Next)
	}
	if len(first.Next.RequiredCapabilities) != 1 || first.Next.RequiredCapabilities[0] != "archive.write" {
		t.Fatalf("Decide(document review) next capabilities = %#v, want only archive.write", first.Next.RequiredCapabilities)
	}
	if len(first.Next.ProposedActions) != 1 || first.Next.ProposedActions[0] != "report.archive" {
		t.Fatalf("Decide(document review) next actions = %#v, want only report.archive", first.Next.ProposedActions)
	}

	second, err := Decide(Input{
		Assessment: Assessment{
			Verdict:     Ready,
			EvidenceIDs: []string{"archive-evidence"},
		},
		Grant:          grant,
		Limits:         Limits{MaxSteps: 3, RemainingBudget: 1},
		CompletedSteps: 2,
	})
	if err != nil {
		t.Fatalf("Decide(archive) error = %v", err)
	}
	if second.Outcome != OutcomeReady {
		t.Fatalf("Decide(archive) outcome = %q, want %q", second.Outcome, OutcomeReady)
	}
	if second.Next != nil {
		t.Fatalf("Decide(archive) next = %#v, want nil", second.Next)
	}
}

func TestDecisionKernelHasNoRecipeSpecificADOHandling(t *testing.T) {
	source, err := os.ReadFile("decision.go")
	if err != nil {
		t.Fatalf("read decision.go: %v", err)
	}
	if strings.Contains(strings.ToLower(string(source)), "ado") {
		t.Fatal("decision.go contains ADO-specific handling")
	}
}
