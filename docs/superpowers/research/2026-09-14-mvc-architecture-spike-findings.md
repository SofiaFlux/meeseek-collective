# Meeseek Collective — MVC Architecture Spike Findings

**Status:** Source/contract and local persistence spikes complete; input to implementation architecture decisions  
**Date:** 2026-09-14  
**Inputs:**
- `docs/superpowers/specs/2026-09-14-meeseek-collective-design.md`
- `docs/superpowers/research/2026-09-14-oss-landscape-mapping.md`

---

## 1. Purpose

This document records the compatibility and feasibility findings requested by the OSS landscape mapping before concrete MVC architecture decisions are frozen.

These are **architecture spikes**, not the MVC implementation. They answer whether candidate technologies can safely sit behind Meeseek-owned contracts and whether the reviewed failure semantics can be expressed without adding distributed infrastructure.

The spikes covered:

1. AgentLedger reliability semantics;
2. Graphiti temporal-memory fit;
3. OPA policy-engine fit;
4. Trusted Enforcement Boundary / sandbox feasibility;
5. SQLite persistence and crash-state model;
6. MCP capability-adapter fit;
7. implementation-language ecosystem compatibility.

No spike is allowed to redefine the approved Meeseek semantics merely to make a candidate library fit.

---

# 2. Spike A — AgentLedger Semantic Compatibility

## 2.1 What was inspected

Primary repository: `yaogdu/AgentLedger` (Apache-2.0).

Reviewed materials included:

- top-level runtime scope and guarantees;
- machine-readable runtime contract;
- language implementation comparison;
- Go runtime implementation;
- official Go adapters, including the Docker sandbox adapter;
- side-effect/idempotency patterns;
- lease/fencing semantics.

AgentLedger remains the closest external project to the reliability portion of the Meeseek design.

## 2.2 Strong overlap

AgentLedger has native concepts corresponding closely to:

- durable runs/steps/checkpoints;
- leases and fencing;
- state-version checks;
- retries and cancellation;
- policy and budget hooks;
- tool/side-effect mediation;
- idempotency keys;
- pending/unknown side-effect handling;
- evidence and replay;
- usage/cost attribution;
- sandbox routing;
- conformance-oriented contracts.

These are exactly the classes of failures highlighted by the written-spec review.

## 2.3 Where the semantics diverge

Embedding AgentLedger as the MVC source of truth would create a second state machine beside Meeseek's canonical model.

AgentLedger primarily models concepts such as `Run` and `Step`. Meeseek requires the independently reviewed semantics of:

```text
Task
  └── Attempt(s)
       └── External Operation(s)
```

with, among other things:

- Task acceptance separate from Attempt completion;
- durable `AWAITING_VERIFICATION`;
- Task-semantic logical-effect identity surviving Attempt replacement;
- Meeseek-specific purpose and authority lineage;
- budget exposure tied to the Task/Goal hierarchy;
- scheduler semantics outside ordinary durable-workflow orchestration.

The AgentLedger Go runtime inspected during this spike also uses its own Store abstraction and local JSONStore path in its reference/baseline implementation. That does not match the desired transactional MVC canonical state model.

The machine-readable AgentLedger contract currently describes Python as the full v1.x reference and Go/TypeScript/Rust baselines as runtime-preview. That is not a reason to reject the project, but it is a reason not to make Meeseek correctness depend on embedding its current Go runtime.

## 2.4 AgentLedger Docker adapter and the TEB

The Go Docker sandbox adapter inspected in AgentLedger provides useful baseline isolation behavior, including read-only root filesystem, configurable network mode, and optional resource constraints.

It does **not by itself** satisfy the complete Meeseek Trusted Enforcement Boundary contract. The Meeseek reference profile additionally needs explicit handling for matters such as:

- no ambient host credentials;
- no host Docker/container socket;
- non-root execution where feasible;
- dropped Linux capabilities;
- no-new-privileges;
- carefully scoped mounts;
- controlled egress;
- an attempt/fence-bound capability channel;
- revocation/fencing at the trusted capability boundary.

Therefore AgentLedger's sandbox adapter is a useful pattern/backend candidate, not evidence that the Meeseek TEB requirement is already solved.

