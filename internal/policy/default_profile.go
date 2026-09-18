package policy

import (
	"fmt"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
)

const (
	DefaultModuleName = "meeseek-default.rego"
	DefaultModule     = "package meeseek\n\ndefault decision := {\"outcome\": \"DENY\", \"reason_codes\": [\"default_deny\"]}\n\ndecision := {\"outcome\": \"ALLOW\", \"reason_codes\": [\"low_risk\"]} if {\n  input.risk == \"LOW\"\n  input.authority_valid == true\n}\n"
	DefaultPolicySetID domain.ID = "policy_mvc_default_v1"
)

type Profile struct {
	PolicySetID      domain.ID
	ModuleName       string
	Module           string
	PolicyHash       string
	CapabilitiesHash string
}

func DefaultProfile() (Profile, error) {
	capabilitiesHash, err := SafeCapabilitiesHash()
	if err != nil {
		return Profile{}, fmt.Errorf("hash safe OPA capabilities: %w", err)
	}
	return Profile{
		PolicySetID:      DefaultPolicySetID,
		ModuleName:       DefaultModuleName,
		Module:           DefaultModule,
		PolicyHash:       digestBytes([]byte(DefaultModule)),
		CapabilitiesHash: capabilitiesHash,
	}, nil
}
