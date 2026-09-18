# Field Dogfooding, Sanitized Feedback, and Local Experience — Design

**Date:** 2026-09-18  
**Status:** Proposed design for Owner review  
**Branch:** `feat/field-dogfooding-feedback`  
**Primary issue:** #1 — Add field dogfooding and sanitized feedback loop  
**Architecture basis:** `docs/superpowers/specs/2026-09-14-meeseek-collective-design.md`

## 1. Context

The Minimal Viable Collective now implements the single-Cube semantic vertical slice: durable Task/Attempt state, evidence and independent verification, resource accounting, policy, capability enforcement, protected External Operations, memory/audit, scheduler/wake semantics, executors, and local control.

Issue #1 adds a field-evaluation loop so a Collective can perform real work in a private or organizational workload environment, observe where its own orchestration fails or creates friction, and convert those observations into privacy-safe feedback without exporting workload data.

The design also adds a deliberately narrow local-experience loop. A Collective may improve selected low-risk orchestration preferences from verified outcomes, but evidence never manufactures authority and learning never bypasses feasibility, policy, TEB, or Constitution.

This extends existing architecture rather than creating a parallel learning system:

- §9 already models `OBSERVATION`, `FAILURE`, `LESSON`, evidence, provenance, and Knowledge Delta.
- §12 already defines outcome-driven process improvement.
- §22 already requires causal audit and deterministic operational observability.
- §25 already defines self-modification as an authority domain whose scope may expand only through demonstrated performance and explicit Owner grants.
- §29 already provides the Commit Boundary and External Operation semantics required for safe issue creation.

## 2. Goals

The implementation SHALL:

1. Capture durable local observations about Collective behavior during real workloads.
2. Separate workload-local observations from any representation eligible to leave the environment.
3. Produce structured feedback candidates from deterministic detectors, operator feedback, or agent-proposed hypotheses.
4. Sanitize candidates through a fail-closed declassification boundary.
5. Make `SanitizedFeedback` immutable and the only feedback artifact an outbound sink may consume.
6. Export feedback through the normal protected External Operation path.
7. Support local-only, Owner-approval-required, and policy-permitted automatic feedback modes.
8. Provide exact deduplication and stable correlation for related feedback.
9. Preserve version, state-transition, enforcement, cost/latency, recovery, and human-intervention metadata needed for later diagnosis.
10. Record enough evidence and audit data to explain why feedback was created, sanitized, approved, and emitted.
11. Add a narrow local-experience mechanism that can improve executor preference among already eligible execution paths.
12. Make automatic local adaptation conditional on verified outcomes and an explicit Owner-authorized adaptation grant.
13. Ensure discovery of a new problem creates new governed work rather than silently widening the current change.
14. Preserve all existing authority, capability, fencing, TEB, resource, and verification invariants.

## 3. Non-goals

This design does NOT implement:

- automatic source-code self-modification;
- a Maintainer Collective that autonomously changes or releases Meeseek Collective;
- uploading source code, diffs, prompts, repository contents, or arbitrary logs for diagnosis;
- general model training on workload data;
- semantic near-duplicate clustering requiring an external embedding service;
- organization-specific sanitization rules hard-coded into core;
- automatic authority expansion;
- automatic policy, Constitution, TEB, runtime, or scheduler-semantic changes from learned experience;
- multi-Cube feedback aggregation;
- a remote central telemetry service.

A future Maintainer Collective may consume sanitized issues and improve the framework through the normal engineering lifecycle, but that is explicitly deferred.

## 4. Architectural principles

### 4.1 Evidence-triggered improvement

Self-improvement must have an evidence-bearing trigger: a field observation, repeated pattern, regression, benchmark degradation, failure, operator correction, experiment result, or a new finding discovered while working an existing issue.

An unsupported idea may become a hypothesis or candidate work item. It does not itself authorize a production change.

### 4.2 Discovery creates work; discovery does not authorize change

If work on Issue A reveals independent Problem B, the default action is to create a separate governed work item for B with its own scope, evidence, authority, verification, and lifecycle.

Discovery is evidence that work may be valuable; it is not authorization to expand scope arbitrarily.

### 4.3 Evidence does not manufacture authority

Outcome history may justify promotion inside an existing Owner grant or support a proposal for more authority. It cannot create a new authority ceiling by itself.

