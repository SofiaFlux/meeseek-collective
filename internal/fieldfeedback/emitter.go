package fieldfeedback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/execution"
	"github.com/SofiaFlux/meeseek-collective/internal/localconfig"
)

var ErrNoMaintenanceBudget = errors.New("field feedback maintenance budget is unavailable")

type emissionRuntime struct {
	execution *execution.Service
	config    localconfig.FieldFeedbackConfig
	ownerID   domain.ID
}

func (s *Feedback) ConfigureEmission(
	executionSvc *execution.Service,
	cfg localconfig.FieldFeedbackConfig,
	ownerID domain.ID,
) error {
	if s == nil {
		return errors.New("feedback service is not configured")
	}
	if cfg.Mode == "" {
		cfg.Mode = localconfig.FeedbackModeLocalOnly
	}
	if cfg.RequiredEnforcement == "" {
		cfg.RequiredEnforcement = domain.EnforcementEnforced
	}
	cfg.Provider = strings.TrimSpace(cfg.Provider)
	cfg.Destination = strings.TrimSpace(cfg.Destination)
	ownerID = domain.ID(strings.TrimSpace(string(ownerID)))
	if err := cfg.Validate(); err != nil {
		return err
	}
	if cfg.Enabled && cfg.Mode != localconfig.FeedbackModeLocalOnly && executionSvc == nil {
		return errors.New("exporting field feedback requires execution service")
	}
	if cfg.Enabled && cfg.Mode == localconfig.FeedbackModeRequireApproval && ownerID == "" {
		return errors.New("REQUIRE_APPROVAL field feedback requires Owner principal id")
	}
	s.emission = &emissionRuntime{execution: executionSvc, config: cfg, ownerID: ownerID}
	return nil
}

func (s *Feedback) OnSanitized(ctx context.Context, feedbackID domain.ID) error {
	if err := s.configured(); err != nil {
		return err
	}
	artifact, err := s.SanitizedFeedback(ctx, feedbackID)
	if err != nil {
		return err
	}
	candidate, err := s.Candidate(ctx, artifact.CandidateID)
	if err != nil {
		return err
	}
	runtime := s.emission
	if runtime == nil || !runtime.config.Enabled || runtime.config.Mode == localconfig.FeedbackModeLocalOnly {
		if candidate.State == domain.FeedbackStateLocalOnly {
			return nil
		}
		return s.transitionCandidate(ctx, candidate.ID, domain.FeedbackStateLocalOnly)
	}
	if candidate.State == domain.FeedbackStateSanitized {
		if err := s.transitionCandidate(ctx, candidate.ID, domain.FeedbackStateExportReady); err != nil {
			return err
		}
	} else if candidate.State != domain.FeedbackStateExportReady {
		return fmt.Errorf("sanitized candidate %s is in state %s, not exportable", candidate.ID, candidate.State)
	}
	_, err = s.RequestEmit(ctx, feedbackID)
	return err
}

