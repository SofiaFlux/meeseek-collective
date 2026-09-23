package workflow

import (
	"fmt"
	"strings"
)

type Verdict string

const (
	Ready    Verdict = "READY"
	Continue Verdict = "CONTINUE"
	Unknown  Verdict = "UNKNOWN"
)

type Outcome string

const (
	OutcomeReady    Outcome = "READY_FOR_VERIFICATION"
	OutcomeContinue Outcome = "CONTINUE"
	OutcomeBlocked  Outcome = "BLOCKED"
)

type Grant struct {
	Capabilities []string
	Actions      []string
}

type WorkProposal struct {
	Kind                 string
	RequiredCapabilities []string
	AuthorityCeiling     []string
	ProposedActions      []string
}

type Assessment struct {
	Verdict     Verdict
	Reason      string
	EvidenceIDs []string
	Next        *WorkProposal
}

type Limits struct {
	MaxSteps        int
	RemainingBudget int64
}

type Input struct {
	Assessment                Assessment
	Grant                     Grant
	Limits                    Limits
	CompletedSteps            int
	ProgressSignature         string
	PreviousProgressSignature string
}

type Decision struct {
	Outcome Outcome
	Reason  string
	Next    *WorkProposal
}

func Decide(input Input) (Decision, error) {
	assessment := input.Assessment
	if len(assessment.EvidenceIDs) == 0 {
		return Decision{}, fmt.Errorf("assessment must include evidence")
	}
	for _, id := range assessment.EvidenceIDs {
		if strings.TrimSpace(id) == "" {
			return Decision{}, fmt.Errorf("assessment evidence ID must not be blank")
		}
	}

	switch assessment.Verdict {
	case Ready:
		if assessment.Next != nil {
			return Decision{}, fmt.Errorf("ready assessment cannot include next work")
		}
		return Decision{Outcome: OutcomeReady, Reason: assessment.Reason}, nil
	case Unknown:
		return Decision{Outcome: OutcomeBlocked, Reason: assessment.Reason}, nil
	case Continue:
		if assessment.Next == nil || assessment.Next.Kind == "" {
			return Decision{}, fmt.Errorf("continue assessment must include a next work kind")
		}
		if !isSubset(assessment.Next.AuthorityCeiling, input.Grant.Capabilities) {
			return Decision{}, fmt.Errorf("next work authority ceiling exceeds grant capabilities")
		}
		if !isSubset(assessment.Next.RequiredCapabilities, assessment.Next.AuthorityCeiling) {
			return Decision{}, fmt.Errorf("next work required capabilities exceed its authority ceiling")
		}
		if !isSubset(assessment.Next.ProposedActions, input.Grant.Actions) {
			return Decision{}, fmt.Errorf("next work proposed actions exceed grant actions")
		}
		if input.CompletedSteps >= input.Limits.MaxSteps {
			return Decision{Outcome: OutcomeBlocked, Reason: "maximum steps exhausted"}, nil
		}
		if input.Limits.RemainingBudget <= 0 {
			return Decision{Outcome: OutcomeBlocked, Reason: "remaining budget exhausted"}, nil
		}
		if input.ProgressSignature != "" && input.ProgressSignature == input.PreviousProgressSignature {
			return Decision{Outcome: OutcomeBlocked, Reason: "no progress detected"}, nil
		}
		return Decision{Outcome: OutcomeContinue, Reason: assessment.Reason, Next: cloneProposal(assessment.Next)}, nil
	default:
		return Decision{}, fmt.Errorf("invalid assessment verdict %q", assessment.Verdict)
	}
}

func isSubset(values, allowed []string) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		set[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func cloneProposal(proposal *WorkProposal) *WorkProposal {
	if proposal == nil {
		return nil
	}
	copy := *proposal
	copy.RequiredCapabilities = append([]string(nil), proposal.RequiredCapabilities...)
	copy.AuthorityCeiling = append([]string(nil), proposal.AuthorityCeiling...)
	copy.ProposedActions = append([]string(nil), proposal.ProposedActions...)
	return &copy
}
