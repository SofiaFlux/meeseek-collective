# Field Dogfooding, Sanitized Feedback, and Local Experience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a privacy-safe field-dogfooding loop that turns real Collective behavior into governed sanitized feedback, plus a bounded evidence-driven local executor-preference loop that cannot expand authority or scheduler eligibility.

**Architecture:** Extend the existing single-Cube MVC with focused `approvals`, `fieldfeedback`, and `experience` services. Raw observations remain local; only immutable `SanitizedFeedback` may cross the declassification boundary, and export is performed by a normal fenced `COLLECTIVE_MAINTENANCE` Task through the existing External Operation commit boundary. Local experience supplies a post-eligibility executor preference only when verified evidence satisfies an explicit Owner-created `AdaptationGrant`.

**Tech Stack:** Go 1.25.x, SQLite/goose migrations, embedded OPA/Rego, Cobra CLI, `net/http`, existing Ed25519 identity/control path, existing External Operations / resources / verification / scheduler services.

**Spec:** `docs/superpowers/specs/2026-09-18-field-dogfooding-feedback-design.md`

## Global Constraints

- Raw `FieldObservation` and raw evidence are never accepted by an outbound feedback provider.
- Sanitization and outbound authorization are separate fail-closed gates.
- Only an immutable, hash-bound `SanitizedFeedback` artifact is export-eligible.
- Export is ordinary governed work: `COLLECTIVE_MAINTENANCE` Task → fenced Attempt → protected External Operation.
- `AUTO_IF_ALLOWED` changes who requests work; it grants no authority, credential, or policy exception.
- `REQUIRE_APPROVAL` is durable and bound to an exact subject/request digest; changed intent requires new approval.
- Approval never overrides a later policy/authority/enforcement denial.
- `ALLOW_WITH_LIMIT` fails closed unless every returned limit is explicitly understood and deterministically enforceable.
- Feedback credentials are provider-owned and are never placed in operation intent, feedback content, audit payloads, logs, or executor-visible context.
- Local learning may only rank already-eligible executor choices; it must never alter task eligibility, authority, policy, TEB, capability/locality checks, or resource limits.
- No `AdaptationGrant` exists by default.
- Only verified outcomes count toward automatic experience-rule promotion.
- Evidence may justify an authority-expansion proposal; evidence never manufactures authority.
- Discovery of independent work creates a new governed work item; it never silently widens current scope.
- Existing MVC semantics and verification matrix remain mandatory.
- No Maintainer Collective, source-code self-modification, auto-PR, or autonomous release automation is implemented in this plan.

---

## File Structure

New focused packages:

- `internal/approvals/` — durable generic approval lifecycle; exact digest binding; approve/reject/consume.
- `internal/fieldfeedback/` — observations, candidates, sanitizer, immutable feedback, governed emit-task creation/query.
- `internal/feedbackgithub/` — GitHub issue External Operation provider and reconciliation. It receives only `SanitizedFeedback` IDs.
- `internal/experience/` — adaptation grants, proposals, shadow/active rules, verified-outcome promotion and rollback.
- `internal/state/sqlite/migrations/00010_field_feedback.sql` — durable schema, including observation-evidence links, emission work links, approval bindings, grant requests, and verified experience outcomes.
- `tests/acceptance/field_feedback_test.go` — complete feedback vertical slice and negative security semantics.
- `tests/acceptance/local_experience_test.go` — bounded adaptation vertical slice.

Existing packages modified only at their domain boundaries:

- `internal/operations/` — policy outcome semantics and approval gating.
- `internal/scheduler/` — post-eligibility executor preference hook only.
- `internal/runtime/` — explicit Box composition.
- `internal/control/`, `cmd/meeseek/` — operator inspection / emit / approve / reject / experience surface.
- `internal/localconfig/` — non-secret field-feedback settings and maintenance envelope ID.
- `cmd/meeseek-box/` — provider/config wiring.
- `README.md` — operation/security contract.

---

### Task 1: Persist the field-feedback, approval, and experience domain

**Files:**
- Create: `internal/state/sqlite/migrations/00010_field_feedback.sql`
- Create: `internal/domain/feedback.go`
- Create: `internal/domain/approval.go`
- Create: `internal/domain/experience.go`
- Modify: `internal/domain/states.go`
- Test: `internal/state/sqlite/store_test.go`
- Test: `internal/domain/states_test.go`

**Interfaces:**
- Produces:
  - `domain.FeedbackCandidateState`
  - `domain.SanitizationOutcome`
  - `domain.ApprovalState`
  - `domain.ExperienceRuleState`
  - `domain.FieldObservation`
  - `domain.FeedbackCandidate`
  - `domain.SanitizationResult`
  - `domain.SanitizedFeedback`
  - `domain.ApprovalRequestRecord`
  - `domain.AdaptationGrant`
  - `domain.ExperienceProposal`
  - `domain.ExperienceRule`

- [ ] **Step 1: Write migration/domain RED tests**

Add a migration test that opens a fresh store and asserts all new tables exist and immutable tables reject UPDATE:

```go
func TestFieldFeedbackMigrationCreatesImmutableExportArtifacts(t *testing.T) {
    store := testutil.OpenStore(t)
    ctx := context.Background()

    for _, table := range []string{
        "field_observations", "field_observation_evidence",
        "feedback_candidates", "feedback_candidate_observations",
        "sanitization_results", "sanitized_feedback", "feedback_emissions",
        "approval_requests", "adaptation_grant_requests", "adaptation_grants",
        "experience_proposals", "experience_rules", "experience_rule_evidence",
        "experience_outcomes",
    } {
        var name string
        if err := store.DB().QueryRowContext(ctx,
            `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
        ).Scan(&name); err != nil {
            t.Fatalf("table %s missing: %v", table, err)
        }
    }
}
```

Add state validation tests for all exact enum values defined below.

- [ ] **Step 2: Run RED**

Run:

```bash
go test ./internal/state/sqlite ./internal/domain -count=1
```

Expected: FAIL because migration/types do not exist.

- [ ] **Step 3: Add exact domain types**

Create `internal/domain/feedback.go` with:

```go
package domain

