package adoreview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/evidence"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/executors"
	"github.com/SofiaFlux/summa42/internal/operations"
	"github.com/SofiaFlux/summa42/internal/verification"
	"github.com/SofiaFlux/summa42/internal/workflowcase"
)

type FinalVerifierConfig struct {
	MissionID domain.ID
	// Project limits verification to ready cases whose Work 2 payload has the same project.
	// An empty value is mission-wide; scoped cases without a resolvable project are skipped.
	Project      string
	Comment      LookupProvider
	Vote         LookupProvider
	VerifierID   domain.ID
	VerifierType string
}

type FinalVerifierResult struct {
	Closed  []domain.ID
	Blocked []domain.ID
}

type FinalVerifier struct {
	cases        *workflowcase.Service
	execution    *execution.Service
	evidence     *evidence.Store
	verification *verification.Service
	config       FinalVerifierConfig
}

type finalVerificationDisposition uint8

const (
	finalVerificationHeld finalVerificationDisposition = iota
	finalVerificationBlocked
	finalVerificationClosed
)

type finalVerificationVerdict struct {
	Slot  string                `json:"slot"`
	State domain.OperationState `json:"state"`
}

type finalVerificationSnapshot struct {
	CaseID   domain.ID                  `json:"caseID"`
	Revision string                     `json:"revision"`
	WorkID   domain.ID                  `json:"workID"`
	Slots    []string                   `json:"slots"`
	Verdicts []finalVerificationVerdict `json:"verdicts"`
}

const finalVerificationEvidenceKind = "ado.workflow.verification"

func NewFinalVerifier(cases *workflowcase.Service, executionSvc *execution.Service, evidenceStore *evidence.Store, verificationSvc *verification.Service, config FinalVerifierConfig) (*FinalVerifier, error) {
	if cases == nil || executionSvc == nil || evidenceStore == nil || verificationSvc == nil {
		return nil, errors.New("final verifier requires case, execution, evidence, and verification services")
	}
	config.MissionID = domain.ID(strings.TrimSpace(string(config.MissionID)))
	config.Project = strings.TrimSpace(config.Project)
	config.VerifierID = domain.ID(strings.TrimSpace(string(config.VerifierID)))
	config.VerifierType = strings.TrimSpace(config.VerifierType)
	if config.MissionID == "" || config.VerifierID == "" || config.VerifierType == "" {
		return nil, errors.New("final verifier mission, verifier ID, and verifier type are required")
	}
	if strings.EqualFold(string(config.VerifierID), "publisher") {
		return nil, errors.New("final verifier identity must be distinct from publisher")
	}
	if config.Comment == nil || config.Vote == nil {
		return nil, errors.New("final verifier comment and vote providers are required")
	}
	if config.Comment.Name() != commentProviderName || config.Vote.Name() != voteProviderName {
		return nil, errors.New("final verifier providers do not match ADO comment and vote providers")
	}
	return &FinalVerifier{
		cases: cases, execution: executionSvc, evidence: evidenceStore,
		verification: verificationSvc, config: config,
	}, nil
}

func (v *FinalVerifier) StepOnce(ctx context.Context) (FinalVerifierResult, error) {
	var result FinalVerifierResult
	if err := v.configured(); err != nil {
		return result, err
	}
	ready, err := v.cases.ListReadyForVerification(ctx, v.config.MissionID)
	if err != nil {
		return result, err
	}
	for _, c := range ready {
		if c.Source != "ado" {
			continue
		}
		disposition, err := v.stepCase(ctx, c)
		if err != nil {
			return result, err
		}
		switch disposition {
		case finalVerificationBlocked:
			result.Blocked = append(result.Blocked, c.ID)
		case finalVerificationClosed:
			result.Closed = append(result.Closed, c.ID)
		}
	}
	return result, nil
}

