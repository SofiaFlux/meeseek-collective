-- +goose Up
CREATE TABLE claims (
    claim_id TEXT PRIMARY KEY,
    subject_id TEXT NOT NULL,
    predicate TEXT NOT NULL,
    statement TEXT NOT NULL,
    normalized_statement TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('SPECULATION', 'HYPOTHESIS', 'SUPPORTED', 'VERIFIED')),
    confidence REAL NOT NULL CHECK (confidence >= 0.0 AND confidence <= 1.0),
    valid_from TEXT NOT NULL,
    valid_to TEXT,
    source_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    CHECK (valid_to IS NULL OR valid_to >= valid_from)
);

CREATE INDEX claims_subject_predicate_validity
    ON claims(subject_id, predicate, valid_from, valid_to);
CREATE INDEX claims_created_at
    ON claims(created_at);

CREATE TABLE claim_evidence (
    claim_id TEXT NOT NULL REFERENCES claims(claim_id),
    evidence_id TEXT NOT NULL REFERENCES evidence_objects(evidence_id),
    linked_at TEXT NOT NULL,
    PRIMARY KEY (claim_id, evidence_id)
);

CREATE INDEX claim_evidence_evidence
    ON claim_evidence(evidence_id, claim_id);

CREATE TABLE claim_relations (
    relation_id TEXT PRIMARY KEY,
    left_claim_id TEXT NOT NULL REFERENCES claims(claim_id),
    right_claim_id TEXT NOT NULL REFERENCES claims(claim_id),
    kind TEXT NOT NULL CHECK (kind IN ('CONTRADICTS', 'SUPERSEDES')),
    created_at TEXT NOT NULL,
    CHECK (left_claim_id <> right_claim_id),
    UNIQUE (left_claim_id, right_claim_id, kind)
);

CREATE INDEX claim_relations_right
    ON claim_relations(right_claim_id, kind);

CREATE TABLE knowledge_deltas (
    delta_id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    ingested_at TEXT NOT NULL
);

CREATE TABLE knowledge_delta_claims (
    delta_id TEXT NOT NULL REFERENCES knowledge_deltas(delta_id),
    claim_id TEXT NOT NULL REFERENCES claims(claim_id),
    submitted_status TEXT NOT NULL,
    submitted_confidence REAL NOT NULL,
    PRIMARY KEY (delta_id, claim_id)
);

CREATE TABLE memory_events (
    event_id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX memory_events_subject_time
    ON memory_events(subject_id, created_at);

CREATE VIRTUAL TABLE claim_fts USING fts5(
    claim_id UNINDEXED,
    statement,
    subject_id,
    predicate
);

-- +goose StatementBegin
CREATE TRIGGER claims_fts_insert AFTER INSERT ON claims BEGIN
    INSERT INTO claim_fts(claim_id, statement, subject_id, predicate)
    VALUES (new.claim_id, new.statement, new.subject_id, new.predicate);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER claims_fts_update AFTER UPDATE OF statement, subject_id, predicate ON claims BEGIN
    DELETE FROM claim_fts WHERE claim_id = old.claim_id;
    INSERT INTO claim_fts(claim_id, statement, subject_id, predicate)
    VALUES (new.claim_id, new.statement, new.subject_id, new.predicate);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER claims_fts_delete AFTER DELETE ON claims BEGIN
    DELETE FROM claim_fts WHERE claim_id = old.claim_id;
END;
-- +goose StatementEnd

CREATE TABLE decision_records (
    decision_id TEXT PRIMARY KEY,
    trigger TEXT NOT NULL,
    alternatives_json TEXT NOT NULL,
    basis_class TEXT NOT NULL,
    evidence_ids_json TEXT NOT NULL,
    policy_decision_id TEXT NOT NULL,
    authority_ids_json TEXT NOT NULL,
    expected_outcome TEXT NOT NULL,
    confidence REAL NOT NULL CHECK (confidence >= 0.0 AND confidence <= 1.0),
    resource_envelope_id TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX decision_records_created_at
    ON decision_records(created_at);

-- +goose StatementBegin
CREATE TRIGGER decision_records_no_update
BEFORE UPDATE ON decision_records
BEGIN
    SELECT RAISE(ABORT, 'decision_records are immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER decision_records_no_delete
BEFORE DELETE ON decision_records
BEGIN
    SELECT RAISE(ABORT, 'decision_records are immutable');
END;
-- +goose StatementEnd

CREATE TABLE audit_events (
    audit_id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX audit_events_subject_time
    ON audit_events(subject_id, created_at);

-- +goose StatementBegin
CREATE TRIGGER audit_events_no_update
BEFORE UPDATE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit_events are immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER audit_events_no_delete
BEFORE DELETE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit_events are immutable');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER audit_events_no_delete;
DROP TRIGGER audit_events_no_update;
DROP INDEX audit_events_subject_time;
DROP TABLE audit_events;
DROP TRIGGER decision_records_no_delete;
DROP TRIGGER decision_records_no_update;
DROP INDEX decision_records_created_at;
DROP TABLE decision_records;
DROP TRIGGER claims_fts_delete;
DROP TRIGGER claims_fts_update;
DROP TRIGGER claims_fts_insert;
DROP TABLE claim_fts;
DROP INDEX memory_events_subject_time;
DROP TABLE memory_events;
DROP TABLE knowledge_delta_claims;
DROP TABLE knowledge_deltas;
DROP INDEX claim_relations_right;
DROP TABLE claim_relations;
DROP INDEX claim_evidence_evidence;
DROP TABLE claim_evidence;
DROP INDEX claims_created_at;
DROP INDEX claims_subject_predicate_validity;
DROP TABLE claims;
