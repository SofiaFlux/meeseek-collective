package execution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
)

func (s *Service) Task(ctx context.Context, taskID domain.ID) (domain.Task, error) {
	if err := s.configured(); err != nil {
		return domain.Task{}, err
	}
	taskID = domain.ID(strings.TrimSpace(string(taskID)))
	if taskID == "" {
		return domain.Task{}, errors.New("task id is required")
	}
	return loadTask(ctx, s.store.DB(), taskID)
}

func (s *Service) Attempt(ctx context.Context, attemptID domain.ID) (domain.Attempt, error) {
	if err := s.configured(); err != nil {
		return domain.Attempt{}, err
	}
	attemptID = domain.ID(strings.TrimSpace(string(attemptID)))
	if attemptID == "" {
		return domain.Attempt{}, errors.New("attempt id is required")
	}
	var attempt domain.Attempt
	var leaseExpires, startedAt string
	var completedAt sql.NullString
	err := s.store.DB().QueryRowContext(ctx, `
		SELECT attempt_id, task_id, state, fence_generation, lease_state, lease_expires_at,
		       executor_kind, started_at, completed_at
		FROM attempts WHERE attempt_id = ?`, attemptID,
	).Scan(
		&attempt.ID, &attempt.TaskID, &attempt.State, &attempt.FenceGeneration, &attempt.LeaseState,
		&leaseExpires, &attempt.ExecutorKind, &startedAt, &completedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Attempt{}, fmt.Errorf("attempt %q not found", attemptID)
	}
	if err != nil {
		return domain.Attempt{}, err
	}
	attempt.LeaseExpiresAt, err = time.Parse(time.RFC3339Nano, leaseExpires)
	if err != nil {
		return domain.Attempt{}, fmt.Errorf("parse attempt lease expiry: %w", err)
	}
	attempt.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return domain.Attempt{}, fmt.Errorf("parse attempt start time: %w", err)
	}
	if completedAt.Valid {
		attempt.CompletedAt, err = time.Parse(time.RFC3339Nano, completedAt.String)
		if err != nil {
			return domain.Attempt{}, fmt.Errorf("parse attempt completion time: %w", err)
		}
	}
	return attempt, nil
}
