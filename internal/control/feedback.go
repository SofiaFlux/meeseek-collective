package control

import (
	"context"
	"net/http"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/fieldfeedback"
)

type FeedbackCandidateDTO struct {
	ID                domain.ID                     `json:"id"`
	State             domain.FeedbackCandidateState `json:"state"`
	GenericTaskClass  domain.GenericTaskClass       `json:"generic_task_class"`
	Category          string                        `json:"category"`
	ExpectedBehavior  string                        `json:"expected_behavior,omitempty"`
	ObservedBehavior  string                        `json:"observed_behavior,omitempty"`
	HumanIntervention bool                          `json:"human_intervention"`
	RecoveryResult    string                        `json:"recovery_result,omitempty"`
	RuntimeVersion    string                        `json:"runtime_version,omitempty"`
	ExecutorKind      string                        `json:"executor_kind,omitempty"`
	ExecutorVersion   string                        `json:"executor_version,omitempty"`
	Enforcement       domain.EnforcementLevel       `json:"enforcement"`
	CorrelationKey    string                        `json:"correlation_key"`
}

type SanitizedFeedbackDTO struct {
	ID              domain.ID `json:"id"`
	CandidateID     domain.ID `json:"candidate_id"`
	SchemaVersion   int       `json:"schema_version"`
	ContentJSON     string    `json:"content_json"`
	ContentHash     string    `json:"content_hash"`
	CorrelationKey  string    `json:"correlation_key"`
	Fingerprint     string    `json:"fingerprint"`
}

type FeedbackInspectDTO struct {
	LocalCandidate *FeedbackCandidateDTO `json:"local_candidate,omitempty"`
	ExportArtifact *SanitizedFeedbackDTO `json:"export_artifact,omitempty"`
}

type FeedbackObserveRequest struct {
	TaskID       domain.ID               `json:"task_id,omitempty"`
	AttemptID    domain.ID               `json:"attempt_id,omitempty"`
	OperationID  domain.ID               `json:"operation_id,omitempty"`
	Category     string                  `json:"category"`
	SummaryLocal string                  `json:"summary_local"`
	Enforcement  domain.EnforcementLevel `json:"enforcement,omitempty"`
}

type FeedbackObservationDTO struct {
	ID         domain.ID `json:"id"`
	TaskID     domain.ID `json:"task_id,omitempty"`
	AttemptID  domain.ID `json:"attempt_id,omitempty"`
	OperationID domain.ID `json:"operation_id,omitempty"`
	Category   string    `json:"category"`
	SourceKind string    `json:"source_kind"`
}

type FeedbackEmitDTO struct {
	CandidateID domain.ID `json:"candidate_id"`
	ArtifactID  domain.ID `json:"artifact_id"`
	TaskID      domain.ID `json:"task_id"`
	Status      string    `json:"status"`
}

type FeedbackService interface {
	Candidates(context.Context, domain.FeedbackCandidateState) ([]domain.FeedbackCandidate, error)
	Candidate(context.Context, domain.ID) (domain.FeedbackCandidate, error)
	LatestSanitizedFeedbackForCandidate(context.Context, domain.ID) (domain.SanitizedFeedback, bool, error)
	RequestEmit(context.Context, domain.ID) (domain.Task, error)
}

type FieldObserver interface {
	Record(context.Context, fieldfeedback.ObservationInput) (domain.FieldObservation, error)
	Scan(context.Context) ([]domain.FieldObservation, error)
}