import "time"

type FeedbackCandidateState string
const (
    FeedbackCandidate         FeedbackCandidateState = "CANDIDATE"
    FeedbackSanitizing        FeedbackCandidateState = "SANITIZATION_PENDING"
    FeedbackSanitized         FeedbackCandidateState = "SANITIZED"
    FeedbackApprovalPending   FeedbackCandidateState = "APPROVAL_PENDING"
    FeedbackExportReady       FeedbackCandidateState = "EXPORT_READY"
    FeedbackReported          FeedbackCandidateState = "REPORTED"
    FeedbackLocalOnly         FeedbackCandidateState = "LOCAL_ONLY"
    FeedbackRejectedUnsafe    FeedbackCandidateState = "REJECTED_UNSAFE"
    FeedbackRejectedPolicy    FeedbackCandidateState = "REJECTED_POLICY"
    FeedbackDuplicate         FeedbackCandidateState = "DUPLICATE"
)

type SanitizationOutcome string
const (
    SanitizationPass      SanitizationOutcome = "PASS"
    SanitizationReject    SanitizationOutcome = "REJECT"
    SanitizationUncertain SanitizationOutcome = "UNCERTAIN"
)

type FieldObservation struct {
    ID              ID
    CollectiveID    ID
    TaskID          ID
    AttemptID       ID
    OperationID     ID
    Category        string
    BasisClass      string
    SourceKind      string
    SummaryLocal    string
    MetricsJSON     string
    RuntimeVersion  string
    ExecutorKind    string
    ExecutorVersion string
    Enforcement     EnforcementLevel
    CreatedAt       time.Time
}

type FeedbackCandidate struct {
    ID                    ID
    State                 FeedbackCandidateState
    GenericTaskClass      string
    Category              string
    ExpectedBehavior      string
    ObservedBehavior      string
    StateTransitionJSON   string
    MetricsJSON           string
    HumanIntervention     bool
    RecoveryResult        string
    RuntimeVersion        string
    ExecutorKind          string
    ExecutorVersion       string
    Enforcement           EnforcementLevel
    CorrelationKey        string
    CreatedAt             time.Time
}

type SanitizationResult struct {
    ID               ID
    CandidateID      ID
    SanitizerVersion string
    RulesetHash      string
    InputDigest      string
    Outcome          SanitizationOutcome
    ReasonCodesJSON  string
    CreatedAt        time.Time
}

