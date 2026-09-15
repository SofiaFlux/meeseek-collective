-- +goose Up
CREATE TABLE evidence_objects (
    evidence_id TEXT PRIMARY KEY,
    content_hash TEXT NOT NULL,
    media_type TEXT NOT NULL,
    kind TEXT NOT NULL,
    size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
    created_at TEXT NOT NULL
);

CREATE INDEX evidence_objects_content_hash
    ON evidence_objects(content_hash);

CREATE TABLE attempt_completion_records (
    completion_id TEXT PRIMARY KEY,
    attempt_id TEXT NOT NULL UNIQUE REFERENCES attempts(attempt_id),
    task_id TEXT NOT NULL REFERENCES tasks(task_id),
    manifest_hash TEXT NOT NULL,
    manifest_json TEXT NOT NULL,
    completed_at TEXT NOT NULL
);

CREATE INDEX attempt_completion_records_task
    ON attempt_completion_records(task_id, completed_at);

CREATE TABLE verification_work (
    verification_id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(task_id),
    attempt_id TEXT NOT NULL UNIQUE REFERENCES attempts(attempt_id),
    completion_id TEXT NOT NULL UNIQUE REFERENCES attempt_completion_records(completion_id),
    state TEXT NOT NULL CHECK (state IN ('PENDING', 'ACCEPTED', 'REJECTED')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX verification_work_task_state
    ON verification_work(task_id, state, created_at);

CREATE TABLE acceptance_records (
    acceptance_id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL UNIQUE REFERENCES tasks(task_id),
    attempt_id TEXT NOT NULL REFERENCES attempts(attempt_id),
    verifier_id TEXT NOT NULL,
    verifier_type TEXT NOT NULL,
    criteria_result_json TEXT NOT NULL,
    evidence_ids_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX acceptance_records_attempt
    ON acceptance_records(attempt_id, created_at);

-- +goose Down
DROP INDEX acceptance_records_attempt;
DROP TABLE acceptance_records;
DROP INDEX verification_work_task_state;
DROP TABLE verification_work;
DROP INDEX attempt_completion_records_task;
DROP TABLE attempt_completion_records;
DROP INDEX evidence_objects_content_hash;
DROP TABLE evidence_objects;
