# Meeseek Collective — MVC Implementation Architecture Decisions

**Status:** Proposed implementation architecture; ready for Owner review  
**Date:** 2026-09-14  
**Scope:** Minimal Viable Collective (single Cube semantic vertical slice)  
**Inputs:**
- `docs/superpowers/specs/2026-09-14-meeseek-collective-design.md`
- `docs/superpowers/research/2026-09-14-oss-landscape-mapping.md`
- `docs/superpowers/research/2026-09-14-mvc-architecture-spike-findings.md`

---

## 1. Architecture Goal

The MVC must implement the approved Meeseek semantics without prematurely building the distributed future.

The architecture therefore optimizes for:

1. correctness of Task/Attempt/External Operation semantics;
2. technically enforceable authority boundaries;
3. crash recovery and duplicate-effect safety;
4. one-Cube operability with near-zero infrastructure;
5. replaceable adapters for models, tools, policy, memory projections and storage;
6. a clean path to Postgres/NATS/SPIRE/multi-Cube later without changing domain meaning.

The governing implementation principle is:

> **The simplest architecture that preserves the full approved semantics wins.**

---

# 2. Decision Summary

| Area | MVC decision |
|---|---|
| Core language | **Go 1.27 line** |
| Primary process | **single Box daemon + CLI**, executors out-of-process |
| Canonical state | **SQLite**, local disk, WAL + `synchronous=FULL` |
| State model | canonical relational state + append-oriented causal/audit events |
| Durable workflow framework | **none required** |
| AgentLedger | conformance/design donor; no canonical runtime dependency |
| Scheduler | Meeseek-owned deterministic scheduler |
| Policy evaluation | embedded **OPA** behind Meeseek `PolicyEngine` |
| Enforcement | Meeseek-owned **Trusted Enforcement Boundary** |
| Agentic isolation | Linux/OCI reference profile; stronger gVisor optional later |
| External operations | Box-owned Commit Boundary and operation dispatcher |
| Dedup identity | Box-assigned task-semantic `logical_effect_key` |
| Agentic tool surface | attempt-scoped capability surface; **MCP** where harness supports it |
| Deterministic executor | built-in command/process executor |
| Canonical memory | SQLite Evidence/Event/Claim model |
| Knowledge graph | no required graph DB in MVC; Graphiti projection later |
| Evidence blobs | content-addressed local blob store + SQLite metadata |
| Observability | OpenTelemetry instrumentation; audit remains canonical DB state |
| Identity | public-key principals + signer abstraction; separate constitutional root |
| Distributed identity | SPIFFE/SPIRE deferred |
| Event bus | none in MVC; NATS deferred |
| Shared DB | Postgres deferred |
| UI | CLI/local control only; no web UI required |

---

# 3. ADR-001 — Core Runtime Language: Go

## Decision

The Meeseek semantic core, Box daemon, CLI, scheduler, persistence layer, policy adapter, Resource Ledger, built-in capability providers, and built-in executor adapters are implemented in **Go**.

Initial module baseline: **Go 1.27**.

The project should track a supported Go release over time rather than permanently pinning the architecture to 1.27.

## Rationale

Go gives the MVC the best balance of:

- low basal memory/CPU footprint;
- straightforward long-running daemon behavior;
- process/subprocess control;
- concurrency;
- cross-platform builds;
- simple deployment;
- direct embedded OPA support;
- official MCP SDK support;
- mature OpenTelemetry SDK;
- suitability for future networking/distributed Box work.

Python remains excellent for AI adapters and Graphiti, but making Python the core would couple the always-on runtime to the heaviest part of the AI ecosystem.

Rust offers stronger low-level control but adds implementation complexity without enough MVC value to justify it.

Kotlin/JVM is viable application technology but brings a larger basal runtime and less direct alignment with the selected OPA/MCP/runtime ecosystem.

## Consequence

Other languages remain first-class **executor/adapter languages**. The architecture is not “Go agents”; it is a Go control/runtime core that can orchestrate arbitrary executors.

---

# 4. ADR-002 — MVC Process Topology

## Decision

MVC uses one logical Box daemon process plus short/long-lived out-of-process executors.

Conceptually:

```text
meeseek CLI
     │
     ▼
┌───────────────────────────────────────┐
│               BOX                     │
│                                       │
│  Scheduler                            │
│  Task/Attempt state                   │
│  Commit Boundary                      │
│  PolicyEngine                         │
│  Resource Ledger                      │
│  Capability Registry                  │
│  TEB / sandbox controller             │
│  Evidence + Memory canonical layer    │
│  Local wake/timer engine              │
└───────────────┬───────────────────────┘
                │ scoped execution
       ┌────────┴────────┐
       ▼                 ▼
 deterministic       agentic
 executor            harness executor
```

The Box is the trusted coordinator. Executors do not receive direct database write credentials or authority-state ownership.

## Why one Box process

Splitting scheduler, policy, resource ledger, operation dispatcher and local state into microservices in MVC would create failure surfaces with no demonstrated value.

The contracts remain modular inside the codebase so they can later move across process boundaries if distribution requires it.

## Executor isolation

Executors run out-of-process because:

- they are replaceable;
- they may crash/hang;
- agentic harnesses are considered untrusted;
- resource accounting is clearer;
- process termination is a useful primitive;
- the TEB must survive executor compromise.

---

# 5. ADR-003 — Canonical Persistence: SQLite

## Decision

SQLite is the **single canonical source of truth for MVC authoritative state**.

Required baseline:

```text
local filesystem only
journal_mode = WAL
synchronous = FULL
foreign_keys = ON
busy_timeout configured
```

The exact Go SQLite driver is selected in the implementation plan after a compile/platform check, but the domain layer talks through a Meeseek-owned persistence interface and transaction helpers rather than driver-specific APIs.

## Why SQLite

The MVC is one Cube. SQLite supplies:

- atomic transactions;
- serializable writes;
- uniqueness constraints;
- conditional/fencing updates;
- crash recovery;
- excellent local operational simplicity.

It avoids introducing Postgres merely to simulate a distributed future that does not yet exist.

## Explicit limitations

- never treat the SQLite database as shared state over a network filesystem;
- do not make multi-Cube semantics depend on SQLite WAL;
- keep domain identifiers and transaction invariants portable to Postgres.

## Future transition

When multi-Cube Class A/B state is justified, add a PostgreSQL persistence adapter. Domain contracts and failure semantics must not change.

---

# 6. ADR-004 — Persistence Model: Authoritative State + Causal Event Log

## Decision

MVC is **not pure event sourcing**.

Use both:

1. canonical relational tables for current authoritative state;
2. append-oriented Event/Audit records for causal history and reconstruction.

Conceptual canonical tables include:

```text
collective_metadata
principals
constitutions
policy_sets
missions
goals
tasks
task_dependencies
attempts
external_operations
reservations
acceptance_records
capabilities
capability_assessments
evidence_objects
events
knowledge_deltas
claims
claim_evidence
decision_records
resource_usage
scheduled_wakeups
```

The implementation plan may consolidate or split tables, but the semantic entities remain distinct.

## Why hybrid

Pure event sourcing would make basic safety invariants unnecessarily indirect.

SQLite constraints are a better place to assert properties such as:

- only one current fence generation;
- unique logical external effect per intended slot;
- budget cannot be double-reserved;
- acceptance happens after durable Attempt output;
- current policy version is known.

The causal event log exists for audit, replay-oriented analysis and learning—not as the only way to know current authority state.

---

# 7. ADR-005 — Critical Transaction Boundaries

The following boundaries are architectural invariants, not implementation suggestions.

## 7.1 Lease / Attempt creation

One transaction:

```text
read Task current state/fence
→ verify eligible
→ increment fencing generation
→ create Attempt with that generation
→ set Task EXECUTING/current_attempt
→ append lease/attempt event
COMMIT
```

A stale worker cannot obtain a second valid current generation by itself.

## 7.2 Authoritative Attempt mutation

Any authoritative write originating from an Attempt uses a predicate equivalent to:

```text
task_id = ? AND current_fence = attempt.fence
```

Zero rows changed means the Attempt is stale. Its evidence may still be stored as historical evidence, but its authoritative mutation is rejected.

## 7.3 Prepare consequential External Operation

One transaction must perform all applicable checks and writes:

```text
verify current fencing generation
verify Task/Attempt state
verify active policy-set identity
verify PolicyDecision still applicable
verify enforcement profile/capability path
resolve/reuse logical effect identity
verify budget + unresolved exposure
verify conflicts/exclusivity
reserve bounded exposure
insert or bind PREPARED External Operation
append Decision/Event records
COMMIT
```