type SanitizedFeedback struct {
    ID                   ID
    CandidateID          ID
    SchemaVersion        int
    ContentJSON          string
    ContentHash          string
    SanitizationResultID ID
    CorrelationKey       string
    Fingerprint          string
    CreatedAt            time.Time
}
```

Create approval/experience types with exact state strings from the spec.

- [ ] **Step 4: Add migration**

`00010_field_feedback.sql` must:
- create all fourteen tables listed in the migration test;
- use CHECK constraints for all state enums;
- foreign-key candidate-observation links;
- make `sanitized_feedback`, `sanitization_results`, and active `adaptation_grants` immutable with no-update/no-delete triggers;
- make `sanitized_feedback.content_hash` UNIQUE;
- make `sanitized_feedback.fingerprint` indexed;
- make approval subject+digest queryable;
- add nullable `approval_id TEXT REFERENCES approval_requests(approval_id)` to `external_operations`;
- persist observation↔evidence links in `field_observation_evidence`;
- persist sanitized_feedback↔emit_task linkage in `feedback_emissions`;
- persist each accepted verified outcome in `experience_outcomes`;
- persist pending Owner-authorized grant definitions in `adaptation_grant_requests`;
- index active experience rules by `adaptation_kind, scope_key, state`.

Do not store raw evidence blobs in these tables.

- [ ] **Step 7: Run GREEN**

```bash
go test ./internal/state/sqlite ./internal/domain -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/state/sqlite/migrations/00010_field_feedback.sql internal/domain
git commit -m "feat: persist field feedback and experience domain"
```

---

### Task 2: Implement generic durable approvals and exact Owner signature binding

**Files:**
- Create: `internal/approvals/service.go`
- Create: `internal/approvals/service_test.go`
- Modify: `internal/control/server.go`
- Modify: `internal/control/client.go`
- Modify: `internal/control/server_test.go`
- Modify: `cmd/meeseek/approve_cmd.go`
- Create: `cmd/meeseek/reject_cmd.go`
- Modify: `cmd/meeseek/root.go`

**Interfaces:**
- Produces:
```go
type CreateRequest struct {
    SubjectKind       string
    SubjectID         domain.ID
    RequestDigest     string
    PolicyDecisionID  domain.ID
    RequiredApprovers []domain.ID
    RequestedBy       domain.ID
    ExpiresAt         time.Time
}
func (s *Service) Create(context.Context, CreateRequest) (domain.ApprovalRequestRecord, error)
func (s *Service) CreateInTx(context.Context, *sql.Tx, CreateRequest) (domain.ApprovalRequestRecord, error)
func (s *Service) Get(context.Context, domain.ID) (domain.ApprovalRequestRecord, error)
func (s *Service) Pending(context.Context) ([]domain.ApprovalRequestRecord, error)
func (s *Service) Approve(context.Context, domain.ID, domain.ID, string) error
func (s *Service) Reject(context.Context, domain.ID, domain.ID, string) error
func (s *Service) IsApproved(context.Context, domain.ID, string) (bool, error)
func (s *Service) Consume(context.Context, domain.ID, string) error
```
- Control signing message:
```go
func ApprovalSigningMessage(challenge, requestDigest, action string) []byte
```

- [ ] **Step 1: Write approval lifecycle tests**

Tests must prove:
- duplicate Create for same subject+digest is idempotent;
- changed digest produces a new approval;
- only required approver can approve;
- expired request cannot approve;
- reject is terminal;
- approve with mismatched digest fails;
- consume only works for APPROVED exact digest;
- consumed approval cannot authorize a different dispatch.

Example:

```go
func TestApprovalIsBoundToExactDigest(t *testing.T) {
    svc, owner := newApprovalHarness(t)
    req, err := svc.Create(context.Background(), approvals.CreateRequest{
        SubjectKind: "EXTERNAL_OPERATION",
        SubjectID: "operation_1",
        RequestDigest: "digest-a",
        PolicyDecisionID: "decision_1",
        RequiredApprovers: []domain.ID{owner},
        RequestedBy: "cube_1",
        ExpiresAt: time.Now().UTC().Add(time.Hour),
    })
    if err != nil { t.Fatal(err) }

    if err := svc.Approve(context.Background(), req.ID, owner, "digest-b"); err == nil {
        t.Fatal("mismatched digest was approved")
    }
}
```

- [ ] **Step 2: Run RED**

```bash
go test ./internal/approvals ./internal/control ./cmd/meeseek -count=1
```

Expected: FAIL.

- [ ] **Step 3: Implement approvals service**

Use transactions and compare current state before every transition. Never mutate subject/digest/required approvers after creation.

- [ ] **Step 4: Bind control challenge to durable request**

Replace `approvalRequestDigest(id)` with the digest loaded from `Approvals.Get`.

Signed material is exactly:

```go
func ApprovalSigningMessage(challenge, requestDigest, action string) []byte {
    return []byte(
        "meeseek-owner-approval-v2\n" +
        "challenge:" + challenge + "\n" +
        "request-digest:" + requestDigest + "\n" +
        "action:" + action + "\n",
    )
}
```

Expose:
- `GET /approvals`
- `GET /approvals/{id}`
- `GET /approvals/{id}/challenge?action=APPROVE|REJECT`
- `POST /approvals/{id}/approve`
- `POST /approvals/{id}/reject`

The challenge cache may remain ephemeral; the approval request itself is durable.

- [ ] **Step 5: Update client/CLI**

`meeseek approve <id>` and `meeseek reject <id>` load the existing Owner key, sign V2 material, and call exact endpoints.

- [ ] **Step 6: Run GREEN**

```bash
go test ./internal/approvals ./internal/control ./cmd/meeseek -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/approvals internal/control cmd/meeseek
git commit -m "feat: add durable exact-bound approvals"
```

---

### Task 3: Teach External Operations `REQUIRE_APPROVAL` without weakening the commit boundary

**Files:**
- Modify: `internal/operations/service.go`
- Modify: `internal/operations/service_test.go`
- Modify: `internal/operations/read.go`
- Modify: `internal/runtime/box.go`

**Interfaces:**
- `operations.New(..., approvals *approvals.Service, ...Provider)`
- Add `ApprovalID domain.ID` to `domain.ExternalOperation`.
- Task 1 migration already adds `approval_id TEXT REFERENCES approval_requests(approval_id)` to `external_operations`.
- Request digest is SHA-256 of canonical consequential dispatch identity:
```text
provider
descriptor_type
effect_slot_id
intent_fingerprint
intent_revision
canonical_intent_hash
```

- [ ] **Step 1: Write policy outcome RED tests**

Add:
- `TestRequireApprovalPreparesButCannotDispatch`
- `TestApprovedExactIntentCanDispatch`
- `TestPolicyDenyAfterApprovalStillBlocksDispatch`
- `TestApprovalForDifferentDigestCannotDispatch`
- `TestAllowWithUnknownLimitFailsClosed`

The first test must assert provider DispatchCount remains zero.

- [ ] **Step 2: Run RED**

```bash
go test ./internal/operations -count=1
```

- [ ] **Step 3: Split policy validation into explicit semantics**

Replace the current “anything except ALLOW = deny” helper with:

```go
type policyGate struct {
    Decision       domain.PolicyDecision
    RequiresApproval bool
}

func validateDecisionProvenance(decision domain.PolicyDecision, now time.Time) error
func evaluatePrepareOutcome(decision domain.PolicyDecision) (policyGate, error)
func validateDispatchOutcome(decision domain.PolicyDecision) error
```

Rules:
- `ALLOW`: continue.
- `DENY`: `ErrPolicyDenied`.
- `ALLOW_WITH_LIMIT`: fail closed with `ErrPolicyDenied` until a named limit enforcer exists.
- `REQUIRE_APPROVAL`: allowed only during PREPARE; create/reuse durable approval request.
- DISPATCH must have exact approved request and current policy must still be either `ALLOW` or `REQUIRE_APPROVAL` with the same required approver set. A current DENY always wins.

- [ ] **Step 4: Bind approval to operation intent before PREPARED commits**

Create the approval request in the same transaction that creates the PREPARED operation, or provide `CreateInTx` so no PREPARED operation can point to a nonexistent approval.

- [ ] **Step 5: Consume approval at dispatch commitment**

Immediately before PREPARED → DISPATCHED:
- re-GuardAttempt;
- recheck authority/enforcement/resource/intent;
- re-evaluate policy;
- verify exact approval if required;
- consume approval atomically with the state transition.

- [ ] **Step 6: Run GREEN and regression**

```bash
go test ./internal/operations ./tests/acceptance -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/operations internal/domain internal/runtime
git commit -m "feat: gate external operations with durable approvals"
```

---

### Task 4: Capture durable local FieldObservations

**Files:**
- Create: `internal/fieldfeedback/observer.go`
- Create: `internal/fieldfeedback/observer_test.go`
- Create: `internal/fieldfeedback/detectors.go`
- Create: `internal/fieldfeedback/detectors_test.go`

**Interfaces:**
```go
type ObservationInput struct {
    TaskID, AttemptID, OperationID domain.ID
    Category, BasisClass, SourceKind, SummaryLocal string
    Metrics map[string]any
    RuntimeVersion, ExecutorKind, ExecutorVersion string
    Enforcement domain.EnforcementLevel
    EvidenceIDs []domain.ID
}

