package workflowcase

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SofiaFlux/summa42/internal/clock"
	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/purpose"
	state "github.com/SofiaFlux/summa42/internal/state/sqlite"
	"github.com/SofiaFlux/summa42/internal/workflow"
)

type Observation struct {
	MissionID       domain.ID
	Source          string
	ObjectID        string
	RevisionID      string
	EvidenceID      string
	FirstWork       workflow.WorkProposal
	Grant           workflow.Grant
	MaxSteps        int
	RemainingBudget int64
}

type State string

const (
	Active               State = "ACTIVE"
	Blocked              State = "BLOCKED"
	ReadyForVerification State = "READY_FOR_VERIFICATION"
)

type Case struct {
	ID, MissionID, CurrentWorkID                        domain.ID
	Source, ObjectID, RevisionID, ObservationEvidenceID string
	State                                               State
	NextWork                                            workflow.WorkProposal
	Grant                                               workflow.Grant
	CompletedSteps, MaxSteps                            int
	RemainingBudget                                     int64
	ProgressSignature                                   string
}

type Service struct {
	store    *state.Store
	clock    clock.Clock
	purposes *purpose.Service
}

func New(store *state.Store, clk clock.Clock, purposes *purpose.Service) *Service {
	return &Service{store: store, clock: clk, purposes: purposes}
}

func (s *Service) Get(ctx context.Context, caseID domain.ID) (Case, error) {
	if s == nil || s.store == nil {
		return Case{}, errors.New("workflow case service is not configured")
	}
	if strings.TrimSpace(string(caseID)) == "" {
		return Case{}, errors.New("case ID is required")
	}
	row := s.store.DB().QueryRowContext(ctx, `SELECT case_id, mission_id, source, object_id, revision_id,
		observation_evidence_id, state, current_work_id, next_work_json, grant_json,
		completed_steps, max_steps, remaining_budget, progress_signature, initial_request_json
		FROM workflow_cases WHERE case_id = ?`, caseID)
	c, _, err := scanCase(row)
	if err != nil {
		return Case{}, fmt.Errorf("get workflow case: %w", err)
	}
	return c, nil
}

func (s *Service) Ensure(ctx context.Context, observation Observation) (Case, error) {
	if s == nil || s.store == nil || s.clock == nil || s.purposes == nil {
		return Case{}, errors.New("workflow case service is not configured")
	}
	if strings.TrimSpace(string(observation.MissionID)) == "" ||
		strings.TrimSpace(observation.Source) == "" ||
		strings.TrimSpace(observation.ObjectID) == "" ||
		strings.TrimSpace(observation.RevisionID) == "" ||
		strings.TrimSpace(observation.EvidenceID) == "" ||
		strings.TrimSpace(observation.FirstWork.Kind) == "" ||
		observation.MaxSteps <= 0 || observation.RemainingBudget <= 0 {
		return Case{}, errors.New("observation identity, evidence, first work, and positive limits are required")
	}
	decision, err := workflow.Decide(workflow.Input{
		Assessment: workflow.Assessment{
			Verdict: workflow.Continue, EvidenceIDs: []string{observation.EvidenceID}, Next: &observation.FirstWork,
		},
		Grant:          observation.Grant,
		Limits:         workflow.Limits{MaxSteps: observation.MaxSteps, RemainingBudget: observation.RemainingBudget},
		CompletedSteps: 0,
	})
	if err != nil {
		return Case{}, fmt.Errorf("invalid first work: %w", err)
	}
	if decision.Outcome != workflow.OutcomeContinue {
		return Case{}, fmt.Errorf("first work decision is %s", decision.Outcome)
	}
	requestJSON, err := json.Marshal(observation)
	if err != nil {
		return Case{}, fmt.Errorf("encode observation: %w", err)
	}
	grantJSON, err := json.Marshal(observation.Grant)
	if err != nil {
		return Case{}, fmt.Errorf("encode grant: %w", err)
	}
	workJSON, err := json.Marshal(observation.FirstWork)
	if err != nil {
		return Case{}, fmt.Errorf("encode first work: %w", err)
	}
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	var result Case
	err = s.store.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.purposes.ValidatePurposeTx(ctx, tx, domain.PurposeRef{Kind: domain.PurposeMission, ID: observation.MissionID}); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO workflow_cases (
			case_id, mission_id, source, object_id, revision_id, observation_evidence_id,
			initial_request_json, grant_json, state, current_work_id, next_work_json,
			completed_steps, max_steps, remaining_budget, progress_signature, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(mission_id, source, object_id, revision_id) DO NOTHING`,
			domain.NewID("case"), observation.MissionID, observation.Source, observation.ObjectID,
			observation.RevisionID, observation.EvidenceID, string(requestJSON), string(grantJSON), Active,
			domain.NewID("work"), string(workJSON), 0, observation.MaxSteps, observation.RemainingBudget, "", now, now,
		)
		if err != nil {
			return err
		}
		var storedRequest string
		row := tx.QueryRowContext(ctx, `SELECT case_id, mission_id, source, object_id, revision_id,
			observation_evidence_id, state, current_work_id, next_work_json, grant_json,
			completed_steps, max_steps, remaining_budget, progress_signature, initial_request_json
			FROM workflow_cases WHERE mission_id = ? AND source = ? AND object_id = ? AND revision_id = ?`,
			observation.MissionID, observation.Source, observation.ObjectID, observation.RevisionID)
		result, storedRequest, err = scanCase(row)
		if err != nil {
			return err
		}
		if storedRequest != string(requestJSON) {
			return errors.New("observation revision already exists with a different initial request")
		}
		return nil
	})
	if err != nil {
		return Case{}, fmt.Errorf("ensure workflow case: %w", err)
	}
	return result, nil
}

type rowScanner interface{ Scan(...any) error }

func scanCase(row rowScanner) (Case, string, error) {
	var c Case
	var workJSON, grantJSON, requestJSON string
	err := row.Scan(&c.ID, &c.MissionID, &c.Source, &c.ObjectID, &c.RevisionID,
		&c.ObservationEvidenceID, &c.State, &c.CurrentWorkID, &workJSON, &grantJSON,
		&c.CompletedSteps, &c.MaxSteps, &c.RemainingBudget, &c.ProgressSignature, &requestJSON)
	if err != nil {
		return Case{}, "", err
	}
	if err := json.Unmarshal([]byte(workJSON), &c.NextWork); err != nil {
		return Case{}, "", fmt.Errorf("decode next work: %w", err)
	}
	if err := json.Unmarshal([]byte(grantJSON), &c.Grant); err != nil {
		return Case{}, "", fmt.Errorf("decode grant: %w", err)
	}
	return c, requestJSON, nil
}
