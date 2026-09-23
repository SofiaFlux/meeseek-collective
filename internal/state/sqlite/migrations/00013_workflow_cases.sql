-- +goose Up
CREATE TABLE workflow_cases (
    case_id TEXT PRIMARY KEY,
    mission_id TEXT NOT NULL REFERENCES missions(mission_id),
    source TEXT NOT NULL,
    object_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    observation_evidence_id TEXT NOT NULL,
    initial_request_json TEXT NOT NULL,
    grant_json TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('ACTIVE', 'BLOCKED', 'READY_FOR_VERIFICATION')),
    current_work_id TEXT NOT NULL,
    next_work_json TEXT NOT NULL,
    completed_steps INTEGER NOT NULL,
    max_steps INTEGER NOT NULL,
    remaining_budget INTEGER NOT NULL,
    progress_signature TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (mission_id, source, object_id, revision_id)
);

CREATE INDEX workflow_cases_state_updated_at ON workflow_cases(state, updated_at);

-- +goose Down
DROP INDEX workflow_cases_state_updated_at;
DROP TABLE workflow_cases;
