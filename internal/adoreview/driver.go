package adoreview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/evidence"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/executors"
	"github.com/SofiaFlux/summa42/internal/operations"
	"github.com/SofiaFlux/summa42/internal/runmanifest"
	"github.com/SofiaFlux/summa42/internal/workflow"
	"github.com/SofiaFlux/summa42/internal/workflowcase"
)

type LookupProvider interface {
	Name() string
	LookupOutcome(context.Context, operations.ProviderDispatchRequest) (operations.ProviderOutcome, error)
}

type DriverConfig struct {
	MissionID          domain.ID
	ResourceEnvelopeID domain.ID
	Comment            LookupProvider
	Vote               LookupProvider
	Caller             PRCaller
	Project            string
}

type DriverResult struct {
	Materialized         []domain.ID
	ReadyForVerification []domain.ID
	Blocked              []domain.ID
}

type Driver struct {
	cases     *workflowcase.Service
	execution *execution.Service
	evidence  *evidence.Store
	manifests *runmanifest.Service
	config    DriverConfig
}

type driverAssessment struct {
	workID           domain.ID
	decisionID       domain.ID
	decision         ReviewDecision
	reviewEvidenceID []domain.ID
}

type workOnePayload struct {
	Project      string `json:"project"`
	Repo         string `json:"repo"`
	PR           int64  `json:"pr"`
	SourceCommit string `json:"sourceCommit"`
	TargetCommit string `json:"targetCommit"`
}

func NewDriver(cases *workflowcase.Service, executionSvc *execution.Service, evidenceStore *evidence.Store, manifests *runmanifest.Service, config DriverConfig) (*Driver, error) {
	if cases == nil || executionSvc == nil || evidenceStore == nil || manifests == nil {
		return nil, errors.New("driver requires case, execution, evidence, and run manifest services")
	}
	config.MissionID = domain.ID(strings.TrimSpace(string(config.MissionID)))
	config.ResourceEnvelopeID = domain.ID(strings.TrimSpace(string(config.ResourceEnvelopeID)))
	config.Project = strings.TrimSpace(config.Project)
	if config.MissionID == "" || config.ResourceEnvelopeID == "" {
		return nil, errors.New("driver mission and resource envelope are required")
	}
	if config.Caller == nil || config.Comment == nil || config.Vote == nil {
		return nil, errors.New("driver caller, comment provider, and vote provider are required")
	}
	if config.Comment.Name() != commentProviderName || config.Vote.Name() != voteProviderName {
		return nil, errors.New("driver providers do not match ADO comment and vote providers")
	}
	return &Driver{cases: cases, execution: executionSvc, evidence: evidenceStore, manifests: manifests, config: config}, nil
}