### 4.4 Sanitization and authorization are separate gates

A report that is safe to disclose may still be unauthorized to emit. An authorized feedback channel may still receive nothing if sanitization cannot prove the artifact safe.

Both gates fail closed.

### 4.5 Raw observations never cross the feedback boundary

`FieldObservation` and its raw local evidence are not valid inputs to an outbound feedback provider.

Only an immutable `SanitizedFeedback` ID is accepted by the feedback External Operation provider.

### 4.6 Local adaptation cannot change eligibility

Learned scheduler preferences may rank execution paths only after normal eligibility filtering.

A learned preference cannot:

- make an authority-invalid path eligible;
- weaken required enforcement;
- bypass capability/locality checks;
- exceed resource envelopes;
- disable approval;
- change Constitution or policy;
- bypass fencing or the Commit Boundary.

## 5. High-level data flow

```text
real workload
    |
    v
Task / Attempt / Executor / Verification / Operations / Operator
    |
    v
FieldObservation ------------------------+
    |                                    |
    |                                    +--> Local Experience evaluation
    |                                           |
    |                                           v
    |                                   ExperienceProposal
    |                                           |
    |                             evidence + AdaptationGrant
    |                                           |
    |                                           v
    |                              bounded ExperienceRule
    |
    v
FeedbackCandidate
    |
    v
Sanitization pipeline
    |
    +--> REJECTED_UNSAFE / LOCAL_ONLY
    |
    v
SanitizedFeedback (immutable)
    |
    v
policy + authority + optional durable approval
    |
    v
ExternalOperation PREPARED -> DISPATCHED
    |
    v
FeedbackSink (initially GitHub Issues)
```

The local-experience branch and outbound-feedback branch share observations and evidence but are otherwise independent.

## 6. Domain model

### 6.1 FieldObservation

A `FieldObservation` is a local durable record describing behavior of the Collective.

Required fields:

- `observation_id`;
- `collective_id`;
- optional `task_id`, `attempt_id`, `operation_id`;
- `category`;
- `basis_class`;
- `source_kind`;
- `summary_local`;
- structured local metrics;
- evidence IDs / event IDs;
- runtime version / commit;
- executor kind/version where known;
- enforcement level where relevant;
- creation timestamp.

Initial categories:

- `DECOMPOSITION`;
- `REDUNDANT_DELEGATION`;
- `EXECUTOR_SELECTION`;
- `RECOVERY`;
- `APPROVAL_FRICTION`;
- `STALE_STATE`;
- `COST`;
- `POLICY_CAPABILITY_FRICTION`;
- `HUMAN_INTERVENTION`;
- `VERIFICATION_QUALITY`;
- `OBSERVABILITY_UX`;
- `OTHER`.

`basis_class` follows the existing audit vocabulary where practical: deterministic rule, evidence-based observation, historical pattern, model estimate, heuristic judgment, or human direction.

A model-proposed observation is a hypothesis until supported by evidence. Multi-agent repetition does not by itself promote confidence.

### 6.2 FeedbackCandidate

A `FeedbackCandidate` is a local hypothesis that one or more observations are useful framework feedback.

It contains only a controlled schema, not arbitrary workload logs:

- `candidate_id`;
- linked observation IDs;
- generic task class;
- feedback category;
- expected Collective behavior;
- observed Collective behavior;
- normalized state-transition summary;
- normalized cost/latency/retry data;
- human-intervention flag;
- recovery result;
- runtime/executor/enforcement metadata;
- correlation key;
- candidate state.

Candidate states:

```text
CANDIDATE
-> SANITIZATION_PENDING
-> SANITIZED
-> APPROVAL_PENDING | EXPORT_READY
-> REPORTED

CANDIDATE/SANITIZATION_PENDING
-> LOCAL_ONLY | REJECTED_UNSAFE | REJECTED_POLICY

SANITIZED
-> DUPLICATE
```

The state machine is monotonic except that a new candidate revision may be created from the same observations after sanitizer/ruleset changes. Existing sanitized artifacts are never rewritten.

### 6.3 SanitizationResult

Each sanitization attempt records:

- `sanitization_id`;
- candidate ID;
- sanitizer/ruleset version and hash;
- input candidate digest;
- deterministic scan result;
- optional semantic-abstraction result metadata;
- post-scan result;
- final outcome: `PASS`, `REJECT`, or `UNCERTAIN`;
- reason codes;
- timestamp.

`UNCERTAIN` is equivalent to fail closed for egress.

### 6.4 SanitizedFeedback

`SanitizedFeedback` is an immutable export-eligible artifact created only after `PASS`.

It contains an allowlisted schema:

- `feedback_id`;
- schema version;
- candidate ID;
- Collective version/commit;
- executor type/version when explicitly marked safe;
- generic task class;
- execution/enforcement profile;
- expected behavior;
- observed behavior;
- failure/friction category;
- generic state transitions;
- normalized cost/latency/retry observations;
- human-intervention flag;
- recovery result;
- correlation key / exact fingerprint;
- optional synthetic reproduction;
- sanitization attestation: sanitizer version, ruleset hash, result ID;
- creation timestamp;
- immutable content hash.

The artifact MUST NOT contain raw local evidence IDs that allow an external sink to dereference workload data.

### 6.5 ApprovalRequest

The existing domain already defines `REQUIRE_APPROVAL` and the control transport already supports Owner-signed approval challenges, but the production adapter currently fails closed because no durable approval service exists.

This feature SHALL implement the missing generic durable approval primitive rather than a feedback-specific bypass.

An `ApprovalRequest` binds:

- `approval_id`;
- subject kind and subject ID;
- exact request/intent digest;
- policy decision ID and policy provenance;
- requesting actor;
- required approver principal(s);
- state: `PENDING | APPROVED | REJECTED | EXPIRED | CONSUMED`;
- created/expires/decided timestamps;
- approver principal and signature provenance.

Owner approval is valid only for the exact immutable request digest. Changed feedback content or changed consequential intent requires a new approval.

The control challenge SHALL sign a digest derived from the durable approval request, not merely an unbound human-readable action.

### 6.6 ExperienceProposal and ExperienceRule

Local experience is intentionally narrower than general self-modification.

An `ExperienceProposal` contains:

- scope: generic task class and optional local workload/repository scope identifier;
- proposed preference;
- evidence/observation IDs;
- baseline outcome statistics;
- expected effect;
- confidence;
- creation timestamp.

The first implemented adaptation kind is:

`EXECUTOR_PREFERENCE`

It may prefer one executor/capability path over another only among paths already declared eligible by the deterministic scheduler.

An active `ExperienceRule` additionally records:

- rule version;
- AdaptationGrant ID;
- promotion evidence;
- evaluation window;
- outcome counters;
- status `CANDIDATE | SHADOW | ACTIVE | ROLLED_BACK | RETIRED`;
- rollback condition;
- created/activated/rolled-back timestamps.

## 7. Observation capture

Observation capture is not one giant model call.

The system supports three sources:

1. **Deterministic detectors** over durable runtime/audit/economic state.
2. **Operator feedback** supplied through the local control/CLI surface.
3. **Agent-proposed observations** produced during ordinary work and treated as hypotheses until evidence supports them.

Initial deterministic detectors SHOULD cover high-signal conditions already represented in canonical state:

- retry count / repeated replacement Attempts above configured threshold;
- recovery following interruption or stale Attempt;
- repeated policy/authority denial;
- unresolved external-operation reconciliation;
- resource/cost outlier relative to a local baseline when enough samples exist;
- explicit human intervention event;
- verification challenge or later operator rejection after an accepted result.

Detectors create observations; they do not directly emit issues.

Observation creation is local and does not require outbound authority.

## 8. Sanitization and declassification boundary

### 8.1 Allowlist-first projection

Sanitization starts by projecting a candidate into the export schema. Fields not explicitly exportable are dropped by construction.

No generic `map[string]any` or arbitrary log blob may pass directly to `SanitizedFeedback`.

### 8.2 Deterministic pre-scan

The sanitizer rejects or removes known sensitive forms, including at minimum:

- repository / organization identifiers when not explicitly allowlisted;
- customer/client/project names supplied by local classification;
- absolute and relative workload file paths;
- source-code/diff fragments;
- URLs, internal hostnames, IPs where not explicitly allowed;
- secrets, tokens, credential-like strings;
- account IDs, ticket IDs, internal identifiers;
- email addresses and user identifiers where not required;
- arbitrary logs;
- prompt/context dumps.