func (s *Observer) Record(context.Context, ObservationInput) (domain.FieldObservation, error)
func (s *Observer) Observation(context.Context, domain.ID) (domain.FieldObservation, error)
func (s *Observer) ForTask(context.Context, domain.ID) ([]domain.FieldObservation, error)
func (s *Observer) Detect(context.Context, DetectorInput) ([]domain.FieldObservation, error)
```

- [ ] **Step 1: Write RED validation/provenance tests**

Prove:
- category/source/summary required;
- referenced Task/Attempt/Operation must exist when supplied;
- evidence IDs must exist;
- arbitrary metrics are JSON-encoded locally but never treated as exportable schema;
- duplicate deterministic detector event is idempotent using a stable detector key.

- [ ] **Step 2: Run RED**

```bash
go test ./internal/fieldfeedback -run 'Observation|Detector' -count=1
```

- [ ] **Step 3: Implement Observer**

Persist observation and observation-evidence links transactionally. Emit an audit event `FIELD_OBSERVATION_RECORDED` with IDs/category only; do not copy `SummaryLocal` into audit payload.

- [ ] **Step 4: Add initial deterministic detectors**

Implement high-signal detectors only:
- repeated attempt failure/retry for one Task;
- stale/expired attempt recovery;
- repeated policy/authority denial event;
- unresolved operation/reconciliation;
- explicit human intervention event;
- verification challenge after earlier acceptance where canonical evidence exists.

Do not implement model-based detectors in this task.

- [ ] **Step 5: Run GREEN**

```bash
go test ./internal/fieldfeedback -run 'Observation|Detector' -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/fieldfeedback
git commit -m "feat: record evidence-backed field observations"
```

---

### Task 5: Implement FeedbackCandidate lifecycle and exact correlation

**Files:**
- Create: `internal/fieldfeedback/feedback.go`
- Create: `internal/fieldfeedback/feedback_test.go`

**Interfaces:**
```go
type CandidateInput struct {
    ObservationIDs []domain.ID
    GenericTaskClass string
    Category string
    ExpectedBehavior string
    ObservedBehavior string
    StateTransitions []string
    Metrics NormalizedMetrics
    HumanIntervention bool
    RecoveryResult string
    RuntimeVersion string
    ExecutorKind string
    ExecutorVersion string
    Enforcement domain.EnforcementLevel
}

type NormalizedMetrics struct {
    RetryCount *int64   `json:"retry_count,omitempty"`
    LatencyMs  *int64   `json:"latency_ms,omitempty"`
    CostUnits  *int64   `json:"cost_units,omitempty"`
}

func (s *Feedback) CreateCandidate(context.Context, CandidateInput) (domain.FeedbackCandidate, error)
func (s *Feedback) Candidate(context.Context, domain.ID) (domain.FeedbackCandidate, error)
func (s *Feedback) Candidates(context.Context, domain.FeedbackCandidateState) ([]domain.FeedbackCandidate, error)
```

- [ ] **Step 1: Write RED tests**

Prove candidate input is controlled:
- observations required;
- arbitrary logs/source code fields do not exist in the API;
- correlation key is deterministic for category + generic task class + executor kind + enforcement + normalized transition shape;
- timestamps do not change correlation key.

- [ ] **Step 2: Run RED**

```bash
go test ./internal/fieldfeedback -run Candidate -count=1
```

- [ ] **Step 3: Implement canonical correlation**

Canonicalize only safe categorical fields and SHA-256 the canonical JSON. Do not hash raw summary content into a value that later gets exported as if safe.

- [ ] **Step 4: Enforce state transitions**

Implement one private transition function and reject illegal transitions, especially:
- REPORTED → anything;
- REJECTED_UNSAFE → SANITIZED;
- LOCAL_ONLY → EXPORT_READY.

- [ ] **Step 5: Run GREEN**

```bash
go test ./internal/fieldfeedback -run Candidate -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/fieldfeedback
git commit -m "feat: add structured feedback candidate lifecycle"
```

---

### Task 6: Build fail-closed deterministic sanitizer and immutable SanitizedFeedback

**Files:**
- Create: `internal/fieldfeedback/sanitizer.go`
- Create: `internal/fieldfeedback/sanitizer_test.go`
- Create: `internal/fieldfeedback/testdata/sensitive_cases.json`

**Interfaces:**
```go
type Sanitizer interface {
    Sanitize(context.Context, domain.ID) (domain.SanitizedFeedback, domain.SanitizationResult, error)
}

type DeterministicSanitizerConfig struct {
    Version string
    DenyPatterns []*regexp.Regexp
    AllowExecutorMetadata bool
    AllowSyntheticReproduction bool
}

func NewDeterministicSanitizer(
    store *state.Store,
    clk clock.Clock,
    feedback *Feedback,
    cfg DeterministicSanitizerConfig,
) (*DeterministicSanitizer, error)
```

Export JSON is a typed private struct, not `map[string]any`.

- [ ] **Step 1: Write leak-corpus RED tests**

`testdata/sensitive_cases.json` includes representative:
- Windows and Unix paths;
- GitHub/internal URLs;
- IPv4/hostname;
- bearer/API/token-like secrets;
- email;
- ticket/account IDs;
- code/diff markers;
- configured project/customer deny names.

For every case, sanitizer must either remove it from final content or return REJECT/UNCERTAIN. Never PASS with the marker present.

- [ ] **Step 2: Add immutable/hash tests**

Prove:
- final content hash is SHA-256 of exact canonical serialized JSON;
- the DB trigger rejects UPDATE/DELETE;
- same canonical safe content gives same fingerprint;
- timestamps are excluded from fingerprint but included in artifact metadata;
- candidate changes after a prior sanitize create a new result/artifact, never mutate old one.

- [ ] **Step 3: Run RED**

```bash
go test ./internal/fieldfeedback -run Sanit -count=1
```

- [ ] **Step 4: Implement allowlist-first sanitizer**

Pipeline:
```go
candidate -> typed exportProjection -> preScan -> canonicalJSON -> postScan -> PASS artifact
```

If scan cannot classify safely, return `SanitizationUncertain` and no `SanitizedFeedback`.

- [ ] **Step 5: Keep semantic sanitizer interface-only**

Do not add an LLM dependency. Provide an optional `Abstracter` interface but default nil/no-op; deterministic post-scan remains authoritative.

- [ ] **Step 6: Run GREEN**

```bash
go test ./internal/fieldfeedback -run Sanit -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/fieldfeedback
git commit -m "feat: add fail-closed feedback sanitization"
```

---

### Task 7: Add field-feedback configuration and governed emit-task creation

**Files:**
- Modify: `internal/localconfig/config.go`
- Modify: `internal/localconfig/config_test.go`
- Create: `internal/fieldfeedback/emitter.go`
- Create: `internal/fieldfeedback/emitter_test.go`
- Modify: `internal/runtime/box.go`

**Interfaces:**
```go
type FeedbackMode string
const (
    FeedbackModeLocalOnly       FeedbackMode = "LOCAL_ONLY"
    FeedbackModeRequireApproval FeedbackMode = "REQUIRE_APPROVAL"
    FeedbackModeAutoIfAllowed   FeedbackMode = "AUTO_IF_ALLOWED"
)

