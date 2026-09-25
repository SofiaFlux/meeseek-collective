package ghissue

import (
	"bytes"
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

const readCapability = "github.issue.read"

// ObserveConfig configures issue intake. Config in client.go is the transport
// configuration; this is the observer's.
type ObserveConfig struct {
	MissionID          domain.ID
	Repository         string
	Maintainers        []string
	Grant              workflow.Grant
	WorkCapabilities   []string
	ResourceEnvelopeID domain.ID
	MaxSteps           int
	RemainingBudget    int64
}

// normalized trims and deduplicates every string collection so validation,
// FirstWork.AuthorityCeiling and workflow.Decide all compare the same tokens.
func (c ObserveConfig) normalized() ObserveConfig {
	normalized := ObserveConfig{
		MissionID:          domain.ID(strings.TrimSpace(string(c.MissionID))),
		Repository:         strings.TrimSpace(c.Repository),
		Maintainers:        normalizeTokens(c.Maintainers),
		Grant:              workflow.Grant{Capabilities: normalizeTokens(c.Grant.Capabilities), Actions: normalizeTokens(c.Grant.Actions)},
		WorkCapabilities:   normalizeTokens(c.WorkCapabilities),
		ResourceEnvelopeID: domain.ID(strings.TrimSpace(string(c.ResourceEnvelopeID))),
		MaxSteps:           c.MaxSteps,
		RemainingBudget:    c.RemainingBudget,
	}
	return normalized
}

func (c ObserveConfig) validate() error {
	if strings.TrimSpace(string(c.MissionID)) == "" {
		return errors.New("mission is required")
	}
	if strings.TrimSpace(c.Repository) == "" {
		return errors.New("repository is required")
	}
	if len(c.Maintainers) == 0 {
		return errors.New("at least one maintainer login is required")
	}
	if strings.TrimSpace(string(c.ResourceEnvelopeID)) == "" {
		return errors.New("resource envelope is required")
	}
	if c.MaxSteps <= 0 || c.RemainingBudget <= 0 {
		return errors.New("positive max steps and remaining budget are required")
	}
	granted := make(map[string]struct{}, len(c.Grant.Capabilities))
	for _, capability := range c.Grant.Capabilities {
		if trimmed := strings.TrimSpace(capability); trimmed != "" {
			granted[trimmed] = struct{}{}
		}
	}
	if len(granted) == 0 {
		return errors.New("grant capabilities are required")
	}
	if _, ok := granted[readCapability]; !ok {
		return fmt.Errorf("grant must include %s", readCapability)
	}
	for _, capability := range c.effectiveCapabilities() {
		if _, ok := granted[capability]; !ok {
			return fmt.Errorf("work capability %q exceeds the configured grant", capability)
		}
	}
	return nil
}

// effectiveCapabilities always includes github.issue.read; extra work
// capabilities are additive only.
func (c ObserveConfig) effectiveCapabilities() []string {
	seen := make(map[string]struct{}, len(c.WorkCapabilities)+1)
	out := make([]string, 0, len(c.WorkCapabilities)+1)
	for _, capability := range append([]string{readCapability}, c.WorkCapabilities...) {
		trimmed := strings.TrimSpace(capability)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

// FailedIssue records one issue whose durable Task creation failed on the hit path.
type FailedIssue struct {
	Issue Issue
	Err   string
}

// ObserveResult summarizes one tick.
type ObserveResult struct {
	Ensured             []domain.ID
	Materialized        []domain.ID
	Excluded            []ExcludedIssue
	Failed              []FailedIssue
	PullRequestsSkipped int
}

func ObserveOnce(ctx context.Context, lister IssueLister, cases *workflowcase.Service, execSvc *execution.Service, evidenceStore *evidence.Store, cfg ObserveConfig) (ObserveResult, error) {
	var result ObserveResult
	if ctx == nil {
		return result, errors.New("observer context is required")
	}
	if cases == nil || execSvc == nil || evidenceStore == nil {
		return result, errors.New("workflow case, execution and evidence services are required")
	}
	if lister == nil {
		return result, errors.New("issue lister is required")
	}
	cfg = cfg.normalized()
	if err := cfg.validate(); err != nil {
		return result, err
	}
	if name := strings.TrimSpace(lister.Name()); !strings.EqualFold(name, cfg.Repository) {
		return result, fmt.Errorf("issue lister repository %q does not match configured repository %q", name, cfg.Repository)
	}
	issues, unparseable, err := CollectIssues(ctx, lister)
	if err != nil {
		return result, err
	}
	for range unparseable {
		result.Excluded = append(result.Excluded, ExcludedIssue{Reason: ReasonUnparseable})
	}
	kept, excluded, skipped := FilterIssues(issues, cfg.Maintainers)
	result.PullRequestsSkipped = skipped
	result.Excluded = append(result.Excluded, excluded...)
	for _, issue := range kept {
		if err := observeIssue(ctx, cases, execSvc, evidenceStore, cfg, issue, &result); err != nil {
			return result, err
		}
	}
	return result, nil
}

func observeIssue(ctx context.Context, cases *workflowcase.Service, execSvc *execution.Service, evidenceStore *evidence.Store, cfg ObserveConfig, issue Issue, result *ObserveResult) error {
	existing, found, err := cases.Find(ctx, cfg.MissionID, "github", issue.ObjectID(), issue.RevisionID())
	if err != nil {
		return err
	}
	if found {
		if existing.State != workflowcase.Active {
			result.Excluded = append(result.Excluded, ExcludedIssue{Issue: issue, Reason: ReasonCaseNotActive})
			return nil
		}
		template, err := taskTemplate(cfg, issue, existing.ObservationEvidenceID)
		if err != nil {
			return err
		}
		task, err := cases.MaterializeTask(ctx, execSvc, existing.ID, existing.CurrentWorkID, template)
		if err != nil {
			result.Failed = append(result.Failed, FailedIssue{Issue: issue, Err: err.Error()})
			return nil
		}
		result.Ensured = append(result.Ensured, existing.ID)
		result.Materialized = append(result.Materialized, task.ID)
		return nil
	}
	snapshot, err := CanonicalSnapshot(issue)
	if err != nil {
		return err
	}
	object, err := evidenceStore.Put(ctx, bytes.NewReader(snapshot), evidence.Metadata{MediaType: "application/json", Kind: "github.issue.snapshot"})
	if err != nil {
		return err
	}
	template, err := taskTemplate(cfg, issue, string(object.ID))
	if err != nil {
		return err
	}
	created, task, err := cases.EnsureAndMaterialize(ctx, execSvc, workflowcase.Observation{
		MissionID: cfg.MissionID, Source: "github", ObjectID: issue.ObjectID(), RevisionID: issue.RevisionID(),
		EvidenceID: string(object.ID),
		FirstWork: workflow.WorkProposal{
			Kind: "github.issue.triage", RequiredCapabilities: cfg.effectiveCapabilities(),
			AuthorityCeiling: append([]string(nil), cfg.Grant.Capabilities...),
		},
		Grant: cfg.Grant, MaxSteps: cfg.MaxSteps, RemainingBudget: cfg.RemainingBudget,
	}, template)
	if err != nil {
		result.Excluded = append(result.Excluded, ExcludedIssue{Issue: issue, Reason: ReasonEnsureFailed})
		return nil
	}
	result.Ensured = append(result.Ensured, created.ID)
	result.Materialized = append(result.Materialized, task.ID)
	return nil
}

func taskTemplate(cfg ObserveConfig, issue Issue, snapshotID string) (execution.TaskRequest, error) {
	payload, err := json.Marshal(map[string]any{
		"repo": issue.Repository, "issue": issue.Number, "revision": issue.RevisionID(),
		"title": issue.Title, "url": issue.URL, "author": issue.Author,
		"labels": nonNil(issue.Labels), "triage": issue.Triage, "issueSnapshot": snapshotID,
	})
	if err != nil {
		return execution.TaskRequest{}, fmt.Errorf("encode task payload: %w", err)
	}
	return execution.TaskRequest{
		Objective:          fmt.Sprintf("Triage GitHub issue %s#%d", issue.Repository, issue.Number),
		PayloadJSON:        payload,
		AcceptanceCriteria: []string{"triage decision recorded for " + issue.RevisionID()},
		ResourceEnvelopeID: cfg.ResourceEnvelopeID,
	}, nil
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func Run(ctx context.Context, lister IssueLister, cases *workflowcase.Service, execSvc *execution.Service, evidenceStore *evidence.Store, cfg ObserveConfig, interval time.Duration) error {
	if ctx == nil {
		return errors.New("observer context is required")
	}
	if interval <= 0 {
		return errors.New("observer requires a positive poll interval")
	}
	if err := ctx.Err(); err != nil {
		return nil
	}
	if _, err := ObserveOnce(ctx, lister, cases, execSvc, evidenceStore, cfg); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := ObserveOnce(ctx, lister, cases, execSvc, evidenceStore, cfg); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}