func (v *FinalVerifier) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return errors.New("final verifier requires a positive poll interval")
	}
	if ctx.Err() != nil {
		return nil
	}
	if _, err := v.StepOnce(ctx); err != nil {
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
			if _, err := v.StepOnce(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

func (v *FinalVerifier) stepCase(ctx context.Context, c workflowcase.Case) (finalVerificationDisposition, error) {
	publication, found, err := discoverPublicationAssessment(ctx, v.cases, v.evidence, c, false)
	if err != nil || !found {
		return finalVerificationHeld, err
	}
	completionIDs, contents, reason := v.loadCompletionEvidence(ctx, publication.evidenceIDs)
	if reason != "" {
		return v.reject(ctx, c, reason, completionIDs)
	}
	workID := publication.workID
	if workID == "" || publication.requestWorkID != workID {
		return v.reject(ctx, c, "producing assessment WorkID does not match case current work", completionIDs)
	}
	producingCase := c
	producingCase.CurrentWorkID = workID
	producing, found, err := discoverPublicationAssessment(ctx, v.cases, v.evidence, producingCase, true)
	if err != nil {
		return finalVerificationHeld, err
	}
	if !found || producing.workID == "" || producing.requestWorkID != producing.workID || producing.caseWorkID != workID {
		return v.reject(ctx, c, "producing assessment WorkID does not match case current work", completionIDs)
	}
	task, found, err := v.execution.FindByIdempotencyKey(ctx, string(workID))
	if err != nil {
		return finalVerificationHeld, err
	}
	if !found {
		return v.reject(ctx, c, "Work 2 task not found for producing assessment", completionIDs)
	}
	if v.config.Project != "" {
		var projectPayload struct {
			Project string `json:"project"`
		}
		if err := json.Unmarshal(task.PayloadJSON, &projectPayload); err != nil {
			return finalVerificationHeld, nil
		}
		project := strings.TrimSpace(projectPayload.Project)
		if project == "" || project != v.config.Project {
			return finalVerificationHeld, nil
		}
	}
	if err := validateFinalTask(c, workID, task); err != nil {
		return v.reject(ctx, c, err.Error(), completionIDs)
	}
	payload, err := decodePublishPayload(task.PayloadJSON)
	if err != nil {
		return v.reject(ctx, c, "invalid Work 2 payload: "+err.Error(), completionIDs)
	}
	if err := bindFinalPayload(c, workID, payload); err != nil {
		return v.reject(ctx, c, err.Error(), completionIDs)
	}
	decisionID := producing.decisionID
	if payload.Decision != string(decisionID) {
		return v.reject(ctx, c, "Work 2 payload decision identity does not match producing assessment", append(append([]domain.ID(nil), completionIDs...), decisionID))
	}
	decisionObject, decisionData, err := v.evidence.Get(ctx, decisionID)
	if err != nil {
		return v.reject(ctx, c, "read review decision: "+err.Error(), completionIDs)
	}
	if decisionObject.Kind != "ado.review.decision" {
		return v.reject(ctx, c, fmt.Sprintf("evidence %s is not an ADO review decision", decisionID), completionIDs)
	}
	var decision ReviewDecision
	if err := json.Unmarshal(decisionData, &decision); err != nil {
		return v.reject(ctx, c, "decode review decision: "+err.Error(), completionIDs)
	}
	verificationEvidenceIDs := append(append([]domain.ID(nil), completionIDs...), decisionID)
	intents, err := buildPublishIntents(payload, decision)
	if err != nil {
		return v.reject(ctx, c, "invalid publication decision: "+err.Error(), verificationEvidenceIDs)
	}
	entries, reason := decodePublisherEntries(contents)
	if reason != "" {
		return v.reject(ctx, c, reason, verificationEvidenceIDs)
	}
	observed, reason := bindPublisherEntries(intents, entries)
	if reason != "" {
		return v.reject(ctx, c, reason, verificationEvidenceIDs)
	}
	verdicts, disposition, reason, err := v.lookupFinalIntents(ctx, task, intents, observed)
	if err != nil {
		return finalVerificationHeld, err
	}
	if disposition == finalVerificationHeld {
		return disposition, nil
	}
	if disposition == finalVerificationBlocked {
		return v.reject(ctx, c, reason, verificationEvidenceIDs)
	}
	proceed, err := v.finalizeTask(ctx, task, completionIDs)
	if err != nil || !proceed {
		return finalVerificationHeld, err
	}
	if err := v.close(ctx, c, workID, intents, verdicts, decisionID, completionIDs); err != nil {
		return finalVerificationHeld, err
	}
	return finalVerificationClosed, nil
}

func discoverPublicationAssessment(ctx context.Context, cases *workflowcase.Service, evidenceStore *evidence.Store, c workflowcase.Case, requireDecision bool) (driverAssessment, bool, error) {
	records, err := cases.ListAssessments(ctx, c.ID)
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
		assessment := driverAssessment{
			workID:        domain.ID(record.WorkID),
			requestWorkID: request.WorkID,
			caseWorkID:    stored.Case.CurrentWorkID,
		}
		decisionCount := 0
		var decisionData []byte
		for _, rawID := range request.Assessment.EvidenceIDs {
			id := domain.ID(strings.TrimSpace(rawID))
			if id == "" {
				continue
			}
			assessment.evidenceIDs = append(assessment.evidenceIDs, id)
			object, data, err := evidenceStore.Get(ctx, id)
			if err != nil {
				return driverAssessment{}, false, err
			}
			if object.Kind == "ado.review.decision" {
				decisionCount++
				if decisionCount == 1 {
					assessment.decisionID = id
					decisionData = data
				}
				continue
			}
			assessment.reviewEvidenceIDs = append(assessment.reviewEvidenceIDs, id)
		}
		if requireDecision {
			if decisionCount != 1 {
				return driverAssessment{}, false, nil
			}
			if err := json.Unmarshal(decisionData, &assessment.decision); err != nil {
				return driverAssessment{}, false, fmt.Errorf("decode review decision %s: %w", assessment.decisionID, err)
			}
		}
		return assessment, true, nil
	}
	return driverAssessment{}, false, nil
}

func (v *FinalVerifier) loadCompletionEvidence(ctx context.Context, rawIDs []domain.ID) ([]domain.ID, [][]byte, string) {
	ids := make([]domain.ID, 0, len(rawIDs))
	contents := make([][]byte, 0, len(rawIDs))
	for _, id := range rawIDs {
		id = domain.ID(strings.TrimSpace(string(id)))
		if id == "" {
			return ids, contents, "Work 2 completion contains a blank evidence ID"
		}
		object, data, err := v.evidence.Get(ctx, id)
		if err != nil {
			return ids, contents, "read Work 2 completion evidence: " + err.Error()
		}
		if object.Kind == string(executors.EvidenceAgentMessage) {
			ids = append(ids, id)
			contents = append(contents, data)
		}
	}
	return ids, contents, ""
}

func validateFinalTask(c workflowcase.Case, workID domain.ID, task domain.Task) error {
	if workID == "" || task.IdempotencyKey != string(workID) {
		return errors.New("Work 2 task identity does not match producing assessment")
	}
	if task.Purpose.Kind != domain.PurposeMission || task.Purpose.ID != c.MissionID {
		return errors.New("Work 2 task Mission identity mismatch")
	}
	if task.TaskClass != "publish-decision" {
		return errors.New("Work 2 task class mismatch")
	}
	return nil
}

func bindFinalPayload(c workflowcase.Case, workID domain.ID, payload PublishPayload) error {
	if payload.CaseID != string(c.ID) || payload.WorkID != string(workID) || payload.Revision != c.RevisionID {
		return errors.New("Work 2 payload identity mismatch")
	}
	separator := strings.LastIndexByte(c.ObjectID, '#')
	if separator <= 0 || separator == len(c.ObjectID)-1 {
		return errors.New("workflow case object identity is not repo#pr")
	}
	repo := c.ObjectID[:separator]
	pr, err := strconv.ParseInt(c.ObjectID[separator+1:], 10, 64)
	if err != nil || pr <= 0 {
		return errors.New("workflow case object identity has an invalid pull request")
	}
	if payload.Repo != repo || payload.PR != pr {
		return errors.New("Work 2 payload repository identity mismatch")
	}
	return nil
}

func bindPublisherEntries(intents []publishIntent, entries []publishEvidenceEntry) (map[string]publishEvidenceEntry, string) {
	expected := make(map[string]publishIntent, len(intents))
	for _, intent := range intents {
		expected[intent.slot] = intent
	}
	observed := make(map[string]publishEvidenceEntry, len(entries))
	for _, entry := range entries {
		slot := strings.TrimSpace(entry.Slot)
		if slot == "" {
			return nil, "publisher evidence has a blank slot"
		}
		if _, duplicate := observed[slot]; duplicate {
			return nil, "duplicate publisher slot " + slot
		}
		observed[slot] = entry
	}
	for _, intent := range intents {
		if _, ok := observed[intent.slot]; !ok {
			return nil, "missing slot " + intent.slot
		}
	}
	for slot := range observed {
		if _, ok := expected[slot]; !ok {
			return nil, "unexpected slot " + slot
		}
	}
	return observed, ""
}

func (v *FinalVerifier) lookupFinalIntents(ctx context.Context, task domain.Task, intents []publishIntent, observed map[string]publishEvidenceEntry) ([]finalVerificationVerdict, finalVerificationDisposition, string, error) {
	ordered := append([]publishIntent(nil), intents...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].slot < ordered[right].slot })
	providers := map[string]LookupProvider{
		commentProviderName: v.config.Comment,
		voteProviderName:    v.config.Vote,
	}
	verdicts := make([]finalVerificationVerdict, 0, len(ordered))
	for _, intent := range ordered {
		entry := observed[intent.slot]
		canonical, err := json.Marshal(intent.intent)
		if err != nil {
			return nil, finalVerificationHeld, "", err
		}
		outcome, err := providers[intent.provider].LookupOutcome(ctx, operations.ProviderDispatchRequest{
			OperationID: entry.Operation, TaskID: task.ID, CanonicalIntent: canonical,
		})
		if err != nil {
			return nil, finalVerificationHeld, "", fmt.Errorf("lookup %s: %w", intent.slot, err)
		}
		switch outcome.State {
		case domain.OperationConfirmedEffect:
			verdicts = append(verdicts, finalVerificationVerdict{Slot: intent.slot, State: outcome.State})
		case domain.OperationOutcomeUnknown:
			return nil, finalVerificationHeld, "", nil
		default:
			return nil, finalVerificationBlocked, fmt.Sprintf("slot %s lookup is %s", intent.slot, outcome.State), nil
		}
	}
	return verdicts, finalVerificationClosed, "", nil
}

