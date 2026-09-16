-- +goose Up
CREATE TABLE capability_definitions (
    capability_id TEXT PRIMARY KEY,
    semantic_name TEXT NOT NULL,
    semantic_version TEXT NOT NULL,
    provider TEXT NOT NULL,
    access_context TEXT NOT NULL,
    authority_requirements_json TEXT NOT NULL,
    minimum_enforcement TEXT NOT NULL CHECK (minimum_enforcement IN ('UNENFORCED', 'PARTIAL', 'ENFORCED')),
    active INTEGER NOT NULL CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX capability_definitions_one_active_name
    ON capability_definitions(semantic_name)
    WHERE active = 1;

CREATE TABLE capability_assessments (
    assessment_id TEXT PRIMARY KEY,
    capability_id TEXT NOT NULL REFERENCES capability_definitions(capability_id),
    provider TEXT NOT NULL,
    enforcement_level TEXT NOT NULL CHECK (enforcement_level IN ('UNENFORCED', 'PARTIAL', 'ENFORCED')),
    access_context TEXT NOT NULL,
    assessed_at TEXT NOT NULL,
    evidence_json TEXT NOT NULL,
    cost_metadata_json TEXT NOT NULL,
    health TEXT NOT NULL CHECK (health IN ('HEALTHY', 'DEGRADED', 'UNHEALTHY')),
    available INTEGER NOT NULL CHECK (available IN (0, 1))
);

CREATE INDEX capability_assessments_capability_time
    ON capability_assessments(capability_id, assessed_at DESC);

CREATE TABLE capability_sessions (
    session_id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    collective_id TEXT NOT NULL,
    task_id TEXT NOT NULL REFERENCES tasks(task_id),
    attempt_id TEXT NOT NULL REFERENCES attempts(attempt_id),
    fence_generation INTEGER NOT NULL,
    lease_expires_at TEXT NOT NULL,
    visible_capabilities_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    revoked_at TEXT
);

CREATE INDEX capability_sessions_attempt
    ON capability_sessions(attempt_id, revoked_at);

-- +goose Down
DROP INDEX capability_sessions_attempt;
DROP TABLE capability_sessions;
DROP INDEX capability_assessments_capability_time;
DROP TABLE capability_assessments;
DROP INDEX capability_definitions_one_active_name;
DROP TABLE capability_definitions;
