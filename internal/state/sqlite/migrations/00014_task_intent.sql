-- +goose Up
ALTER TABLE tasks ADD COLUMN objective TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN payload_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE tasks ADD COLUMN idempotency_key TEXT;
ALTER TABLE tasks ADD COLUMN request_hash TEXT;
CREATE UNIQUE INDEX tasks_idempotency_key ON tasks(idempotency_key) WHERE idempotency_key IS NOT NULL;

-- +goose Down
DROP INDEX tasks_idempotency_key;
ALTER TABLE tasks DROP COLUMN request_hash;
ALTER TABLE tasks DROP COLUMN idempotency_key;
ALTER TABLE tasks DROP COLUMN payload_json;
ALTER TABLE tasks DROP COLUMN objective;
