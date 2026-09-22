-- +goose Up
CREATE TABLE field_observations (
    observation_id TEXT PRIMARY KEY,
    collective_id TEXT NOT NULL,
    task_id TEXT REFERENCES tasks(task_id),
    attempt_id TEXT REFERENCES attempts(attempt_id),
    operation_id TEXT REFERENCES external_operations(operation_id),
    detector_key TEXT UNIQUE,
    category TEXT NOT NULL,
    basis_class TEXT NOT NULL,
    source_kind TEXT NOT NULL,
    summary_local TEXT NOT NULL,
    metrics_json TEXT NOT NULL DEFAULT '{}',
    runtime_version TEXT NOT NULL DEFAULT '',
    executor_kind TEXT NOT NULL DEFAULT '',
    executor_version TEXT NOT NULL DEFAULT '',
    enforcement TEXT NOT NULL DEFAULT 'UNENFORCED'
        CHECK (enforcement IN ('ENFORCED','PARTIAL','UNENFORCED')),
    created_at TEXT NOT NULL
);
CREATE INDEX field_observations_task_time ON field_observations(task_id, created_at);
CREATE INDEX field_observations_category_time ON field_observations(category, created_at);

CREATE TABLE field_observation_evidence (
    observation_id TEXT NOT NULL REFERENCES field_observations(observation_id),
    evidence_id TEXT NOT NULL REFERENCES evidence_objects(evidence_id),
    linked_at TEXT NOT NULL,
    PRIMARY KEY (observation_id, evidence_id)
);
CREATE INDEX field_observation_evidence_evidence ON field_observation_evidence(evidence_id, observation_id);

CREATE TABLE feedback_candidates (
    candidate_id TEXT PRIMARY KEY,
    state TEXT NOT NULL CHECK (state IN (
        'CANDIDATE','SANITIZATION_PENDING','SANITIZED','APPROVAL_PENDING','EXPORT_READY',
        'REPORTED','LOCAL_ONLY','REJECTED_UNSAFE','REJECTED_POLICY','DUPLICATE'
    )),
    generic_task_class TEXT NOT NULL CHECK (generic_task_class IN (
        'DEBUGGING','REVIEW','REFACTOR','DOCUMENTATION','MIGRATION','TESTING','MAINTENANCE','OTHER'
    )),
    category TEXT NOT NULL,
    expected_behavior TEXT NOT NULL,
    observed_behavior TEXT NOT NULL,
    state_transition_json TEXT NOT NULL DEFAULT '[]',
    metrics_json TEXT NOT NULL DEFAULT '{}',
    human_intervention INTEGER NOT NULL DEFAULT 0 CHECK (human_intervention IN (0,1)),
    recovery_result TEXT NOT NULL DEFAULT '',
    runtime_version TEXT NOT NULL DEFAULT '',
    executor_kind TEXT NOT NULL DEFAULT '',
    executor_version TEXT NOT NULL DEFAULT '',
    enforcement TEXT NOT NULL DEFAULT 'UNENFORCED'
        CHECK (enforcement IN ('ENFORCED','PARTIAL','UNENFORCED')),
    correlation_key TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX feedback_candidates_state_time ON feedback_candidates(state, created_at);
CREATE INDEX feedback_candidates_correlation ON feedback_candidates(correlation_key, created_at);

CREATE TABLE feedback_candidate_observations (
    candidate_id TEXT NOT NULL REFERENCES feedback_candidates(candidate_id),
    observation_id TEXT NOT NULL REFERENCES field_observations(observation_id),
    linked_at TEXT NOT NULL,
    PRIMARY KEY (candidate_id, observation_id)
);

CREATE TABLE sanitization_results (
    sanitization_id TEXT PRIMARY KEY,
    candidate_id TEXT NOT NULL REFERENCES feedback_candidates(candidate_id),
    sanitizer_version TEXT NOT NULL,
    ruleset_hash TEXT NOT NULL,
    input_digest TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('PASS','REJECT','UNCERTAIN')),
    reason_codes_json TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL
);
CREATE INDEX sanitization_results_candidate ON sanitization_results(candidate_id, created_at);

