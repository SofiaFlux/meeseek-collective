# Meeseek Collective — OSS Landscape Mapping

**Status:** Research complete; ready for implementation architecture decisions  
**Date:** 2026-09-14  
**Input:** `docs/superpowers/specs/2026-09-14-meeseek-collective-design.md`  
**Decision framework:** **ADOPT / ADAPT / BUILD**

---

## 1. Purpose

This document maps the approved Meeseek Collective design onto the current open-source ecosystem.

The goal is not to maximize reuse. The goal is to reuse components **only where their semantics fit the approved design without weakening its invariants**.

Definitions:

- **ADOPT** — use an existing project substantially as-is behind a Meeseek contract.
- **ADAPT** — reuse an existing subsystem, engine, protocol, or implementation behind a Meeseek-owned semantic adapter.
- **BUILD** — implement Meeseek-owned semantics because they are differentiating, cross-cutting, security-critical, or not safely supplied by an existing project.

This is a landscape decision document, not the implementation plan. Exact language, package boundaries, database schemas, deployment topology, and API signatures belong in the next architecture-decision stage.

---

## 2. Evaluation Criteria

Candidates were evaluated against the approved design rather than against generic agent-framework feature lists.

Important dimensions were:

1. **Semantic fit** — does the project implement the meaning Meeseek requires, not merely a similarly named feature?
2. **Enforcement fit** — can the component participate in the Trusted Enforcement Boundary where required?
3. **Failure semantics** — leases, fencing, crash recovery, unknown external effects, idempotency, and durable verification.
4. **Local-first fit** — can the system degenerate to one Cube without requiring a distributed control plane?
5. **Future distributed fit** — can it later participate in multi-Cube operation without redefining semantics?
6. **Replaceability** — can it stay behind a stable contract rather than become the architecture?
7. **Operational footprint** — does it introduce infrastructure disproportionate to current value?
8. **License** — preference for genuine OSS and permissive licensing compatible with an Apache-2.0 project.
9. **Maturity** — evidence of maintained releases, documented contracts, tests, production use, or stable ecosystem position.
10. **Complexity earned** — no component is accepted merely because it is powerful.

No single numeric score is used. A project can be excellent software and still be the wrong owner of a Meeseek semantic boundary.

---

## 3. Executive Decision

There is **no existing OSS project that should be forked and renamed into Meeseek Collective**.

The recommended shape is:

```text
Meeseek-owned semantic core
│
├── ADAPT reliability/runtime substrate
├── ADAPT temporal graph/memory projection
├── ADAPT policy engine
├── ADAPT workload identity later
├── ADOPT standard observability protocol
├── ADAPT sandbox backends
├── ADAPT infrastructure automation
├── ADAPT event transport later
└── BUILD the organizational semantics that make Meeseek Collective unique
```

The strongest external candidates found are:

- **AgentLedger** — closest fit to the reliability boundary: leases, fencing, side-effect ledger, replay, evidence, policy hooks, sandbox routing, costs and budgets.
- **Graphiti** — closest fit to temporal knowledge-graph projection, provenance, changing facts and historical queries.
- **OPA** — strongest default candidate for a general-purpose deterministic policy evaluation engine.
- **MCP** — strong standard transport for tool/capability interoperability, but not an authority model.
- **OpenTelemetry** — clear ADOPT for telemetry interchange.
- **SPIFFE/SPIRE** — strong later-stage workload identity candidate for multi-Cube trust.
- **OpenTofu** — preferred OSS infrastructure-as-code adapter candidate.
- **NATS JetStream** — strong later-stage lightweight event/distributed-work transport candidate.
- **SQLite** and **PostgreSQL** — strong storage substrates at different scale points.

The central conclusion is:

> **Meeseek Collective should own semantics and conformance; OSS projects should supply engines and implementations behind those semantics.**

---

## 4. Mapping Summary