type FieldFeedbackConfig struct {
    Enabled bool                  `json:"enabled"`
    Mode FeedbackMode             `json:"mode"`
    Provider string               `json:"provider,omitempty"`
    Destination string            `json:"destination,omitempty"`
    MaintenanceEnvelopeID domain.ID `json:"maintenance_envelope_id,omitempty"`
    DenyPatterns []string         `json:"deny_patterns,omitempty"`
}

func (s *Feedback) RequestEmit(context.Context, domain.ID) (domain.Task, error)
```

No token/credential fields are added to config.

- [ ] **Step 1: Write config RED tests**

Defaults:
- disabled;
- LOCAL_ONLY;
- no AdaptationGrant;
- no provider/destination/envelope.

Validate AUTO/REQUIRE_APPROVAL requires provider, destination, and maintenance envelope.

- [ ] **Step 2: Write governed-work RED tests**

Prove:
- LOCAL_ONLY never creates Task;
- no maintenance budget leaves feedback EXPORT_READY without Task;
- request creates one idempotent Task with:
  - purpose kind `COLLECTIVE_MAINTENANCE`;
  - capability `feedback.github.issue.create` for GitHub;
  - matching authority ceiling;
  - configured enforcement;
  - configured maintenance resource envelope;
- repeat request returns same active logical work item.

- [ ] **Step 3: Run RED**

```bash
go test ./internal/localconfig ./internal/fieldfeedback -run 'Config|Emit' -count=1
```

- [ ] **Step 4: Implement config and RequestEmit**

Create Task using existing `execution.Service.CreateTask`; do not call provider.

Persist feedback_id ↔ emit_task_id link so retries/wake do not mint unrelated Tasks.

- [ ] **Step 5: Run GREEN**

```bash
go test ./internal/localconfig ./internal/fieldfeedback -run 'Config|Emit' -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/localconfig internal/fieldfeedback internal/runtime
git commit -m "feat: schedule governed feedback emission"
```

---

### Task 8: Implement a feedback External Operation provider contract and fake provider

**Files:**
- Create: `internal/fieldfeedback/provider.go`
- Create: `internal/fieldfeedback/provider_test.go`
- Create: `internal/fieldfeedback/executor.go`
- Create: `internal/fieldfeedback/executor_test.go`
- Create: `internal/fieldfeedback/fake_sink_test.go`

**Interfaces:**
```go
type EmitIntent struct {
    SanitizedFeedbackID domain.ID
    Destination string
}
func (EmitIntent) DescriptorType() string { return "feedback.issue.emit.v1" }

type Sink interface {
    Create(context.Context, IssuePayload) (reference string, actualCost int64, err error)
    FindByMarker(context.Context, string) (reference string, found bool, err error)
}

type IssuePayload struct {
    Title string
    Body string
    Marker string
}
```

Provider implements existing `operations.Provider`.

The deterministic governed-work executor implements:

```go
type EmitTaskLookup interface {
    FeedbackForEmitTask(context.Context, domain.ID) (domain.SanitizedFeedback, string, error)
}

type EmitExecutor struct {
    feedback EmitTaskLookup
    operations *operations.Service
    providerName string
}

func (e *EmitExecutor) Start(
    ctx context.Context,
    envelope executors.AttemptEnvelope,
) (executors.ExecutionResult, error)
```

It resolves `envelope.TaskID -> SanitizedFeedback`, constructs `EmitIntent`, then calls `Operations.Prepare` and `Operations.Dispatch` using `envelope.AttemptID`. It never calls `Sink.Create` directly.

- [ ] **Step 1: Write RED provider-boundary tests**

Prove:
- `CanonicalIntent` rejects every descriptor except `EmitIntent`;
- intent contains only feedback ID + destination;
- provider loads immutable `SanitizedFeedback` internally;
- provider cannot be constructed without a FeedbackReader;
- rendered body comes only from `SanitizedFeedback.ContentJSON`;
- stable marker is `meeseek-feedback:<fingerprint>`;
- LookupOutcome uses marker and does not create.

- [ ] **Step 2: Run RED**

```bash
go test ./internal/fieldfeedback -run Provider -count=1
```

- [ ] **Step 3: Implement provider adapter**

Use `CostProfile` with explicitly bounded provider exposure. Fake sink returns deterministic references.

- [ ] **Step 4: Implement the deterministic governed-work executor**

Tests must prove:
- executor refuses a Task with no `feedback_emissions` linkage;
- executor uses the current fenced Attempt ID from its envelope;
- executor invokes `operations.Service`, never the sink;
- PREPARED/approval-pending returns a non-success result without provider dispatch;
- CONFIRMED_EFFECT produces deterministic evidence containing only feedback ID/provider reference, never local candidate text;
- OUTCOME_UNKNOWN is returned as an execution error so a replacement Attempt can reconcile later.

- [ ] **Step 5: Test unknown-outcome reconciliation through Operations**

Use fake sink that creates the issue but loses acknowledgement. Assert:
- first Dispatch → OUTCOME_UNKNOWN;
- replacement Attempt → LookupOutcome;
- DispatchCount/CreateCount == 1;
- final state CONFIRMED_EFFECT.

- [ ] **Step 5: Run GREEN**

```bash
go test ./internal/fieldfeedback ./internal/operations -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/fieldfeedback
git commit -m "feat: protect feedback sinks behind governed execution"
```

---

### Task 9: Implement the GitHub issue sink with mediated credentials and reconciliation

**Files:**
- Create: `internal/feedbackgithub/client.go`
- Create: `internal/feedbackgithub/provider.go`
- Create: `internal/feedbackgithub/provider_test.go`
- Modify: `cmd/meeseek-box/main.go`

**Interfaces:**
```go
type CredentialSource interface {
    Token(context.Context) (string, error)
}

