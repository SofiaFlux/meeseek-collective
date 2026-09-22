-- +goose Up

-- Approval requests are lifecycle records: state/decision fields may advance, but
-- the exact authorization subject and digest binding must never be rewritten.
-- +goose StatementBegin
CREATE TRIGGER approval_requests_no_insert_replace
BEFORE INSERT ON approval_requests WHEN EXISTS (SELECT 1 FROM approval_requests WHERE approval_id = NEW.approval_id)
BEGIN SELECT RAISE(ABORT, 'approval requests are durable governance history'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER approval_requests_binding_immutable
BEFORE UPDATE ON approval_requests
WHEN NEW.subject_kind <> OLD.subject_kind
  OR NEW.subject_id <> OLD.subject_id
  OR NEW.request_digest <> OLD.request_digest
  OR NEW.policy_decision_id <> OLD.policy_decision_id
  OR NEW.required_approvers_json <> OLD.required_approvers_json
  OR NEW.requested_by <> OLD.requested_by
  OR NEW.expires_at <> OLD.expires_at
  OR NEW.created_at <> OLD.created_at
BEGIN
    SELECT RAISE(ABORT, 'approval request binding is immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER approval_requests_no_delete
BEFORE DELETE ON approval_requests
BEGIN
    SELECT RAISE(ABORT, 'approval requests are durable governance history');
END;
-- +goose StatementEnd

-- Individual approver decisions are immutable facts.
-- +goose StatementBegin
CREATE TRIGGER approval_decisions_no_insert_replace
BEFORE INSERT ON approval_decisions WHEN EXISTS (SELECT 1 FROM approval_decisions WHERE approval_id = NEW.approval_id AND approver_id = NEW.approver_id)
BEGIN SELECT RAISE(ABORT, 'approval decisions are immutable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER approval_decisions_no_update
BEFORE UPDATE ON approval_decisions
BEGIN
    SELECT RAISE(ABORT, 'approval decisions are immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER approval_decisions_no_delete
BEFORE DELETE ON approval_decisions
BEGIN
    SELECT RAISE(ABORT, 'approval decisions are immutable');
END;
-- +goose StatementEnd

-- Proposal identity/scope is immutable; only lifecycle state and updated_at may advance.
-- +goose StatementBegin
CREATE TRIGGER experience_proposals_no_insert_replace
BEFORE INSERT ON experience_proposals WHEN EXISTS (SELECT 1 FROM experience_proposals WHERE proposal_id = NEW.proposal_id)
BEGIN SELECT RAISE(ABORT, 'experience proposals are immutable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER experience_proposals_binding_immutable
BEFORE UPDATE ON experience_proposals
WHEN NEW.grant_id <> OLD.grant_id
  OR NEW.generic_task_class <> OLD.generic_task_class
  OR NEW.scope_key <> OLD.scope_key
  OR NEW.preferred_executor <> OLD.preferred_executor
  OR NEW.created_at <> OLD.created_at
BEGIN
    SELECT RAISE(ABORT, 'experience proposal binding is immutable');
END;
-- +goose StatementEnd

-- Rule versions and verified outcomes are append-only evidence.
-- +goose StatementBegin
CREATE TRIGGER experience_rules_no_insert_replace
BEFORE INSERT ON experience_rules WHEN EXISTS (SELECT 1 FROM experience_rules WHERE rule_id = NEW.rule_id)
BEGIN SELECT RAISE(ABORT, 'experience rules are immutable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER experience_rules_no_update
BEFORE UPDATE ON experience_rules
BEGIN
    SELECT RAISE(ABORT, 'experience rules are immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER experience_rules_no_delete
BEFORE DELETE ON experience_rules
BEGIN
    SELECT RAISE(ABORT, 'experience rules are immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER experience_outcomes_no_insert_replace
BEFORE INSERT ON experience_outcomes WHEN EXISTS (SELECT 1 FROM experience_outcomes WHERE outcome_id = NEW.outcome_id)
BEGIN SELECT RAISE(ABORT, 'experience outcomes are immutable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER experience_outcomes_no_update
BEFORE UPDATE ON experience_outcomes
BEGIN
    SELECT RAISE(ABORT, 'experience outcomes are immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER experience_outcomes_no_delete
BEFORE DELETE ON experience_outcomes
BEGIN
    SELECT RAISE(ABORT, 'experience outcomes are immutable');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER experience_outcomes_no_delete;
DROP TRIGGER experience_outcomes_no_update;
DROP TRIGGER experience_rules_no_delete;
DROP TRIGGER experience_rules_no_update;
DROP TRIGGER experience_proposals_binding_immutable;
DROP TRIGGER approval_decisions_no_delete;
DROP TRIGGER approval_decisions_no_update;
DROP TRIGGER approval_requests_no_delete;
DROP TRIGGER approval_requests_binding_immutable;