| Meeseek contract / concern | Decision | Primary candidate(s) | Rationale |
|---|---|---|---|
| `DurableWork` / runtime reliability | **ADAPT + BUILD** | AgentLedger; DBOS; Hatchet; Temporal | AgentLedger is unusually close to Meeseek failure semantics, but Meeseek still owns Task/Attempt/External Operation, purpose, acceptance and scheduler semantics. |
| Canonical Task/Attempt/External Operation model | **BUILD** | — | Differentiating core semantics; must match approved spec exactly. |
| `OperationReconciler` | **BUILD + ADAPT** | AgentLedger patterns/ledger | Logical-effect identity, confirmed/unknown effect reuse and reconciliation are core Meeseek guarantees. |
| `CollectiveMemory` | **BUILD + ADAPT** | Graphiti primary; Caura/Cognee alternatives | Meeseek owns evidence/epistemic semantics; graph system can provide temporal projection and retrieval. |
| `EvidenceStore` | **BUILD contract; ADOPT substrate** | Local files/object storage; AgentLedger evidence patterns | Evidence meaning/provenance is Meeseek-owned; blob persistence is commodity infrastructure. |
| `PolicyEngine` | **ADAPT** | OPA primary; Cedar secondary | Reuse deterministic policy evaluation, build Meeseek decision schema and constitutional hierarchy around it. |
| Relationship authorization | **DEFER / optional ADAPT** | OpenFGA | Useful later for relationship-heavy resource access, but not sufficient as Meeseek policy engine. |
| `IdentityProvider` | **BUILD local/Owner + ADAPT distributed** | SPIFFE/SPIRE later | Owner constitutional identity differs from workload identity; SPIRE fits Cube/workload identity once distribution earns its existence. |
| `Scheduler` | **BUILD** | Hatchet/Temporal as optional execution backends | Meeseek arbitration semantics are unique and should not be delegated to a generic workflow scheduler. |
| `Executor` | **BUILD adapters** | Claude/Codex/etc.; shell/script; DBOS/Hatchet/Temporal backends | Stable executor contract is core; implementations remain replaceable. |
| `CapabilityProvider` | **BUILD + ADAPT** | MCP | MCP is excellent interoperability plumbing but capability != authority; assessment and enforcement remain Meeseek-owned. |
| `InteractionProvider` | **BUILD adapters** | MCP + service-specific APIs | Interaction semantics, human attention and commit boundaries are Meeseek-specific. |
| `EventSource` | **BUILD contract; ADAPT later** | NATS JetStream | MVC should stay local/simple; NATS becomes attractive when distributed/event durability is required. |
| `CostProvider` | **BUILD semantics + ADOPT telemetry** | OpenTelemetry + provider billing APIs | OTel transports observations; Resource Ledger, reservations and spend exposure remain Meeseek semantics. |
| Observability | **ADOPT** | OpenTelemetry | Mature vendor-neutral standard; no reason to invent custom trace/metric/log transport. |
| TEB sandbox implementation | **BUILD contract + ADAPT backends** | rootless containers, nsjail, gVisor, E2B, Firecracker | No single backend covers all local platforms/security levels. Enforcement guarantees must remain explicit. |
| `InfrastructureProvider` | **BUILD adapter + ADAPT engine** | OpenTofu; cloud SDK/CLI | OpenTofu is genuine OSS and suitable as one implementation, not as the authority layer. |
| MVC durable DB | **ADOPT candidate** | SQLite | Very low operational footprint and excellent single-Cube fit. |
| Later shared durable DB | **ADOPT candidate** | PostgreSQL | Mature transactional base for multi-Cube strong-state classes. |
| Later event bus | **ADAPT candidate** | NATS JetStream | Lightweight persistent messaging, deduplication, work queues, KV/object storage. |

---

# 5. Durable Work and Runtime Reliability

## 5.1 AgentLedger — **ADAPT (highest-priority spike)**

Repository: `yaogdu/AgentLedger`  
License: Apache-2.0  
Observed maturity: v1.x runtime-core contract, Python reference implementation, native Go/TypeScript/Rust baselines, conformance tooling; project documentation describes production-pilot preparation rather than universal production maturity.

AgentLedger is the most significant finding in this mapping because its scope overlaps heavily with the **hard reliability substrate** of the approved Meeseek design.

It explicitly covers:

- durable runs/steps/checkpoints;
- leases and fencing tokens;
- retry/cancellation semantics;
- tool/side-effect ledger;
- idempotency keys;
- pending/unknown side-effect states;
- policy/approval hooks;
- sandbox routing;
- evidence and replay;
- cost and failure attribution;
- SQLite/Postgres/MySQL state adapters;
- blob/evidence adapters;
- cross-language conformance contracts.

That alignment is unusually strong with the findings from the Meeseek written-design review.

### Why not ADOPT wholesale?

AgentLedger deliberately does **not** own several concepts that are fundamental to Meeseek:

- Mission and authorized purpose;
- obligations and missionless work;
- Goal generation/Goal Market;
- Meeseek-specific Task vs Attempt vs External Operation acceptance semantics;
- strategic scheduling and capability scarcity;
- Collective cognition tiers;
- long-term epistemic memory;
- Collective identity, partitions and federation;
- earned autonomy;
- resource readiness economics;
- Strategic Pulse.

Its model is a runtime reliability substrate for agent harnesses, while Meeseek is an autonomous organization runtime.

### Recommendation

Do not fork AgentLedger immediately.

Perform a **semantic compatibility spike** against the eight MVC failure-path scenarios. Determine whether Meeseek can:

1. run AgentLedger as an internal library/runtime adapter;
2. reuse its runtime contract/conformance ideas while implementing a Meeseek-native state model;
3. reuse only selected concepts/tests;
4. upstream generic improvements instead of carrying a fork.

The preferred outcome is **composition**, not identity merger.

### Key risk

AgentLedger is much younger than infrastructure such as Temporal/Postgres/OPA. The architecture should benefit from it without making Meeseek correctness dependent on one young project surviving indefinitely.

---

## 5.2 DBOS — **ADAPT candidate, especially for lightweight durable execution**

Repositories: `dbos-inc/dbos-transact-*`  
License: MIT (Python project verified)  
Model: database-backed durable workflows with Postgres/SQLite support in the ecosystem.

Strengths:

- lightweight compared with a dedicated workflow cluster;
- checkpointed durable functions/workflows;
- excellent local-first philosophy;
- natural affinity with transactional application state;
- suitable as an execution backend for deterministic or application-style workflows.

Limitations relative to Meeseek:

- workflow-as-code semantics are not Meeseek Task/Attempt/External Operation semantics;
- no Meeseek authority hierarchy;
- no Collective cognition/memory model;
- does not eliminate need for TEB and logical-effect ledger semantics.

Recommendation: keep as a **backend candidate**, especially if the implementation language/runtime aligns, but do not let DBOS become the semantic core.

---

## 5.3 Hatchet — **ADAPT candidate for future distributed execution**

Repository: `hatchet-dev/hatchet`  
License: MIT

Hatchet provides:

- durable tasks;
- retries;
- cron/scheduling;
- event triggers/webhooks;
- priorities;
- rate limits;
- worker slots;
- routing/affinity;
- concurrency policies;
- Postgres-backed self-hosting.

This maps well to **physical work dispatch** and later multi-Cube capacity management.

It does **not** replace the Meeseek scheduler because the approved scheduler includes semantics such as:

- purpose/Mission value;
- capability scarcity;
- deadline scheduling shadow;
- Opportunistic Surplus;
- cognition planning;
- authority-aware eligibility;
- dynamic value/cost arbitration;
- Swarm resource domains.

Recommendation: potential **Local Dispatcher / distributed work backend**, not logical Scheduler owner.

---

## 5.4 Temporal — **ADAPT candidate for high-maturity distributed workflows**

Repository: `temporalio/temporal`  
License: MIT

Temporal is the most mature durable-execution option evaluated. It is attractive for later workloads that need very strong workflow recovery and long-lived distributed orchestration.

Why it is not the MVC default:

- larger operational footprint than the single-Cube design needs;
- its workflow model is not Meeseek's organizational model;
- it does not provide Meeseek policy, memory, TEB or economic semantics;
- adopting it too early would violate the project's “complexity must earn its existence” rule.

Recommendation: retain as a production execution-backend option when a workload demonstrates the need.

---

## 5.5 Restate — **DO NOT SELECT as core OSS dependency**

Restate is technically interesting durable execution infrastructure, but its current server license is **Business Source License 1.1**, explicitly stating that BSL is not an Open Source license until its change date.

For an Apache-2.0 open-source framework whose ecosystem story matters, this is an avoidable licensing complication.

Recommendation: do not use as the default core substrate. Re-evaluate only if licensing requirements or project policy change.

---

# 6. Collective Memory and Knowledge

## 6.1 Core decision — **BUILD canonical epistemic model**

No evaluated memory project should own canonical Meeseek truth.

Meeseek must retain ownership of:

- Evidence vs Claim distinction;
- Knowledge Delta ingestion;
- provenance;
- epistemic statuses;
- verification method;
- contradiction and supersession;
- temporal validity;
- decision-time belief reconstruction;
- governed deletion/redaction lineage;
- Known Unknowns;
- durable failures/lessons;
- audit linkage.

The canonical model must remain valid even if every graph/vector product is replaced.

Therefore graph/vector systems are **derived indexes/projections and retrieval engines**, not the sole Source of Truth.

---

## 6.2 Graphiti — **ADAPT (primary temporal graph candidate)**

Repository: `getzep/graphiti`  
License: Apache-2.0

Graphiti is highly aligned with Meeseek's temporal knowledge requirements:

- temporal context graphs;
- validity windows for facts;
- changing relationships;
- historical queries;
- provenance back to source episodes;
- graph + semantic + keyword retrieval;
- incremental updates.

This directly maps to several approved Meeseek requirements.

### Boundary

Graphiti should not autonomously decide which Meeseek claims are `VERIFIED`, nor should its internal graph become the only retained evidence history.

Recommended pattern:

```text
Evidence / Events / Knowledge Delta (Meeseek canonical)
                │
                ├──> temporal relational/event record
                │
                └──> Graphiti projection/index
                         │
                         └── retrieval/context assembly
```