type FileCredentialSource struct {
    Path string
}

type Config struct {
    APIBaseURL string
    Repository string
    CredentialSource CredentialSource
    HTTPClient *http.Client
}

func New(cfg Config, feedback FeedbackReader) (*Provider, error)
```

Production secret path is supplied separately from ordinary JSON config (environment variable may point to a root-owned/user-owned 0600 file; the token itself is not stored in local config).

- [ ] **Step 1: Write httptest RED tests**

Cover:
- Authorization header exists only on GitHub sink requests;
- token never appears in request body, canonical intent, error text, or returned provider reference;
- create uses `POST /repos/{owner}/{repo}/issues`;
- reconcile uses GitHub issue search/list endpoint and stable marker;
- 401/403 returns no sensitive response body;
- 5xx/connection-loss maps to unknown outcome where effect could have occurred;
- exact existing marker returns CONFIRMED_EFFECT without POST.

- [ ] **Step 2: Run RED**

```bash
go test ./internal/feedbackgithub -count=1
```

- [ ] **Step 3: Implement minimal `net/http` client**

Do not add a broad GitHub SDK. Limit response body reads, set explicit User-Agent, validate repository as `owner/name`, and sanitize API error messages.

- [ ] **Step 4: Implement file credential source**

Read at dispatch time; require regular file and reject group/world-readable mode on Unix. Never cache token into DTO/config/audit.

- [ ] **Step 5: Wire Box only when explicitly configured**

If provider config is absent, no GitHub feedback provider exists. Startup must not fail merely because dogfooding is disabled.

- [ ] **Step 6: Run GREEN**

```bash
go test ./internal/feedbackgithub ./cmd/meeseek-box -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/feedbackgithub cmd/meeseek-box
git commit -m "feat: add reconciled GitHub feedback sink"
```

---

### Task 10: Add operator control/CLI for feedback and approvals

**Files:**
- Modify: `internal/control/server.go`
- Modify: `internal/control/client.go`
- Modify: `internal/control/dto_json.go`
- Modify: `internal/control/server_test.go`
- Create: `cmd/meeseek/feedback_cmd.go`
- Create: `cmd/meeseek/feedback_cmd_test.go`
- Create: `cmd/meeseek/approvals_cmd.go`
- Modify: `cmd/meeseek/root.go`

**Interfaces:**
Control routes:
- `GET /feedback`
- `GET /feedback/{id}`
- `POST /feedback/{id}/emit`
- `GET /approvals`
- `GET /approvals/{id}`

CLI:
- `meeseek feedback list`
- `meeseek feedback inspect <id>`
- `meeseek feedback emit <id>`
- `meeseek approvals list`
- existing `approve`
- new `reject`

- [ ] **Step 1: Write RED API tests**

`feedback inspect` DTO has two explicit sections:
```go
type FeedbackInspectDTO struct {
    LocalCandidate *FeedbackCandidateDTO `json:"local_candidate,omitempty"`
    ExportArtifact *SanitizedFeedbackDTO `json:"export_artifact,omitempty"`
}
```

Never return local observation summaries in the export artifact.

- [ ] **Step 2: Write RED CLI tests**

Prove human output labels:
- `LOCAL — DO NOT EXPORT`
- `SANITIZED EXPORT ARTIFACT`
and emit says it scheduled governed work rather than “sent issue”.

- [ ] **Step 3: Run RED**

```bash
go test ./internal/control ./cmd/meeseek -run 'Feedback|Approval|Reject' -count=1
```

- [ ] **Step 4: Implement API/CLI**

Keep JSON DTOs stable and no secret fields.

- [ ] **Step 5: Run GREEN**

```bash
go test ./internal/control ./cmd/meeseek -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/control cmd/meeseek
git commit -m "feat: expose field feedback control surface"
```

---

### Task 11: Implement AdaptationGrant and evidence-driven ExperienceRule lifecycle

**Files:**
- Create: `internal/experience/service.go`
- Create: `internal/experience/service_test.go`
- Create: `internal/experience/evaluator.go`
- Create: `internal/experience/evaluator_test.go`

**Interfaces:**
```go
const AdaptationExecutorPreference = "EXECUTOR_PREFERENCE"

type GrantInput struct {
    Kind string
    ScopeKey string
    AllowedExecutors []string
    MinVerifiedSamples int
    MaxAcceptanceRegressionBps int
    MaxCostRegressionBps int
    ExpiresAt time.Time
}

type GrantRequest struct {
    ID domain.ID
    Digest string
    ApprovalID domain.ID
}

type ProposalInput struct {
    GrantID domain.ID
    GenericTaskClass string
    ScopeKey string
    PreferredExecutor string
    EvidenceObservationIDs []domain.ID
}

type VerifiedOutcome struct {
    TaskID domain.ID
    GenericTaskClass string
    ScopeKey string
    ExecutorKind string
    Accepted bool
    HumanIntervention bool
    RetryCount int64
    CostUnits *int64
    LatencyMs *int64
}