CREATE TABLE sanitized_feedback (
    feedback_id TEXT PRIMARY KEY,
    candidate_id TEXT NOT NULL REFERENCES feedback_candidates(candidate_id),
    schema_version INTEGER NOT NULL,
    content_json TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    sanitization_result_id TEXT NOT NULL REFERENCES sanitization_results(sanitization_id),
    correlation_key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX sanitized_feedback_content_hash ON sanitized_feedback(content_hash);
CREATE INDEX sanitized_feedback_fingerprint ON sanitized_feedback(fingerprint);
CREATE INDEX sanitized_feedback_correlation ON sanitized_feedback(correlation_key);

CREATE TABLE approval_requests (
    approval_id TEXT PRIMARY KEY,
    subject_kind TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    request_digest TEXT NOT NULL,
    policy_decision_id TEXT NOT NULL,
    required_approvers_json TEXT NOT NULL,
    requested_by TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('PENDING','APPROVED','REJECTED','EXPIRED','CONSUMED')),
    expires_at TEXT NOT NULL,
    decided_at TEXT,
    approver_id TEXT,
    decision_action TEXT,
    created_at TEXT NOT NULL,
    UNIQUE(subject_kind, subject_id, request_digest)
);
CREATE INDEX approval_requests_state_expiry ON approval_requests(state, expires_at);
CREATE INDEX approval_requests_subject_digest ON approval_requests(subject_kind, subject_id, request_digest);

CREATE TABLE approval_decisions (
    approval_id TEXT NOT NULL REFERENCES approval_requests(approval_id),
    approver_id TEXT NOT NULL,
    request_digest TEXT NOT NULL,
    decision_action TEXT NOT NULL CHECK (decision_action IN ('APPROVE','REJECT')),
    decided_at TEXT NOT NULL,
    PRIMARY KEY (approval_id, approver_id)
);
CREATE INDEX approval_decisions_action
    ON approval_decisions(approval_id, decision_action);

ALTER TABLE external_operations
    ADD COLUMN approval_id TEXT REFERENCES approval_requests(approval_id);

ALTER TABLE external_operations
    ADD COLUMN caller_required_approvers_json TEXT NOT NULL DEFAULT '[]';

ALTER TABLE tasks
    ADD COLUMN task_class TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX feedback_emit_task_per_artifact
    ON tasks(purpose_kind, purpose_id, task_class)
    WHERE purpose_kind = 'COLLECTIVE_MAINTENANCE'
      AND task_class = 'collective.feedback.emit';

CREATE TABLE feedback_emissions (
    feedback_id TEXT PRIMARY KEY REFERENCES sanitized_feedback(feedback_id),
    task_id TEXT NOT NULL UNIQUE REFERENCES tasks(task_id),
    provider TEXT NOT NULL,
    destination TEXT NOT NULL,
    generic_task_class TEXT NOT NULL DEFAULT 'MAINTENANCE'
        CHECK (generic_task_class = 'MAINTENANCE'),
    required_approvers_json TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL
);

