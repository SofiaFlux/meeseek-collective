-- +goose Up
CREATE TABLE principals (
    principal_id TEXT PRIMARY KEY,
    principal_kind TEXT NOT NULL,
    public_key BLOB NOT NULL,
    custody_profile TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE constitutions (
    version INTEGER PRIMARY KEY,
    content BLOB NOT NULL,
    content_hash TEXT NOT NULL UNIQUE,
    signature BLOB NOT NULL,
    signer_principal_id TEXT NOT NULL REFERENCES principals(principal_id),
    active INTEGER NOT NULL CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL
);

CREATE UNIQUE INDEX constitutions_one_active
    ON constitutions(active)
    WHERE active = 1;

CREATE TABLE collective_metadata (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    collective_id TEXT NOT NULL UNIQUE,
    owner_principal_id TEXT NOT NULL REFERENCES principals(principal_id),
    cube_principal_id TEXT NOT NULL REFERENCES principals(principal_id),
    constitutional_root_principal_id TEXT NOT NULL REFERENCES principals(principal_id),
    active_constitution_version INTEGER NOT NULL REFERENCES constitutions(version),
    created_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE collective_metadata;
DROP INDEX constitutions_one_active;
DROP TABLE constitutions;
DROP TABLE principals;
