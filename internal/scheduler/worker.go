package scheduler

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/SofiaFlux/summa42/internal/clock"
	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/evidence"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/executors"
	"github.com/SofiaFlux/summa42/internal/verification"
)

type Outcome string

const (
	StepIdle      Outcome = "IDLE"
	StepCompleted Outcome = "COMPLETED"
	StepFailed    Outcome = "FAILED"
)

type StepResult struct {
	Outcome     Outcome
	TaskID      domain.ID
	AttemptID   domain.ID
	EvidenceIDs []domain.ID
}

type Worker struct {
	scheduler     *Service
	execution     *execution.Service
	evidence      *evidence.Store
	verification  *verification.Service
	executors     map[string]executors.Executor
	clock         clock.Clock
	workspaceRoot string
}

func NewWorker(schedulerSvc *Service, executionSvc *execution.Service, evidenceStore *evidence.Store, verificationSvc *verification.Service, registry map[string]executors.Executor, clk clock.Clock, workspaceRoot string) (*Worker, error) {
	if schedulerSvc == nil || executionSvc == nil || evidenceStore == nil || verificationSvc == nil || clk == nil {
		return nil, errors.New("worker requires scheduler, execution, evidence, verification and clock")
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		return nil, errors.New("worker requires a workspace root")
	}
	cleaned := make(map[string]executors.Executor, len(registry))
	for kind, executor := range registry {
		kind = strings.TrimSpace(kind)
		if kind != "" && executor != nil {
			cleaned[kind] = executor
		}
	}
	if len(cleaned) == 0 {
		return nil, errors.New("worker requires at least one executor")
	}
	return &Worker{scheduler: schedulerSvc, execution: executionSvc, evidence: evidenceStore, verification: verificationSvc, executors: cleaned, clock: clk, workspaceRoot: workspaceRoot}, nil
}

func (w *Worker) registryKinds() []string {
	kinds := make([]string, 0, len(w.executors))
	for kind := range w.executors {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

func (w *Worker) StepOnce(ctx context.Context, capacity CapacitySnapshot) (StepResult, error) {
	candidate, err := w.scheduler.Next(ctx, capacity)
	if err != nil {
		return StepResult{}, err
	}
	if candidate == nil {
		return StepResult{Outcome: StepIdle}, nil
	}
	return StepResult{}, errors.New("not implemented")
}

func (w *Worker) Run(ctx context.Context, capacity CapacitySnapshot, interval time.Duration) error {
	return errors.New("not implemented")
}