## 2.5 Decision from the spike

**Do not embed AgentLedger as the canonical MVC runtime or state store.**

Use it as:

1. a **conformance and design donor** for generic reliability semantics;
2. a reference when writing Meeseek failure tests;
3. a source of selected reusable/adaptable code only where semantics line up exactly;
4. a possible future runtime/executor interoperability adapter;
5. a potential upstream collaboration target for generic improvements.

If source or tests are copied rather than merely reimplemented from concepts, preserve Apache-2.0 attribution/NOTICE obligations.

### Classification after spike

`ADAPT`, but specifically at the **patterns/conformance/adapter layer**, not as the Meeseek source-of-truth runtime.

---

# 3. Spike B — Graphiti Temporal Memory Projection

## 3.1 Fit

Primary repository: `getzep/graphiti` (Apache-2.0 source).

Graphiti is strongly aligned with several projection/query requirements:

- temporal facts;
- historical validity;
- provenance through episodes;
- changing/superseded relationships;
- point-in-time retrieval;
- graph-oriented entity/fact lookup;
- deletion of episodes and cascade cleanup of facts/entities solely created by an episode.

Its MCP surface also exposes provenance-oriented and deletion operations such as retrieving episode entities and deleting episodes/edges.

## 3.2 Why Graphiti should not be canonical MVC memory

The Meeseek canonical memory model must be able to represent and govern:

- Evidence as immutable/retractable artifacts with lineage;
- Event history;
- Claims with epistemic status;
- verification state;
- confidence and evidence lineage;
- contradiction/supersession;
- Known Unknowns;
- governed redaction/purge propagation;
- exact decision-time evidence;
- rebuildable projections.

A graph-memory engine may derive excellent retrieval structures, but it must not become the authority that decides whether a claim is FACT/HYPOTHESIS/VERIFIED or whether evidence is legally retainable.

There is also no need to introduce Python + a graph database merely to prove the MVC semantic slice.

## 3.3 Governed deletion consequence

Graphiti's episode deletion support is useful, but Meeseek must treat the graph as **rebuildable derived state**.

The canonical deletion flow should therefore be:

```text
canonical Evidence/Event/Claim lineage changes
→ governed deletion/redaction committed
→ graph projection receives deletion/update event
→ if projection lineage is uncertain, rebuild affected projection from legally retained canonical state
```

This is safer than relying on a graph engine to discover all legally derived deletion obligations itself.

## 3.4 Decision from the spike

For MVC:

- keep canonical memory in the local transactional store;
- use SQL/FTS/basic structured queries initially;
- do not require Graphiti or a graph database to start the Collective.

After MVC semantics are proven:

- add a `CollectiveMemoryProjection` adapter;
- Graphiti is the first candidate;
- projection must be disposable/rebuildable from canonical retained evidence/events/claims.

### Classification after spike

`ADAPT`, **deferred from mandatory MVC startup**.

---

# 4. Spike C — OPA Policy Decision Fit

## 4.1 Fit

Primary project: Open Policy Agent.

OPA can be embedded directly in Go through its Go API. It can evaluate structured inputs and return structured result documents, which is sufficient to represent Meeseek's stable decision model:

```text
ALLOW
ALLOW_WITH_LIMIT
REQUIRE_APPROVAL
DENY
```

plus limits, reasons, required approvals, and metadata.

## 4.2 Boundary of responsibility

OPA is a **decision engine**, not the Meeseek enforcement mechanism.

The architecture remains:

```text
request/context
→ Meeseek PolicyDecision input
→ OPA evaluation
→ stable Meeseek PolicyDecision
→ Commit Boundary / TEB enforcement
```

OPA never grants itself capabilities and does not own:

- Constitution hierarchy;
- Owner identity;
- fencing validation;
- credential release;
- resource reservation;
- External Operation dispatch;
- sandbox enforcement.

## 4.3 Policy version race

A policy decision used for a consequential operation must identify the exact policy set used, for example:

- policy-set version;
- content hash;
- input digest;
- decision timestamp;
- output/limits.

The transaction preparing an External Operation must verify that the active policy-set identity has not changed since evaluation. If it changed, preparation aborts and policy is re-evaluated.

