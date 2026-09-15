-- +goose Up
CREATE TABLE resource_envelopes (
    envelope_id TEXT PRIMARY KEY,
    parent_envelope_id TEXT REFERENCES resource_envelopes(envelope_id),
    hard_limit INTEGER NOT NULL CHECK (hard_limit >= 0),
    created_at TEXT NOT NULL
);

CREATE TABLE resource_reservations (
    reservation_id TEXT PRIMARY KEY,
    envelope_id TEXT NOT NULL REFERENCES resource_envelopes(envelope_id),
    state TEXT NOT NULL CHECK (state IN ('HELD', 'SETTLED', 'RELEASED', 'UNRESOLVED')),
    reserved_amount INTEGER NOT NULL CHECK (reserved_amount > 0),
    settled_amount INTEGER CHECK (settled_amount IS NULL OR settled_amount >= 0),
    cost_control TEXT NOT NULL CHECK (cost_control IN (
        'TECHNICALLY_CAPPED',
        'PREPAID_QUOTA',
        'VERIFIED_STOP',
        'ESTIMATED_ONLY',
        'POTENTIALLY_UNBOUNDED'
    )),
    cost_source TEXT NOT NULL,
    require_hard_cap INTEGER NOT NULL CHECK (require_hard_cap IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX resource_reservations_envelope_state
    ON resource_reservations(envelope_id, state);

-- +goose Down
DROP INDEX resource_reservations_envelope_state;
DROP TABLE resource_reservations;
DROP TABLE resource_envelopes;
