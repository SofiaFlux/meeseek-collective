-- +goose Up
CREATE TABLE collective_metadata (
    collective_id TEXT PRIMARY KEY,
    created_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE collective_metadata;