**No external side effect occurs inside this database transaction.**

## 7.4 Dispatch boundary

After PREPARED commits, the Box—not the untrusted executor—owns the transition to possible external effect.

Before invoking the external provider, the Box durably records that dispatch has begun by transitioning the operation into the spec's `DISPATCHED` state (or recording equivalent dispatch-start metadata).

Important semantic meaning:

> `DISPATCHED` means the effect may have escaped the local transactional boundary. It is not proof that the provider received or applied it.

The external call then occurs.

If the Box crashes after dispatch-start persistence but before durable outcome persistence, recovery treats the operation as `OUTCOME_UNKNOWN` unless provider semantics prove otherwise.

This intentionally permits conservative false-unknown cases in exchange for preventing unsafe duplicate effects.

## 7.5 External outcome settlement

One transaction:

```text
validate operation identity
persist evidence/provider reference
set CONFIRMED_EFFECT | CONFIRMED_NO_EFFECT | OUTCOME_UNKNOWN
settle / release / keep unresolved exposure
append Event/Audit entries
COMMIT
```

## 7.6 Attempt completion → verification

Attempt output/artifacts are durably stored first.

Then one transaction:

```text
verify current Attempt/fence where authoritative
link output/evidence
mark Attempt COMPLETED
move Task to AWAITING_VERIFICATION
create/schedule durable verification state/work
append event
COMMIT
```

If the process crashes after this point, it resumes verification instead of re-running the successful Attempt.

## 7.7 Acceptance

One transaction:

```text
persist AcceptanceRecord
mark Task SUCCEEDED
record accepted Attempt/evidence
make success-dependent Tasks eligible
append event
COMMIT
```

Attempt completion alone never performs this transition.

---

# 8. ADR-006 — Logical Effect Identity

## Decision

An executor/LLM must **not** be allowed to choose an arbitrary deduplication key that can defeat duplicate protection.

The Box/Capability Provider owns logical-effect identity.

Conceptually:

```text
logical_effect_key = HASH(
    collective_id,
    task_id,
    trusted_effect_slot_id,
    capability_contract_version,
    canonical_effect_descriptor
)
```

The exact hash/encoding belongs in the implementation plan.

## Trusted effect slot

`trusted_effect_slot_id` represents the intended occurrence of the effect within the Task.

Examples:

- `purchase-primary-test-resource`
- `send-customer-notification`
- `create-proposed-pull-request`

The executor may **request/propose** an effect, but the core assigns or resolves the trusted slot.

For a repeated identical effect that is intentionally distinct, a new effect slot must be explicitly created through governed Task logic rather than by simply changing a client-supplied idempotency key.

## Canonical effect descriptor

The trusted Capability Provider normalizes the semantically material effect data, for example:

```text
action
provider/resource type
target identity
material parameters
quantity
currency/value where applicable
```

Secrets or volatile transport details should not be required in the key when they do not change effect semantics.

## Retry behavior

A replacement Attempt resolving the same effect slot:

- `CONFIRMED_EFFECT` → reuse result;
- `OUTCOME_UNKNOWN` → reconcile;
- `CONFIRMED_NO_EFFECT` → may prepare dispatch again after Commit Boundary checks;
- PREPARED/DISPATCHED → resume/reconcile rather than mint unrelated effect.

---

# 9. ADR-007 — AgentLedger Usage

## Decision

AgentLedger is **not an MVC runtime dependency or authoritative state store**.

It is used as:

- a design/conformance reference;
- a source of reliability patterns;
- a comparison target for Meeseek failure tests;
- optional code reuse where semantics match exactly;
- a future interoperability adapter candidate.

## Why

Embedding it now would create overlapping durable state models (`Run/Step` and `Task/Attempt/External Operation`) and weaken the “one canonical state machine” rule.

Its current cross-language contract is still evolving and its Go implementation is not the right canonical storage layer for the transactional MVC design.

## Conformance strategy

Where useful, Meeseek tests should be inspired by/compatible with generic AgentLedger invariants such as fencing and unknown side effects, while adding the Meeseek-specific requirements:

- durable Task verification;
- task-semantic logical-effect identity;
- purpose/authority lineage;
- shared hierarchical resource envelope;
- TEB guarantee levels.

---

# 10. ADR-008 — Policy Engine: Embedded OPA