func (s *Server) handleFeedbackList(w http.ResponseWriter, r *http.Request) {
	if s.deps.Feedback == nil {
		writeError(w, http.StatusServiceUnavailable, "field feedback control is not configured")
		return
	}
	candidates, err := s.deps.Feedback.Candidates(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]FeedbackCandidateDTO, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, feedbackCandidateDTO(candidate))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleFeedbackInspect(w http.ResponseWriter, r *http.Request) {
	if s.deps.Feedback == nil {
		writeError(w, http.StatusServiceUnavailable, "field feedback control is not configured")
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	candidate, err := s.deps.Feedback.Candidate(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	local := feedbackCandidateDTO(candidate)
	result := FeedbackInspectDTO{LocalCandidate: &local}
	if artifact, found, err := s.deps.Feedback.LatestSanitizedFeedbackForCandidate(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if found {
		exported := sanitizedFeedbackDTO(artifact)
		result.ExportArtifact = &exported
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFeedbackEmit(w http.ResponseWriter, r *http.Request) {
	if s.deps.Feedback == nil {
		writeError(w, http.StatusServiceUnavailable, "field feedback control is not configured")
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	artifact, found, err := s.deps.Feedback.LatestSanitizedFeedbackForCandidate(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusConflict, "candidate has no sanitized export artifact")
		return
	}
	task, err := s.deps.Feedback.RequestEmit(r.Context(), artifact.ID)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, FeedbackEmitDTO{
		CandidateID: id, ArtifactID: artifact.ID, TaskID: task.ID, Status: "GOVERNED_WORK_SCHEDULED",
	})
}

func (s *Server) handleFeedbackObserve(w http.ResponseWriter, r *http.Request) {
	if s.deps.FieldObserver == nil {
		writeError(w, http.StatusServiceUnavailable, "field observation control is not configured")
		return
	}
	var request FeedbackObserveRequest
	if err := decodeBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.Enforcement == "" {
		request.Enforcement = domain.EnforcementEnforced
	}
	observation, err := s.deps.FieldObserver.Record(r.Context(), fieldfeedback.ObservationInput{
		TaskID: request.TaskID, AttemptID: request.AttemptID, OperationID: request.OperationID,
		Category: request.Category, BasisClass: "OPERATOR_OBSERVATION", SourceKind: "OPERATOR",
		SummaryLocal: request.SummaryLocal, Metrics: map[string]any{},
		Enforcement: request.Enforcement,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, feedbackObservationDTO(observation))
}

func (s *Server) handleFeedbackScan(w http.ResponseWriter, r *http.Request) {
	if s.deps.FieldObserver == nil {
		writeError(w, http.StatusServiceUnavailable, "field observation control is not configured")
		return
	}
	observations, err := s.deps.FieldObserver.Scan(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]FeedbackObservationDTO, 0, len(observations))
	for _, observation := range observations {
		out = append(out, feedbackObservationDTO(observation))
	}
	writeJSON(w, http.StatusOK, out)
}

func feedbackCandidateDTO(candidate domain.FeedbackCandidate) FeedbackCandidateDTO {
	return FeedbackCandidateDTO{
		ID: candidate.ID, State: candidate.State, GenericTaskClass: candidate.GenericTaskClass,
		Category: candidate.Category, ExpectedBehavior: candidate.ExpectedBehavior,
		ObservedBehavior: candidate.ObservedBehavior, HumanIntervention: candidate.HumanIntervention,
		RecoveryResult: candidate.RecoveryResult, RuntimeVersion: candidate.RuntimeVersion,
		ExecutorKind: candidate.ExecutorKind, ExecutorVersion: candidate.ExecutorVersion,
		Enforcement: candidate.Enforcement, CorrelationKey: candidate.CorrelationKey,
	}
}

func sanitizedFeedbackDTO(artifact domain.SanitizedFeedback) SanitizedFeedbackDTO {
	return SanitizedFeedbackDTO{
		ID: artifact.ID, CandidateID: artifact.CandidateID, SchemaVersion: artifact.SchemaVersion,
		ContentJSON: artifact.ContentJSON, ContentHash: artifact.ContentHash,
		CorrelationKey: artifact.CorrelationKey, Fingerprint: artifact.Fingerprint,
	}
}

func feedbackObservationDTO(observation domain.FieldObservation) FeedbackObservationDTO {
	return FeedbackObservationDTO{
		ID: observation.ID, TaskID: observation.TaskID, AttemptID: observation.AttemptID,
		OperationID: observation.OperationID, Category: observation.Category, SourceKind: observation.SourceKind,
	}
}