This prevents:

```text
policy says ALLOW
→ policy changes to DENY
→ stale decision still commits effect
```

## 4.4 Decision from the spike

For MVC:

- embed OPA in the Box process using its Go API;
- keep Rego/policy data versioned as Meeseek-managed artifacts/state;
- wrap OPA behind a small `PolicyEngine` contract;
- keep the wire/domain `PolicyDecision` independent of OPA so Cedar or another engine can replace it later.

### Classification after spike

`ADAPT` — strong fit.

---

# 5. Spike D — Trusted Enforcement Boundary / Sandbox

## 5.1 What could and could not be executed

The research environment used for this architecture stage does not provide Docker or Podman, so an executable escape/bypass experiment was **not** performed here.

That limitation is explicit. The architecture does not claim a sandbox guarantee based only on configuration review.

The executable bypass scenario from §32.6 remains a mandatory MVC acceptance test on the actual supported runtime environment.

## 5.2 Source/documentation feasibility

Container/OCI tooling provides primitives necessary for a useful reference profile:

- read-only root filesystem;
- no-new-privileges;
- capability dropping;
- non-root users;
- resource quotas;
- controlled mounts;
- isolated namespaces;
- restricted networking.

Stronger isolation can later use gVisor (`runsc`), which inserts an application-kernel boundary between workload and host and integrates with OCI/Docker/containerd.

## 5.3 The agentic-harness network problem

A blanket `network=none` is not sufficient for all agentic harnesses because a CLI harness may itself need network access to its model provider.

Therefore the reference TEB needs to distinguish:

1. **model-control egress** — narrowly allowed connectivity required to reach the configured LLM/provider;
2. **Meeseek capability channel** — attempt-scoped access to Box-controlled capabilities;
3. **arbitrary external egress** — denied by default for the strong profile.

Web research, GitHub changes, cloud operations, email, etc. should normally pass through governed capabilities rather than ambient unrestricted network + credentials.

If a harness requires unrestricted arbitrary network, it runs under a lower enforcement profile and cannot be treated as satisfying the normal consequential-action guarantee unless the Owner explicitly accepts a guarantee downgrade.

## 5.4 Enforcement profiles

The implementation architecture should expose the **actual guarantee**, not a boolean `sandboxed` flag.

At minimum:

- `ENFORCED` — the required effect boundary is technically enforced for the task;
- `PARTIAL` — some isolation exists, but the task's full protected-effect boundary cannot be proven;
- `UNENFORCED` — logical restrictions are descriptive only.

Consequential tasks that require `ENFORCED` are ineligible on weaker paths unless the Owner grants an explicit, audited guarantee downgrade.

This is an implementation of the approved TEB semantics, not a new authority model.

## 5.5 Platform consequence

The Go Box can remain cross-platform, but the strongest reference agentic-executor profile should initially be **Linux/OCI-oriented** (including an appropriate Linux VM/WSL/container environment on non-Linux hosts).

macOS/Windows native execution may still be supported for deterministic/unprivileged work, but the runtime must not claim Linux-equivalent containment where it does not exist.

## 5.6 Decision from the spike

- BUILD the TEB/enforcement-profile contract;
- ADAPT OCI/container backends;
- define a strict Linux reference profile first;
- keep gVisor as a stronger optional backend after the baseline profile;
- require a real bypass test before MVC acceptance.

### Classification after spike

`BUILD contract + ADAPT backend`.

---

# 6. Spike E — SQLite Persistence and Crash-State Model

## 6.1 Why this spike matters

The written-design review introduced invariants that must survive process failure:

- fencing;
- durable PREPARED operation before external effect;
- logical-effect identity across Attempts;
- budget reservation/exposure;
- `AWAITING_VERIFICATION`;
- recovery without duplicate effect.

If those cannot live naturally in one local transactional store, SQLite would be a bad MVC choice regardless of operational simplicity.

## 6.2 Local transaction experiment

A temporary SQLite schema was used to model:

- `tasks` with current fencing generation and budget state;
- `attempts` with fence generation;
- `effects` with `UNIQUE(task_id, logical_effect_key)`;
- reservation/exposure values;
- Task verification state.

