package policy

import (
	"context"
	"time"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
)

type PolicyInput struct {
	Risk           string         `json:"risk"`
	AuthorityValid bool           `json:"authority_valid"`
	Now            time.Time      `json:"now"`
	Attributes     map[string]any `json:"attributes,omitempty"`
}

type PolicyEngine interface {
	Evaluate(ctx context.Context, in PolicyInput) (domain.PolicyDecision, error)
}
