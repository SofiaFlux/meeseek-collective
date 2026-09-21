-- +goose Up
CREATE TABLE attempt_run_manifests (
    attempt_id TEXT PRIMARY KEY REFERENCES attempts(attempt_id),
    task_id TEXT NOT NULL REFERENCES tasks(task_id),
    manifest_hash TEXT NOT NULL,
    manifest_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX attempt_run_manifests_task
    ON attempt_run_manifests(task_id, created_at);

-- +goose StatementBegin
CREATE TRIGGER attempt_run_manifests_no_insert_replace
BEFORE INSERT ON attempt_run_manifests
WHEN EXISTS (SELECT 1 FROM attempt_run_manifests WHERE attempt_id = NEW.attempt_id)
BEGIN
    SELECT RAISE(ABORT, 'attempt run manifests are immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER attempt_run_manifests_no_update
BEFORE UPDATE ON attempt_run_manifests
BEGIN
    SELECT RAISE(ABORT, 'attempt run manifests are immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER attempt_run_manifests_no_delete
BEFORE DELETE ON attempt_run_manifests
BEGIN
    SELECT RAISE(ABORT, 'attempt run manifests are immutable');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER attempt_run_manifests_no_delete;
DROP TRIGGER attempt_run_manifests_no_update;
DROP TRIGGER attempt_run_manifests_no_insert_replace;
DROP INDEX attempt_run_manifests_task;
DROP TABLE attempt_run_manifests;