CREATE TABLE adaptation_grant_requests (
    grant_request_id TEXT PRIMARY KEY,
    grant_digest TEXT NOT NULL UNIQUE,
    definition_json TEXT NOT NULL,
    approval_id TEXT NOT NULL UNIQUE REFERENCES approval_requests(approval_id),
    requested_by TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE adaptation_grants (
    grant_id TEXT PRIMARY KEY,
    grant_request_id TEXT NOT NULL UNIQUE REFERENCES adaptation_grant_requests(grant_request_id),
    adaptation_kind TEXT NOT NULL,
    scope_key TEXT NOT NULL,
    allowed_executors_json TEXT NOT NULL,
    min_verified_samples INTEGER NOT NULL CHECK (min_verified_samples > 0),
    max_acceptance_regression_bps INTEGER NOT NULL CHECK (max_acceptance_regression_bps >= 0),
    max_cost_regression_bps INTEGER NOT NULL CHECK (max_cost_regression_bps >= 0),
    owner_principal_id TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    activated_at TEXT NOT NULL
);
CREATE INDEX adaptation_grants_kind_scope ON adaptation_grants(adaptation_kind, scope_key);

CREATE TABLE experience_proposals (
    proposal_id TEXT PRIMARY KEY,
    grant_id TEXT NOT NULL REFERENCES adaptation_grants(grant_id),
    generic_task_class TEXT NOT NULL CHECK (generic_task_class IN (
        'DEBUGGING','REVIEW','REFACTOR','DOCUMENTATION','MIGRATION','TESTING','MAINTENANCE','OTHER'
    )),
    scope_key TEXT NOT NULL,
    preferred_executor TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('CANDIDATE','SHADOW','ACTIVE','ROLLED_BACK','RETIRED')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX experience_proposals_scope ON experience_proposals(scope_key, generic_task_class, state);

CREATE TABLE experience_proposal_evidence (
    proposal_id TEXT NOT NULL REFERENCES experience_proposals(proposal_id),
    observation_id TEXT NOT NULL REFERENCES field_observations(observation_id),
    linked_at TEXT NOT NULL,
    PRIMARY KEY (proposal_id, observation_id)
);

CREATE TABLE experience_rules (
    rule_id TEXT PRIMARY KEY,
    proposal_id TEXT NOT NULL REFERENCES experience_proposals(proposal_id),
    grant_id TEXT NOT NULL REFERENCES adaptation_grants(grant_id),
    version INTEGER NOT NULL CHECK (version > 0),
    adaptation_kind TEXT NOT NULL,
    scope_key TEXT NOT NULL,
    generic_task_class TEXT NOT NULL CHECK (generic_task_class IN (
        'DEBUGGING','REVIEW','REFACTOR','DOCUMENTATION','MIGRATION','TESTING','MAINTENANCE','OTHER'
    )),
    preferred_executor TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('CANDIDATE','SHADOW','ACTIVE','ROLLED_BACK','RETIRED')),
    verified_samples INTEGER NOT NULL DEFAULT 0 CHECK (verified_samples >= 0),
    activated_at TEXT,
    rolled_back_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(proposal_id, version)
);
CREATE INDEX experience_rules_active_scope
    ON experience_rules(adaptation_kind, scope_key, state, generic_task_class);

CREATE TABLE experience_rule_evidence (
    rule_id TEXT NOT NULL REFERENCES experience_rules(rule_id),
    observation_id TEXT NOT NULL REFERENCES field_observations(observation_id),
    linked_at TEXT NOT NULL,
    PRIMARY KEY (rule_id, observation_id)
);

CREATE TABLE experience_outcomes (
    outcome_id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES tasks(task_id),
    generic_task_class TEXT NOT NULL CHECK (generic_task_class IN (
        'DEBUGGING','REVIEW','REFACTOR','DOCUMENTATION','MIGRATION','TESTING','MAINTENANCE','OTHER'
    )),
    scope_key TEXT NOT NULL,
    executor_kind TEXT NOT NULL,
    accepted INTEGER NOT NULL CHECK (accepted IN (0,1)),
    human_intervention INTEGER NOT NULL CHECK (human_intervention IN (0,1)),
    retry_count INTEGER NOT NULL CHECK (retry_count >= 0),
    cost_units INTEGER,
    latency_ms INTEGER,
    recorded_at TEXT NOT NULL
);
CREATE INDEX experience_outcomes_scope_time
    ON experience_outcomes(scope_key, generic_task_class, executor_kind, recorded_at);
CREATE UNIQUE INDEX experience_outcomes_dedupe
    ON experience_outcomes(task_id, generic_task_class, scope_key, executor_kind, accepted);

CREATE TABLE experience_rule_outcomes (
    rule_id TEXT NOT NULL REFERENCES experience_rules(rule_id),
    outcome_id TEXT NOT NULL REFERENCES experience_outcomes(outcome_id),
    linked_at TEXT NOT NULL,
    PRIMARY KEY (rule_id, outcome_id)
);

-- +goose StatementBegin
CREATE TRIGGER sanitization_results_no_insert_replace
BEFORE INSERT ON sanitization_results
WHEN EXISTS (SELECT 1 FROM sanitization_results WHERE result_id = NEW.result_id)
BEGIN
    SELECT RAISE(ABORT, 'sanitization_results are immutable');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER sanitization_results_no_update
BEFORE UPDATE ON sanitization_results
BEGIN
    SELECT RAISE(ABORT, 'sanitization_results are immutable');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER sanitization_results_no_delete
BEFORE DELETE ON sanitization_results
BEGIN
    SELECT RAISE(ABORT, 'sanitization_results are immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER sanitized_feedback_no_insert_replace
BEFORE INSERT ON sanitized_feedback
WHEN EXISTS (SELECT 1 FROM sanitized_feedback WHERE feedback_id = NEW.feedback_id)
BEGIN
    SELECT RAISE(ABORT, 'sanitized_feedback is immutable');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER sanitized_feedback_no_update
BEFORE UPDATE ON sanitized_feedback
BEGIN
    SELECT RAISE(ABORT, 'sanitized_feedback is immutable');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER sanitized_feedback_no_delete
BEFORE DELETE ON sanitized_feedback
BEGIN
    SELECT RAISE(ABORT, 'sanitized_feedback is immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER adaptation_grants_no_insert_replace
BEFORE INSERT ON adaptation_grants
WHEN EXISTS (SELECT 1 FROM adaptation_grants WHERE grant_id = NEW.grant_id)
BEGIN
    SELECT RAISE(ABORT, 'adaptation_grants are immutable');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER adaptation_grants_no_update
BEFORE UPDATE ON adaptation_grants
BEGIN
    SELECT RAISE(ABORT, 'adaptation_grants are immutable');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER adaptation_grants_no_delete
BEFORE DELETE ON adaptation_grants
BEGIN
    SELECT RAISE(ABORT, 'adaptation_grants are immutable');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER adaptation_grants_no_delete;
DROP TRIGGER adaptation_grants_no_update;
DROP TRIGGER adaptation_grants_no_insert_replace;
DROP TRIGGER sanitized_feedback_no_delete;
DROP TRIGGER sanitized_feedback_no_update;
DROP TRIGGER sanitized_feedback_no_insert_replace;
DROP TRIGGER sanitization_results_no_delete;
DROP TRIGGER sanitization_results_no_update;
DROP TRIGGER sanitization_results_no_insert_replace;
DROP TABLE experience_rule_outcomes;
DROP INDEX experience_outcomes_dedupe;
DROP INDEX experience_outcomes_scope_time;
DROP TABLE experience_outcomes;
DROP TABLE experience_rule_evidence;
DROP INDEX experience_rules_active_scope;
DROP TABLE experience_rules;
DROP TABLE experience_proposal_evidence;
DROP INDEX experience_proposals_scope;
DROP TABLE experience_proposals;
DROP INDEX adaptation_grants_kind_scope;
DROP TABLE adaptation_grants;
DROP TABLE adaptation_grant_requests;
DROP TABLE feedback_emissions;
DROP INDEX feedback_emit_task_per_artifact;
ALTER TABLE tasks DROP COLUMN task_class;
ALTER TABLE external_operations DROP COLUMN caller_required_approvers_json;
ALTER TABLE external_operations DROP COLUMN approval_id;
DROP INDEX approval_decisions_action;
DROP TABLE approval_decisions;
DROP INDEX approval_requests_subject_digest;
DROP INDEX approval_requests_state_expiry;
DROP TABLE approval_requests;
DROP INDEX sanitized_feedback_correlation;
DROP INDEX sanitized_feedback_fingerprint;
DROP INDEX sanitized_feedback_content_hash;
DROP TABLE sanitized_feedback;
DROP INDEX sanitization_results_candidate;
DROP TABLE sanitization_results;
DROP TABLE feedback_candidate_observations;
DROP INDEX feedback_candidates_correlation;
DROP INDEX feedback_candidates_state_time;
DROP TABLE feedback_candidates;
DROP INDEX field_observation_evidence_evidence;
DROP TABLE field_observation_evidence;
DROP INDEX field_observations_category_time;
DROP INDEX field_observations_task_time;
DROP TABLE field_observations;