The experiment used WAL mode and synchronous FULL semantics for the local test store.

The following properties were exercised successfully:

| Property | Result |
|---|---|
| PREPARED External Operation and exposure reservation committed atomically | PASS |
| stale Attempt authoritative Task write rejected by fence predicate | PASS |
| second Attempt cannot create duplicate row for same Task logical effect | PASS |
| confirmed effect is discoverable/reusable by replacement Attempt | PASS |
| Task can move durably to `AWAITING_VERIFICATION` under current fence | PASS |
| close/reopen preserves `AWAITING_VERIFICATION`, cost and fence state | PASS |
| unresolved exposure reduces remaining budget and blocks overcommitting retry | PASS |

This was a semantic persistence experiment, not the Go implementation and not a full crash/power-failure harness.

## 6.3 SQLite properties relevant to the architecture

SQLite provides ACID/serializable transaction semantics and serializes writers. WAL allows simultaneous readers and a writer on one host.

For Meeseek MVC, the important consequence is that Class A/B local state can be protected by **ordinary database transactions and uniqueness/conditional-update constraints** rather than an additional consensus system.

WAL is explicitly a same-host mechanism and is not suitable as a shared database over a network filesystem. That aligns with the design: SQLite is the single-Cube MVC store, not the future multi-Cube store.

## 6.4 Durability choice

For the canonical MVC state database, use:

```text
journal_mode = WAL
synchronous = FULL
```

The throughput cost is acceptable for MVC because a PREPARED-before-dispatch record is safety-critical: after a power loss, the system should prefer preserving a committed effect intent over maximizing write throughput.

Do not place the SQLite database on a network filesystem.

## 6.5 Hybrid state + event log

Do **not** make MVC a pure event-sourced system.

Use:

- canonical relational state tables for current authoritative state;
- an append-oriented Event/Audit table for causal history;
- evidence blobs referenced by hash/metadata.

This keeps operational invariants simple while preserving replay/audit history.

## 6.6 Decision from the spike

SQLite is technically suitable as the **single canonical MVC state store**, provided schema and transaction boundaries encode Meeseek invariants explicitly.

A later PostgreSQL adapter may replace the physical store for multi-Cube strong state without changing domain semantics.

### Classification after spike

`ADOPT substrate` for MVC.

---

# 7. Spike F — MCP Capability Adapter

## 7.1 Fit

The official MCP Go SDK supports MCP clients and servers, tools, resources, prompts, transports, cancellation/progress, and security/authorization mechanisms.

MCP is a strong interoperability layer for Meeseek because many agentic harnesses can consume MCP tools natively.

## 7.2 What MCP must not own

MCP tool discovery does not imply:

- capability verification;
- Meeseek authority;
- policy approval;
- resource budget;
- fencing validity;
- external-effect idempotency;
- Task success.

A discovered MCP tool is only a candidate primitive capability until Meeseek wraps and assesses it.

## 7.3 Attempt-scoped MCP surface

For an MCP-capable agentic harness, the preferred architecture is an **attempt-scoped capability surface**.

The harness sees only tools relevant to its leased Task/Attempt. Calls flow through Box-owned capability adapters that can enforce:

- Attempt identity;
- fencing generation;
- policy;
- Commit Boundary;
- budget/exposure;
- evidence capture;
- credential isolation.

This creates a useful separation:

```text
agentic harness
→ scoped MCP tools
→ Box capability boundary
→ provider/service
```

The Box holds the durable authority; MCP is the transport presented to the harness.

Deterministic local executors do not need to speak MCP internally.

## 7.4 Go compatibility

The official `modelcontextprotocol/go-sdk` currently declares Go 1.25 as its module language baseline. It is therefore compatible with the proposed current Go baseline for Meeseek.

## 7.5 Decision from the spike

- ADAPT MCP for agent/tool interoperability;
- expose a scoped server surface where it simplifies harness integration;
- never use MCP registration/discovery as the authority source;
- retain native internal contracts for deterministic executors and core modules.

### Classification after spike

`ADAPT`.

---

# 8. Spike G — Core Implementation Language

## 8.1 Candidates considered

### Python