The implementation SHALL support environment-provided additional deny patterns without hard-coding employer-specific names into core.

### 8.3 Semantic abstraction

A semantic sanitizer MAY transform expected/observed behavior or reproduction text into a generic form.

If an LLM is used, it is not itself proof of safety. Its output must pass the same deterministic post-scan.

The initial implementation SHALL define the sanitizer as an interface and ship a deterministic implementation. Optional model-backed abstraction may be added only behind the same boundary and tests.

### 8.4 Deterministic post-scan and attestation

Before a `SanitizedFeedback` artifact is created, the final serialized representation is scanned again.

Only a PASS creates the immutable artifact and sanitization attestation.

Any uncertainty remains local.

### 8.5 Synthetic reproduction

Reproduction data must be generated from generic/synthetic names and content.

A reproduction derived from real workload material must be reconstructed from the abstracted problem, not copied then superficially redacted.

## 9. Feedback export

### 9.1 Generic sink contract

Core depends on a `FeedbackSink`/provider abstraction, not GitHub semantics.

Initial implementation: GitHub Issues provider.

Future providers may include local JSONL, private GitHub, GitLab, Azure DevOps, Jira, or a dedicated evaluation service.

### 9.2 Protected External Operation

Feedback emission is a consequential external effect.

The provider descriptor accepts:

- `sanitized_feedback_id`;
- configured sink identity / destination.

It does NOT accept arbitrary title/body text from the executor.

The provider loads the immutable sanitized artifact and renders the issue payload internally.

Normal External Operation semantics apply:

- authority ceiling;
- policy decision;
- enforcement level;
- resource reservation;
- stable effect slot;
- PREPARED/DISPATCHED boundary;
- reconciliation after unknown outcome;
- audit;
- deduplication.

### 9.3 GitHub provider

The GitHub issue provider SHALL:

- use an explicitly configured destination repository;
- require an explicitly mediated/scoped credential; no ambient credential inheritance;
- render title/body only from `SanitizedFeedback`;
- include a stable non-sensitive dedupe marker such as `meeseek-feedback:<fingerprint>`;
- store returned issue number/URL as provider reference;
- reconcile `OUTCOME_UNKNOWN` by searching for the stable marker before creating another issue;
- never receive local observation data or raw evidence.

The provider capability is separate from ordinary GitHub/code capabilities, e.g. `feedback.github.issue.create`.

### 9.4 Feedback modes

Configuration supports:

- `LOCAL_ONLY` — never emit;
- `REQUIRE_APPROVAL` — every candidate must be sanitized, policy-allowed, and explicitly Owner-approved;
- `AUTO_IF_ALLOWED` — sanitized feedback may emit automatically when policy, authority, resource, and provider checks allow it.

Changing mode does not grant authority. The Task/Collective still needs the appropriate capability and policy outcome.

## 10. Durable approval integration

`operations.Service` currently treats every policy outcome other than `ALLOW` as denied. This feature SHALL add first-class handling for `REQUIRE_APPROVAL`.

Semantics:

1. PREPARE evaluates current policy and authority.
2. For `ALLOW`, existing behavior continues.
3. For `ALLOW_WITH_LIMIT`, limits must be deterministically enforceable before continuing.
4. For `REQUIRE_APPROVAL`, the operation may be durably PREPARED with its exact intent, reservation, policy provenance, and linked PENDING approval request.
5. DISPATCH is impossible while required approval is absent, rejected, expired, consumed by a different subject, or bound to a different digest.
6. After approval, DISPATCH rechecks current lease, authority, policy, enforcement path, reservation, intent fingerprint, and approval binding.
7. If current policy now denies the effect, historical approval does not override the new denial.
8. Approval is consumed/idempotently associated with the operation dispatch semantics.

This general mechanism replaces the production `unavailableApprovalService` adapter and makes the already-existing `meeseek approve` path durable.

## 11. Deduplication and correlation

### 11.1 Exact dedupe

A stable feedback fingerprint is computed from the canonical sanitized schema, excluding timestamps and other non-semantic metadata.

The same sanitized effect for the same sink resolves to the same logical effect slot.

### 11.2 Correlation