func (s *Service) RequestGrant(
    context.Context,
    GrantInput,
    domain.ID, // requesting actor
    []domain.ID, // required Owner approvers
) (GrantRequest, error)
func (s *Service) ActivateGrant(context.Context, domain.ID) (domain.AdaptationGrant, error)
func (s *Service) Propose(context.Context, ProposalInput) (domain.ExperienceProposal, error)
func (s *Service) ObserveVerifiedOutcome(context.Context, VerifiedOutcome) error
func (s *Service) Evaluate(context.Context, domain.ID) (domain.ExperienceRule, error)
func (s *Service) Preference(context.Context, PreferenceQuery) (Preference, bool, error)
```

- [ ] **Step 1: Write RED grant tests**

Prove:
- no implicit/default grant;
- `RequestGrant` creates an exact-digest durable approval request but no active grant;
- `ActivateGrant` fails before exact Owner approval;
- approval for a different grant digest cannot activate;
- expired grant cannot promote;
- preferred executor must be explicitly allowed;
- grant cannot name policy/authority/runtime modification kinds;
- required Owner approver set is non-empty;
- proposal evidence must exist.

- [ ] **Step 2: Write RED promotion tests**

Deterministic promotion:
- only accepted/verified Task outcomes count;
- model self-score does not exist in input;
- below `MinVerifiedSamples` remains CANDIDATE/SHADOW;
- sufficient non-regressing evidence promotes ACTIVE;
- configured regression rolls ACTIVE back to ROLLED_BACK;
- missing cost remains unknown and is not invented as zero.

- [ ] **Step 3: Run RED**

```bash
go test ./internal/experience -count=1
```

- [ ] **Step 4: Implement Owner-authorized grant activation**

Canonicalize `GrantInput`, hash it, persist `adaptation_grant_requests`, and create a generic approval request with subject kind `ADAPTATION_GRANT`. `ActivateGrant` requires the exact approval to be APPROVED, consumes it, then inserts the immutable active grant. Passing an Owner principal ID is never sufficient by itself.

- [ ] **Step 5: Implement exact evaluation**

Keep evaluator deterministic and integer/basis-point based where possible. Persist counters and evidence IDs used for each transition.

- [ ] **Step 6: Audit every promotion/rollback**

Append concise audit events with rule/grant/evidence IDs, never raw workload text.

- [ ] **Step 6: Run GREEN**

```bash
go test ./internal/experience -count=1
```

- [ ] **Step 8: Commit**

```bash
git add internal/experience
git commit -m "feat: add evidence-driven local experience rules"
```

---

### Task 12: Apply executor preference only after scheduler eligibility

**Files:**
- Modify: `internal/scheduler/service.go`
- Modify: `internal/scheduler/service_test.go`
- Modify: `internal/runtime/box.go`

**Interfaces:**
Add:
```go
type ExecutorPreference interface {
    PreferredExecutor(
        context.Context,
        domain.Task,
        []string,
    ) (string, bool, error)
}
```

Do not let the preference service receive or mutate task authority.

- [ ] **Step 1: Write RED scheduler safety tests**

Tests must prove:
1. preferred executor cannot make missing capability eligible;
2. preferred executor cannot satisfy insufficient enforcement;
3. preferred executor not in available executor set is ignored/rejected;
4. with two already eligible executor kinds, ACTIVE rule changes only executor choice;
5. rule rollback restores baseline choice.

- [ ] **Step 2: Run RED**

```bash
go test ./internal/scheduler -run Preference -count=1
```

- [ ] **Step 3: Separate task selection from executor choice**

Preserve current `Next` task ranking. Add a post-selection method such as:

```go
func (s *Service) ChooseExecutor(
    ctx context.Context,
    task domain.Task,
    eligibleExecutorKinds []string,
) (string, error)
```

Baseline deterministic choice is lexical/stable. Experience may reorder only `eligibleExecutorKinds`.

- [ ] **Step 4: Run GREEN and existing scheduler tests**

```bash
go test ./internal/scheduler -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler internal/runtime
git commit -m "feat: apply learned preference after scheduler eligibility"
```

---

### Task 13: Complete Box composition, production approval wiring, and field-dogfooding runtime flow

**Files:**
- Modify: `internal/runtime/box.go`
- Modify: `cmd/meeseek-box/main.go`
- Modify: `cmd/meeseek-box/main_test.go`
- Modify: `README.md`

**Interfaces:**
`runtime.Box` gains:
```go
Approvals    *approvals.Service
FieldObserver *fieldfeedback.Observer
Feedback     *fieldfeedback.Feedback
Sanitizer    fieldfeedback.Sanitizer
Experience   *experience.Service
```

The Box executor registry also includes the deterministic `feedback-emitter` executor when a feedback provider is configured. It receives no workspace and no workload credentials.

`runtime.Config` gains typed non-secret field-feedback config and optional provider dependencies.

- [ ] **Step 1: Write RED composition test**

Open a Box on fresh SQLite and assert all new services are non-nil even when outbound feedback is disabled. GitHub provider remains absent unless configured.

- [ ] **Step 2: Replace production approval stub**

Remove `unavailableApprovalService`. Wire real `box.Approvals` into control server.

- [ ] **Step 3: Register the governed feedback executor**

Construct the feedback provider first, then `fieldfeedback.EmitExecutor`, and register it as executor kind `feedback-emitter`. A queued feedback Task is therefore executable through the same scheduler/lease/executor abstraction as ordinary work.

Production `AUTO_IF_ALLOWED` means the feedback Task is created automatically. It is dispatched only when the normal worker/orchestration driver leases and runs it; this feature must not invent a second hidden scheduler.

- [ ] **Step 4: Wire detector/event hooks conservatively**

Do not create a new event bus. At this stage invoke deterministic observation detection at explicit lifecycle points where canonical state already exists (verification completion, failed/replacement attempt, operation reconciliation, explicit operator intervention). Keep hooks idempotent.

- [ ] **Step 5: Add safe runtime config documentation**

README must state:
- dogfooding disabled + LOCAL_ONLY by default;
- sanitizer boundary;
- separate secret-file credential;
- emit is governed work;
- local experience requires explicit AdaptationGrant;
- no source-code self-modification.

- [ ] **Step 6: Run component regression**

```bash
go test ./internal/runtime ./cmd/meeseek-box ./internal/control -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/runtime cmd/meeseek-box README.md
git commit -m "feat: compose field dogfooding runtime"
```

---

### Task 14: Prove end-to-end field feedback, privacy, approval, reconciliation, and local adaptation

**Files:**
- Create: `tests/acceptance/field_feedback_test.go`
- Create: `tests/acceptance/local_experience_test.go`
- Modify: `.github/workflows/ci.yml` only if the existing acceptance command does not already include these files.

**Interfaces:**
- Uses public service interfaces from Tasks 1–13.
- Produces no new runtime API except test helpers.

- [ ] **Step 1: Write the field-feedback acceptance slice**

Exact flow:

```text
realistic Task
-> failed/retried Attempt creates local observation
-> FeedbackCandidate
-> deterministic sanitizer PASS
-> immutable SanitizedFeedback
-> governed COLLECTIVE_MAINTENANCE Task
-> scheduler selects Task and feedback-emitter executor
-> scheduler leases fenced Attempt
-> feedback-emitter calls Operations.Prepare/Dispatch
-> policy REQUIRE_APPROVAL
-> PREPARED ExternalOperation + durable approval
-> signed Owner approval
-> dispatch fake GitHub sink
-> CONFIRMED_EFFECT
-> feedback Task verification/acceptance
-> candidate REPORTED
-> provider reference + audit evidence present
```

Assert raw observation summary, evidence content, local path marker, and secret marker do not occur in canonical outbound intent/payload.

- [ ] **Step 2: Add unknown-outcome acceptance case**

Fake sink creates issue then loses ACK. Revoke/replace Attempt and prove reconciliation finds marker and exactly one issue exists.

- [ ] **Step 3: Add negative privacy/authority table**

Exact subtests:
- `raw_observation_cannot_be_emitted`
- `modified_sanitized_artifact_cannot_dispatch`
- `uncertain_sanitizer_retains_local_only`
- `approval_for_feedback_a_cannot_authorize_b`
- `expired_approval_cannot_dispatch`
- `policy_deny_after_approval_wins`
- `feedback_provider_bypass_blocked_in_enforced_profile`
- `auto_mode_grants_no_authority`
- `missing_maintenance_budget_creates_no_task`
- `credential_never_enters_executor_context`

- [ ] **Step 4: Write local-experience acceptance slice**

Exact flow:

```text
Owner-created AdaptationGrant
-> multiple accepted verified outcomes
-> ExperienceProposal
-> SHADOW
-> deterministic promotion
-> scheduler receives two already eligible executors
-> preferred executor chosen
-> negative verified outcomes exceed rollback threshold
-> rule ROLLED_BACK
-> baseline executor choice restored
```

- [ ] **Step 5: Add negative adaptation subtests**

- `no_grant_no_automatic_adaptation`
- `unverified_outcome_does_not_count`
- `preference_cannot_make_missing_capability_eligible`
- `preference_cannot_lower_enforcement`
- `preference_cannot_expand_authority`
- `discovered_independent_problem_creates_new_work_not_scope_mutation`

- [ ] **Step 6: Run fresh targeted acceptance**

```bash
go test ./tests/acceptance -run 'FieldFeedback|LocalExperience' -count=1 -v
```

Expected: PASS with every named subtest executed; inspect output for accidental “[no tests to run]”.

- [ ] **Step 7: Run complete fresh verification**

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./cmd/meeseek ./cmd/meeseek-box
python3 docs/superpowers/research/spikes/2026-09-14-sqlite-persistence-spike.py
```

