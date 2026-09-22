-- +goose Up
CREATE TABLE scheduled_wakeups (
    wakeup_id TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('TASK_REEVALUATION', 'STRATEGIC_PULSE')),
    task_id TEXT REFERENCES tasks(task_id),
    due_at TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('PENDING', 'FIRED', 'CANCELLED')),
    created_at TEXT NOT NULL,
    fired_at TEXT
);

CREATE INDEX scheduled_wakeups_state_due
    ON scheduled_wakeups(state, due_at);

CREATE INDEX scheduled_wakeups_strategic_pulse_due
    ON scheduled_wakeups(kind, state, due_at)
    WHERE kind = 'STRATEGIC_PULSE' AND state = 'PENDING';

-- A due but unacknowledged Strategic Pulse and its already-scheduled successor
-- must be allowed to coexist. Delivery acknowledgement, not successor creation,
-- is what consumes the current wakeup.

-- +goose Down
DROP INDEX scheduled_wakeups_strategic_pulse_due;
DROP INDEX scheduled_wakeups_state_due;
DROP TABLE scheduled_wakeups;