The projection must be rebuildable from canonical retained data where policy permits.

### Required spike

Test:

1. validity interval behavior;
2. provenance preservation;
3. contradictory concurrent claims;
4. historical query at time T;
5. deletion/redaction propagation;
6. custom Meeseek epistemic metadata;
7. ability to rebuild graph state from canonical events.

---

## 6.3 Caura — **ADAPT alternative / specialized shared-memory candidate**

Repository: `caura-ai/caura`  
License: Apache-2.0

Caura is particularly interesting because it already targets multi-agent shared memory and includes:

- trust tiers;
- visibility scopes;
- audit trails;
- contradiction handling;
- knowledge graph;
- multi-agent/multi-tenant sharing;
- MCP interface;
- Postgres + pgvector architecture.

It may be valuable if Meeseek later needs an off-the-shelf **shared semantic/social memory service**.

Reasons not to make it canonical today:

- its governance model is its own, not Meeseek Constitution/Authority semantics;
- its write path includes opinionated enrichment behavior;
- Meeseek requires evidence and epistemic lineage independent of any memory service;
- its current ecosystem/maturity is smaller than the largest alternatives.

Recommendation: keep as a serious adapter candidate, especially for Semantic/Social Memory, but do not delegate the entire CollectiveMemory contract to it.

---

## 6.4 Cognee — **ADAPT alternative for ingestion/knowledge enrichment**

Repository: `topoteretes/cognee`  
License: Apache-2.0

Cognee is a large and active memory ecosystem with flexible graph/vector backends. It is attractive for:

- ingesting heterogeneous data;
- building knowledge graphs;
- pluggable vector and graph stores;
- broad ecosystem integration.

Its default conceptual focus is more “AI memory/context” than Meeseek's “evidence-driven organizational epistemology”.

Recommendation: consider as a **data-ingestion/enrichment adapter** or alternative memory projection layer, not as canonical truth semantics.

---

## 6.5 Neo4j Agent Memory — **ADAPT reference, not default dependency**

Repository: `neo4j-labs/agent-memory`  
License: Apache-2.0  
Status: Neo4j Labs/community-supported project.

It provides useful patterns for graph-native memory, entity resolution/dedup, reasoning/tool linkage and audit edges.

Reasons not to choose as baseline:

- direct Neo4j dependency would make one database disproportionately architectural;
- project is Labs/community-supported;
- Meeseek memory must remain storage-product-neutral.

Use as a reference and possible adapter if Neo4j is chosen for a deployment.

---

# 7. Policy and Authority

## 7.1 Open Policy Agent — **ADAPT (primary PolicyEngine candidate)**

Repository: `open-policy-agent/opa`  
License: Apache-2.0  
Project status: CNCF graduated.

OPA is a strong match for the **deterministic decision engine**, because:

- policy is separated from application code;
- decisions can be context-aware;
- integrations exist through SDK/API mechanisms;
- Rego can return structured data, not only a boolean.

Meeseek still must own:

- Constitution semantics;
- constitutional ceremony;
- authority envelope schema;
- ALLOW / ALLOW_WITH_LIMIT / REQUIRE_APPROVAL / DENY decision contract;
- lease/fencing state inputs;
- semantic-effect classification;
- guarantee downgrade;
- Owner approval workflow;
- policy versioning/audit rules.

Therefore the recommended model is:

```text
Meeseek Authority Model + Policy Decision Contract
                     │
                     └── OPA evaluation engine
```

OPA is an engine, not the constitution.

---

## 7.2 Cedar — **ADAPT secondary candidate**

Repository: `cedar-policy/cedar`  
License: Apache-2.0

Cedar is purpose-built for authorization and offers schema validation and strong fine-grained permission modeling.

It becomes especially attractive if the runtime language and embedding model make its Rust implementation convenient.

Compared with OPA, Cedar is more authorization-focused while Meeseek frequently needs richer policy outputs such as `REQUIRE_APPROVAL`, resource limits, risk constraints and semantic-effect decisions.

Recommendation: keep as a serious alternative, but OPA currently appears to fit the broad PolicyEngine contract more naturally.

---

## 7.3 OpenFGA — **DEFER / optional ADAPT**

OpenFGA is excellent relationship-based authorization inspired by Zanzibar and supports multiple storage backends.

It is useful for questions such as:

> principal X has relation Y to resource Z

It is not by itself the right engine for Meeseek's context-rich policy decisions, budget limits and safety semantics.

Recommendation: use later only if resource/principal relationship graphs become complex enough to justify it.

---

# 8. Identity

## 8.1 Owner identity — **BUILD integration semantics**

The Owner/root-human identity and constitutional ceremony are Meeseek-specific security semantics.