## Decision

OPA is embedded inside the Go Box and hidden behind:

```text
PolicyEngine.Evaluate(context) -> PolicyDecision
```

Stable Meeseek `PolicyDecision` contains conceptually:

```text
outcome: ALLOW | ALLOW_WITH_LIMIT | REQUIRE_APPROVAL | DENY
limits
required_approvals
reason_codes
policy_set_id
policy_set_hash
input_digest
evaluated_at
```

## Policy storage/versioning

Policies are Meeseek-managed versioned artifacts/state.

The active set has a stable id/hash in canonical storage. The Box prepares compiled OPA queries in memory for efficiency.

## Commit-time freshness

Consequential preparation only commits if the policy-set identity used by the PolicyDecision still equals the current active policy set.

If not:

```text
ABORT preparation
→ reevaluate policy
→ retry transaction with fresh decision
```

## OPA is not enforcement

OPA can say `DENY`; the TEB/Commit Boundary is what makes `DENY` impossible to bypass through the normal path.

---

# 11. ADR-009 — Trusted Enforcement Boundary Architecture

## Decision

The Box owns a small Trusted Enforcement Boundary comprising at least:

- authority/policy decision binding;
- current fencing validation;
- capability session issuance/revocation;
- credential isolation;
- Commit Boundary;
- External Operation dispatch/reconciliation;
- budget/reservation enforcement;
- authoritative persistence writes;
- executor sandbox profile selection.

The scheduler itself may reason about eligibility, but consequential enforcement occurs inside the TEB path.

## Enforcement profile

Each execution path advertises an enforcement level:

```text
ENFORCED
PARTIAL
UNENFORCED
```

Tasks/capabilities can require a minimum level.

The scheduler cannot route an `ENFORCED` consequential task to a weaker profile unless an authorized Owner guarantee downgrade is recorded.

## Reference strong profile

The first strong profile is Linux/OCI-oriented.

Baseline sandbox policy should include, subject to actual runtime support:

- dedicated container/process namespace;
- non-root executor identity;
- read-only root filesystem;
- `no-new-privileges`;
- drop unnecessary/all Linux capabilities;
- no host container-engine socket;
- only task-scoped writable workspace mount;
- no ambient cloud/GitHub/email credentials;
- CPU/memory/process/time limits;
- restricted egress;
- attempt-scoped capability channel;
- automatic capability-session revocation on lease/fence invalidation.

## Stronger optional backend

A gVisor `runsc` adapter is a candidate after the baseline OCI profile, because it creates a stronger syscall/kernel isolation layer while retaining OCI compatibility.

It is not mandatory for the first MVC boot.

## Cross-platform behavior

The Go Box may run natively on Windows/macOS/Linux.

However, **containment guarantees are platform/profile specific**. A native host executor that cannot prevent bypass is classified `PARTIAL` or `UNENFORCED`, never silently treated as equivalent to the Linux reference sandbox.

Windows/macOS users may use a Linux VM/WSL/container backend for the strong agentic profile.

---

# 12. ADR-010 — Network and Credential Architecture for Agentic Harnesses

## Problem

Agentic CLI harnesses often need internet access just to reach their LLM provider. Therefore `network=none` cannot be the only TEB strategy.

## Decision

Strong agentic execution distinguishes three network planes:

### 1. Model-control egress

Only connectivity necessary for the configured harness/model provider.

### 2. Meeseek capability channel

A scoped channel back to the Box for authorized capabilities.

### 3. General external egress

Denied by default in the strong profile.

If the Task needs web/GitHub/cloud/email/etc., the executor should use a governed capability presented by the Box rather than ambient credentials and unrestricted egress.

## Why

Giving a harness arbitrary internet plus credentials would let it recreate the same protected effect outside Commit Boundary mediation.

## Fallback

A harness that fundamentally requires unrestricted egress may run under a weaker enforcement profile for eligible low-risk work or through an explicit guarantee downgrade.

---

# 13. ADR-011 — Executor Architecture

## Core executor contract

The exact Go interface is deferred to the implementation plan, but semantically every Executor must support:

```text
start Attempt
receive scoped task/context
report liveness/progress
checkpoint if supported
cancel/terminate
return artifacts/evidence
return usage/cost observations
return failure metadata
```

An Executor cannot:

- mark Task `SUCCEEDED`;
- grant itself capabilities;
- mutate canonical DB state directly;
- mint trusted fencing generations;
- bypass External Operation preparation.

## Required MVC executors

### A. Deterministic CommandExecutor

Runs a configured command/script/process under a declared enforcement/resource profile.

Purpose:

- prove `Meeseek != LLM`;
- run tests/builds/scripts;
- exercise deterministic capabilities cheaply.

### B. AgenticHarnessExecutor

Invokes one supported agentic harness out-of-process under the TEB profile.

The first harness is an adapter choice, not a core dependency.

## Child work

An agentic executor requests child Tasks through Box capability APIs. It does not create authoritative Task rows directly.

---

# 14. ADR-012 — MCP as Attempt-Scoped Capability Surface

## Decision

MCP is the preferred adapter transport for agentic harnesses that natively support it.

For one Attempt, the Box exposes a scoped capability view containing only the tools/resources appropriate for that lease.

Conceptually:

```text
Agentic Harness
      │ MCP
      ▼
Attempt Capability Surface
      │
      ▼
Box / TEB
      │
      ├── read web
      ├── GitHub operation
      ├── create child Task
      ├── checkpoint/report evidence
      └── provider-specific capabilities
```

## Session binding

Every capability call is bound to:

- `collective_id`;
- `task_id`;
- `attempt_id`;
- current fencing generation;
- capability session/lease;
- enforcement profile.

A capability session is revocable independently of the harness process.

## What MCP does not define

MCP tool names and descriptions never define authority. The Box decides whether the tool is exposed and whether each call is authorized/committable.

## Non-MCP executors

Built-in deterministic executors can use native Go calls/local RPC and do not need to simulate MCP internally.

---

# 15. ADR-013 — Canonical Memory for MVC

## Decision

MVC stores canonical memory in SQLite using Meeseek-owned records for:

- Evidence;
- Events;
- Claims;
- Claim↔Evidence links;
- Knowledge Deltas;
- contradiction/supersession links;
- temporal validity;
- confidence/verification metadata;
- Known Unknowns where needed;
- governed deletion/redaction lineage.

No graph database is required to start.

## Retrieval

MVC can use:

- indexed relational queries;
- SQLite FTS for textual recall;
- direct provenance joins;
- task-specific context assembly.

Vector/graph retrieval is optional after the semantic core works.

## Graphiti

Graphiti becomes a later **derived projection**:

```text
Canonical Meeseek Memory
      ↓ events/deltas
Graphiti Projection
      ↓
Graph-oriented retrieval
```

It is disposable/rebuildable and never the only copy of evidence/provenance.

Governed deletion first changes canonical retained state; projections then update/rebuild.

---

# 16. ADR-014 — Evidence Blob Store

## Decision

Large/raw evidence is stored outside SQLite in a local content-addressed blob directory.

SQLite stores:

- evidence id;
- content hash;
- media/type metadata;
- size;
- origin/provenance;
- timestamps;
- retention classification;
- redaction/deletion state;
- path/blob reference.

## Local write protocol

Conceptually:

```text
write temporary blob
→ flush/sync as required
→ verify hash
→ atomic rename into content-addressed path
→ DB transaction records evidence reference
```

Unreferenced temp/orphan blobs can be cleaned by deterministic maintenance.

## Why not BLOB everything into SQLite

Evidence may include large files, logs, archives, model outputs and later multimedia. Keeping heavy immutable data outside the transactional state store reduces DB churn and makes object-storage adaptation straightforward.

## Future

Add S3/Azure Blob/object-store adapters without changing Evidence semantics.

---

# 17. ADR-015 — Scheduler and Wake Engine

## Decision

MVC implements its own deterministic scheduler in the Box.

No Temporal, Hatchet, DBOS, NATS or external queue is required.

## Scheduler responsibilities

- eligibility filtering;
- capability/access/enforcement matching;
- authority prerequisites;
- budget/resource envelope;
- deadline/scheduling shadow;
- current local capacity;
- lease creation/fencing;
- dynamic scheduling pressure;
- wake scheduling;
- durable blocked/eligible state transitions.

## Cognitive planning

If deterministic scheduling cannot resolve a planning/decomposition issue, it creates an ordinary internal planning Task. The result is still validated by the deterministic scheduler.

## Timers

Future wakeups/deadlines live in canonical SQLite state. The Box computes the next wake deadline and sleeps cheaply rather than relying on LLM polling.

