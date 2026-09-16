package executors

import (
	"context"
	"time"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
)

type Executor interface {
	Start(ctx context.Context, envelope AttemptEnvelope) (ExecutionResult, error)
}

type AttemptEnvelope struct {
	TaskID              domain.ID
	AttemptID           domain.ID
	Objective           string
	AcceptanceCriteria  []string
	Workspace           string
	VisibleCapabilities []string
	ResourceEnvelopeID  domain.ID
}

type EvidenceKind string

const (
	EvidenceStdout EvidenceKind = "STDOUT"
	EvidenceStderr EvidenceKind = "STDERR"
)

type Evidence struct {
	Kind    EvidenceKind
	Content string
}

type Usage struct {
	WallTime time.Duration
}

type ExecutionResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Evidence []Evidence
	Usage    Usage
}