The implementation should use established cryptographic/passkey/FIDO mechanisms rather than invent cryptography, but the mapping from those credentials into:

- Owner authority;
- constitutional authority;
- recovery/break-glass roles;
- one-time high-risk grants;

must remain Meeseek-owned.

---

## 8.2 SPIFFE/SPIRE — **ADAPT later for Cube/workload identity**

SPIFFE defines workload identities (SPIFFE IDs), verifiable identity documents and a workload API. SPIRE is the reference implementation.

This fits future multi-Cube needs extremely well:

- heterogeneous infrastructure;
- short-lived workload credentials;
- machine/service identity not equal to human identity;
- federation between trust domains;
- reduced shared-secret usage.

It is intentionally **not an MVC dependency**. A one-Cube local Collective does not need a SPIRE control plane merely to prove the architecture.

Recommendation: design `IdentityProvider` so SPIFFE/SPIRE can be added without changing identity semantics.

---

# 9. Executor and Capability Interoperability

## 9.1 Executor contract — **BUILD**

Meeseek's `Executor` contract is a core abstraction because it must normalize:

- execution;
- checkpoint/cancel;
- evidence/artifact reporting;
- cost/usage reporting;
- cancellation certainty;
- sandbox/enforcement context;
- capability assessment;
- failure taxonomy;
- external-operation interaction.

No existing harness should define this contract.

---

## 9.2 Model Context Protocol — **ADAPT / standard transport**

Project: `modelcontextprotocol/modelcontextprotocol`  
License: MIT

MCP is an excellent interoperability layer for connecting LLM hosts to tools and context. The current specification defines JSON-RPC communication, capability negotiation and standard transports such as stdio and Streamable HTTP.

MCP is useful for Meeseek because many existing harnesses already understand it.

But:

> **MCP capability advertisement is not Meeseek authority.**

An MCP server saying “I expose `delete_resource`” must not cause that operation to become authorized.

Recommended layering:

```text
MCP tool discovery/invocation
          ↓
CapabilityProvider adapter
          ↓
Meeseek Capability Assessment
          ↓
Policy / Authority / TEB
          ↓
Commit Boundary when consequential
```

Recommendation: support MCP early as an adapter protocol, but never make it the internal authority model.

---

# 10. Scheduling and Distributed Work

## 10.1 Logical Scheduler — **BUILD**

This remains one of the strongest BUILD decisions.

Generic workflow systems provide queues, timers, retries, affinity and priorities, but the approved Meeseek scheduler combines:

- purpose/Mission/obligation value;
- capability/access/authority eligibility;
- scarce capability protection;
- deadline scheduling shadows;
- cost of delay;
- Opportunistic Surplus;
- cognition budget;
- elastic epistemic parallelism;
- Swarm caps/reservations;
- human-attention cost;
- internal planning Tasks;
- dynamic backlog re-evaluation.

This is not a conventional queue priority formula.

Therefore:

> **Meeseek owns scheduling decisions; external engines may execute the resulting decisions.**

Hatchet and Temporal remain possible future dispatch/execution backends.

---

# 11. Eventing and Transport

## 11.1 MVC — **keep local**

The first version should use local durable state, timers and direct event adapters. Introducing a cluster event bus before a second Cube exists would solve a problem the project does not yet have.

---

## 11.2 NATS JetStream — **ADAPT when distribution earns it**

Project: NATS / JetStream  
License: Apache-2.0 (NATS server source)

Relevant capabilities include:

- persistent streams;
- work queues;
- at-least-once delivery;
- publication deduplication / exactly-once-style mechanisms;
- KV with watchers/versioning/TTL;
- object storage;
- clustering;
- request/reply;
- lightweight single-binary operation.

This is an unusually good match for Meeseek's “small system that can later distribute” principle.

It still must not own authoritative Meeseek semantics such as External Operation logical identity or Task acceptance.

Recommendation: preferred event-bus candidate for the multi-Cube architecture-decision stage unless a workload demonstrates that Kafka-scale ecosystem features are actually required.

---

# 12. Observability and Economics

## 12.1 OpenTelemetry — **ADOPT**

OpenTelemetry is the clear standard choice for telemetry interchange:

- traces;
- metrics;
- logs;
- vendor-neutral instrumentation/export.

Meeseek should emit OTel-compatible telemetry rather than inventing an observability protocol.

However OTel does not replace:

- Resource Ledger;
- budget reservations;
- unresolved spend exposure;
- readiness/reconstruction economics;
- economic attribution by Goal/Task/Attempt/Operation.

Those are semantic state and must remain durable in Meeseek.

Recommendation:

```text
Meeseek Resource Ledger = source of economic semantics
OpenTelemetry            = telemetry/export channel
```

---