On restart, pending wakeups are reconstructed from DB state.

---

# 18. ADR-016 — Resource Ledger and Budget Enforcement

## Decision

The Resource Ledger is canonical Meeseek state in SQLite.

Track independently:

- measured/estimated resource consumption;
- settled monetary cost;
- held reservation;
- unresolved exposure;
- enforceable hard ceiling;
- quality/source of cost data;
- allocated/readiness economics where applicable.

## Budget transaction rule

A consequential operation cannot reserve cost from a stale snapshot.

Reservation occurs in the same transaction that PREPAREs the External Operation.

Conceptually:

```text
available = hard_limit
          - settled_cost
          - active_reservations
          - unresolved_exposure
```

A child/retry/verification Task does not create additional parent budget unless an authorized allocation changes.

## Hard budget honesty

A provider with no enforceable maximum cannot be represented as satisfying a hard cap merely because expected cost is low.

The capability metadata must say whether cost control is:

- technically capped;
- cancellable with verified stop semantics;
- bounded by prepaid quota;
- estimated only;
- potentially unbounded after control loss.

---

# 19. ADR-017 — Identity and Signing for MVC

## Decision

Authority principals are represented by cryptographic public-key identities, not OS usernames or device names.

At minimum distinguish:

- operational Owner principal;
- constitutional/root principal;
- Cube/Box identity;
- executor/attempt identities derived/scoped by the Box.

## Signer abstraction

The core uses a `Signer` abstraction so key custody can evolve independently.

Initial development/MVC providers may include a local protected key store, while the architecture leaves room for:

- OS credential stores;
- hardware keys;
- passkeys/WebAuthn-style Owner approval;
- external KMS/HSM;
- SPIFFE/SPIRE workload identity later.

## Constitutional root

The constitutional root private key is not a normal Box runtime secret. Constitutional ceremony is intentionally separate from ordinary operational authority.

## Cube identity

Each Cube has its own key pair/identity. In single-Cube MVC this already establishes the identity model needed for later membership and revocation.

## Important limitation

A locally stored development Owner key does not magically satisfy the full long-term `Identity ≠ Device` goal. The runtime must accurately represent which signer/custody profile is active.

---

# 20. ADR-018 — Local Control Plane

## Decision

MVC is controlled through CLI + local authenticated IPC abstraction.

Preferred transports:

- Unix domain socket on Unix-like systems;
- named pipe on Windows;
- loopback TCP only as an explicit fallback/profile, not the authority root.

The transport is not itself Owner authority. Consequential approvals/constitutional actions are represented through principal/signer semantics.

## CLI

Conceptual commands can include:

```text
meeseek init
meeseek up
meeseek down
meeseek status
meeseek task ...
meeseek approve ...
meeseek inspect ...
```

Exact UX belongs in the implementation plan.

No web UI is required for MVC.

---

# 21. ADR-019 — Observability vs Audit

## Decision

Adopt OpenTelemetry for operational telemetry.

Use:

- traces for task/attempt/capability execution paths;
- metrics for runtime health/resource use;
- logs for operational diagnostics.

OTLP export is optional. MVC can operate with local logging without a Collector.

## Critical distinction

OpenTelemetry is **not** the authoritative audit log.

Security/decision/audit history that must survive sampling/export configuration remains canonical Event/Decision state in SQLite/EvidenceStore.

Telemetry may reference authoritative ids:

```text
collective_id
task_id
attempt_id
operation_id
logical_effect_key
```

subject to privacy policy.

---

# 22. ADR-020 — Deferred Distributed Infrastructure

The following are explicitly **not MVC dependencies**:

### PostgreSQL

Add when more than one Cube needs shared Class A/B authoritative state.

### NATS JetStream

Add when durable cross-Cube event/work transport solves an observed need. It must not redefine Task semantics.

### SPIFFE/SPIRE

Add when distributed workload identity/membership makes a local key registry insufficient.

### Temporal / Hatchet / DBOS

May become physical execution backends if they provide value, but do not own the logical Meeseek Scheduler/Task semantics.

### Graphiti

Add as rebuildable temporal graph projection when canonical SQL/FTS retrieval becomes insufficient.

### gVisor / Firecracker / E2B

Add stronger/remote TEB backends according to security and execution needs.

### Kubernetes

No role until actual deployment scale makes it useful.

---