A broader `correlation_key` groups likely-related feedback using safe categorical fields such as:

- category;
- generic task class;
- executor kind;
- enforcement profile;
- normalized state-transition/failure shape.

This satisfies the first implementation need for near-duplicate correlation without introducing an embedding service.

Semantic clustering can be added later.

## 12. Local experience and earned adaptation

### 12.1 What may adapt automatically

The first automatic adaptation surface is deliberately constrained to scheduler preference among already eligible paths.

Example:

> for generic task class `review`, prefer deterministic inspection before agentic executor X when historical verified outcomes show lower retry/cost with no acceptance regression.

A rule cannot select an otherwise-ineligible executor.

### 12.2 AdaptationGrant

Automatic activation requires an explicit Owner-authorized `AdaptationGrant` declaring:

- allowed adaptation kind;
- allowed scope;
- maximum semantic effect;
- required evidence classes;
- minimum verified sample count;
- acceptable regression/override conditions;
- rollback conditions;
- expiry/review time.

The grant controls what the Collective may learn automatically. Evidence controls whether a candidate has earned activation inside that grant.

### 12.3 Outcome evidence

Only verified outcomes count toward automatic promotion.

Useful measurements include:

- accepted Task outcome;
- verification/challenge result;
- retries;
- human intervention;
- recovery success;
- cost/latency/resource usage;
- subsequent rollback or operator correction.

Unknown values remain unknown.

A model's self-assessment does not count as verified success.

### 12.4 Promotion and rollback

Conceptual lifecycle:

```text
observation pattern
-> ExperienceProposal
-> CANDIDATE
-> SHADOW
-> ACTIVE
-> keep | ROLLED_BACK | RETIRED
```

Promotion is deterministic against the active AdaptationGrant.

An active rule is automatically rolled back or suspended when its configured regression/override condition is met.

### 12.5 No authority ratchet from performance alone

Successful performance may generate an `AuthorityExpansionProposal`, but only the Owner/authorized constitutional mechanism may grant it.

The framework must distinguish:

- earned evidence that broader autonomy may be safe;
- actual authority to exercise broader autonomy.

## 13. Work discovery semantics

When an agent or detector discovers an independent framework problem during current work:

1. record a new `FieldObservation`;
2. classify whether it belongs to current acceptance criteria;
3. if independent, create a separate local work candidate / sanitized feedback candidate;
4. do not silently modify unrelated framework behavior;
5. let normal scheduling, authority, budget, and review decide whether/when it is worked.

This encodes:

> **Discovery creates work; discovery does not authorize change.**

## 14. Security and privacy invariants

The implementation MUST preserve these invariants:

1. Raw observation/evidence objects are never accepted by outbound feedback providers.
2. A sanitizer cannot mark its own uncertain result as safe.
3. Sanitization PASS is bound to the exact immutable serialized artifact hash.
4. Modifying content after sanitization invalidates export eligibility.
5. Approval is bound to an exact subject/request digest.
6. Approval cannot override a later policy/authority denial.
7. Feedback credentials are scoped and mediated; no ambient executor credentials.
8. Agentic executors cannot call the feedback sink directly in an ENFORCED profile.
9. `AUTO_IF_ALLOWED` adds no authority.
10. Learned experience rules run after scheduler feasibility/authority/enforcement filtering.
11. Local experience cannot edit policy, Constitution, TEB configuration, authority records, runtime code, or scheduler eligibility semantics.
12. All export attempts and adaptation promotions are auditable.
13. Workload-specific deny patterns/configuration stay local and are never copied into emitted feedback.
14. Failures in sanitization, policy, approval, provider dispatch, or reconciliation fail closed.

## 15. Persistence

Add a migration after `00009_memory_audit.sql` with durable tables for:

- `field_observations`;
- `feedback_candidates`;
- `feedback_candidate_observations`;
- `sanitization_results`;
- `sanitized_feedback`;
- `approval_requests`;
- `experience_proposals`;
- `experience_rules`;
- `experience_rule_evidence`;
- `adaptation_grants`.

Where practical, existing evidence/audit/external-operation tables remain canonical rather than duplicating their data.

Large/raw local diagnostic artifacts belong in the existing evidence store with hashes and references, not duplicated into feedback rows.

## 16. Services and Box composition

Add focused services rather than a monolithic learning subsystem:

- `FieldObserver` — durable observation capture and deterministic detectors;
- `Feedback` — candidate lifecycle, correlation, and query API;
- `Sanitizer` — allowlist projection, scan, attestation;
- `Approvals` — generic durable approval requests and Owner decisions;
- `Experience` — proposal/rule lifecycle and outcome evaluation;
- GitHub feedback provider under the existing External Operations abstraction.

`runtime.Box` wires these explicitly.

Existing services remain responsible for their domains:

- `Execution` owns Task/Attempt lifecycle;
- `Verification` owns acceptance;
- `Resources` owns cost/exposure;
- `Operations` owns external-effect commitment;
- `Memory` owns claims/evidence semantics;
- `Audit` owns causal records;
- `Scheduler` owns eligibility and arbitration.

The Experience service supplies bounded preference hints; it does not become another scheduler.

## 17. Control and CLI surface

Initial operator surface:

```text
meeseek feedback list
meeseek feedback inspect <candidate-or-feedback-id>
meeseek feedback emit <feedback-id>
meeseek approvals list
meeseek approve <approval-id>
meeseek experience list
meeseek experience inspect <rule-id>
```

`feedback inspect` must clearly distinguish LOCAL raw/candidate information from the exact immutable artifact proposed for export.

Approval UI/CLI must show the exact sanitized representation/digest being approved, not merely an opaque ID.

## 18. Configuration

Local config gains a field-feedback section with safe defaults:

- feedback enabled: false by default;
- mode: `LOCAL_ONLY` by default;
- configured sink/provider;
- destination identity;
- local deny-pattern sources;
- detector thresholds;
- optional AdaptationGrant references.

Secrets/tokens are not stored as ordinary feedback config values.

Enabling dogfooding does not automatically enable outbound feedback or learned adaptation.

## 19. Observability, audit, and economics

Metrics/events should distinguish:

- observations created by source/category;
- candidates created;
- sanitizer PASS/REJECT/UNCERTAIN;
- candidates retained LOCAL_ONLY;
- approvals requested/approved/rejected/expired;
- feedback emitted/reconciled/duplicate;
- feedback provider cost/latency;
- experience proposals/rules promoted/rolled back;
- verified outcome statistics by rule version;
- human override/intervention rates.

Audit should answer:

- What observation caused this candidate?
- What evidence existed?
- What sanitizer/ruleset produced the exported artifact?
- Why was it permitted or denied?
- What exact artifact was approved?
- What External Operation emitted it?
- What issue/provider reference resulted?
- What evidence caused a local experience rule to activate or roll back?

## 20. Failure and recovery semantics

### Box crash

All candidate, sanitization, approval, operation, and experience state is durable. Restart resumes from the last durable state.

### Sanitizer crash/uncertainty

No `SanitizedFeedback` is created until PASS commits atomically with its immutable hash and result reference.

### Crash after issue creation but before response persistence

External Operation becomes/re-enters `OUTCOME_UNKNOWN`; provider reconciliation searches the stable marker before any retry.

### Approval after intent change

Rejected as stale/mismatched. A new approval request is required.

### Policy changes after approval

Dispatch re-evaluates policy. A new DENY wins.

### Adaptation regression

The bounded ExperienceRule is suspended/rolled back according to its grant; the underlying authority and scheduler feasibility semantics are unchanged.

## 21. Testing strategy

### Unit tests

Cover:

- observation validation/provenance;
- candidate state transitions;
- allowlist projection;
- deterministic secret/path/identifier scanning;
- sanitizer fail-closed behavior;
- immutable content hashing;
- approval digest binding and expiry;
- policy `REQUIRE_APPROVAL` handling;
- exact feedback fingerprinting;
- correlation keys;
- AdaptationGrant validation;
- experience promotion and rollback;
- scheduler preference never changing eligibility.

### Integration tests

Cover:

- Task/Attempt events -> FieldObservation -> Candidate;
- sanitizer PASS -> immutable feedback;
- sanitizer uncertainty -> no export artifact;
- require-approval -> PREPARED operation cannot dispatch;
- signed Owner approval -> exact operation may dispatch;
- policy revocation after approval -> dispatch denied;
- feedback GitHub provider through Operations;
- unknown outcome -> reconciliation -> no duplicate issue;
- no raw evidence accessible to provider;
- experience rule improves only tie-break/ranking among eligible paths.