Pros:
- strongest current agent/AI ecosystem;
- Graphiti and many memory tools are Python-first;
- fast experimentation.

Cons:
- weaker fit for a low-idle-footprint always-available Box daemon;
- packaging/process isolation is less simple for a cross-platform systems runtime;
- increases temptation to fuse semantic core with Python agent frameworks.

### Kotlin/JVM

Pros:
- strong type system and application architecture;
- excellent general-purpose engineering environment.

Cons:
- larger basal runtime footprint;
- weaker direct alignment with AgentLedger's language baselines;
- OPA embedding and official MCP integration are less direct than Go;
- JVM is unnecessary operational weight for the single-binary Box goal.

### Rust

Pros:
- excellent low-level safety and footprint;
- strong systems/runtime fit.

Cons:
- higher implementation complexity and slower iteration for this project;
- does not create enough additional value over Go for MVC to justify that cost.

### Go

Pros:
- excellent daemon/CLI/process-control fit;
- low operational footprint;
- simple cross-compilation/single-binary distribution;
- good concurrency primitives;
- OPA embeds directly through its Go packages;
- official MCP Go SDK exists;
- mature OpenTelemetry support;
- AgentLedger provides a Go baseline/reference for comparison;
- natural fit for future Box networking/distributed runtime work.

Cons:
- Graphiti remains external/Python if adopted later;
- some AI-specific libraries are less rich than Python.

The second point is acceptable because Meeseek explicitly treats model/memory implementations as adapters rather than the semantic core.

## 8.2 Version baseline

As of this design stage, Go 1.27 is the current major stable line. OPA currently declares Go 1.26 and the official MCP Go SDK declares Go 1.25, so Go 1.27 satisfies both baselines.

The implementation should follow a supported Go release rather than freeze permanently to 1.27, but the initial repository baseline can be Go 1.27.

## 8.3 Decision from the spike

Use **Go** for the Meeseek semantic core, Box, CLI, scheduler, policy adapter, persistence layer, and built-in executor/capability adapters.

Python or other runtimes remain valid **executor/adapter implementation languages**, not core requirements.

---

# 9. Consolidated Spike Conclusions

The spikes reduce the MVC architecture to a deliberately small set of mandatory moving parts:

```text
Go Box / semantic core
│
├── SQLite canonical authoritative state
├── Meeseek Task/Attempt/External Operation state machines
├── embedded OPA policy evaluator
├── deterministic Scheduler
├── Commit Boundary + Resource Ledger
├── TEB / enforcement-profile abstraction
├── Executor adapters
│    ├── deterministic command/script
│    └── agentic harness
│         └── optional attempt-scoped MCP surface
├── canonical Evidence/Event/Claim memory
└── OpenTelemetry instrumentation
```

Not mandatory for MVC startup:

- AgentLedger runtime;
- Graphiti;
- Neo4j/FalkorDB/Kuzu;
- PostgreSQL;
- NATS;
- SPIFFE/SPIRE;
- Temporal/Hatchet/DBOS;
- gVisor (stronger optional sandbox backend);
- Kubernetes.

This is consistent with the Single-Cube Principle and Principle of Sufficient Effort while leaving clean adapters for later scale.

---

# 10. What Remains an Implementation Acceptance Test

The following cannot be honestly closed by architecture/source inspection alone and must be verified on the actual implementation:

1. sandbox/TEB bypass resistance with the supported agentic executor;
2. executable crash injection between operation preparation, dispatch, acknowledgement and outcome persistence;
3. actual provider/model egress restriction behavior;
4. exact Go SQLite driver's power/crash/concurrency behavior under the selected pragmas;
5. cancellation behavior of each remote executor/provider;
6. MCP harness compatibility for the first chosen agentic harness;
7. real cost attribution and hard-cap behavior for the first paid executor/provider.

These become tests/gates in the MVC implementation plan rather than reasons to add more conceptual mechanisms.

---

## 11. Exit Gate

These findings are sufficient to make the implementation architecture decisions without selecting a distributed control plane or turning external frameworks into semantic owners.

Next artifact:

`docs/superpowers/architecture/2026-09-14-mvc-architecture-decisions.md`