# 13. Trusted Enforcement Boundary and Sandboxing

## 13.1 Core sandbox/enforcement contract — **BUILD**

The approved design requires the runtime to know **what is actually enforced**.

The contract should represent properties such as:

- filesystem isolation;
- process/user isolation;
- network allow/deny scope;
- secret/credential visibility;
- syscall/kernel isolation strength;
- resource limits;
- host platform;
- root/admin availability;
- whether egress can bypass the capability provider;
- whether cancellation is enforceable;
- whether the sandbox itself is trusted enough for the required risk class.

No backend should be labeled “secure” as a single boolean.

---

## 13.2 Rootless containers — **ADAPT for low/medium-risk local execution**

Docker rootless mode runs daemon and containers without host root privileges and is useful as a practical baseline on supported systems.

It should not be mistaken for a perfect hostile-code boundary; Docker documentation itself emphasizes the daemon and mount attack surface when configured with broad privileges.

Recommendation: suitable as one local adapter where the declared guarantees match the task risk.

---

## 13.3 nsjail / gVisor — **ADAPT for stronger Linux isolation**

Both are Apache-2.0 projects suitable for stronger Linux sandbox profiles.

- **nsjail** offers namespaces/cgroups/seccomp-style jail construction.
- **gVisor** adds a stronger userspace-kernel isolation layer and is attractive for hostile executor workloads.

Recommendation: evaluate both in the TEB spike; do not bake either into the semantic core.

---

## 13.4 Firecracker — **ADAPT for high-isolation/server environments, not MVC default**

Firecracker is Apache-2.0 and provides microVM isolation used in serious production environments.

Its isolation strength is attractive for high-risk executor profiles, but its setup/host requirements are disproportionate to a first local laptop Collective.

Recommendation: future high-assurance sandbox backend.

---

## 13.5 E2B — **ADAPT optional remote sandbox provider**

E2B exposes an open-source microVM sandbox stack and managed service, Apache-2.0.

It is useful as an optional remote execution capability, especially for agent-generated code, but cannot be required for local-first operation.

Recommendation: external sandbox adapter, not TEB definition.

---

# 14. Infrastructure Provisioning

## 14.1 InfrastructureProvider semantics — **BUILD adapter boundary**

Meeseek owns:

- authority check;
- minimal capability-gap escalation;
- budget and risk evaluation;
- plan approval requirements;
- External Operation ledger;
- verification of resulting capability.

IaC software only implements the infrastructure change.

---

## 14.2 OpenTofu — **ADAPT preferred OSS IaC engine**

Project: `opentofu/opentofu`  
License: MPL-2.0  
Governance: Linux Foundation project.

OpenTofu is the preferred generic IaC candidate because it remains genuinely open source and supports a broad provider ecosystem.

Recommendation: one `InfrastructureProvider` implementation can invoke OpenTofu plans/applies under the Commit Boundary.

---

## 14.3 Terraform — **do not make the default OSS dependency**

Current Terraform is source-available under BSL 1.1 rather than an OSI open-source license.

Users may still choose a Terraform adapter if their environment requires it, but the project's own open-source reference path should prefer OpenTofu.

---

# 15. Persistence Substrate

## 15.1 SQLite — **ADOPT candidate for MVC**

SQLite is public domain, self-contained, extremely mature and requires no server process.

It is a natural candidate for a one-Cube semantic vertical slice because it supports the Principle of Sufficient Effort:

- near-zero operations burden;
- transactions;
- durable local state;
- easy backup/testing;
- no cluster dependency.

Critical design note: using SQLite does not mean designing “single-node semantics”. Schemas and transaction rules must still contain IDs, fencing generations and logical-effect constraints that later map to shared storage.

---

## 15.2 PostgreSQL — **ADOPT candidate for later shared state**

PostgreSQL uses a permissive BSD-like PostgreSQL License and is a mature transactional database.

It is the leading candidate for later Class A/B shared state because it can host:

- transactional task/attempt state;
- fencing generations;
- External Operation unique constraints;
- reservations/exposure;
- policy/identity metadata;
- JSON/relational hybrid data;
- optional pgvector.

Recommendation: ensure the MVC persistence abstraction can move from SQLite to Postgres without changing semantic contracts.

---

# 16. Candidate Projects Not Recommended as the Core

## Generic agent frameworks

LangGraph, CrewAI, AutoGen, Letta and similar projects can be useful **executors or internal planning libraries**, but should not own Meeseek's runtime semantics.

Reasons:

- often assume an LLM-centric agent abstraction;
- planning graph is not durable organizational authority;
- tool exposure is not capability authority;
- their memory models are not the full Meeseek epistemic model;
- Meeseek must remain vendor/harness neutral.

## Kubernetes

