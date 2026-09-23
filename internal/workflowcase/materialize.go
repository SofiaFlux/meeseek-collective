package workflowcase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/execution"
)

// MaterializeTask creates the durable Task for the case's current work proposal.
// The work ID is its stable creation key; execution owns replay and drift checks.
func (s *Service) MaterializeTask(ctx context.Context, executionSvc *execution.Service, caseID, workID domain.ID, template execution.TaskRequest) (domain.Task, error) {
	if executionSvc == nil {
		return domain.Task{}, errors.New("execution service is required")
	}
	c, err := s.Get(ctx, caseID)
	if err != nil {
		return domain.Task{}, err
	}
	if c.State != Active || c.CurrentWorkID != workID || strings.TrimSpace(string(workID)) == "" {
		return domain.Task{}, errors.New("work is not the active work for this case")
	}
	if strings.TrimSpace(template.Objective) == "" ||
		!hasNonblankCriterion(template.AcceptanceCriteria) ||
		strings.TrimSpace(string(template.ResourceEnvelopeID)) == "" {
		return domain.Task{}, errors.New("task template requires objective, acceptance criteria, and resource envelope")
	}
	template.Purpose = domain.PurposeRef{Kind: domain.PurposeMission, ID: c.MissionID}
	template.TaskClass = c.NextWork.Kind
	template.RequiredCapabilities = append([]string(nil), c.NextWork.RequiredCapabilities...)
	template.AuthorityCeiling = append([]string(nil), c.NextWork.AuthorityCeiling...)
	template.IdempotencyKey = string(c.CurrentWorkID)
	task, err := executionSvc.CreateTask(ctx, template)
	if err != nil {
		return domain.Task{}, fmt.Errorf("materialize workflow work: %w", err)
	}
	return task, nil
}

func hasNonblankCriterion(criteria []string) bool {
	for _, criterion := range criteria {
		if strings.TrimSpace(criterion) != "" {
			return true
		}
	}
	return false
}