func (v *FinalVerifier) finalizeTask(ctx context.Context, task domain.Task, completionIDs []domain.ID) (bool, error) {
	switch task.State {
	case domain.TaskSucceeded:
		return true, nil
	case domain.TaskAwaitingVerification:
		_, err := v.verification.AcceptTask(ctx, task.ID, verification.AcceptanceRequest{
			VerifierID: v.config.VerifierID, VerifierType: v.config.VerifierType,
			CriteriaMet: true, EvidenceIDs: completionIDs,
		})
		if err == nil {
			return true, nil
		}
		current, loadErr := v.execution.Task(ctx, task.ID)
		if loadErr == nil && current.State == domain.TaskSucceeded {
			return true, nil
		}
		return false, err
	default:
		return false, nil
	}
}

func (v *FinalVerifier) close(ctx context.Context, c workflowcase.Case, workID domain.ID, intents []publishIntent, verdicts []finalVerificationVerdict, decisionID domain.ID, completionIDs []domain.ID) error {
	ordered := append([]publishIntent(nil), intents...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].slot < ordered[right].slot })
	slots := make([]string, 0, len(ordered))
	for _, intent := range ordered {
		slots = append(slots, intent.slot)
	}
	snapshot := finalVerificationSnapshot{
		CaseID: c.ID, Revision: c.RevisionID, WorkID: workID, Slots: slots, Verdicts: verdicts,
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(body)
	snapshotHash := hex.EncodeToString(digest[:])
	request, err := v.finalVerificationRequest(ctx, c, body, snapshotHash, decisionID, completionIDs)
	if err != nil {
		return err
	}
	_, err = v.cases.Close(ctx, request)
	if !errors.Is(err, domain.ErrIntentConflict) {
		return err
	}

	record, found, findErr := v.cases.FindVerification(ctx, c.ID)
	if findErr != nil {
		return findErr
	}
	if !found || record.SnapshotHash != snapshotHash || record.SnapshotJSON != string(body) {
		return err
	}
	verificationEvidenceID, found, findErr := v.verificationEvidenceIDFromRecord(ctx, record, body)
	if findErr != nil {
		return findErr
	}
	if !found {
		return errors.New("existing workflow verification has no matching verification evidence")
	}
	replayRequest := request
	replayRequest.EvidenceIDs = make([]domain.ID, 0, len(completionIDs)+2)
	replayRequest.EvidenceIDs = append(replayRequest.EvidenceIDs, completionIDs...)
	replayRequest.EvidenceIDs = append(replayRequest.EvidenceIDs, decisionID, verificationEvidenceID)
	_, replayErr := v.cases.Close(ctx, replayRequest)
	return replayErr
}

