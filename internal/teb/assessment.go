package teb

import "strings"

// ModelEgressAssessment records runtime evidence about the network path used by
// an agentic harness. The trusted runtime owns this assessment; executor
// adapters may consume the result but do not establish the guarantee.
type ModelEgressAssessment struct {
	DirectGeneralEgressBlocked     bool
	RequiredModelEndpointsViaProxy bool
	Evidence                       []string
}

// Verified reports whether the assessment proves both parts required by the
// strong agentic profile. Evidence is mandatory so configuration alone cannot
// upgrade an execution path to ENFORCED.
func (a *ModelEgressAssessment) Verified() bool {
	if a == nil || !a.DirectGeneralEgressBlocked || !a.RequiredModelEndpointsViaProxy {
		return false
	}
	for _, evidence := range a.Evidence {
		if strings.TrimSpace(evidence) != "" {
			return true
		}
	}
	return false
}
