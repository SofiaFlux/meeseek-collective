-- +goose NO TRANSACTION
-- +goose Up
PRAGMA foreign_keys = OFF;

CREATE TABLE workflow_cases_new (
    case_id TEXT PRIMARY KEY,
    mission_id TEXT NOT NULL REFERENCES missions(mission_id),
    source TEXT NOT NULL,
    object_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    observation_evidence_id TEXT NOT NULL,
    initial_request_json TEXT NOT NULL,
    grant_json TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('ACTIVE', 'BLOCKED', 'READY_FOR_VERIFICATION', 'CLOSED')),
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

INSERT INTO workflow_cases_new (
    case_id, mission_id, source, object_id, revision_id, observation_evidence_id,
    initial_request_json, grant_json, state, current_work_id, next_work_json,
    completed_steps, max_steps, remaining_budget, progress_signature, created_at, updated_at
)
SELECT
    case_id, mission_id, source, object_id, revision_id, observation_evidence_id,
    initial_request_json, grant_json, state, current_work_id, next_work_json,
    completed_steps, max_steps, remaining_budget, progress_signature, created_at, updated_at
FROM workflow_cases;

DROP TABLE workflow_cases;
ALTER TABLE workflow_cases_new RENAME TO workflow_cases;
CREATE INDEX workflow_cases_state_updated_at ON workflow_cases(state, updated_at);

CREATE TABLE workflow_verifications (
    verification_id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL UNIQUE REFERENCES workflow_cases(case_id),
    verifier_id TEXT NOT NULL,
    verifier_type TEXT NOT NULL,
    snapshot_hash TEXT NOT NULL,
    snapshot_json TEXT NOT NULL,
    evidence_ids_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
PRAGMA foreign_keys = ON;

-- +goose Down
CREATE TEMP TABLE IF NOT EXISTS workflow_00015_closed_case_guard (
    closed_case_count INTEGER NOT NULL
);

-- +goose StatementBegin
CREATE TEMP TRIGGER IF NOT EXISTS workflow_00015_closed_case_guard
BEFORE INSERT ON workflow_00015_closed_case_guard
WHEN EXISTS (SELECT 1 FROM workflow_cases WHERE state = 'CLOSED')
BEGIN
    SELECT RAISE(ABORT, 'cannot roll back 00015: closed workflow cases exist');
END;
-- +goose StatementEnd

INSERT INTO workflow_00015_closed_case_guard (closed_case_count)
SELECT count(*) FROM workflow_cases WHERE state = 'CLOSED';
DROP TABLE workflow_00015_closed_case_guard;

PRAGMA foreign_keys = OFF;
DROP TABLE workflow_verifications;

CREATE TABLE workflow_cases_old (
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

INSERT INTO workflow_cases_old (
    case_id, mission_id, source, object_id, revision_id, observation_evidence_id,
    initial_request_json, grant_json, state, current_work_id, next_work_json,
    completed_steps, max_steps, remaining_budget, progress_signature, created_at, updated_at
)
SELECT
    case_id, mission_id, source, object_id, revision_id, observation_evidence_id,
    initial_request_json, grant_json, state, current_work_id, next_work_json,
    completed_steps, max_steps, remaining_budget, progress_signature, created_at, updated_at
FROM workflow_cases;

DROP TABLE workflow_cases;
ALTER TABLE workflow_cases_old RENAME TO workflow_cases;
CREATE INDEX workflow_cases_state_updated_at ON workflow_cases(state, updated_at);
PRAGMA foreign_keys = ON;