On Linux/Docker also run the existing OCI gate:

```bash
go test -tags=oci ./internal/teb ./tests/acceptance -count=1 -v
```

- [ ] **Step 8: Self-review the implementation against every acceptance criterion in the spec**

Create a closure ledger in the commit/PR description, not a new architecture document. Each of the 19 spec acceptance criteria must point to a test or concrete runtime evidence.

- [ ] **Step 9: Commit final acceptance work**

```bash
git add tests/acceptance .github/workflows/ci.yml
git commit -m "test: prove field feedback and local experience semantics"
```

- [ ] **Step 10: Mandatory reviewer checkpoint before Astra**

Run the Superpowers requesting-code-review workflow over the entire diff from `main` to `feat/field-dogfooding-feedback`. Resolve all Critical/Important findings or explicitly document why a finding is false positive with evidence.

Do **not** merge this branch. The next external gate is the requested Astra review of the complete Collective implementation.

---

## Plan Self-Review Checklist

Before execution, verify:

- [ ] Every spec section 1–24 maps to at least one task above.
- [ ] Every spec acceptance criterion maps to Task 14.
- [ ] Approval is generic and durable, not feedback-specific.
- [ ] AdaptationGrant activation is Owner-authorized through exact durable approval; passing an Owner ID never creates authority.
- [ ] Governed feedback work has a deterministic executor that invokes Operations rather than the sink directly.
- [ ] Observation evidence, feedback emission linkage, and verified experience outcomes have durable normalized persistence.
- [ ] `REQUIRE_APPROVAL` can PREPARE but cannot DISPATCH before exact approval.
- [ ] Current policy/authority is rechecked after approval.
- [ ] `ALLOW_WITH_LIMIT` does not silently become ALLOW.
- [ ] Raw observation data has no provider API path.
- [ ] Sanitized artifact is immutable and hash-bound.
- [ ] Feedback export is a normal Task/Attempt, never a CLI/provider shortcut.
- [ ] GitHub reconciliation prevents duplicate issue effects.
- [ ] Credential source is outside ordinary config and executor-visible intent.
- [ ] Local adaptation has no default grant.
- [ ] Only verified outcomes promote rules.
- [ ] Learned preference executes after eligibility filtering.
- [ ] No source-code self-modification or Maintainer Collective implementation leaked into scope.
- [ ] No TODO/TBD/FIXME/placeholders remain.
