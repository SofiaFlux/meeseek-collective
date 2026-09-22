-- +goose Up
CREATE TABLE tasks (
    task_id TEXT PRIMARY KEY,
    parent_task_id TEXT REFERENCES tasks(task_id),
    purpose_kind TEXT NOT NULL CHECK (purpose_kind IN (
        'MISSION',
        'OBLIGATION',
        'COLLECTIVE_MAINTENANCE',
        'GOVERNANCE',
        'STRATEGIC_PULSE',
        'RECOVERY',
        'OWNER_DIRECTIVE'
    )),
    purpose_id TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN (
        'CREATED', 'ELIGIBLE', 'EXECUTING', 'AWAITING_VERIFICATION',
        'SUCCEEDED', 'FAILED', 'BLOCKED', 'CANCELLED', 'CHALLENGED', 'EXPIRED'
    )),
    current_attempt_id TEXT,
    current_fence INTEGER NOT NULL DEFAULT 0 CHECK (current_fence >= 0),
    acceptance_criteria_json TEXT NOT NULL,
    required_capabilities_json TEXT NOT NULL,
    required_enforcement TEXT NOT NULL CHECK (required_enforcement IN ('ENFORCED', 'PARTIAL', 'UNENFORCED')),
    authority_ceiling_json TEXT NOT NULL,
    resource_envelope_id TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    earliest_start TEXT,
    deadline TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX tasks_state_priority
    ON tasks(state, priority DESC, created_at);

CREATE INDEX tasks_purpose
    ON tasks(purpose_kind, purpose_id, state);

CREATE TABLE task_dependencies (
    task_id TEXT NOT NULL REFERENCES tasks(task_id) ON DELETE CASCADE,
    depends_on_task_id TEXT NOT NULL REFERENCES tasks(task_id),
    PRIMARY KEY (task_id, depends_on_task_id),
    CHECK (task_id <> depends_on_task_id)
);

CREATE TABLE attempts (
    attempt_id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(task_id),
    state TEXT NOT NULL CHECK (state IN ('LEASED', 'RUNNING', 'COMPLETED', 'FAILED', 'CANCELLED', 'EXPIRED')),
    fence_generation INTEGER NOT NULL CHECK (fence_generation > 0),
    lease_state TEXT NOT NULL CHECK (lease_state IN ('ACTIVE', 'REVOKED', 'EXPIRED')),
    lease_expires_at TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT,
    executor_kind TEXT NOT NULL
);

CREATE INDEX attempts_task_generation
    ON attempts(task_id, fence_generation DESC);

CREATE TABLE task_challenges (
    challenge_id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(task_id),
    scope TEXT NOT NULL CHECK (scope IN ('TASK', 'PARENT', 'GOAL', 'MISSION_ASSUMPTION')),
    reason TEXT NOT NULL,
    evidence_ids_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX task_challenges_task
    ON task_challenges(task_id, created_at);

CREATE TABLE attempt_failures (
    failure_id TEXT PRIMARY KEY,
    attempt_id TEXT NOT NULL REFERENCES attempts(attempt_id),
    task_id TEXT NOT NULL REFERENCES tasks(task_id),
    failure_class TEXT NOT NULL CHECK (failure_class IN (
        'TRANSIENT', 'CAPABILITY', 'EPISTEMIC', 'PLANNING', 'RESOURCE',
        'AUTHORITY', 'OBJECTIVE_IMPOSSIBLE', 'EXECUTION'
    )),
    signature TEXT NOT NULL,
    evidence_ids_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX attempt_failures_task_signature
    ON attempt_failures(task_id, signature, created_at);

CREATE TABLE execution_events (
    event_id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(task_id),
    attempt_id TEXT REFERENCES attempts(attempt_id),
    event_type TEXT NOT NULL,
    details_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX execution_events_task
    ON execution_events(task_id, created_at);

-- +goose Down
DROP INDEX execution_events_task;
DROP TABLE execution_events;
DROP INDEX attempt_failures_task_signature;
DROP TABLE attempt_failures;
DROP INDEX task_challenges_task;
DROP TABLE task_challenges;
DROP INDEX attempts_task_generation;
DROP TABLE attempts;
DROP TABLE task_dependencies;
DROP INDEX tasks_purpose;
DROP INDEX tasks_state_priority;
DROP TABLE tasks;