# 23. Package / Module Boundaries

Exact file names may change, but the Go codebase should preserve boundaries resembling:

```text
cmd/
  meeseek/              CLI entrypoint
  meeseek-box/          optional dedicated daemon entrypoint if split binary is useful

internal/
  domain/               pure Meeseek domain types/invariants
  state/                persistence contracts + SQLite adapter
  events/               canonical events/audit writing
  scheduler/            deterministic scheduler
  policy/               stable PolicyEngine + OPA adapter
  authority/            principals/envelopes/approval checks
  teb/                  enforcement profiles, scoped capability sessions
  capabilities/         registry, assessment, providers
  executors/            executor contract + built-ins
  operations/           Commit Boundary, logical effects, reconciliation
  resources/            Resource Ledger/reservations/cost
  memory/               Evidence/Event/Claim canonical semantics
  evidence/             content-addressed blob store
  identity/             principal/signature/signer abstractions
  control/              local control-plane API
  wake/                 timers/dormancy/wake logic
  observability/        OTel bridge

adapters/
  mcp/                  MCP client/server capability bridge
  harnesses/            Claude/Codex/other harness adapters over time
  infrastructure/       OpenTofu/cloud adapters later
  memory/               Graphiti projection later
```

The key rule is dependency direction:

```text
Domain semantics
      ↑
core services
      ↑
adapters / external libraries
```

Domain packages do not import Graphiti, OPA-specific types, MCP-specific types, AgentLedger-specific types, Docker-specific types, or cloud-specific SDK types.

---

# 24. First Agentic Executor Architecture

The implementation plan will select the first concrete harness adapter, but the architecture requires this shape:

```text
Task + Attempt envelope
       ↓
AgenticHarnessExecutor
       ↓
TEB Sandbox
       ├── model-provider egress
       └── attempt-scoped capability channel
                         ↓
                      Box/TEB
```

The harness receives:

- task objective;
- acceptance criteria;
- scoped working context;
- workspace/artifact paths;
- visible capabilities;
- resource/time envelope;
- attempt identity.

It does **not** receive:

- SQLite credentials/path for authoritative writes;
- Owner private keys;
- constitutional root key;
- ambient cloud credentials;
- unrestricted provider credentials for protected external effects.

The harness can report a proposed completion; the Box persists Attempt output and invokes verification. It cannot self-accept the Task.

---

# 25. Canonical Capability Call Flow

For a normal read-only capability:

```text
Executor
→ capability session
→ validate attempt/fence
→ policy/authority if required
→ provider implementation
→ evidence + usage capture
→ result
```

For a consequential external capability:

```text
Executor proposes action
→ Capability Provider canonicalizes intent
→ resolve trusted effect slot/logical_effect_key
→ PolicyEngine
→ Commit Boundary transaction
     ├── fence
     ├── authority
     ├── policy version
     ├── enforcement path
     ├── budget/exposure
     ├── dedup/logical effect
     └── PREPARED operation + reservation
→ durable DISPATCHED marker
→ Box/provider performs external call
→ outcome transaction
→ evidence/result returned to Attempt
```

The executor never gets a raw shortcut around this path in the normal strong profile.

---

# 26. Mapping the Eight MVC Failure Scenarios to Architecture

## 1. Unknown external result

Protected by:

- PREPARED operation transaction;
- durable dispatch-start/`DISPATCHED` marker;
- `OUTCOME_UNKNOWN`;
- OperationReconciler;
- no blind duplicate dispatch.

## 2. Unresolved billing exposure

Protected by:

- Resource Ledger;
- `UNRESOLVED` exposure;
- retry reservation from the same parent envelope;
- provider hard-cap metadata.

## 3. Stale Attempt

Protected by:

- monotonically increasing Task fencing generation;
- conditional authoritative writes;
- capability-session fence validation;
- Commit Boundary fence validation.

## 4. Crash before verification

Protected by:

- Attempt output durable before Task transition;
- `AWAITING_VERIFICATION` canonical state;
- restart scheduler/wake reconstruction.

## 5. Capability bypass attempt

Protected by:

- TEB enforcement profile;
- sandbox isolation;
- no ambient secrets;
- restricted external egress;
- Box-owned capability channel.

Still requires executable acceptance testing on the target platform.

## 6. Constraint precedence

Protected by:

- deterministic PolicyEngine output;
- Commit Boundary enforcement;
- authority hierarchy represented outside executor prompt;
- no raw provider credentials bypassing the path.

## 7. Hard-budget honesty

Protected by:

- explicit cost-control capability metadata;
- Resource Ledger distinction between estimate and enforceable cap;
- Commit Boundary refusal/escalation when hard-limit proof is absent.

## 8. Confirmed effect survives Attempt replacement

Protected by:

- trusted Task-semantic effect slot;
- Box-generated logical effect key;
- unique canonical operation identity;
- replacement Attempt resolving existing operation before new dispatch.

---

# 27. MVC Startup Footprint

A fresh local Collective should be able to start with approximately this dependency topology:

```text
meeseek Box binary
SQLite DB
local evidence directory
policy bundle/files
Owner/Cube signer material
optional container runtime for strong agentic executor
optional agentic harness executable
```

No always-running:

- Postgres;
- Redis;
- NATS;
- Kafka;
- Neo4j;
- Graphiti service;
- AgentLedger service;
- Temporal cluster;
- OPA sidecar;
- Kubernetes.

This is intentional.

The runtime should be able to reach:

```text
0 active Meeseeks
near-zero active cognition
```

while retaining durable Mind/Memory/Task/authority state.

---

# 28. Architecture Risks

## Risk A — TEB portability

Full sandbox guarantees will differ across host platforms.

Mitigation:

- explicit enforcement profiles;
- Linux reference strong profile;
- executable bypass suite;
- never overclaim guarantees.

## Risk B — Go SQLite driver choice

Different drivers have different CGO/platform/performance tradeoffs.

Mitigation:

- keep SQL/domain boundary driver-neutral;
- run crash/concurrency/build matrix before locking implementation dependency.

## Risk C — Agentic harness behavior changes

CLI harnesses evolve rapidly and may change flags/tooling/network requirements.

Mitigation:

- adapter contract;
- conformance tests;
- never put harness-specific behavior in domain packages.

## Risk D — OPA coupling

Rego could leak into domain code if policy objects are not separated.

Mitigation:

- stable Meeseek `PolicyDecision`/input DTOs;
- OPA adapter only in policy infrastructure package.

## Risk E — canonical memory becomes too relational

SQL-only retrieval may become inadequate for large histories.

Mitigation:

- canonical semantics are independent of query projection;
- Graphiti/vector/search projections can be added without migration of truth ownership.

## Risk F — single Box becomes mistaken for permanent central SPOF

MVC physical topology may become accidental architectural dogma.

Mitigation:

- preserve CollectiveState/Scheduler/EventSource contracts;
- include `collective_id`, `cube_id`, fencing and epoch-ready identifiers;
- migrate physical coordination only when multi-Cube need is observed.

---

# 29. Decisions Explicitly Left for the Implementation Plan

The architecture is intentionally not deciding every library/file detail.

The implementation plan must still choose and verify:

1. concrete Go SQLite driver;
2. migration library/strategy;
3. exact schema/DDL and indexes;
4. exact domain Go interfaces;
5. exact cryptographic primitive/key serialization and local signer provider;
6. Unix socket/named-pipe library choices;
7. exact OCI engine invocation abstraction;
8. first agentic harness adapter;
9. model-provider egress enforcement implementation;
10. exact MCP session transport for the selected harness;
11. OpenTelemetry exporter defaults;
12. CLI UX and config file format;
13. detailed test harness/crash injection mechanics;
14. package/file-level task sequence.

These are implementation decisions within the architecture, not reasons to reopen the conceptual design.

---

# 30. Architecture Acceptance Criteria

This architecture is ready for detailed implementation planning when the Owner accepts that:

- Go owns the semantic core;
- SQLite is the sole canonical MVC state store;
- AgentLedger is not an authoritative runtime dependency;
- Graphiti is deferred to a rebuildable memory projection;
- OPA is embedded as policy evaluation only;
- the Box owns Commit Boundary/TEB/resource enforcement;
- logical effect identity is assigned by trusted core logic, not an LLM;
- agentic harnesses run out-of-process under explicit enforcement profiles;
- MCP is scoped interoperability, not authority;
- no distributed infrastructure is required by MVC;
- all eight reviewed MVC failure scenarios map to concrete architecture mechanisms.

After acceptance, the Superpowers `writing-plans` stage may produce the detailed MVC implementation plan.
