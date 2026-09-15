-- +goose Up
CREATE TABLE missions (
    mission_id TEXT PRIMARY KEY,
    statement TEXT NOT NULL,
    active INTEGER NOT NULL CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    deactivated_at TEXT
);

CREATE UNIQUE INDEX missions_one_active
    ON missions(active)
    WHERE active = 1;

CREATE TABLE obligations (
    obligation_id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    statement TEXT NOT NULL,
    active INTEGER NOT NULL CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    resolved_at TEXT
);

CREATE TABLE goals (
    goal_id TEXT PRIMARY KEY,
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
    parent_goal_id TEXT REFERENCES goals(goal_id),
    statement TEXT NOT NULL,
    active INTEGER NOT NULL CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    retired_at TEXT
);

CREATE INDEX goals_active_purpose
    ON goals(purpose_kind, purpose_id, active);

CREATE INDEX goals_parent
    ON goals(parent_goal_id);

-- +goose Down
DROP INDEX goals_parent;
DROP INDEX goals_active_purpose;
DROP TABLE goals;
DROP TABLE obligations;
DROP INDEX missions_one_active;
DROP TABLE missions;