Powerful later infrastructure, but explicitly unnecessary for MVC. It should be an optional environment, not an architectural dependency.

## Kafka

Mature event streaming, but substantially larger operational and conceptual footprint than the likely initial event requirements. NATS deserves evaluation first under the Principle of Sufficient Effort.

## Mandatory Neo4j

Graph database lock-in would conflict with the design's global logical graph / federated physical storage principle. Neo4j can be an adapter, not a mandatory brain.

---

# 17. Provisional Reference Shape After Mapping

This is **not yet a final architecture decision**, but the mapping supports the following lowest-complexity direction for the next stage:

```text
Meeseek Semantic Core                    BUILD
│
├── Task / Attempt / External Operation  BUILD
├── Scheduler                            BUILD
├── Authority model                      BUILD
│    └── OPA engine                      ADAPT
├── Runtime reliability                  ADAPT AgentLedger where proven
├── Executor adapters                    BUILD
│    └── MCP transport                   ADAPT
├── Capability Registry/Assessment       BUILD
├── Evidence/Event canonical model       BUILD
├── CollectiveMemory semantics           BUILD
│    └── Graphiti projection             ADAPT
├── Resource Ledger                      BUILD
│    └── OpenTelemetry export            ADOPT
├── TEB contract                         BUILD
│    └── sandbox implementations         ADAPT
├── InfrastructureProvider contract      BUILD
│    └── OpenTofu                        ADAPT
└── MVC state substrate                  ADOPT SQLite candidate

Later when justified:

PostgreSQL + NATS + SPIFFE/SPIRE + distributed execution backend
```

The most important architectural boundary is that **SQLite/Postgres, AgentLedger, Graphiti, OPA, MCP, NATS and sandbox products remain replaceable**.

---

# 18. Required Validation Spikes Before Architecture Decisions

These are research/compatibility spikes, not implementation of the MVC.

## Spike A — AgentLedger semantic compatibility

Goal: determine exactly what can be reused without weakening Meeseek semantics.

Validate against all eight MVC failure scenarios:

1. unknown external result;
2. unresolved billing exposure;
3. stale Attempt fencing;
4. crash before verification;
5. capability bypass attempt;
6. higher-level constraint precedence;
7. hard-budget honesty;
8. confirmed effect reused across Attempt replacement.

Output:

- feature/semantic matrix;
- gaps;
- adapter shape;
- reuse recommendation: library vs protocol/conformance reuse vs selected code/patterns.

## Spike B — Graphiti temporal memory projection

Build a tiny test corpus containing:

- evidence artifact;
- claim A valid in period 1;
- superseding claim B valid in period 2;
- contradictory claim for the same time;
- source provenance;
- redaction request.

Verify current-state and historical queries and identify which semantics must remain canonical outside Graphiti.

## Spike C — OPA Meeseek policy decision

Model at least:

- ordinary ALLOW;
- ALLOW_WITH_LIMIT;
- REQUIRE_APPROVAL;
- DENY;
- lease/fencing context;
- money limit;
- semantic-effect risk class;
- Owner-scoped exceptional grant;
- a request that must fail closed.

Confirm that the Meeseek decision contract remains stable even if the underlying policy engine changes.

## Spike D — TEB local bypass test

Run a harness with shell capability and deliberately attempt:

- direct network egress outside approved provider;
- reading ambient credentials;
- modifying protected host files;
- invoking an effect after fencing token expiry.

Compare realistic local sandbox adapters and document the exact guarantee each provides on supported platforms.

## Spike E — persistence crash model

Prototype only the storage invariants, ideally on the lightweight MVC candidate:

- Task / Attempt separation;
- fencing-generation conditional writes;
- `logical_effect_key` uniqueness;
- PREPARED operation durable before dispatch;
- OUTCOME_UNKNOWN recovery;
- AWAITING_VERIFICATION recovery;
- budget reservation/exposure transaction.

This spike determines whether the chosen persistence abstraction supports the reviewed design before application architecture is committed.

## Spike F — MCP capability adapter

Verify that an MCP-discovered tool can be wrapped as a Meeseek capability while preserving:

- independent authority evaluation;
- scoped context;
- cost reporting;
- commit interception for consequential operations;
- sandbox routing;
- stable evidence capture.

---

# 19. Decisions Ready to Carry Forward

The following mapping decisions are strong enough to use as inputs to the architecture stage:

1. **BUILD the Meeseek semantic core.** No current framework owns the approved organizational model.
2. **Do not build durable-runtime primitives blindly before testing AgentLedger.** It is too close to the required reliability semantics to ignore.
3. **Keep canonical memory Meeseek-owned; treat graph memory as a projection.** Graphiti is the first candidate to test.
4. **Use an existing deterministic policy engine rather than inventing a policy language.** OPA is the primary candidate, Cedar the secondary candidate.
5. **Support MCP as interoperability, never as authority.**
6. **Adopt OpenTelemetry for observability transport.**
7. **Keep sandbox semantics in the TEB contract and support multiple isolation backends.**
8. **Prefer OpenTofu to Terraform for the project's OSS reference infrastructure path.**
9. **Do not deploy distributed infrastructure for MVC without demonstrated need.** SQLite is a strong MVC persistence candidate; PostgreSQL/NATS/SPIRE are later-scale candidates.
10. **Do not let a workflow engine own the logical Scheduler.** Hatchet/Temporal/DBOS may execute work behind it.
11. **Reject BSL infrastructure as a required core dependency when a credible OSS path exists.**

---

# 20. Architecture Questions Opened by This Mapping

The OSS mapping intentionally leaves these for the next formal stage:

1. What language/runtime owns the Meeseek semantic core?
2. Do we embed/adapt AgentLedger, implement its contract in another language, or reuse only its patterns/conformance tests?
3. Is SQLite the MVC canonical state store, and what exact transaction model protects Task/Attempt/Operation invariants?
4. What is the canonical Evidence/Event representation from which Knowledge Graph projections can be rebuilt?
5. Is OPA run embedded/sidecar/subprocess, and how is policy versioning tied atomically to Commit Boundary decisions?
6. What minimum sandbox profile is required for the first agentic executor on each supported host OS?
7. Which external effects in MVC are mediated through a provider versus permitted directly through target-service IAM?
8. How is `logical_effect_key` derived and scoped without letting an LLM choose a value that defeats deduplication?
9. What is the smallest executor contract that supports deterministic scripts and agentic harnesses without becoming harness-specific?
10. Which parts of Graphiti/Caura/Cognee, if any, enter MVC versus a later memory milestone?
11. What is the migration boundary from SQLite/local events to Postgres/NATS/multi-Cube without changing semantics?

These should be resolved through explicit architecture decisions and the validation spikes above, not by adding more conceptual mechanisms to the approved design.

---

# 21. Sources Reviewed

Primary/official project materials reviewed during this mapping include:

- `yaogdu/AgentLedger` — runtime scope, leases/fencing, side-effect ledger, evidence/replay, budgets, language-neutral conformance and maturity notes; Apache-2.0.
- `temporalio/temporal` — durable execution platform; MIT.
- `dbos-inc/dbos-transact-*` — database-backed durable workflows; MIT for the verified Python package.
- `hatchet-dev/hatchet` — durable tasks, worker routing/concurrency, events and Postgres self-hosting; MIT.
- `restatedev/restate` — durable execution; Business Source License 1.1 at time of review.
- `getzep/graphiti` — temporal context graph, validity periods, provenance, historical queries; Apache-2.0 source.
- `caura-ai/caura` — governed shared multi-agent memory, audit, trust tiers, knowledge graph; Apache-2.0.
- `topoteretes/cognee` — open-source agent memory / knowledge graph platform; Apache-2.0.
- `neo4j-labs/agent-memory` — graph-native agent memory; Apache-2.0; Neo4j Labs.
- `open-policy-agent/opa` — general-purpose policy engine, CNCF graduated.
- `cedar-policy/cedar` — authorization policy engine/language; Apache-2.0.
- `openfga/openfga` — Zanzibar-style fine-grained authorization; Apache-2.0 ecosystem.
- SPIFFE/SPIRE specifications and reference implementation — workload identity.
- Model Context Protocol specification — open tool/context interoperability protocol; MIT.
- OpenTelemetry specification — vendor-neutral traces/metrics/logs; Apache-2.0.
- NATS/JetStream documentation and NATS server source — persistent messaging/dedup/work queues; Apache-2.0 server source.
- Docker rootless/security documentation.
- `google/gvisor`, `google/nsjail`, `firecracker-microvm/firecracker`, `e2b-dev/E2B` — sandbox/isolation candidates.
- `opentofu/opentofu` — open-source IaC, MPL-2.0.
- `hashicorp/terraform` — BSL 1.1 at time of review.
- SQLite official project — public-domain embedded database.
- PostgreSQL official project — permissive PostgreSQL License.
- `pgvector/pgvector` — PostgreSQL-compatible vector extension licensing/reference.

Because OSS evolves quickly, license and maturity checks should be repeated before the first public release and before elevating an optional adapter into a required dependency.

---

## 22. Exit Gate

The OSS landscape-mapping gate is complete when the Owner accepts this document as sufficient input for concrete architecture decisions.

The next stage is:

```text
Approved written design
→ OSS landscape mapping                 [this document]
→ implementation architecture decisions [next]
→ detailed MVC implementation plan
→ implementation
```