func (s *Feedback) RequestEmit(ctx context.Context, feedbackID domain.ID) (domain.Task, error) {
	if err := s.configured(); err != nil {
		return domain.Task{}, err
	}
	runtime := s.emission
	if runtime == nil || !runtime.config.Enabled {
		return domain.Task{}, errors.New("field feedback emission is disabled")
	}
	if runtime.config.Mode == localconfig.FeedbackModeLocalOnly {
		return domain.Task{}, errors.New("LOCAL_ONLY field feedback cannot schedule emission")
	}
	if runtime.execution == nil {
		return domain.Task{}, errors.New("feedback execution service is not configured")
	}
	artifact, err := s.SanitizedFeedback(ctx, feedbackID)
	if err != nil {
		return domain.Task{}, err
	}
	candidate, err := s.Candidate(ctx, artifact.CandidateID)
	if err != nil {
		return domain.Task{}, err
	}
	if candidate.State == domain.FeedbackStateSanitized {
		if err := s.transitionCandidate(ctx, candidate.ID, domain.FeedbackStateExportReady); err != nil {
			return domain.Task{}, err
		}
	} else if candidate.State != domain.FeedbackStateExportReady && candidate.State != domain.FeedbackStateApprovalPending {
		return domain.Task{}, fmt.Errorf("candidate %s is %s, not ready for emission", candidate.ID, candidate.State)
	}

	if task, found, err := s.emissionTask(ctx, feedbackID); err != nil {
		return domain.Task{}, err
	} else if found {
		return task, nil
	}

	available, err := s.maintenanceBudgetAvailable(ctx, runtime.config.MaintenanceEnvelopeID)
	if err != nil {
		return domain.Task{}, err
	}
	if !available {
		return domain.Task{}, ErrNoMaintenanceBudget
	}

	logicalKey := emissionLogicalKey(runtime.config.Provider, runtime.config.Destination, artifact.Fingerprint)
	if task, found, err := s.logicalEmitTask(ctx, logicalKey); err != nil {
		return domain.Task{}, err
	} else if found {
		linked, err := s.taskHasEmission(ctx, task.ID)
		if err != nil {
			return domain.Task{}, err
		}
		if !linked {
			if err := s.linkEmission(ctx, artifact, task.ID); err != nil {
				return domain.Task{}, err
			}
		}
		return task, nil
	}

	capability := feedbackCapability(runtime.config.Provider)
	task, err := runtime.execution.CreateTask(ctx, execution.TaskRequest{
		Purpose: domain.PurposeRef{
			Kind: domain.PurposeCollectiveMaintenance,
			ID:   logicalKey,
		},
		TaskClass: "collective.feedback.emit",
		AcceptanceCriteria: []string{
			"sanitized feedback artifact is emitted through the protected external-operation boundary and reconciled",
		},
		RequiredCapabilities: []string{capability},
		RequiredEnforcement:  runtime.config.RequiredEnforcement,
		AuthorityCeiling:     []string{capability},
		ResourceEnvelopeID:   runtime.config.MaintenanceEnvelopeID,
	})
	if err != nil {
		// The partial unique index is the concurrency/crash-safe logical idempotency gate.
		existing, found, loadErr := s.logicalEmitTask(ctx, logicalKey)
		if loadErr != nil {
			return domain.Task{}, loadErr
		}
		if !found {
			return domain.Task{}, err
		}
		task = existing
	}
	if err := s.linkEmission(ctx, artifact, task.ID); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func (s *Feedback) FeedbackForEmitTask(
	ctx context.Context,
	taskID domain.ID,
) (domain.SanitizedFeedback, string, []domain.ID, error) {
	if err := s.configured(); err != nil {
		return domain.SanitizedFeedback{}, "", nil, err
	}
	taskID = domain.ID(strings.TrimSpace(string(taskID)))
	if taskID == "" {
		return domain.SanitizedFeedback{}, "", nil, errors.New("emit task id is required")
	}
	var feedbackID domain.ID
	var destination, requiredJSON string
	err := s.store.DB().QueryRowContext(ctx,
		"SELECT feedback_id, destination, required_approvers_json FROM feedback_emissions WHERE task_id = ?",
		taskID,
	).Scan(&feedbackID, &destination, &requiredJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SanitizedFeedback{}, "", nil, fmt.Errorf("task %s has no feedback emission linkage", taskID)
	}
	if err != nil {
		return domain.SanitizedFeedback{}, "", nil, err
	}
	var required []domain.ID
	if err := json.Unmarshal([]byte(requiredJSON), &required); err != nil {
		return domain.SanitizedFeedback{}, "", nil, fmt.Errorf("decode emission required approvers: %w", err)
	}
	artifact, err := s.SanitizedFeedback(ctx, feedbackID)
	if err != nil {
		return domain.SanitizedFeedback{}, "", nil, err
	}
	return artifact, destination, cleanIDs(required), nil
}

func (s *Feedback) ValidateEmitAttempt(ctx context.Context, taskID, attemptID domain.ID) error {
	taskID = domain.ID(strings.TrimSpace(string(taskID)))
	attemptID = domain.ID(strings.TrimSpace(string(attemptID)))
	if taskID == "" || attemptID == "" {
		return errors.New("emit task and attempt ids are required")
	}
	var attemptTaskID domain.ID
	if err := s.store.DB().QueryRowContext(ctx, "SELECT task_id FROM attempts WHERE attempt_id = ?", attemptID).Scan(&attemptTaskID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("emit attempt %s does not exist", attemptID)
		}
		return err
	}
	if attemptTaskID != taskID {
		return fmt.Errorf("emit attempt %s does not belong to task %s", attemptID, taskID)
	}
	return nil
}

func (s *Feedback) emissionTask(ctx context.Context, feedbackID domain.ID) (domain.Task, bool, error) {
	var taskID domain.ID
	err := s.store.DB().QueryRowContext(ctx,
		"SELECT task_id FROM feedback_emissions WHERE feedback_id = ?", feedbackID,
	).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Task{}, false, nil
	}
	if err != nil {
		return domain.Task{}, false, err
	}
	task, err := s.emission.execution.Task(ctx, taskID)
	return task, err == nil, err
}

func (s *Feedback) logicalEmitTask(ctx context.Context, feedbackID domain.ID) (domain.Task, bool, error) {
	var taskID domain.ID
	err := s.store.DB().QueryRowContext(ctx,
		"SELECT task_id FROM tasks WHERE purpose_kind = ? AND purpose_id = ? AND task_class = 'collective.feedback.emit' ORDER BY created_at LIMIT 1",
		domain.PurposeCollectiveMaintenance, feedbackID,
	).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Task{}, false, nil
	}
	if err != nil {
		return domain.Task{}, false, err
	}
	task, err := s.emission.execution.Task(ctx, taskID)
	return task, err == nil, err
}

