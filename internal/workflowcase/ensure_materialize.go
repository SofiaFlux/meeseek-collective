package workflowcase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/execution"
)

// EnsureAndMaterialize creates the workflow case and its first Task in one
// transaction: any Task failure rolls the case back, so no partial state
// survives.
func (s *Service) EnsureAndMaterialize(ctx context.Context, executionSvc *execution.Service, observation Observation, template execution.TaskRequest) (Case, domain.Task, error) {
	if s == nil || s.store == nil || s.clock == nil || s.purposes == nil {
		return Case{}, domain.Task{}, errors.New("workflow case service is not configured")
	}
	if executionSvc == nil {
		return Case{}, domain.Task{}, errors.New("execution service is required")
	}
	prepared, err := s.prepareObservation(observation)
	if err != nil {
		return Case{}, domain.Task{}, err
	}
	var (
		result Case
		task   domain.Task
	)
	if err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		created, err := s.ensureTx(ctx, tx, prepared)
		if err != nil {
			return err
		}
		taskTemplate, err := templateForCase(created, template)
		if err != nil {
			return err
		}
		createdTask, err := executionSvc.CreateTaskWithGuardInTx(ctx, tx, taskTemplate, activeWorkGuard(created))
		if err != nil {
			return fmt.Errorf("materialize workflow work: %w", err)
		}
		result, task = created, createdTask
		return nil
	}); err != nil {
		return Case{}, domain.Task{}, fmt.Errorf("ensure and materialize workflow case: %w", err)
	}
	return result, task, nil
}