### Acceptance tests

Add a field-dogfooding vertical slice:

```text
realistic Task
-> observable orchestration friction
-> local FieldObservation
-> FeedbackCandidate
-> sanitization PASS
-> policy REQUIRE_APPROVAL
-> Owner approval
-> ExternalOperation
-> fake GitHub sink CONFIRMED_EFFECT
-> provider reference stored
-> audit/evidence complete
```

And a local-experience slice:

```text
verified outcomes
-> repeated evidence pattern
-> ExperienceProposal
-> valid AdaptationGrant
-> SHADOW
-> deterministic promotion
-> scheduler chooses preferred already-eligible executor
-> regression evidence
-> automatic rollback
```

Negative acceptance cases must prove:

- raw observation cannot be emitted;
- modified sanitized body invalidates attestation;
- uncertain sanitizer cannot export;
- approval for feedback A cannot approve B;
- expired approval cannot dispatch;
- direct provider bypass fails under enforced capability profile;
- learned rule cannot make an ineligible path eligible;
- learned rule cannot expand authority;
- discovery of unrelated issue does not silently mutate current task scope.

The full pre-existing MVC verification matrix remains required.

## 22. Delivery scope before Astra review

The branch intended for Astra review should contain the complete implementation of:

1. field observation persistence/service;
2. feedback candidate lifecycle;
3. deterministic fail-closed sanitizer and immutable sanitized artifact;
4. exact dedupe + safe correlation;
5. durable generic approval service and policy `REQUIRE_APPROVAL` support;
6. protected feedback External Operation provider abstraction;
7. fake/local sink tests plus GitHub provider contract/reconciliation implementation without embedded credentials;
8. operator CLI/control surfaces;
9. bounded local ExperienceRule + AdaptationGrant for executor preference;
10. runtime Box wiring;
11. acceptance/failure semantics;
12. README/security documentation.

No Maintainer Collective, self-code modification, automatic PR implementation, or release automation is part of this branch.

## 23. Future direction: Maintainer Collective

A future separate Collective may run continuously on private infrastructure and consume sanitized issues from Field Collectives.

Its mission may be to maintain Meeseek Collective through:

```text
sanitized issue corpus
-> triage / clustering
-> reproduce
-> root cause
-> spec / implementation
-> independent QA
-> benchmark against baseline
-> review / rollout
-> field evidence
```

A problem discovered while processing one issue becomes another governed issue rather than an implicit permission to modify unrelated code.

The Maintainer Collective may eventually earn authority for specific low-risk engineering/release classes through verified outcomes and explicit Owner grants, but source-code self-improvement remains an ordinary authority domain.

This future direction is documented now so the field-feedback schema preserves useful provenance, correlation, version, and outcome data without prematurely building the maintainer system.

## 24. Acceptance criteria

The design is complete when implementation can demonstrate:

- [ ] A real workload run can create a local `FieldObservation`.
- [ ] Raw observations and evidence are structurally non-exportable through the feedback provider.
- [ ] A candidate can be sanitized into an immutable allowlisted `SanitizedFeedback`.
- [ ] Sanitization uncertainty fails closed.
- [ ] Sanitization and outbound authorization are independent.
- [ ] Outbound issue creation uses protected External Operation semantics.
- [ ] Unknown issue-creation outcomes reconcile without duplicate effects.
- [ ] An environment can remain LOCAL_ONLY.
- [ ] An environment can require exact Owner approval for every emitted feedback artifact.
- [ ] An environment can allow automatic emission only when existing policy/authority allows it.
- [ ] The report contains enough version/state/outcome metadata for diagnosis without workload content.
- [ ] Duplicate reports can be deduplicated and related reports correlated.
- [ ] Feedback mode grants no new executor credentials or authority.
- [ ] A narrow local experience rule can be proposed from verified outcomes.
- [ ] Automatic local activation requires an explicit AdaptationGrant.
- [ ] Learned preferences cannot alter scheduler eligibility or authority.
- [ ] Regression evidence can roll back an active learned preference.
- [ ] New independent findings become new governed work rather than silent scope expansion.
- [ ] Existing MVC tests plus new feedback/approval/experience tests are green.