func (s *Feedback) taskHasEmission(ctx context.Context, taskID domain.ID) (bool, error) {
	var one int
	err := s.store.DB().QueryRowContext(ctx,
		"SELECT 1 FROM feedback_emissions WHERE task_id = ?", taskID,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func emissionLogicalKey(provider, destination, fingerprint string) domain.ID {
	return domain.ID("feedback:" + strings.TrimSpace(provider) + ":" + strings.TrimSpace(destination) + ":" + strings.TrimSpace(fingerprint))
}

func (s *Feedback) linkEmission(ctx context.Context, artifact domain.SanitizedFeedback, taskID domain.ID) error {
	runtime := s.emission
	required := []domain.ID{}
	if runtime.config.Mode == localconfig.FeedbackModeRequireApproval {
		required = []domain.ID{runtime.ownerID}
	}
	requiredJSON, err := json.Marshal(cleanIDs(required))
	if err != nil {
		return err
	}
	_, err = s.store.DB().ExecContext(ctx,
		"INSERT OR IGNORE INTO feedback_emissions("+
			"feedback_id, task_id, provider, destination, generic_task_class, required_approvers_json, created_at"+
			") VALUES (?, ?, ?, ?, ?, ?, ?)",
		artifact.ID, taskID, runtime.config.Provider, runtime.config.Destination,
		domain.GenericTaskMaintenance, string(requiredJSON), formatTime(s.clock.Now().UTC()),
	)
	if err != nil {
		return fmt.Errorf("link feedback emission: %w", err)
	}
	var linkedTask domain.ID
	if err := s.store.DB().QueryRowContext(ctx,
		"SELECT task_id FROM feedback_emissions WHERE feedback_id = ?", artifact.ID,
	).Scan(&linkedTask); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		linked, linkErr := s.taskHasEmission(ctx, taskID)
		if linkErr != nil {
			return linkErr
		}
		if linked {
			return nil
		}
		return err
	}
	if linkedTask != taskID {
		return fmt.Errorf("feedback %s is already linked to a different emit task %s", artifact.ID, linkedTask)
	}
	return nil
}

func (s *Feedback) maintenanceBudgetAvailable(ctx context.Context, envelopeID domain.ID) (bool, error) {
	envelopeID = domain.ID(strings.TrimSpace(string(envelopeID)))
	if envelopeID == "" {
		return false, nil
	}
	var available int64
	err := s.store.DB().QueryRowContext(ctx,
		"SELECT e.hard_limit - "+
			"COALESCE(SUM(CASE WHEN r.state = 'SETTLED' THEN r.settled_amount ELSE 0 END), 0) - "+
			"COALESCE(SUM(CASE WHEN r.state IN ('HELD','UNRESOLVED') THEN r.reserved_amount ELSE 0 END), 0) "+
			"FROM resource_envelopes e LEFT JOIN resource_reservations r ON r.envelope_id = e.envelope_id "+
			"WHERE e.envelope_id = ? GROUP BY e.envelope_id, e.hard_limit",
		envelopeID,
	).Scan(&available)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return available > 0, nil
}

func feedbackCapability(provider string) string {
	provider = strings.TrimSpace(provider)
	return "feedback." + provider + ".issue.create"
}

func (s *Feedback) MarkReported(ctx context.Context, feedbackID domain.ID, operationID domain.ID, providerReference string) error {
	if err := s.configured(); err != nil {
		return err
	}
	artifact, err := s.SanitizedFeedback(ctx, feedbackID)
	if err != nil {
		return err
	}
	candidate, err := s.Candidate(ctx, artifact.CandidateID)
	if err != nil {
		return err
	}
	if candidate.State != domain.FeedbackStateReported {
		if err := s.transitionCandidate(ctx, candidate.ID, domain.FeedbackStateReported); err != nil {
			return err
		}
	}
	payload, err := json.Marshal(map[string]any{
		"candidate_id":       candidate.ID,
		"feedback_id":        artifact.ID,
		"operation_id":       operationID,
		"provider_reference": strings.TrimSpace(providerReference),
	})
	if err != nil {
		return err
	}
	_, err = s.store.DB().ExecContext(ctx,
		"INSERT INTO audit_events(audit_id, kind, actor_id, subject_id, payload_json, created_at) VALUES (?, 'FEEDBACK_REPORTED', 'feedback-emitter', ?, ?, ?)",
		domain.NewID("audit"), candidate.ID, string(payload), formatTime(s.clock.Now().UTC()),
	)
	return err
}