func (d *Driver) StepOnce(ctx context.Context) (DriverResult, error) {
	var result DriverResult
	if err := d.configured(); err != nil {
		return result, err
	}
	active, err := d.cases.ListActive(ctx, d.config.MissionID)
	if err != nil {
		return result, err
	}
	for _, c := range active {
		if c.Source != "ado" || c.NextWork.Kind != "publish-decision" {
			continue
		}
		if err := d.stepCase(ctx, c, &result); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (d *Driver) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return errors.New("driver requires a positive poll interval")
	}
	if ctx.Err() != nil {
		return nil
	}
	if _, err := d.StepOnce(ctx); err != nil {
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
			if ctx.Err() != nil {
				return nil
			}
			if _, err := d.StepOnce(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

func (d *Driver) stepCase(ctx context.Context, c workflowcase.Case, result *DriverResult) error {
	assessment, found, err := d.discoverAssessment(ctx, c)
	if err != nil || !found {
		return err
	}
	work1, ready, err := d.orderingReady(ctx, assessment)
	if err != nil || !ready {
		return err
	}
	payload, err := decodeWorkOnePayload(work1.PayloadJSON)
	if err != nil {
		return err
	}
	project, err := d.resolveProject(ctx, payload)
	if err != nil {
		return err
	}
	publishPayload := PublishPayload{
		Decision: string(assessment.decisionID), CaseID: string(c.ID), WorkID: string(c.CurrentWorkID),
		Project: project, Repo: payload.Repo, PR: payload.PR, Revision: c.RevisionID,
	}
	payloadJSON, err := json.Marshal(publishPayload)
	if err != nil {
		return err
	}
	task, err := d.cases.MaterializeTask(ctx, d.execution, c.ID, c.CurrentWorkID, execution.TaskRequest{
		Objective:          fmt.Sprintf("Publish ADO review decision for %s", c.ObjectID),
		PayloadJSON:        payloadJSON,
		AcceptanceCriteria: []string{"publication evidence recorded for " + c.RevisionID},
		ResourceEnvelopeID: d.config.ResourceEnvelopeID,
	})
	if err != nil {
		return err
	}
	result.Materialized = append(result.Materialized, task.ID)
	switch task.State {
	case domain.TaskEligible, domain.TaskExecuting:
		return nil
	case domain.TaskAwaitingVerification:
		return d.verifyPublication(ctx, c, task, publishPayload, assessment.decision, result)
	case domain.TaskBlocked, domain.TaskChallenged, domain.TaskCancelled, domain.TaskExpired:
		return d.hold(ctx, c, task, "work2 "+string(task.State), nil, result)
	default:
		return fmt.Errorf("work2 task %s has unsupported state %s", task.ID, task.State)
	}
}

func (d *Driver) discoverAssessment(ctx context.Context, c workflowcase.Case) (driverAssessment, bool, error) {
	records, err := d.cases.ListAssessments(ctx, c.ID)
	if err != nil {
		return driverAssessment{}, false, err
	}
	for _, record := range records {
		var stored workflowcase.AssessmentResult
		if err := json.Unmarshal([]byte(record.ResultJSON), &stored); err != nil {
			return driverAssessment{}, false, fmt.Errorf("decode assessment %s result: %w", record.ID, err)
		}
		if stored.Case.CurrentWorkID != c.CurrentWorkID {
			continue
		}
		var request workflowcase.AssessmentRequest
		if err := json.Unmarshal([]byte(record.RequestJSON), &request); err != nil {
			return driverAssessment{}, false, fmt.Errorf("decode assessment %s request: %w", record.ID, err)
		}
		assessment := driverAssessment{workID: domain.ID(record.WorkID)}
		for _, rawID := range request.Assessment.EvidenceIDs {
			id := domain.ID(strings.TrimSpace(rawID))
			if id == "" {
				continue
			}
			object, data, err := d.evidence.Get(ctx, id)
			if err != nil {
				return driverAssessment{}, false, err
			}
			if object.Kind != "ado.review.decision" {
				assessment.reviewEvidenceID = append(assessment.reviewEvidenceID, id)
				continue
			}
			if assessment.decisionID != "" {
				continue
			}
			var decision ReviewDecision
			if err := json.Unmarshal(data, &decision); err != nil {
				return driverAssessment{}, false, fmt.Errorf("decode review decision %s: %w", id, err)
			}
			assessment.decisionID = id
			assessment.decision = decision
		}
		if assessment.decisionID == "" {
			return driverAssessment{}, false, nil
		}
		return assessment, true, nil
	}
	return driverAssessment{}, false, nil
}

func (d *Driver) orderingReady(ctx context.Context, assessment driverAssessment) (domain.Task, bool, error) {
	task, found, err := d.execution.FindByIdempotencyKey(ctx, string(assessment.workID))
	if err != nil {
		return domain.Task{}, false, err
	}
	if !found || task.State != domain.TaskAwaitingVerification || task.CurrentAttemptID == "" {
		return domain.Task{}, false, nil
	}
	provenance, err := d.manifests.Provenance(ctx, task.CurrentAttemptID)
	if err != nil {
		if ctx.Err() != nil {
			return domain.Task{}, false, ctx.Err()
		}
		return domain.Task{}, false, nil
	}
	output := make(map[domain.ID]struct{}, len(provenance.OutputEvidence))
	for _, ref := range provenance.OutputEvidence {
		output[ref.ID] = struct{}{}
	}
	for _, id := range assessment.reviewEvidenceID {
		if _, ok := output[id]; ok {
			return task, true, nil
		}
	}
	return domain.Task{}, false, nil
}

func decodeWorkOnePayload(raw json.RawMessage) (workOnePayload, error) {
	var payload workOnePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return workOnePayload{}, fmt.Errorf("decode Work 1 payload: %w", err)
	}
	payload.Project = strings.TrimSpace(payload.Project)
	payload.Repo = strings.TrimSpace(payload.Repo)
	payload.SourceCommit = strings.TrimSpace(payload.SourceCommit)
	payload.TargetCommit = strings.TrimSpace(payload.TargetCommit)
	if payload.Repo == "" || payload.PR <= 0 || payload.SourceCommit == "" || payload.TargetCommit == "" {
		return workOnePayload{}, errors.New("Work 1 payload requires repo, positive pr, sourceCommit, and targetCommit")
	}
	return payload, nil
}

func (d *Driver) resolveProject(ctx context.Context, payload workOnePayload) (string, error) {
	if payload.Project != "" {
		return payload.Project, nil
	}
	args := map[string]any{"action": "get", "repository": payload.Repo, "pullRequestId": payload.PR}
	if d.config.Project != "" {
		args["project"] = d.config.Project
	}
	raw, err := d.config.Caller.Call(ctx, "ado.pr.get", args)
	if err != nil {
		return "", fmt.Errorf("resolve ADO project: %w", err)
	}
	project := adoProjectField(raw)
	if project == "" {
		project = d.config.Project
	}
	if project == "" {
		return "", errors.New("resolve ADO project: PR get response is missing project")
	}
	return project, nil
}

func adoProjectField(raw any) string {
	pr, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"project", "projectName"} {
		if value, ok := pr[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		if value, ok := pr[key].(map[string]any); ok {
			for _, nameKey := range []string{"name", "projectName"} {
				if name, ok := value[nameKey].(string); ok && strings.TrimSpace(name) != "" {
					return strings.TrimSpace(name)
				}
			}
		}
	}
	return ""
}

func (d *Driver) verifyPublication(ctx context.Context, c workflowcase.Case, task domain.Task, payload PublishPayload, decision ReviewDecision, result *DriverResult) error {
	provenance, err := d.manifests.Provenance(ctx, task.CurrentAttemptID)
	if err != nil {
		return fmt.Errorf("read Work 2 provenance: %w", err)
	}
	evidenceIDs := make([]string, 0, len(provenance.OutputEvidence))
	contents := make([][]byte, 0, 1)
	for _, ref := range provenance.OutputEvidence {
		evidenceIDs = append(evidenceIDs, string(ref.ID))
		object, data, err := d.evidence.Get(ctx, ref.ID)
		if err != nil {
			return err
		}
		if object.Kind == string(executors.EvidenceAgentMessage) {
			contents = append(contents, data)
		}
	}
	if len(evidenceIDs) == 0 {
		return d.hold(ctx, c, task, "work2 completion has no evidence", nil, result)
	}
	entries, reason := decodePublisherEntries(contents)
	if reason != "" {
		return d.hold(ctx, c, task, reason, evidenceIDs, result)
	}
	intents, err := buildPublishIntents(payload, decision)
	if err != nil {
		return d.hold(ctx, c, task, "invalid publication decision: "+err.Error(), evidenceIDs, result)
	}
	expected := make(map[string]publishIntent, len(intents))
	for _, intent := range intents {
		expected[intent.slot] = intent
	}
	observed := make(map[string]publishEvidenceEntry, len(entries))
	for _, entry := range entries {
		slot := strings.TrimSpace(entry.Slot)
		if slot == "" {
			return d.hold(ctx, c, task, "publisher evidence has a blank slot", evidenceIDs, result)
		}
		if _, duplicate := observed[slot]; duplicate {
			return d.hold(ctx, c, task, "duplicate publisher slot "+slot, evidenceIDs, result)
		}
		observed[slot] = entry
	}
	for _, intent := range intents {
		entry, ok := observed[intent.slot]
		if !ok {
			return d.hold(ctx, c, task, "missing slot "+intent.slot, evidenceIDs, result)
		}
		if entry.Skipped {
			return d.hold(ctx, c, task, "skipped slot "+intent.slot, evidenceIDs, result)
		}
		if entry.RecordedOnly {
			return d.hold(ctx, c, task, "recorded-only slot "+intent.slot, evidenceIDs, result)
		}
		if entry.State != domain.OperationConfirmedEffect {
			return d.hold(ctx, c, task, fmt.Sprintf("slot %s is %s", intent.slot, entry.State), evidenceIDs, result)
		}
	}
	for slot := range observed {
		if _, ok := expected[slot]; !ok {
			return d.hold(ctx, c, task, "unexpected slot "+slot, evidenceIDs, result)
		}
	}
	providers := map[string]LookupProvider{
		commentProviderName: d.config.Comment,
		voteProviderName:    d.config.Vote,
	}
	for _, intent := range intents {
		entry := observed[intent.slot]
		canonical, err := json.Marshal(intent.intent)
		if err != nil {
			return err
		}
		outcome, err := providers[intent.provider].LookupOutcome(ctx, operations.ProviderDispatchRequest{
			OperationID: entry.Operation, TaskID: task.ID, CanonicalIntent: canonical,
		})
		if err != nil {
			return d.hold(ctx, c, task, fmt.Sprintf("lookup %s: %v", intent.slot, err), evidenceIDs, result)
		}
		if outcome.State != domain.OperationConfirmedEffect {
			return d.hold(ctx, c, task, fmt.Sprintf("slot %s lookup is %s", intent.slot, outcome.State), evidenceIDs, result)
		}
	}
	_, err = d.cases.Assess(ctx, workflowcase.AssessmentRequest{
		CaseID: c.ID, WorkID: c.CurrentWorkID, RemainingBudget: c.RemainingBudget,
		ProgressSignature: "ado:publish:" + string(c.ID) + ":" + string(c.CurrentWorkID) + ":verified",
		Assessment: workflow.Assessment{
			Verdict: workflow.Ready, Reason: "ADO publication effects verified", EvidenceIDs: evidenceIDs,
		},
	})
	if err != nil {
		return err
	}
	result.ReadyForVerification = append(result.ReadyForVerification, c.ID)
	return nil
}

func decodePublisherEntries(contents [][]byte) ([]publishEvidenceEntry, string) {
	entries := make([]publishEvidenceEntry, 0)
	for _, content := range contents {
		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				return nil, "publisher evidence contains a blank line"
			}
			decoder := json.NewDecoder(strings.NewReader(line))
			decoder.DisallowUnknownFields()
			var entry publishEvidenceEntry
			if err := decoder.Decode(&entry); err != nil {
				return nil, "decode publisher evidence: " + err.Error()
			}
			var trailing any
			if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
				return nil, "decode publisher evidence trailer"
			}
			entries = append(entries, entry)
		}
	}
	if len(entries) == 0 {
		return nil, "work2 completion has no publisher evidence"
	}
	return entries, ""
}

func (d *Driver) hold(ctx context.Context, c workflowcase.Case, task domain.Task, reason string, evidenceIDs []string, result *DriverResult) error {
	filtered := make([]string, 0, len(evidenceIDs))
	for _, id := range evidenceIDs {
		if strings.TrimSpace(id) != "" {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) == 0 {
		filtered = []string{string(task.ID)}
	}
	_, err := d.cases.Assess(ctx, workflowcase.AssessmentRequest{
		CaseID: c.ID, WorkID: c.CurrentWorkID, RemainingBudget: c.RemainingBudget,
		ProgressSignature: "ado:publish:" + string(c.ID) + ":" + string(c.CurrentWorkID) + ":unverified",
		Assessment:        workflow.Assessment{Verdict: workflow.Unknown, Reason: reason, EvidenceIDs: filtered},
	})
	if err != nil {
		return err
	}
	result.Blocked = append(result.Blocked, c.ID)
	return nil
}

func (d *Driver) configured() error {
	if d == nil || d.cases == nil || d.execution == nil || d.evidence == nil || d.manifests == nil {
		return errors.New("driver is not configured")
	}
	return nil
}
