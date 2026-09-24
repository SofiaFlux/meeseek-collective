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

type AssessmentRecord struct {
	ID, WorkID  string
	RequestJSON string
	ResultJSON  string
	CreatedAt   time.Time
}

const caseColumns = `case_id, mission_id, source, object_id, revision_id,
		observation_evidence_id, state, current_work_id, next_work_json, grant_json,
		completed_steps, max_steps, remaining_budget, progress_signature, initial_request_json`

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
	row := s.store.DB().QueryRowContext(ctx, `SELECT `+caseColumns+` FROM workflow_cases WHERE case_id = ?`, caseID)
	c, _, err := scanCase(row)
	if err != nil {
		return Case{}, fmt.Errorf("get workflow case: %w", err)
	}
	return c, nil
}

func (s *Service) ListActive(ctx context.Context, missionID domain.ID) ([]Case, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("workflow case service is not configured")
	}
	missionID = domain.ID(strings.TrimSpace(string(missionID)))
	if missionID == "" {
		return nil, errors.New("mission ID is required")
	}
	rows, err := s.store.DB().QueryContext(ctx,
		`SELECT `+caseColumns+` FROM workflow_cases WHERE state = ? AND mission_id = ? ORDER BY case_id`,
		Active, missionID)
	if err != nil {
		return nil, fmt.Errorf("list active workflow cases: %w", err)
	}
	defer rows.Close()
	cases := make([]Case, 0)
	for rows.Next() {
		c, _, err := scanCase(rows)
		if err != nil {
			return nil, fmt.Errorf("scan active workflow case: %w", err)
		}
		cases = append(cases, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list active workflow cases: %w", err)
	}
	return cases, nil
}

func (s *Service) ListAssessments(ctx context.Context, caseID domain.ID) ([]AssessmentRecord, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("workflow case service is not configured")
	}
	caseID = domain.ID(strings.TrimSpace(string(caseID)))
	if caseID == "" {
		return nil, errors.New("case ID is required")
	}
	rows, err := s.store.DB().QueryContext(ctx,
		`SELECT assessment_id, work_id, request_json, result_json, created_at
		 FROM workflow_assessments WHERE case_id = ? ORDER BY created_at, assessment_id`, caseID)
	if err != nil {
		return nil, fmt.Errorf("list workflow assessments: %w", err)
	}
	defer rows.Close()
	records := make([]AssessmentRecord, 0)
	for rows.Next() {
		var record AssessmentRecord
		var createdAt string
		if err := rows.Scan(&record.ID, &record.WorkID, &record.RequestJSON, &record.ResultJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("scan workflow assessment: %w", err)
		}
		record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse workflow assessment %s created_at: %w", record.ID, err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow assessments: %w", err)
	}
	return records, nil
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
		row := tx.QueryRowContext(ctx, `SELECT `+caseColumns+` FROM workflow_cases WHERE mission_id = ? AND source = ? AND object_id = ? AND revision_id = ?`,
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

func (s *Service) Find(ctx context.Context, missionID domain.ID, source, objectID, revisionID string) (Case, bool, error) {
	if s == nil || s.store == nil {
		return Case{}, false, errors.New("workflow case service is not configured")
	}
	row := s.store.DB().QueryRowContext(ctx, `SELECT `+caseColumns+` FROM workflow_cases WHERE mission_id = ? AND source = ? AND object_id = ? AND revision_id = ?`,
		missionID, source, objectID, revisionID)
	c, _, err := scanCase(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Case{}, false, nil
	}
	if err != nil {
		return Case{}, false, fmt.Errorf("find workflow case: %w", err)
	}
	return c, true, nil
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
