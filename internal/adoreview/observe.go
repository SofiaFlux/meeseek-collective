package adoreview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/evidence"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/workflow"
	"github.com/SofiaFlux/summa42/internal/workflowcase"
)

type Config struct {
	MissionID          domain.ID
	ReviewerID         string
	Grant              workflow.Grant
	WorkCapabilities   []string
	ResourceEnvelopeID domain.ID
	MaxSteps           int
	RemainingBudget    int64
	Project            string
	Repository         string
}

type FailedPR struct {
	PR  PullRequest
	Err string
}

type ObserveResult struct {
	Ensured      []domain.ID
	Materialized []domain.ID
	Excluded     []ExcludedPR
	Failed       []FailedPR
}

func (c Config) validate() error {
	if strings.TrimSpace(string(c.MissionID)) == "" || strings.TrimSpace(c.ReviewerID) == "" {
		return errors.New("mission and reviewer are required")
	}
	if strings.TrimSpace(string(c.ResourceEnvelopeID)) == "" {
		return errors.New("resource envelope is required")
	}
	if c.MaxSteps <= 0 || c.RemainingBudget <= 0 {
		return errors.New("positive max steps and remaining budget are required")
	}
	return nil
}

func (c Config) workCapabilities() []string {
	if len(c.WorkCapabilities) > 0 {
		return append([]string(nil), c.WorkCapabilities...)
	}
	return append([]string(nil), c.Grant.Capabilities...)
}

func Run(ctx context.Context, caller PRCaller, cases *workflowcase.Service, execSvc *execution.Service, evidenceStore *evidence.Store, cfg Config, interval time.Duration) error {
	if interval <= 0 {
		return errors.New("observer requires a positive poll interval")
	}
	if err := ctx.Err(); err != nil {
		return nil
	}
	if _, err := ObserveOnce(ctx, caller, cases, execSvc, evidenceStore, cfg); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := ctx.Err(); err != nil {
				return nil
			}
			if _, err := ObserveOnce(ctx, caller, cases, execSvc, evidenceStore, cfg); err != nil {
				return err
			}
		}
	}
}

func ObserveOnce(ctx context.Context, caller PRCaller, cases *workflowcase.Service, execSvc *execution.Service, evidenceStore *evidence.Store, cfg Config) (ObserveResult, error) {
	var result ObserveResult
	if err := cfg.validate(); err != nil {
		return result, err
	}
	if caller == nil || cases == nil || execSvc == nil || evidenceStore == nil {
		return result, errors.New("caller, case, execution and evidence services are required")
	}
	prs, bad, err := ListPRs(ctx, caller, cfg.Project, cfg.Repository)
	if err != nil {
		return result, err
	}
	for range bad {
		// Unparseable items carry no repo/number identity; only the reason is recorded.
		result.Excluded = append(result.Excluded, ExcludedPR{Reason: ReasonUnparseable})
	}
	kept, excluded := FilterPRs(prs, cfg.ReviewerID)
	result.Excluded = append(result.Excluded, excluded...)
	for _, pr := range kept {
		if err := observeOne(ctx, cases, execSvc, evidenceStore, cfg, pr, &result); err != nil {
			result.Failed = append(result.Failed, FailedPR{PR: pr, Err: err.Error()})
		}
	}
	return result, nil
}

func observeOne(ctx context.Context, cases *workflowcase.Service, execSvc *execution.Service, evidenceStore *evidence.Store, cfg Config, pr PullRequest, result *ObserveResult) error {
	existing, found, err := cases.Find(ctx, cfg.MissionID, "ado", pr.ObjectID(), pr.RevisionID())
	if err != nil {
		return err
	}
	if found {
		task, err := materialize(ctx, cases, execSvc, cfg, existing, pr)
		if err != nil {
			return err
		}
		result.Ensured = append(result.Ensured, existing.ID)
		result.Materialized = append(result.Materialized, task.ID)
		return nil
	}
	canonical, err := json.Marshal(pr)
	if err != nil {
		return err
	}
	object, err := evidenceStore.Put(ctx, strings.NewReader(string(canonical)), evidence.Metadata{MediaType: "application/json", Kind: "ado.pr.snapshot"})
	if err != nil {
		return err
	}
	workCaps := cfg.workCapabilities()
	created, err := cases.Ensure(ctx, workflowcase.Observation{
		MissionID: cfg.MissionID, Source: "ado", ObjectID: pr.ObjectID(), RevisionID: pr.RevisionID(),
		EvidenceID: string(object.ID),
		FirstWork:  workflow.WorkProposal{Kind: "ado.pr.review", RequiredCapabilities: workCaps, AuthorityCeiling: append([]string(nil), cfg.Grant.Capabilities...)},
		Grant:      cfg.Grant, MaxSteps: cfg.MaxSteps, RemainingBudget: cfg.RemainingBudget,
	})
	if err != nil {
		return err
	}
	task, err := materialize(ctx, cases, execSvc, cfg, created, pr)
	if err != nil {
		return err
	}
	result.Ensured = append(result.Ensured, created.ID)
	result.Materialized = append(result.Materialized, task.ID)
	return nil
}

func materialize(ctx context.Context, cases *workflowcase.Service, execSvc *execution.Service, cfg Config, c workflowcase.Case, pr PullRequest) (domain.Task, error) {
	payload, err := json.Marshal(map[string]any{
		"repo": pr.Repository, "pr": pr.Number, "sourceCommit": pr.SourceCommit, "targetCommit": pr.TargetCommit,
	})
	if err != nil {
		return domain.Task{}, err
	}
	return cases.MaterializeTask(ctx, execSvc, c.ID, c.CurrentWorkID, execution.TaskRequest{
		Objective:          fmt.Sprintf("Review ADO PR %s", pr.ObjectID()),
		PayloadJSON:        payload,
		AcceptanceCriteria: []string{"review evidence recorded for " + pr.RevisionID()},
		ResourceEnvelopeID: cfg.ResourceEnvelopeID,
	})
}
