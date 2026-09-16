-- +goose Up
CREATE TABLE effect_slots (
    effect_slot_id TEXT PRIMARY KEY,
    collective_id TEXT NOT NULL,
    task_id TEXT NOT NULL REFERENCES tasks(task_id),
    trusted_slot_key TEXT NOT NULL,
    provider TEXT NOT NULL,
    descriptor_type TEXT NOT NULL,
    intent_fingerprint TEXT NOT NULL,
    intent_revision INTEGER NOT NULL CHECK (intent_revision > 0),
    canonical_intent_json TEXT NOT NULL,
    adapter_version TEXT NOT NULL,
    adapter_version_semantic INTEGER NOT NULL CHECK (adapter_version_semantic IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (collective_id, task_id, trusted_slot_key)
);

CREATE INDEX effect_slots_task
    ON effect_slots(task_id, trusted_slot_key);

CREATE TABLE external_operations (
    operation_id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(task_id),
    attempt_id TEXT NOT NULL REFERENCES attempts(attempt_id),
    effect_slot_id TEXT NOT NULL REFERENCES effect_slots(effect_slot_id),
    operation_sequence INTEGER NOT NULL CHECK (operation_sequence > 0),
    state TEXT NOT NULL CHECK (state IN (
        'PREPARED', 'DISPATCHED', 'CONFIRMED_EFFECT', 'CONFIRMED_NO_EFFECT',
        'OUTCOME_UNKNOWN', 'CANCELLED'
    )),
    intent_fingerprint TEXT NOT NULL,
    intent_revision INTEGER NOT NULL CHECK (intent_revision > 0),
    provider TEXT NOT NULL,
    adapter_version TEXT NOT NULL,
    reservation_id TEXT NOT NULL REFERENCES resource_reservations(reservation_id),
    risk TEXT NOT NULL,
    prepare_policy_decision_id TEXT NOT NULL,
    prepare_policy_set_id TEXT NOT NULL,
    prepare_policy_set_hash TEXT NOT NULL,
    prepare_policy_capabilities_hash TEXT NOT NULL,
    dispatch_policy_decision_id TEXT,
    dispatch_policy_set_id TEXT,
    dispatch_policy_set_hash TEXT,
    dispatch_policy_capabilities_hash TEXT,
    dispatcher_claim TEXT,
    provider_reference TEXT,
    cancel_requested INTEGER NOT NULL DEFAULT 0 CHECK (cancel_requested IN (0, 1)),
    reconciliation_required INTEGER NOT NULL DEFAULT 0 CHECK (reconciliation_required IN (0, 1)),
    actual_cost INTEGER CHECK (actual_cost IS NULL OR actual_cost >= 0),
    created_at TEXT NOT NULL,
    dispatched_at TEXT,
    settled_at TEXT,
    UNIQUE (effect_slot_id, operation_sequence)
);

-- +goose StatementBegin
CREATE TRIGGER external_operations_prepare_policy_profile_active
BEFORE INSERT ON external_operations
FOR EACH ROW
WHEN NOT EXISTS (
    SELECT 1
    FROM policy_sets p
    WHERE p.active = 1
      AND p.policy_set_id = NEW.prepare_policy_set_id
      AND p.policy_hash = NEW.prepare_policy_set_hash
      AND p.capabilities_hash = NEW.prepare_policy_capabilities_hash
)
BEGIN
    SELECT RAISE(ABORT, 'MEESEEK_POLICY_PROFILE_INACTIVE');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER external_operations_dispatch_policy_profile_active
BEFORE UPDATE OF state, dispatch_policy_set_id, dispatch_policy_set_hash, dispatch_policy_capabilities_hash ON external_operations
FOR EACH ROW
WHEN OLD.state = 'PREPARED'
 AND NEW.state = 'DISPATCHED'
 AND NOT EXISTS (
    SELECT 1
    FROM policy_sets p
    WHERE p.active = 1
      AND p.policy_set_id = NEW.dispatch_policy_set_id
      AND p.policy_hash = NEW.dispatch_policy_set_hash
      AND p.capabilities_hash = NEW.dispatch_policy_capabilities_hash
)
BEGIN
    SELECT RAISE(ABORT, 'MEESEEK_POLICY_PROFILE_INACTIVE');
END;
-- +goose StatementEnd

CREATE UNIQUE INDEX external_operations_dispatcher_claim
    ON external_operations(dispatcher_claim)
    WHERE dispatcher_claim IS NOT NULL;

CREATE INDEX external_operations_state
    ON external_operations(state, reconciliation_required, created_at);

CREATE INDEX external_operations_slot
    ON external_operations(effect_slot_id, operation_sequence DESC);

-- +goose Down
DROP INDEX external_operations_slot;
DROP INDEX external_operations_state;
DROP INDEX external_operations_dispatcher_claim;
DROP TRIGGER external_operations_dispatch_policy_profile_active;
DROP TRIGGER external_operations_prepare_policy_profile_active;
DROP TABLE external_operations;
DROP INDEX effect_slots_task;
DROP TABLE effect_slots;