func (v *FinalVerifier) finalVerificationRequest(ctx context.Context, c workflowcase.Case, body []byte, snapshotHash string, decisionID domain.ID, completionIDs []domain.ID) (workflowcase.VerificationRequest, error) {
	verificationEvidenceID, err := v.finalVerificationEvidenceID(ctx, c.ID, body, snapshotHash)
	if err != nil {
		return workflowcase.VerificationRequest{}, err
	}
	evidenceIDs := make([]domain.ID, 0, len(completionIDs)+2)
	evidenceIDs = append(evidenceIDs, completionIDs...)
	evidenceIDs = append(evidenceIDs, decisionID, verificationEvidenceID)
	return workflowcase.VerificationRequest{
		CaseID: c.ID, VerifierID: v.config.VerifierID, VerifierType: v.config.VerifierType,
		SnapshotHash: snapshotHash, SnapshotJSON: string(body), EvidenceIDs: evidenceIDs,
	}, nil
}

func (v *FinalVerifier) finalVerificationEvidenceID(ctx context.Context, caseID domain.ID, body []byte, snapshotHash string) (domain.ID, error) {
	record, found, err := v.cases.FindVerification(ctx, caseID)
	if err != nil {
		return "", err
	}
	if found {
		if record.SnapshotHash != snapshotHash || record.SnapshotJSON != string(body) {
			return "", fmt.Errorf("%w: verification already exists for case %s", domain.ErrIntentConflict, caseID)
		}
		verificationEvidenceID, found, err := v.verificationEvidenceIDFromRecord(ctx, record, body)
		if err != nil {
			return "", err
		}
		if !found {
			return "", errors.New("existing workflow verification has no matching verification evidence")
		}
		return verificationEvidenceID, nil
	}
	object, found, err := v.evidence.FindByContentHash(ctx, snapshotHash, finalVerificationEvidenceKind)
	if err != nil {
		return "", err
	}
	if found {
		return object.ID, nil
	}
	object, err = v.evidence.Put(ctx, bytes.NewReader(body), evidence.Metadata{
		MediaType: "application/json", Kind: finalVerificationEvidenceKind,
	})
	if err != nil {
		return "", fmt.Errorf("store final verification evidence: %w", err)
	}
	return object.ID, nil
}

func (v *FinalVerifier) verificationEvidenceIDFromRecord(ctx context.Context, record workflowcase.VerificationRecord, body []byte) (domain.ID, bool, error) {
	for _, id := range record.EvidenceIDs {
		object, data, err := v.evidence.Get(ctx, id)
		if err != nil {
			return "", false, err
		}
		if object.Kind == finalVerificationEvidenceKind && bytes.Equal(data, body) {
			return id, true, nil
		}
	}
	return "", false, nil
}

func (v *FinalVerifier) reject(ctx context.Context, c workflowcase.Case, reason string, evidenceIDs []domain.ID) (finalVerificationDisposition, error) {
	_, err := v.cases.Reject(ctx, c.ID, reason, evidenceIDs)
	if err != nil {
		return finalVerificationHeld, err
	}
	return finalVerificationBlocked, nil
}

func (v *FinalVerifier) configured() error {
	if v == nil || v.cases == nil || v.execution == nil || v.evidence == nil || v.verification == nil {
		return errors.New("final verifier is not configured")
	}
	return nil
}
