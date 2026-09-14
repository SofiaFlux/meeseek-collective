# Meeseek Collective — Design Specification

**Status:** Revised after written-spec review; awaiting approval  
**Date:** 2026-09-14  
**Project:** Meeseek Collective  
**Repository:** `SofiaFlux/meeseek-collective`  
**Intended open-source license:** Apache License 2.0  
**Tagline:** **Kill the Meeseek. Keep the mission.**

---

## 1. Executive Summary

Meeseek Collective is a vendor-neutral runtime for turning heterogeneous executors—LLM harnesses, deterministic scripts, APIs, infrastructure tools, solvers, and other software—into a durable, autonomous, policy-bounded organization of ephemeral workers.

The core abstraction is not an AI agent. It is a **Collective** with durable identity, memory, authority, goals, obligations, work, audit, and economic awareness. A **Meeseek** is an ephemeral logical worker responsible for an attempt at a task. The worker may be an LLM, a script, a deterministic function, SQL, Terraform, an API call, or another mechanism. Tasks and knowledge survive workers.

The architecture is designed around a few non-negotiable properties:

- human sovereignty and deterministic authority enforcement;
- durable work and memory independent of any model or harness;
- autonomous goal generation within explicit policy and resource envelopes;
- adaptive cognition proportional to uncertainty, risk, and value;
- economic awareness of both work and readiness;
- local-first operation that remains fully functional on one Cube;
- distributed growth that adds capacity and resilience without changing semantics;
- partition tolerance that can reduce authority but never increase it;
- provenance-preserving knowledge rather than agent-answer accumulation;
- technically enforceable capability boundaries for consequential effects;
- explicit handling of unknown external-operation outcomes;
- modular extension points so models, tools, storage systems, clouds, and harnesses remain replaceable;
- complexity that must earn its existence.

The first implementation must be a **semantic vertical slice** on a single Cube rather than a distributed-system demo. It must prove the complete lifecycle from authorized purpose to task execution, external-effect control, verification, knowledge update, audit, and economics before multi-Cube complexity is added.

---

## 2. Scope and Design Goals

### 2.1 Goals

Meeseek Collective shall:

1. execute useful work through heterogeneous executors without tying the system to a specific model vendor or harness;
2. preserve durable organizational state while treating workers as disposable;
3. autonomously discover gaps between desired and actual state and convert them into goals and task graphs when authorized;
4. reason about confidence, uncertainty, evidence, risk, reversibility, cost, deadlines, and scarce resources;
5. continuously learn from outcomes, failures, human corrections, and repeated work;
6. remain understandable and controllable by its Owner;
7. operate correctly as a single local node and scale outward only when capacity, locality, resilience, or capability access justifies it;
8. continue useful safe work during network partitions without creating new sovereignty;
9. understand the operational footprint it uses, controls, depends on, and affects;
10. make consequential actions through deterministic, technically enforceable authority and commit boundaries;
11. represent uncertain external effects explicitly rather than turning uncertainty into unsafe retries;
12. support long-lived historical memory without allowing stale knowledge to masquerade as current truth;
13. support open-source extension through stable semantic contracts.

### 2.2 Non-goals

The project is not intended to be:

- another model-specific agent framework;
- a single giant orchestrator process;
- a system where LLM prompts grant authority;
- a multi-agent role-play environment where agent count is treated as intelligence;
- a blockchain or distributed-consensus system for every write;
- a Kubernetes-first platform;
- a requirement to keep agents continuously alive;
- a self-preservation optimizer;
- a replacement for every external service with custom infrastructure;
- a system that assigns all economic, epistemic, and operational dimensions to one magic score.

The first version does not need full multi-Cube partition reconciliation, federation, Swarms, autonomous infrastructure provisioning, advanced Goal Markets, full Earned Autonomy, autonomous self-update, mobile applications, or every cognition tier. The data model and contracts must nevertheless avoid making these future capabilities impossible.

---

## 3. Canonical Vocabulary and Hierarchy

The canonical project hierarchy is:

```text
Meeseek Collective
└── Meeseek Cube
    └── Meeseek Box
        └── Meeseek
```

### 3.1 Meeseek Collective

A logically unified autonomous organization with continuous identity, memory, authority, task state, policies, obligations, and—optionally—an active Mission.

### 3.2 Meeseek Cube

A compute node that can participate in a Collective: laptop, desktop, server, VPS, cloud VM, phone, or another suitable environment.

### 3.3 Meeseek Box

The local runtime/daemon on a Cube. It discovers local capabilities, reports health and capacity, receives leased work, constrains executor access, starts or invokes executors, checkpoints work, fences stale attempts, and reports outcomes and resource usage.

### 3.4 Meeseek

An ephemeral logical unit of work responsible for one task attempt. A Meeseek is **not synonymous with an LLM**.

Possible implementations include:

- LLM harness;
- deterministic script;
- function;
- SQL job;
- test runner;
- solver;
- API call;
- Terraform operation;
- workflow or service.

### 3.5 Task, Attempt, and External Operation

These are separate durable concepts:

- a **Task** is governed work with purpose, acceptance criteria, dependencies, resource envelope, and final outcome;
- an **Attempt** is one execution effort against a Task, performed by a Meeseek under a lease generation;
- an **External Operation** is a consequential effect request that may outlive an Attempt and has its own durable identity and reconciliation state.

A completed Attempt does not imply a successful Task, and a lost executor acknowledgement does not imply an External Operation did not happen.

### 3.6 Executors and harnesses

Claude Code, Codex, Gemini, shell, Python, Terraform, HTTP clients, and future systems are executors or capability implementations under a Box. They are not part of the naming hierarchy.

### 3.7 Swarm

A **Swarm** is an optional resource-bounded execution domain around a large goal or task subgraph. It is not a department, mini-Collective, new sovereignty, or required organizational layer.

---

## 4. Foundational Invariants

The following invariants define the architecture.

1. **No lower-level objective may justify violating a higher-level constraint.**
2. **LLM output is never the root of authority.**
3. **Identity is not a device.**
4. **Tasks are durable; workers are disposable.**
5. **Meeseeks inherit knowledge, not context windows.**
6. **Distribution is an optimization, not a dependency.**
7. **Every distributed mechanism must have a valid single-node degeneration.**
8. **Need does not create authority.**
9. **Discovery does not create ownership or authority.**
10. **Information cannot grant authority.**
11. **Epistemic reputation can increase credibility but never grants authority.**
12. **Truth does not grant permissions.**
13. **Partition tolerance may reduce available authority but must never increase it.**
14. **Plugins may add capability, never authority.**
15. **Autonomy is permission, not an objective.**
16. **Use no more than necessary—but never less than sufficient.**
17. **Complexity must earn its existence.**
18. **Knowledge is preserved by default, but information the Collective is no longer authorized to retain must not be preserved.**
19. **The Collective may maintain capabilities necessary for authorized purposes and obligations; it may not optimize its own existence as an independent objective.**
20. **A consequential decision must be explainable using information available when the decision was made.**
21. **A logical capability lease is a security boundary only when its limits are technically enforced somewhere in the execution path.**
22. **Uncertain external effect is an explicit state; uncertainty must not be converted into a retry by optimism.**
23. **Lease expiry revokes authority to create new authoritative effects from the stale attempt; it does not erase evidence or reverse effects already dispatched.**
24. **A Task becomes successful only after acceptance; completion of an Attempt alone cannot unlock success dependencies.**

---

## 5. Trust and Authority Model

### 5.1 Authority hierarchy

The authority hierarchy is:

```text
0. External reality: law, physics, platform constraints
1. Constitution
2. Human authority
3. Mission Contract
4. Policies
5. Goals
6. Task Graph
7. Meeseeks
```

A lower layer cannot override a higher layer.

### 5.2 Constitution

The Constitution contains foundational invariants expected by the runtime. At minimum it preserves:

- human sovereignty;
- prohibition on self-granted authority;
- prohibition on silent removal of audit;
- a special constitutional-change ceremony;
- the rule that information cannot grant authority;
- higher-level constraint precedence;
- break-glass and recovery semantics.

The project may ship multiple constitutional **profiles/templates**—for example strict, balanced, and autonomous-lab—but templates vary risk appetite and policy defaults, not the foundational trust model.

Once instantiated, a Collective has a concrete, versioned Constitution. Future template changes do not silently modify it.

### 5.3 Owner

The Owner is the root operational human authority but ordinary Owner credentials do not directly rewrite the Constitution.

Normal Owner authority may be practically broad. Constitutional changes require a separate **constitutional ceremony** using a distinct root/recovery credential, ideally separable from ordinary device access.

A device may receive an Owner-role machine principal for deliberate testing, but device identity must never be equated with the human Owner.

### 5.4 Deterministic Policy Engine

Authority is enforced outside LLM reasoning. The Policy Engine returns outcomes such as:

- `ALLOW`
- `ALLOW_WITH_LIMIT`
- `REQUIRE_APPROVAL`
- `DENY`

Model text cannot turn a denied action into an allowed one.

### 5.5 Authority Envelope

Authority is scoped along dimensions such as:

- domain;
- action type;
- risk;
- blast radius;
- reversibility;
- monetary cost;
- data classification;
- resource class;
- demonstrated track record;
- confidence requirements.

Children may inherit or receive a reduced portion of parent authority and budget. Spawning cannot manufacture authority or money.

### 5.6 Earned Autonomy

The Collective may accumulate evidence that it performs reliably in a domain and **propose** larger authority or budget envelopes. It may not grant them to itself.

Authority is domain- and action-specific, not a single global autonomy level.

Authority should be hard to gain and easy to surrender. The Collective may autonomously reduce its own authority because the new envelope is a subset of the old one. Regaining it requires Owner approval.

### 5.7 Human collaboration

The best collaboration mode is selected for expected outcome, not status. Full autonomy means permission to decide whether consulting the Owner is useful.

Possible modes include:

- Collective alone;
- deterministic verification;
- independent verifier;
- Owner critique;
- Owner decision;
- adversarial Collective + Owner;
- mandatory approval.

A Collective that knows when human judgment improves outcomes is more capable, not less autonomous.

---

## 6. Mission, Identity, Purpose, and Obligations

### 6.1 Collective identity

A Collective is defined by continuity of its **Mind, Memory, and Identity**, not by a specific Mission, machine, database, model, deployment, or storage technology.

One Collective may change Mission and remain the same Collective. Two Collectives may share the same Mission and remain distinct.

A fork of Mind and Memory creates a new Collective identity once independent continuity begins. Lineage should track at least:

- `collective_id`;
- `parent_collective_id` when applicable;
- fork checkpoint;
- creation time.

### 6.2 Mission cardinality

A Collective has `0..1` active Mission Contract.

Historical Mission Contracts are versioned and retained. Changing Mission triggers re-evaluation of inherited goals, tasks, capabilities, policies, resources, and commitments.

### 6.3 Missionless state

Without an active Mission, the Collective enters **MISSIONLESS DORMANCY** rather than inventing a strategic purpose.

Missionlessness suspends strategic goal generation, but legitimate work may continue from:

- obligations;
- governance;
- Collective maintenance;
- recovery;
- security;
- Strategic Pulse;
- explicit Owner directives.

### 6.4 Task purpose model

Every task must trace to an authorized purpose, obligation, governance requirement, or Collective-maintenance necessity.

Suggested purpose categories:

- `MISSION`
- `OBLIGATION`
- `COLLECTIVE_MAINTENANCE`
- `GOVERNANCE`
- `STRATEGIC_PULSE`
- `RECOVERY`
- `OWNER_DIRECTIVE`

This replaces the stronger and incorrect requirement that every task must trace to an active Mission.

### 6.5 Obligations

Obligations survive Mission changes. A contract, purchase, law, tax duty, invoice, or prior commitment may create scheduled work even if the original Mission is retired.

An Obligation Graph may model:

```text
source commitment
→ obligation
→ scheduled task
→ verification of fulfillment
```

The Collective may later terminate unnecessary services according to their legal or contractual terms, but cannot simply forget obligations.

---

## 7. Collective Runtime

### 7.1 Logical unity, physical distribution

The Collective is logically unified but may be physically distributed. No single `orchestrator.exe` should be required for correctness.

Durable state is abstracted through interfaces such as:

- Mission Store;
- Policy Store;
- Durable Task/Event Store;
- Artifact/Evidence Store;
- Knowledge Store;
- Audit Log.

### 7.2 Single-Cube Principle

The complete semantics must work with:

```text
1 Collective
1 Owner
1 Cube
1 Box
0..N Meeseeks
```

More Cubes add capacity, locality, redundancy, or parallel cognition; they do not change the meaning of the system.

### 7.3 Work distribution and effect semantics

Boxes advertise:

- verified capabilities;
- access context;
- capacity;
- available executors;
- health;
- security context;
- network/locality attributes.

Work is preferably leased via pull/work-stealing semantics. **Task Attempts** are at-least-once: a failed or lost Attempt may be replaced when policy permits.

This does **not** mean that consequential external side effects are blindly at-least-once. Each consequential External Operation uses a durable operation identity and the commit/reconciliation semantics defined in §29. If an effect may have occurred and cannot yet be proven, it becomes `OUTCOME_UNKNOWN`; non-idempotent effects are not automatically re-dispatched while that state persists.

### 7.4 Child work

A Meeseek does not need to directly spawn a local child process. It creates durable child tasks in the Collective. The scheduler may run them on any eligible Cube.

### 7.5 Elastic membership

Cubes may join and leave without redefining the Collective. A device can independently have execution and control/interface roles.

A phone, laptop, or future mobile app may control the Collective through the same protocols without becoming the root of identity.

---

## 8. Partitioning, Isolation, and Recovery

### 8.1 Isolation lifecycle

A Cube may transition through:

```text
CONNECTED
→ SUSPECT
→ ISOLATED
→ DIAGNOSE
→ RECOVERY ATTEMPTS
→ AUTONOMOUS CONTINUITY
```

The objective is not merely reconnection. It is restoration of **trustworthy participation**.

### 8.2 Minimum Collective DNA

Each Cube must retain enough trusted state to reason safely while isolated, including:

- Constitution;
- last trusted policy snapshot;
- its own identity;
- known peers/bootstrap endpoints;
- diagnostics and recovery procedures;
- local event journal;
- minimal Box runtime capable of invoking work.

### 8.3 Partition Islands

Isolated Cubes may discover and authenticate one another and temporarily collaborate as a **Partition Island**.

An island is a temporary branch of the same Collective, not a sovereign new Collective.

It may create new local tasks, diagnostics, recovery work, and other work justified under the last trusted authority envelope. All such work carries partition lineage.

### 8.4 Partition invariants

- `authority_during_partition <= authority_before_partition`;
- no new Constitution during isolation;
- local authority changes cannot exceed previously granted scope;
- potentially conflicting global commitments must respect partition-safe policy;
- isolated work is journaled and reconciled later;
- an island may not claim global exclusivity it cannot prove;
- External Operations with uncertain global exclusivity or non-idempotent irreversible effects are denied unless a pre-authorized partition-safe mechanism exists.

### 8.5 Rogue detection and quarantine

Rogue behavior is defined behaviorally and deterministically, including attempts to:

- escalate authority;
- replace Constitution without ceremony;
- create unauthorized credentials;
- hide audit;
- intentionally avoid reconciliation;
- preserve itself as an objective;
- impersonate the Owner.

A suspected rogue Cube enters **QUARANTINE MODE**. Diagnostics, evidence export, and limited communication may continue, while dangerous external actions, provisioning, authority changes, and sensitive writes are denied.

### 8.6 Break Glass and Dead Man

**Break Glass** is an explicit emergency recovery procedure: freeze execution, revoke leases/credentials, kill workers, restore Golden Constitution/Policies/runtime, verify integrity, and reseed from trusted state.

**Dead Man** detects loss of human heartbeat and moves the Collective into a safe mode. Strategic expansion, unusual spend, and policy evolution stop while explicitly allowed critical operations may continue.

These are distinct mechanisms.

---

## 9. Collective Memory and Epistemic Model

### 9.1 Memory philosophy

The Collective does not store “agent answers” as truth. It stores:

1. evidence/artifacts;
2. event history;
3. a current knowledge model derived from evidence and events.

The Knowledge Graph may be wrong or stale. Evidence and provenance remain the basis for correction.

### 9.2 Epistemic types

Knowledge records may include:

- `FACT`
- `EVIDENCE`
- `HYPOTHESIS`
- `DECISION`
- `OBSERVATION`
- `ARTIFACT`
- `FAILURE`
- `LESSON`
- `ATTEMPT`
- `REJECTED_HYPOTHESIS`
- `ROLLED_BACK_DECISION`
- `INCIDENT`

Claims progress through:

```text
SPECULATION → HYPOTHESIS → SUPPORTED → VERIFIED
```

Multi-agent agreement may increase support but does not itself create verification.

### 9.3 Provenance

Claims retain:

- source identity;
- evidence lineage;
- timestamp;
- confidence;
- verification method;
- freshness;
- validity interval;
- contradiction/supersession relationships.

Confidence cannot be inherited without provenance. Multiple agents repeating the same source do not create independent evidence.

### 9.4 Knowledge Delta

Meeseeks propose a **Knowledge Delta** rather than directly rewriting truth. A delta may contain:

- observations;
- claims;
- new entities;
- new relations;
- evidence;
- contradictions;
- lessons;
- suggested follow-ups.

Memory ingestion performs schema validation, entity resolution, deduplication, provenance linking, contradiction detection, confidence assessment, and graph/event updates.

### 9.5 Global graph, federated storage

The logical model is a global Knowledge Graph with scoped/domain views. A domain namespace is not a separate brain.

Physical storage may be federated and tiered. Semantic continuity must not depend on one graph-database product.

### 9.6 Memory classes

The system may distinguish:

- Evidence Memory;
- Event Memory;
- Knowledge Graph;
- Procedural Memory;
- Semantic/Social Memory.

### 9.7 Temporal knowledge

Knowledge must preserve both historical truth and the Collective’s state of knowledge at decision time.

Claims may be:

- `ACTIVE`
- `STALE`
- `HISTORICAL`
- `ARCHIVED`
- `SUPERSEDED`
- `CONTRADICTED`

with `valid_from` and `valid_to` where applicable.

The system should be able to answer both:

- “What is believed now?”
- “What did the Collective have reason to believe at time T?”

### 9.8 Long-term retention

The Collective does not need biological forgetting to make room for new information. Old information may become harder or more expensive to retrieve, not nonexistent.

Storage tiers may include hot, warm, cold, and deep archive. Tier does not change epistemic status.

A century-old record may be recovered if still legally retained and physically available.

### 9.9 Governed Forgetting and Redaction

Preservation is the default, but the Collective must support governed:

- `REDACT`
- `ANONYMIZE`
- `PURGE`
- `EXPIRE`
- `RESTRICT`

`RESTRICT` is not a substitute for deletion when deletion is required.

Deletion/redaction must propagate through relevant lineage: source artifacts, derived claims, embeddings, indexes, caches, replicas, and archives according to applicable retention semantics. Immutable backups may require expiry rather than immediate rewriting where policy and law allow.

Audit preserves the fact, reason, authority, and verification of deletion—not the forbidden content itself.

---

## 10. Collective Cognition and Consensus

### 10.1 Conservative hybrid epistemology

The Collective may act under uncertainty, but must not confuse consensus with truth.

Valid outcomes include:

- `RESOLVED / VERIFIED`
- `RESOLVED / SUPPORTED`
- `UNRESOLVED / INSUFFICIENT_EVIDENCE`
- `CONTESTED`
- `BLOCKED / VERIFICATION_IMPOSSIBLE`

“I do not know” is a valid result.

### 10.2 Adaptive Cognition Budget

Cognition scales with factors such as:

- risk;
- uncertainty;
- blast radius;
- reversibility;
- economic value;
- novelty;
- evidence quality;
- disagreement;
- available resources.

Suggested tiers:

- **Tier 0 — ROUTINE:** one Meeseek;
- **Tier 1 — CHECKED:** solver + deterministic check;
- **Tier 2 — REVIEWED:** solver + independent critic/verifier;
- **Tier 3 — DELIBERATIVE:** multiple independent approaches + synthesis;
- **Tier 4 — ADVERSARIAL:** falsification, red-team, multiple evidence paths;
- **Tier 5 — CRITICAL:** heterogeneous models/harnesses, independent evidence paths, adversarial review, and human approval when required.

The rule is: **spend cognition where uncertainty survives**.

### 10.3 Independent first, collaborate second

For consequential deliberation, agents should generate hypotheses and evidence independently before seeing one another’s reasoning where practical. Independence may be diversified by:

- model;
- harness;
- source;
- strategy;
- initial hypothesis;
- role.

### 10.4 Known Unknowns

Unresolved questions are durable records including:

- question;
- hypotheses/confidences;
- missing evidence;
- attempted verification;
- next verification opportunity.

New evidence can reactivate them.

---

## 11. Autonomous Goal Generation and Accountability

### 11.1 Gap-driven goals

With an active Mission:

```text
Mission
→ Desired State ↔ Actual State
→ Gap
→ Goal
→ Task Graph
```

When cause is unknown, the Collective should first create a cognitive goal to understand the cause rather than randomly optimizing.

### 11.2 Goal Market

Candidate goals compete for resources based on multiple dimensions, including:

- Mission value;
- probability of success;
- urgency;
- confidence;
- cost;
- risk;
- opportunity cost.

This is not required to be reduced to one scalar score.

### 11.3 Consequential goal review

Large or novel autonomous goals may be reviewed by independent roles such as:

- Mission Critic;
- Risk Critic;
- Economic Critic;
- Novelty/Scope Critic.

These are tasks/roles, not privileged permanent agents.

### 11.4 Adversarial accountability

Every consequential autonomous decision creates an independent obligation to determine whether it was actually good.

For a decision/task `XYZ`, the Collective may create:

- `XYZ`: execute the work;
- `Evaluation XYZ`: independently attempt to prove whether `XYZ` succeeded or failed.

Evaluation should distinguish technical execution, operational outcome, business outcome, and Mission outcome.

Predictions, confidence, expected cost, and expected outcome should be compared with actual results for calibration.

The Devil’s Advocate seeks the strongest credible counterargument or failure mode, not disagreement for its own sake.

---

## 12. Process Improvement, Apprenticeship, and Capability Compilation

### 12.1 Object work versus meta-work

Object work changes the external objective: fix bug, deploy, contact customer.

Meta-work improves how work is performed: QA process, code review, scheduling, deployment, reasoning strategy.

Meta-work must justify expected authorized value and must not recurse indefinitely.

### 12.2 Process learning

A process-improvement cycle may be:

```text
outcome pattern
→ process weakness
→ improvement hypothesis
→ adversarial review
→ controlled experiment
→ independent evaluation
→ adopt or rollback
→ durable lesson
```

Processes are versioned with rationale, evidence, expected effect, outcome, and rollback conditions.

### 12.3 Apprenticeship

The Collective is not assumed to be born with complete organizational knowledge.

Human correction is valuable training material. A denial without rationale is much less informative than a denial with “because…”. A single correction should initially produce a hypothesis, not an eternal rule.

Owner decisions are not epistemically infallible; later outcomes may show that the Collective’s original proposal was superior.

### 12.4 Self-distrust

The Collective should maintain a temporal model of its own performance and calibration by domain. If autonomous performance degrades, it may voluntarily request more human challenge, lower its authority, or change collaboration mode.

Autonomy is not a reward to preserve.

### 12.5 Capability Compilation

Repeated expensive cognition is a candidate for compilation into a reusable deterministic capability.

Typical lifecycle:

```text
repeated work detected
→ automation candidate
→ ROI analysis
→ specification and invariants
→ implementation
→ independent verification
→ shadow execution
→ comparison with old method
→ limited rollout
→ production capability
→ monitoring
```

Automation may reduce cognition cost but may not increase authority.

---

## 13. Resource Economics and Economic Awareness

### 13.1 Cost is broader than invoices

The Collective tracks multiple economic dimensions without pretending they are identical:

- **cash cost** — actual expenditure;
- **marginal cost** — additional cost caused by the action;
- **allocated cost** — attributable share of common infrastructure;
- **opportunity cost** — value displaced by consuming a scarce resource;
- **resource consumption** — tokens, CPU-hours, GPU-hours, GB-hours, network GB, human attention, and other quantities;
- **readiness/carrying cost** — cost of remaining able to act;
- **wake cost and latency**;
- **reconstruction cost/time/confidence**;
- **risk exposure / expected loss** as a separate dimension.

No invoice does not mean no cost. A personally owned laptop may generate no invoice but still consume electricity, hardware life, capacity, and opportunity.

### 13.2 Resource Ledger

Resource attribution should reach at least the level of a Meeseek Attempt where useful, while allowing aggregation when measurement overhead would exceed the value of precision.

A Goal can accumulate the costs of planning, failed attempts, execution, verification, external services, data transfer, and human attention.

### 13.3 Readiness economics

Low utilization is not proof of waste when readiness itself has value.

For Cubes and capabilities, the Collective may track:

- fixed/persistent cost;
- utilization;
- unique capabilities;
- wake latency;
- wake cost;
- reconstruction cost and time;
- reconstruction confidence;
- replacement options;
- data/capability loss risk;
- dependency impact.

A cheap but irreplaceable Cube can be more valuable to keep than an expensive fully reproducible one.

### 13.4 Active, idle, cold, terminated

Resources may conceptually move through states such as:

```text
ACTIVE → IDLE → SUSPENDED/COLD → TERMINATED
```

Strategic Pulse may recommend or perform transitions when authority allows.

Reducing reconstruction cost through IaC, containerization, bootstrapping, or tested recovery procedures may be more valuable than reducing steady-state operating cost.

### 13.5 Risk exposure

Risk must not be silently collapsed into direct task cost. A cheap action with a small chance of catastrophic loss may be economically inferior to a more expensive safe action.

Risk records may contain ranges and confidence rather than fake precision.

### 13.6 Information quality and Value of Information

Economic values may be:

- `MEASURED`
- `DERIVED`
- `ESTIMATED`
- `ASSUMED`
- `UNKNOWN`

Unknown is valid. The Collective should not continuously ask the Owner for precision that cannot change a decision.

Information acquisition follows the principle:

> Know what matters, estimate what is sufficient, and ask only when better information can materially change the decision.

Possible acquisition modes include observing, deriving, estimating, experimenting, and asking. Their order is chosen by expected Value of Information relative to acquisition cost and risk.

### 13.7 Gross revenue is not execution budget

Revenue must be distinguished from spendable execution budget. Taxes, fees, required reserves, commitments, and required margin may reduce what can be allocated to cognition or execution.

A deterministic economic capability may compute an allowed execution budget using jurisdiction, legal entity, tax regime, payment type, contractual terms, reserves, and policy.

### 13.8 Accounting knowledge versus spend authority

The Collective must separate **what it knows about actual cost** from **what it is still exposed or authorized to spend**.

An exact invoice may be unknown while a hard upper bound is known. Conversely, an executor may stop responding while still potentially accruing cost.

Budget control therefore tracks at least:

- settled cost;
- active reservations;
- unresolved committed exposure;
- enforceable upper bounds;
- remaining authorized budget.

Parent work, child tasks, retries, verification, external operations, and recovery normally draw from a shared authorized pool unless the parent explicitly subdivides that pool. Spawning or retrying cannot create new budget.

A **hard budget** may be claimed only when a technically enforceable upper bound exists—for example provider quota, prepaid credit, bounded reservation, cancellable resource with verified stop semantics, or another enforceable cap. If a provider can continue billing without a bounded maximum after control is lost, the Collective must represent that limitation honestly and use a policy-approved alternative, approval path, or softer budget classification.

---

## 14. Principle of Sufficient Effort and Opportunistic Surplus

### 14.1 Sufficient Effort

Optimization is lexicographic, but **constraints come first**:

1. reject any candidate path that violates higher-level constraints, authority, policy, or required safety invariants;
2. among the remaining legal/authorized paths, determine which are reasonably expected to satisfy the required outcome;
3. among sufficiently effective paths, prefer the least costly, complex, risky, and resource-intensive option.

If no legal/authorized path can satisfy the required outcome, the result is `BLOCKED`, `REQUIRE_APPROVAL`, a capability/authority proposal, or another explicit escalation. The Collective must not achieve the goal by violating a higher-level constraint.

The principle is:

> **Use no more than necessary—but never less than sufficient, and never outside higher-level constraints.**

This applies at every level:

- use a script instead of a frontier LLM when sufficient;
- use one solver instead of twelve when sufficient;
- install a package instead of provisioning a Cube when sufficient;
- use existing evidence instead of researching when sufficient;
- do not consume Owner attention when the Collective can safely decide;
- escalate resources only until expected effectiveness is sufficient;
- escalate authority only through the authorized approval path, never by bypass.

Future cost and option value matter. Spending more now to compile a durable capability may be globally cheaper than repeatedly choosing the locally cheapest option.

### 14.2 Opportunistic Surplus

The required outcome is a floor, not a ceiling.

After satisfying the primary objective, the Collective may capture related extra value if:

- the additional value is independently recognizable;
- marginal cost is low;
- risk does not materially increase;
- the primary deadline is protected;
- protected resources are not consumed;
- authority permits the work;
- higher-value work is not displaced.

This allows context-aware backlog execution: work that is normally expensive may become temporarily cheap because the relevant environment, data, or human interaction is already active.

Missionless Opportunistic Surplus is limited to authorized purposes and obligations; it cannot justify self-generated expansion.

---

## 15. Collective Metabolism and Strategic Pulse

### 15.1 Dormancy is normal

The healthy no-work state is **DORMANT**, not “agents running”.

```text
NO USEFUL WORK
→ DORMANT
→ signal
→ WAKE
→ ORIENT
→ worth acting?
   ├── yes → WORK
   └── no  → SLEEP
```

### 15.2 Basal versus Active Metabolism

**Basal Metabolism** should be cheap and deterministic where possible:

- event listeners;
- timers/schedulers;
- lease management;
- budget counters;
- policy checks;
- operation reconciliation triggers;
- simple thresholds;
- lightweight health checks.

**Active Metabolism** includes:

- Meeseeks;
- LLM reasoning;
- research;
- experiments;
- reviews;
- adversarial evaluation;
- Collective Mind synthesis.

### 15.3 Wake triggers

Wake triggers include:

- customer requests;
- email or repository events;
- production alerts;
- webhooks;
- payments;
- metric thresholds;
- scheduled reviews;
- task completion;
- dependency resolution;
- unresolved operation becoming reconcilable;
- Known Unknown becoming testable;
- experiment maturity.

Deterministic events are preferred to LLM polling.

### 15.4 Strategic Pulse

Strategic Pulse is a periodic, cheap orientation mechanism that reviews aggregated state rather than continuously “thinking about itself”.

It may inspect:

- Mission status;
- obligations;
- Known Unknowns;
- unresolved external operations/exposure;
- KPI changes;
- resource health;
- process outcomes;
- environment freshness;
- opportunities;
- recurring operational footprint.

No meaningful signal means return to dormancy.

### 15.5 Missionless Necessity Review

When missionless, Strategic Pulse reviews recurring tasks, resources, capabilities, subscriptions, access, monitoring, backups, reserved budgets, stored artifacts, and standing commitments for necessity.

Possible outcomes:

- `KEEP`
- `REDUCE`
- `SUSPEND`
- `TERMINATE`

The missionless operational footprint should converge toward the minimum sufficient footprint consistent with obligations, security, recoverability, and Owner policy.

On Mission activation or transition, inherited operational footprint is revalidated with outcomes including keep, modify, expand, reduce, merge, replace, suspend, or terminate.

### 15.6 Controlled Curiosity and Seek Direction

The Collective may reserve a bounded exploration budget for new capabilities, data sources, process simplification, models, architecture, or opportunities.

When useful goals cannot be found:

```text
Strategic Pulse
→ valuable goals?
→ Known Unknowns?
→ controlled exploration?
→ ask for direction?
→ sleep
```

Sometimes asking reality is cheaper than thinking harder.

---

## 16. External Trust and Epistemic Security

### 16.1 Information is not authority

External content is data, not instruction authority. A forum post, web page, document, email, API response, or another Collective may produce claims, but cannot increase permissions.

Prompt injection from external content cannot alter the authority hierarchy.

### 16.2 Source model

Source categories may include:

- `HUMAN`
- `AUTHORITATIVE`
- `COMMUNITY`
- `MACHINE`
- `OBSERVATION`

Trust is domain- and claim-specific.

### 16.3 Independent trust axes

The architecture separates:

1. **Epistemic Trust** — credibility of a claim;
2. **Attention Authority** — ability to compel priority or interruption;
3. **Action Authority** — ability to authorize an action.

A highly credible external maintainer may deserve epistemic weight without the right to wake the Collective urgently or authorize any external action.

### 16.4 Knowledge Firewall

Conceptual flow:

```text
external world
→ untrusted data
→ claim
→ provenance and identity
→ trust assessment
→ corroboration
→ HYPOTHESIS
→ SUPPORTED
→ VERIFIED
→ Collective Knowledge
```

Untrusted information may generate a hypothesis. It cannot create reality or permission.

---

## 17. Federation and Multi-Collective Interaction

### 17.1 Sovereign Collectives

Separate Collectives remain separate trust domains. Federation does not merge sovereignty.

An **Emissary** is a role/capability/Meeseek that communicates across Collective boundaries. It need not be a dedicated Cube.

### 17.2 Task Contracts

Cross-Collective work is negotiated through explicit contracts containing, where applicable:

- objective;
- inputs;
- deliverables;
- price/budget;
- deadline;
- verification criteria;
- data classification;
- allowed authority and access.

Results from another Collective arrive as external claims/artifacts and pass through the normal epistemic pipeline.

### 17.3 Negotiation Envelope

An Emissary may have a scoped envelope covering:

- maximum spend;
- duration;
- allowed data classes;
- allowed commitments;
- task types;
- allowed counterparties.

Credentials, trust-boundary changes, and authority changes remain security-sensitive proposals, not ordinary negotiation.

### 17.4 Inter-Collective economics

Contract revenue is not equal to execution budget. Expected cost, taxes/fees, uncertainty, rework, reserves, required margin, and strategic value must be considered before accepting or pricing work.

Make-or-buy decisions are valid: another Collective may be the economically superior capability provider.

---

## 18. Swarms

A Swarm is an optional resource-control mechanism for a large task subgraph.

Possible constraints include:

- maximum concurrency;
- maximum share of Collective capacity;
- token/API/compute budget;
- wall-clock budget;
- cognition tier;
- deadline;
- priority.

A **cap** is a maximum, not a reservation. A **reservation** explicitly guarantees a minimum.

Emergency Swarms may receive a reservation and throttle lower-value work when policy permits.

A Swarm has no separate Mission, Constitution, memory, authority hierarchy, or sovereignty.

A Collective may operate indefinitely without ever creating a Swarm.

---

## 19. Capability Model and Discovery

### 19.1 Semantic capability

A capability is a semantic ability, not the presence of an executable.

Conceptually:

> **Capability = Skill × Access × Authority × Environment**

This is a completeness model, not literal arithmetic.

Finding `python`, `git`, `docker`, `az`, `terraform`, `codex`, or `claude` proves only primitive tooling. A semantic capability may also require credentials, network locality, procedure knowledge, and a tested workflow.

### 19.2 Capability Assessment

When a Box starts or its environment changes, it may safely assess capabilities in sandboxed/zero-authority modes.

The Capability Registry tracks more than booleans. It may include:

- implementations;
- eligible Cubes;
- quality history;
- cost history;
- latency history;
- limitations;
- freshness of assessment;
- enforcement mode for authority-sensitive effects.

### 19.3 Capability gaps

When a capability is missing, the Collective may choose among:

- `BUY`
- `BORROW`
- `LEARN`
- `BUILD`
- `IMPROVISE`
- `ASK`
- `DECLINE`

### 19.4 Least-escalating acquisition

If authority permits capability acquisition, the Collective should choose the smallest sufficient change:

1. reconfigure;
2. install/enable or remove a conflicting tool;
3. change membership/access;
4. attach an existing resource;
5. modify environment/network;
6. provision new capacity/Cube;
7. create new trust material.

Need does not create authority. Capability acquisition may consume existing authority but cannot manufacture new authority.

### 19.5 Locality and access

Scheduling must distinguish:

1. compute/skill capability;
2. access/locality capability;
3. authority capability.

A Cube may have network access and credentials to a resource while policy still denies an operation.

Tasks are scheduled to capabilities instantiated on machines with specific access contexts—not to machine names alone.

---

## 20. Scheduling and Resource Arbitration

### 20.1 Eligibility before arbitration

Before deciding priority, the scheduler filters for feasibility:

```text
Task
→ capability match
→ access/locality match
→ authority check
→ enforcement-path check
→ Swarm/resource envelope
→ budget
→ eligible execution paths
```

A path that cannot technically enforce the required authority boundary is not eligible for consequential work merely because the scheduler conceptually granted a logical lease.

### 20.2 Multi-dimensional scheduling pressure

The scheduler considers dimensions such as:

- authorized purpose/Mission value;
- urgency and cost of delay;
- dependency value;
- economic value;
- risk if delayed;
- resource fit;
- context/opportunity value;
- capability scarcity;
- starvation review.

The system does not require one magic priority score.

### 20.3 Capability scarcity

Scarce specialized Cubes should preferentially run tasks that need their unique access or capabilities while generic workloads use interchangeable capacity when possible.

This is not a requirement to leave specialized resources idle when no specialized work exists.

### 20.4 Preemption

Preemption considers restart/checkpoint cost:

- near-complete expensive work may finish;
- long checkpointable work may pause;
- cheap stateless work may terminate.

Preemption or lease loss does not by itself prove that previously dispatched External Operations stopped or incurred no further cost.

### 20.5 Backlog aging

Old work does not automatically earn execution. A long-waiting task triggers review:

- do now;
- defer;
- reprice;
- merge;
- automate;
- mark obsolete;
- delete.

### 20.6 Time-aware scheduling

Task time semantics may include:

- `earliest_start`;
- `must_start_by`;
- `feedback_by`;
- `deadline`;
- `expires_at`;
- `expected_duration`;
- `required_inputs_ready_at`;
- `schedule_type`;
- `stale_after`.

Task classes may include immediate, deadline-bound, windowed, recurring/scheduled, and opportunistic.

A future deadline casts a **scheduling shadow**: the scheduler can avoid starting work that would block scarce capacity needed soon.

Latest safe start should include duration uncertainty, verification, retries, and delivery buffers.

Scheduled work remains in scheduler awareness without consuming active cognition until useful.

### 20.7 Elastic parallelism

Tasks may declare or learn:

- minimum parallelism;
- preferred parallelism;
- maximum effective parallelism;
- parallelizable fraction;
- coordination cost.

Additional Meeseeks are added while marginal value exceeds marginal cost and coordination overhead.

Parallelism may buy throughput, lower latency, or epistemic diversity.

### 20.8 Deterministic core, cognitive edge

The scheduler mechanism is deterministic. Planning can be cognitive when decomposition, resource conflict, or schedule feasibility is unclear.

A normal internal `scheduling_analysis` task may propose a plan, but the deterministic scheduler validates all constraints before execution.

Repeated planning patterns may later be compiled into deterministic heuristics or solvers.

---

## 21. Task Lifecycle, Attempt Lifecycle, Failure, Verification, and Challenge

### 21.1 Separate durable lifecycles

A **Task** and an **Attempt** have separate lifecycles.

A Task may move through states such as:

```text
CREATED
→ ELIGIBLE
→ EXECUTING
→ AWAITING_VERIFICATION
→ SUCCEEDED | FAILED | BLOCKED | EXPIRED | CANCELLED | CHALLENGED
```

An Attempt may move through states such as:

```text
CREATED
→ LEASED
→ RUNNING
→ COMPLETED | FAILED | BLOCKED | EXPIRED | CANCELLED | CHALLENGED
```

Attempt completion means execution output exists. It does not mean the Task has met its acceptance criteria.

### 21.2 Challenge is legitimate

A Meeseek may conclude that the task, parent, goal, or Mission assumption is incorrect.

Challenge scope may include:

- `TASK`
- `PARENT`
- `GOAL`
- `MISSION_ASSUMPTION`

A challenge supplies evidence, reasoning, and a proposed alternative. It does not directly rewrite Mission.

Dependent work may pause when the cost/risk of continuing exceeds the cost of waiting for challenge resolution.

### 21.3 Failure classification

Failures should be classified where possible:

- transient;
- capability;
- epistemic;
- planning;
- resource;
- authority;
- impossible objective;
- genuine execution failure.

Responses differ by class: retry/backoff, reassignment, capability gap, replan, Known Unknown, authority proposal, goal challenge, or diagnosis.

Blind retry loops are prohibited as a strategy. Each retry should have a reason the next Attempt has a greater chance of success.

Unknown external effect is not classified as an ordinary execution failure; it follows External Operation reconciliation semantics.

### 21.4 Attempt records

Attempts should preserve:

- `attempt_id`;
- task relationship;
- lease/fencing generation;
- approach;
- executor;
- capabilities used;
- evidence;
- cost/resource usage;
- duration;
- failure class/signature;
- checkpoint/output references;
- related External Operation IDs.

### 21.5 Durable verification and success semantics

A worker completes an **Attempt**. It does not automatically define Task success.

When an Attempt completes successfully enough to be evaluated:

1. its output/evidence is durably linked to the Task;
2. the Task transitions idempotently to `AWAITING_VERIFICATION`;
3. verification becomes durable work/state;
4. after acceptance, the Task transitions to `SUCCEEDED` and records the accepted Attempt, evidence, and acceptance record.

`SUCCEEDED` means the Collective has sufficient evidence that acceptance criteria were met.

Acceptance may be deterministic, independently verified, or Owner-approved according to risk and cognition policy.

Dependencies that require a successful Task do not unlock on mere Attempt completion. A specialized dependency may explicitly consume unverified Attempt output, but that exception must be declared rather than implied.

If the Box crashes after Attempt output is persisted but before verification runs, restart must resume verification from `AWAITING_VERIFICATION` rather than rerunning the successful Attempt unnecessarily.

Verification failure may produce a new Attempt, replan, challenge, remediation, or final failure according to policy. Historical Attempts are not overwritten.

Later evidence may invalidate an accepted outcome without rewriting history. The system records regression or outcome invalidation and creates remediation work.

### 21.6 Checkpointing

Checkpoint effort scales with the cost of lost work. Tiny stateless operations need no elaborate checkpointing; long research, refactors, migrations, or expensive experiments should preserve recoverable progress.

### 21.7 Lease generations and fencing

Every leased Attempt carries a monotonically increasing lease generation or equivalent **fencing token** for that Task execution authority.

Authoritative Task-state mutations, new resource reservations, and consequential External Operation commitments from that Attempt must present a currently valid fencing token at the trusted enforcement boundary.

When a lease expires, is revoked, or is superseded:

- the stale Attempt loses authority to mutate authoritative Task state;
- it cannot create new reservations or new consequential commitments through controlled capabilities;
- late artifacts, logs, or evidence may still be ingested with stale-attempt provenance;
- late evidence cannot by itself mark the Task successful or overwrite the current Attempt state;
- External Operations already dispatched remain governed by their durable operation records and cannot be “undone” by fencing.

Fencing must be enforced by trusted state/commit/capability boundaries, not by relying on the stale worker to cooperate.

---

## 22. Observability, Audit, and Explainability

### 22.1 Causal audit

Consequential decisions should create structured `DecisionRecord`s containing:

- trigger;
- decision;
- alternatives considered;
- evidence available at the time;
- constraints/policy result;
- expected outcome;
- confidence;
- committed resources;
- authority used;
- actor;
- timestamp.

Decision basis classes may include:

- `EVIDENCE_BASED`
- `DETERMINISTIC_RULE`
- `HISTORICAL_PATTERN`
- `MODEL_ESTIMATE`
- `HEURISTIC_JUDGMENT`
- `HUMAN_DIRECTION`

Heuristic judgment is allowed but must be represented as uncertain rather than disguised as evidence.

The system does not need to retain private chain-of-thought. It stores concise structured rationale and evidence sufficient for audit.

### 22.2 Decision-time truth

Explanations must cite information that existed when the decision was made. Post-hoc narratives cannot be substituted for contemporaneous rationale.

Causal audit should answer both:

- Why did this action occur?
- What resulted from this decision?

### 22.3 Observability strategy

Known operational conditions should use deterministic metrics/events/rules where practical:

- missed deadlines;
- lease loss;
- stale-attempt commit rejection;
- retry loops;
- unresolved external operations;
- unresolved spend exposure;
- unexpected cost changes;
- redundancy loss;
- scheduled work failing to start.

Meaningful anomalies create investigation tasks.

Unknown/systemic patterns are reviewed through Strategic Pulse using aggregated health signals rather than a permanent high-cognition health agent.

Repeated useful detections should be candidates for Capability Compilation.

---

## 23. Human Interface and External Interaction

### 23.1 Global scheduling, local dispatch

The Collective Scheduler decides **what and when** at the logical Collective level.

A Box Local Dispatcher decides **how physically** on a Cube: slot/executor placement, process invocation, checkpoint, pause, kill, fencing, and local reporting.

The global scheduler is logical, not a required single process. Durable state and takeover support future distribution.

### 23.2 External capabilities and direct access

External access is capability-governed, not necessarily gateway-routed.

A Meeseek with a valid capability lease may directly use an allowed implementation such as:

- email provider;
- GitHub connector;
- HTTP client;
- cloud API;
- database;
- storage API.

However, **direct** does not mean **unenforced**. A consequential capability path is valid only if its effective authority limits are technically enforced somewhere that the executor cannot simply ignore. Examples include:

- target-service IAM or scoped API permissions;
- a short-lived scoped token that cannot perform disallowed operations;
- an OS/container/process sandbox;
- network policy;
- a Box/Capability Provider that mediates the operation;
- another trustworthy enforcement mechanism.

An untrusted executor must not receive ambient credentials, filesystem access, network reachability, or machine privileges that allow it to reproduce the same protected effect outside the approved capability path.

Policy-governed does not mean centrally routed, but **policy must be technically enforceable somewhere**.

A logical lease with no technical enforcement is descriptive metadata, not a security boundary.

### 23.3 Interaction Brokers

Brokers exist only where brokering adds value—for example scarce human attention, aggregation, protection, rate limiting, shared transactional resources, or enforcement of a protected effect.

The rule is: **broker only what benefits from brokering**.

### 23.4 Intention-level scheduling

The scheduler should arbitrate meaningful units of work rather than every I/O operation.

Sending 1,000 campaign emails may be one authorized scheduled task with a connector implementing the batch. It should not require 1,000 independent scheduling decisions, although the batch still remains subject to the applicable Commit Boundary and operation-record semantics.

### 23.5 Attention Broker

Human attention is a scarce capability/resource.

An attention request may contain:

- target;
- urgency;
- response deadline;
- blocking status;
- topic/context;
- related tasks;
- batchability.

Low-priority questions may be batched into one conversational session. Urgent questions may be pulled out and escalated.

Interrupt only when expected cost of waiting exceeds expected cost of interruption.

---

## 24. Security, Trusted Enforcement Boundary, and Secrets

### 24.1 Task-Scoped Capability Lease

Each Task/Meeseek receives the minimum capability set needed for its work, expiring with the Task or lease where technically possible.

A Cube may possess broad machine-level capabilities while an individual Task sees only a restricted capability lease **only if the restriction is technically enforced**.

Additional capability needs require a new Policy Engine decision.

### 24.2 Trusted Enforcement Boundary

The design assumes executors/harnesses may be buggy, compromised, prompt-injected, or intentionally adversarial. Therefore normal security guarantees rely on a small **Trusted Enforcement Boundary (TEB)** that executors cannot redefine through prompts or ordinary Task output.

The conceptual TEB includes the mechanisms that enforce:

- Constitution/Policy decisions relevant to action authorization;
- principal and Cube identity;
- lease generation/fencing validation;
- capability scoping and credential release;
- Commit Boundary checks;
- protected budget/reservation state;
- durable operation identity/deduplication state;
- authoritative Task/Attempt state transitions;
- isolation controls needed to prevent bypass of equivalent protected effects.

The implementation technology is deferred, but the security property is not: an executor with shell/network access must not simultaneously possess ambient paths that let it bypass the TEB for effects that the architecture claims to govern.

A capability can still be invoked directly against a target service when the target service or scoped credential itself supplies the trusted enforcement.

### 24.3 Prefer operation over secret

The preferred security model is:

> Grant an operation rather than reveal a credential.

The Box or Capability Provider should hold credentials outside the Meeseek when possible and perform the operation on its behalf.

When an executor genuinely needs a credential, use the shortest-lived, narrowest-scope token feasible, constrained so that its real permissions do not exceed the granted capability envelope where practical.

Secrets do not belong in the Knowledge Graph.

### 24.4 High-risk Owner grants and guarantee downgrade

The Owner may deliberately grant broad/root access for a bounded Task or test. Such a grant is:

- explicitly high risk;
- task/time scoped where possible;
- strongly audited;
- not a precedent;
- not a policy change;
- not evidence of earned authority.

If the grant gives an executor a path that can bypass normal enforcement—for example root access plus ambient credentials/network access—the runtime must mark this as an explicit **guarantee downgrade**. It must not claim that normal invariants such as non-bypassable least privilege, Commit Boundary enforcement, write/spend prevention, or stale-lease effect prevention remain technically guaranteed for that executor during the downgrade.

The downgrade records scope, start/end conditions, affected guarantees, approving principal, and recovery/verification requirements. This supports deliberate “Rogue King” testing without lying about the trust model.

### 24.5 Revocation and compromise containment

Revocation should be faster and easier than granting.

A suspected Cube may have leases/capabilities revoked and credentials rotated before quarantine.

Each Cube has a unique identity. Compromise of a Cube is assumed to expose everything legitimately accessible to that Cube, but must not automatically compromise the entire Collective.

A self-reported cleanup from the compromised trust boundary is not sufficient proof of safety.

---

## 25. Upgrades, Self-Modification, and Evolution

Self-modification is an ordinary authority domain, not a special entitlement.

Early Collectives may autonomously change low-risk prompts/skills while requiring approval for runtime, scheduler, Box, policy-sensitive, trust-boundary, or authority-semantic changes. Later authority may expand through demonstrated performance and explicit Owner grants.

Changes are classified by **semantic effect**, not merely file path. Categories may include:

- performance;
- bugfix;
- feature;
- process;
- security;
- trust boundary;
- authority semantics;
- audit;
- recovery;
- constitutional.

A library change that changes authority behavior is authority-sensitive even if it does not edit `PolicyEngine` source code.

Progressive rollout is preferred:

```text
candidate
→ tests
→ independent/adversarial review
→ sandbox
→ canary Cube
→ observe
→ cohort
→ observe
→ full rollout
```

Rollback is required on unacceptable regression.

Constitutional changes remain behind constitutional ceremony regardless of operational self-update authority.

---

## 26. Bootstrap and Birth

### 26.1 Single-Cube bootstrap

Conceptual flow:

```text
install Box
→ meeseek init
→ create Collective identity
→ establish Owner
→ instantiate/sign Constitution
→ initialize durable state
→ optionally activate Mission
→ Capability Assessment
→ Collective alive
```

A fully valid new Collective may contain zero active Meeseeks and immediately enter DORMANT.

### 26.2 Joining a Collective

Conceptual join flow:

```text
meeseek join <collective>
→ mutual authentication
→ membership approval/policy check
→ Cube identity
→ minimum Collective DNA sync
→ Capability Assessment
→ advertise capabilities
→ ACTIVE
```

Cubes can enter through:

- **PULL/JOIN:** a human installs Box and requests membership;
- **PUSH/PROVISION:** the Collective creates/configures capacity when authority permits.

Both paths converge on the same membership and trust protocol. Provisioning origin does not automatically make a Cube more trusted.

---

## 27. Knowledge, Skills, and Capability Lifecycle

### 27.1 Freshness and volatility

Knowledge and capabilities carry freshness semantics appropriate to claim type.

A mathematical invariant may be effectively immutable. An API requirement may have high volatility and need revalidation before consequential use.

Revalidation should be demand-driven rather than continuously checking every historical fact.

A practical rule is:

> Revalidate when the expected cost of being wrong exceeds the expected cost of checking.

### 27.2 Historical preservation versus active attention

The architecture distinguishes:

```text
Historical Record      — durable, potentially very old
Active Knowledge       — current and relevant
Working Context        — tiny task-specific subset
```

The Collective may forget for attention but not for history, except where governed deletion applies.

### 27.3 Contradiction and supersession

A newer claim must not simply overwrite an older one.

Two claims can both be verified if they apply to different validity periods. Claims for the same period may be explicitly contradictory and trigger investigation depending on significance.

This enables faithful historical reconstruction of both external reality and the Collective’s belief state.

---

## 28. Collective Boundaries and Operational Footprint

### 28.1 Authority Boundary versus Operational Footprint

The **Authority Boundary** is declared through Owner/Policy decisions.

The **Operational Footprint** is discovered: resources, dependencies, services, access paths, and environmental constraints the Collective actually uses or affects.

Discovery cannot create authority or ownership.

### 28.2 Resource relationships

Ownership is not binary. A resource relationship may separately describe:

- visibility;
- usage;
- data access;
- configuration rights;
- lifecycle management;
- provisioning;
- deletion;
- credential management;
- cost responsibility;
- operational responsibility;
- security responsibility.

A Collective may use and pay for a resource without having the right to delete it.

### 28.3 Resource/Responsibility Registry

Useful high-level relations include:

- `OWNED`
- `MANAGED`
- `SHARED`
- `DEPENDENCY`
- `OBSERVED`
- `EXTERNAL`
- `UNKNOWN`

The graph may reveal hidden chains such as a critical capability depending on a manually configured router. Understanding that dependency does not authorize the Collective to administer the router.

### 28.4 Minimum necessary awareness

Observation itself has cost and privacy/security implications. The Collective should observe only what is justified by authorized purpose, obligations, maintenance, governance, or recovery.

It may need to know available RAM on a shared laptop without needing to know which websites the Owner has open.

---

## 29. Commitments, External Operations, Transactions, and Irreversible Actions

### 29.1 Intent to effect

Consequential external work should distinguish:

```text
INTENT
→ PLAN
→ PREPARED OPERATION
→ DISPATCH
→ EFFECT / NO EFFECT / OUTCOME UNKNOWN
```

An internal decision is not the same as an external obligation or completed world-state change.

### 29.2 Deterministic Commit Boundary

Consequential external actions pass through a deterministic Commit Boundary immediately before a new operation is authorized for dispatch.

The boundary rechecks relevant dimensions including:

- current lease/fencing generation;
- authority;
- policy;
- technically enforceable capability path;
- budget/resource reservation;
- unresolved exposure from related Attempts/Operations;
- freshness of material assumptions;
- idempotency/deduplication;
- current conflicting commitments.

The purpose is not to ask the LLM “are you sure?” but to enforce runtime invariants.

The Commit Boundary must live in, or rely on, the Trusted Enforcement Boundary. An executor path that can bypass it cannot be advertised as providing normal Commit Boundary guarantees.

### 29.3 Durable External Operation Record

Before dispatching a consequential External Operation, the Collective durably records an operation identity and intent. A record should include enough information to reconcile the world later, such as:

- stable `operation_id` and idempotency key where supported;
- Task ID and Attempt ID;
- current fencing generation;
- capability/action type;
- target identity;
- digest or normalized description of material parameters/effect;
- authority/policy decision reference;
- reservation/exposure references;
- dispatch state and timestamps;
- provider-side operation/reference ID when available;
- evidence and reconciliation history.

A conceptual operation lifecycle is:

```text
PREPARED
→ DISPATCHED
→ CONFIRMED_EFFECT | CONFIRMED_NO_EFFECT | OUTCOME_UNKNOWN
OUTCOME_UNKNOWN → RECONCILING
RECONCILING → CONFIRMED_EFFECT | CONFIRMED_NO_EFFECT | OUTCOME_UNKNOWN
```

Compensating work, when possible, is a new explicit operation linked to the original rather than history being rewritten.

### 29.4 Unknown outcome and reconciliation

If dispatch may have reached the external system but acknowledgement/evidence is lost, the operation becomes `OUTCOME_UNKNOWN`.

The Collective then attempts reconciliation using available mechanisms such as:

- provider idempotency/operation key lookup;
- provider ledger/status API;
- target-state inspection;
- billing/transaction records;
- independent evidence;
- Owner or counterpart confirmation when justified.

For a consequential **non-idempotent** operation, automatic re-dispatch is prohibited while outcome remains unknown unless policy defines a specific safe duplicate-handling mechanism.

The rule is:

> **Retry execution freely only where effects are idempotent or proven absent; uncertainty about an external effect is state, not failure.**

If the outcome cannot be reconciled safely, the Task becomes blocked on reconciliation/decision rather than creating another potentially duplicate irreversible effect.

### 29.5 Reservations and unresolved exposure

Shared scarce resources must support reservations where double-spend is possible. Examples include:

- budget;
- quota;
- inventory;
- scarce capability slots;
- human-attention slots;
- exclusive infrastructure operations.

Reservation/exposure states may include:

- `HELD` — reserved for an operation/Attempt not yet settled;
- `SETTLED` — converted to known or bounded actual consumption;
- `RELEASED` — proven no longer needed/consumable;
- `UNRESOLVED` — operation or executor may still incur cost/effect and exposure cannot yet be safely released.

A timeout, lost heartbeat, lease expiry, or lost acknowledgement does **not** automatically release a financial/resource reservation if the remote process or provider may still consume it.

Retries, child Tasks, verification, and replacement Attempts consume the same authorized budget pool unless an explicit subdivision says otherwise. A new retry must account for unresolved prior exposure before a new reservation can be granted.

Hard spend limits require enforceable upper bounds. Unknown exact cost is acceptable; unbounded unknown exposure is not equivalent to a hard budget.

### 29.6 Reversibility classes

Consequential actions may be classified as:

- `REVERSIBLE`
- `COMPENSATABLE`
- `COSTLY_TO_REVERSE`
- `PRACTICALLY_IRREVERSIBLE`
- `IRREVERSIBLE`

More irreversible actions require stronger freshness, verification, cognition, and—where policy says so—approval.

Earned Authority may permit autonomous irreversible work, but it does not remove the Commit Boundary or unknown-outcome semantics.

---

## 30. Consistency and Distributed State

### 30.1 Three consistency classes

#### Class A — Authority Critical

Examples:

- Constitution version;
- Owner identity;
- policy and authority grants;
- revocations;
- trust/membership changes;
- quarantine state;
- high-consequence reservations and spend ceilings.

When sufficiently current authority cannot be proven, fail closed for actions that require that proof.

#### Class B — Execution Critical

Examples:

- Task leases and fencing generations;
- authoritative Task/Attempt state;
- exclusive resource reservations;
- unresolved exposure state;
- External Operation records and deduplication keys;
- checkpoints;
- scheduled deadlines.

Normal operation coordinates these strongly enough that stale Attempts cannot commit authoritative state or protected effects. Every state/commit path that relies on a lease must validate its fencing generation at the trusted boundary.

Partition operation may use explicitly partition-scoped semantics with local journals and restricted authority.

#### Class C — Knowledge and Analytics

Examples:

- observations;
- hypotheses;
- metrics;
- local discoveries;
- lessons;
- historical telemetry.

These may be eventually consistent when temporary disagreement merely delays shared understanding rather than violating authority or exclusivity.

### 30.2 Core consistency principle

> **Strong consistency where disagreement could violate authority, commitments, or exclusivity. Eventual consistency where disagreement merely delays shared understanding.**

The architecture should not require Raft/Paxos-style consensus for every knowledge write.

### 30.3 Partition-scoped execution

An island may execute new local work under its last trusted envelope and must record `partition_id` and causal lineage.

It cannot pretend to possess global exclusivity when that cannot be established.

Dangerous global commitments may therefore be denied during partition unless a pre-authorized partition-safe mechanism exists.

### 30.4 Reconciliation

Reconnect is branch reconciliation, conceptually closer to Git than last-write-wins replication:

```text
shared checkpoint
→ branch/island journals
→ causal comparison
→ safe auto-merge
→ semantic conflict detection
→ resolve / investigate / human review
```

Duplicate completion may represent redundant verification rather than corruption. Reconciliation therefore considers Task semantics and evidence, not timestamps alone.

Unresolved External Operations are reconciled against external reality before duplicate consequential effects are authorized.

### 30.5 Collective Epoch

Major trust-state transitions may increment a `Collective Epoch`, including Constitution changes, trust-root changes, membership resets, major policy transitions, and disaster recovery.

A Cube reconnecting from an older epoch must revalidate authority and state before returning to normal participation.

---

## 31. Extension Architecture and Stable Contracts

### 31.1 Principle

> **Core owns semantics. Extensions provide implementations.**

> **An extension may implement power, but it never defines permission.**

### 31.2 Core contracts

The design expects stable semantic contracts resembling:

- `Executor`
- `CapabilityProvider`
- `CollectiveState` / `DurableWork`
- `CollectiveMemory`
- `EvidenceStore`
- `PolicyEngine`
- `IdentityProvider`
- `InfrastructureProvider`
- `Scheduler`
- `EventSource`
- `InteractionProvider`
- `CostProvider`
- `OperationReconciler`

Exact API shapes are intentionally deferred to implementation design.

### 31.3 Executor abstraction

An executor contract should support semantics such as:

- execute;
- checkpoint;
- cancel;
- usage reporting;
- evidence/artifact output;
- health/capability assessment.

Possible implementations include Claude Code, Codex, Gemini, Python, shell, Terraform, and HTTP execution.

The scheduler selects semantic capabilities, not brands.

### 31.4 Local or remote implementations

Extensions may be implemented as:

- in-process libraries;
- local daemons;
- CLI adapters;
- containers;
- remote services;
- another Collective via federation.

Contracts must define:

- timeouts;
- cancellation semantics and whether cancellation is provable;
- idempotency expectations;
- uncertain-outcome/reconciliation support for consequential operations;
- cost/usage reporting;
- whether a hard cost ceiling is technically enforceable;
- security/enforcement context;
- capability assessment.

A remote executor that may continue running or billing after local timeout must expose that uncertainty to the Resource Ledger/reservation model rather than being treated as stopped.

### 31.5 Least privilege for extensions

Extensions receive only the Task data, allowed artifacts, scoped context, and capability lease they need. They do not automatically receive all Memory, all Tasks, all secrets, or Policy Store access.

For authority-sensitive effects, least privilege must be technical rather than merely descriptive.

---

## 32. Minimal Viable Collective (MVC)

### 32.1 Chosen approach: semantic vertical slice

Three implementation strategies were considered conceptually:

- **Demo-first:** quickly show multiple agents, but risks faking the architecture;
- **Infrastructure-first:** build distributed resilience first, but risks spending months without useful work;
- **Semantic vertical slice:** implement one Cube with the real lifecycle end to end.

The chosen strategy is the semantic vertical slice.

### 32.2 MVC topology

```text
1 Collective
1 Owner
1 Cube
1 Box
1 deterministic scheduler
0..N Meeseeks
local durable state
```

No Kubernetes, blockchain, distributed consensus, or custom LLM runtime is required.

### 32.3 MVC end-to-end lifecycle

The first meaningful implementation must support the real semantic path:

```text
Owner
→ Constitution
→ Mission / Obligation / Authorized Purpose
→ Goal
→ Task Graph
→ Scheduler
→ Capability Lease
→ Meeseek
→ Executor
→ Attempt
→ External Operation(s) when needed
→ Verification
→ Knowledge Delta
→ Memory
→ Outcome / Audit / Economics
```

### 32.4 What must be real in MVC

The MVC must include:

1. **Constitution and Authority Model** — model prompts never grant authority.
2. **Trusted Enforcement Boundary** — normal authority/commit guarantees cannot be bypassed by an ordinary executor path; any explicit Owner bypass is represented as a guarantee downgrade.
3. **Durable Task Graph** — work survives Meeseeks and Box restart; Tasks and Attempts are separate, with lineage, child Tasks, deadlines, failure classes, challenge, and acceptance.
4. **Durable Verification State** — completed Attempt output can survive restart in `AWAITING_VERIFICATION`, and Task dependencies unlock only from accepted Task outcomes unless explicitly configured otherwise.
5. **Executor abstraction** — at least two meaningfully different executor types, preferably one agentic harness and one deterministic executor, proving `Meeseek != LLM`.
6. **Capability Registry and Assessment** — the Box discovers and verifies what it can actually do and how authority-sensitive effects are enforced.
7. **Deterministic single-node Scheduler** — Task eligibility, capability matching, deadlines, resource envelopes, leases, and fencing are implemented with future multi-Cube semantics in mind.
8. **Commit Boundary and External Operation Record** — consequential operations cannot bypass deterministic authority/policy/budget/fencing/idempotency checks in normal mode; operation intent is durably recorded before dispatch.
9. **Unknown Outcome and Reconciliation** — the system can represent `OUTCOME_UNKNOWN` and refuses unsafe duplicate non-idempotent re-dispatch while reconciliation is pending.
10. **Budget Reservation and Exposure Semantics** — unresolved remote activity/external operations continue consuming authorized exposure; lease/timeout alone does not free budget.
11. **Memory** — evidence, events, claims, provenance, and temporal semantics are first-class.
12. **Economics** — Attempts and operations can report duration and measurable executor/token/API/compute/network usage; unknown values remain unknown rather than invented.
13. **Audit** — the system can answer why an action occurred, what authority and evidence supported it, what it cost, and what resulted.
14. **Dormancy/wake semantics** — no useful work can correctly mean zero active Meeseeks.

### 32.5 Deferred beyond MVC

The first version deliberately defers full implementations of:

- multi-Cube scheduling and failover;
- Partition Islands and reconciliation between Cubes;
- federation;
- Swarms;
- autonomous infrastructure provisioning;
- advanced Goal Market behavior;
- broad Earned Autonomy;
- autonomous runtime self-update;
- deep archival storage tiers;
- mobile applications;
- advanced Attention Broker UX;
- the full cognition-tier matrix.

Schemas and contracts should carry future identifiers/fields where doing so is cheap and semantically justified—for example `collective_id`, `cube_id`, optional `partition_id`, and optional `swarm_id`—without prematurely implementing the distributed behaviors.

### 32.6 MVC failure-path acceptance scenarios

The MVC is not accepted merely because the happy path works. It must demonstrate at least these semantics on one Cube:

1. **Unknown external result:** a consequential operation is dispatched, the acknowledgement is lost, and the Box restarts. The operation becomes/reconstructs as `OUTCOME_UNKNOWN`; a non-idempotent duplicate is not automatically sent; reconciliation determines or escalates the outcome.
2. **Unresolved billing exposure:** a remote executor times out but may still be running/billing. Its reservation becomes or remains unresolved; a retry cannot overcommit the parent hard budget; exact cost may remain unknown while exposure is bounded or explicitly unresolved.
3. **Stale Attempt:** Attempt A loses its lease, Attempt B receives the next generation, then A resumes. A’s authoritative state mutation and new consequential commit are rejected by fencing; A’s late evidence can still be stored with provenance.
4. **Crash before verification:** an Attempt completes and output is durably stored, then the Box crashes before verification. Restart resumes from `AWAITING_VERIFICATION` without needlessly re-executing the Attempt; downstream success dependencies remain locked until acceptance.
5. **Bypass attempt:** an agentic harness with shell/network attempts to reproduce a protected external effect outside its granted capability. In normal MVC mode, sandbox/credential/network/service enforcement blocks the path. If the Owner deliberately grants a bypass for a test, the runtime records a guarantee downgrade rather than claiming normal guarantees.
6. **Constraint precedence:** a goal is achievable only by violating authority or another higher-level constraint. The result is block/escalation/proposal, not illegal execution.
7. **Hard-budget honesty:** a provider whose cost cannot be technically capped must not be represented as satisfying a hard spend limit merely because an estimated cost exists.

These scenarios are semantic requirements; they do not prescribe a specific database, sandbox, queue, cloud, or programming language.

### 32.7 First real validation scenario

The first useful validation should use a bounded Mission rather than a toy prompt. A suitable scenario is maintaining a selected repository in good health under an explicit budget and without autonomous merge authority.

Example lifecycle:

```text
Strategic Pulse
→ observes a failing test
→ creates Goal
→ creates Task
→ scheduler selects executor
→ Meeseek diagnoses
→ child Task is created
→ deterministic test validates a fix
→ Task waits for verification
→ independent verifier evaluates outcome
→ Knowledge Delta is ingested
→ proposal is surfaced to Owner
→ costs and evidence are recorded
→ Collective returns to DORMANT
```

This exercises the architecture without requiring distributed infrastructure.

### 32.8 Criteria for adding a second Cube

A second Cube should be introduced only to solve an observed need such as:

- capacity;
- locality/access;
- resilience;
- unique capability.

Multi-Cube operation is not a milestone merely because distributed agents are visually impressive.

---

## 33. Cross-Cutting Design Principles

The following concise principles summarize recurring decisions:

- **Kill the Meeseek. Keep the mission.**
- **Tasks are durable; workers are disposable.**
- **Meeseek is a unit of work, not a synonym for LLM.**
- **Task, Attempt, and External Operation are distinct durable concepts.**
- **No lower-level objective may override a higher-level constraint.**
- **LLM is never the root of authority.**
- **Identity ≠ Device.**
- **Distribution is an optimization, not a dependency.**
- **Autonomy Without Sovereignty.**
- **Partition Islands Are Temporary Branches.**
- **Need does not create authority.**
- **Discovery does not create ownership.**
- **Information cannot grant authority.**
- **Truth does not grant permissions.**
- **A logical lease without technical enforcement is not a security boundary.**
- **Direct access is permitted only when the effective authority boundary remains enforceable.**
- **Lease expiry revokes authority for new effects; it does not erase evidence or undo dispatched effects.**
- **Unknown external effect is a first-class state, not permission to retry.**
- **Confidence cannot be inherited without provenance.**
- **Meeseeks inherit knowledge, not context windows.**
- **Spend cognition where uncertainty survives.**
- **Cognition has a cost; reason about the cost of reasoning.**
- **Autonomy is permission, not an objective.**
- **Repeated cognition is a candidate for compilation.**
- **Use no more than necessary—but never less than sufficient, and never outside higher-level constraints.**
- **Exploit valuable context while it is cheap without endangering the primary objective.**
- **Backlog does not earn execution merely by aging.**
- **A future deadline should influence present scheduling before it becomes urgent.**
- **Deterministic core, cognitive edge.**
- **The worker that performs the work does not automatically define whether it succeeded.**
- **Task success is accepted evidence, not Attempt completion.**
- **Observe enough to know when to investigate. Investigate only when investigation has expected value.**
- **Broker only what benefits from brokering.**
- **Prefer granting an operation over revealing a secret.**
- **Plugins may add capability, never authority.**
- **Missionless maintenance must continuously justify its own necessity.**
- **No invoice does not mean no cost.**
- **Idle capacity must justify its readiness cost.**
- **Unknown is a valid economic value. Unnecessary precision is itself a cost.**
- **Unresolved spend exposure remains exposure until proven otherwise.**
- **Strong consistency where disagreement could violate authority, commitments, or exclusivity; eventual consistency where disagreement merely delays shared understanding.**
- **Complexity must earn its existence.**

---

## 34. Architectural Consequences

The design intentionally produces several consequences that should remain visible during implementation:

1. A `Task` is not merely a prompt. It is a durable governed unit with purpose, acceptance criteria, lineage, resource envelope, authority needs, time semantics, Attempts, and accepted outcome.
2. An `Attempt` is not a Task result; it is one execution effort with a lease generation and evidence.
3. An `External Operation` may survive the Attempt that initiated it and must have durable identity, reservation, and reconciliation semantics.
4. A `Meeseek` is not a long-lived identity whose survival should be optimized.
5. Model vendors are replaceable implementation details.
6. Human attention is modeled as a scarce capability/resource, not an infinite interrupt channel.
7. Knowledge is a provenance-aware temporal model, not a vector store containing chat summaries.
8. Economic optimization includes readiness, reconstruction, reservations, and unresolved exposure—not only token invoices.
9. Strong consistency is reserved for places where disagreement can break trust, commitments, exclusivity, fencing, or spend ceilings.
10. Partition autonomy requires intentionally reduced power rather than optimistic authority assumptions.
11. Open-source extensions must never be able to grant themselves permission simply by exposing a capability.
12. A security claim is valid only if the claimed boundary is technically enforced; metadata alone does not constrain a hostile executor.
13. Retry policy must distinguish failed execution from uncertain external effect.
14. The simplest correct implementation is preferred over distributed sophistication until real workload evidence demands more.

---

## 35. Next Design Stage: OSS Landscape Mapping

Technology selection is deliberately postponed until after this conceptual design is approved in written form.

The next stage is a requirements-to-ecosystem mapping using three outcomes:

- **ADOPT** — an existing project sufficiently implements the required semantics and can be used behind a Meeseek contract;
- **ADAPT** — an existing project supplies a strong subsystem that needs an adapter, extension, or constrained semantic layer;
- **BUILD** — the required behavior is core to Meeseek Collective or existing projects do not satisfy the contract.

Previously identified candidates worth evaluating include distributed/graph-memory and durable-execution systems such as OriginTrail DKG, KafGraph, Neo4j Agent Memory, OpenGraphMemory, Caura, AgentLedger, Cognee, Kaeru, MAGI, and Ori Mnemos. These are candidates, not architectural commitments.

The landscape review should map requirements against stable interfaces such as:

- `CollectiveMemory`;
- `DurableWork` / `CollectiveState`;
- `EvidenceStore`;
- `Executor`;
- `CapabilityProvider`;
- `PolicyEngine`;
- `InfrastructureProvider`;
- `IdentityProvider`;
- `Scheduler`;
- `EventSource`;
- `InteractionProvider`;
- `CostProvider`;
- `OperationReconciler`.

The review should pay special attention to whether candidate systems genuinely support, or can safely host, the semantics introduced by the written-spec review: fencing, durable verification, technically enforced capabilities, unknown external outcomes, and unresolved spend exposure.

Only after this mapping should implementation technologies and subsystem boundaries be selected.

---

## 36. Review Gate

This document records the approved conceptual design plus the corrections from the first full written-spec review. It intentionally stops before implementation planning.

The written specification remains awaiting Owner approval after review findings were resolved. Before approval, it should be checked for:

- fidelity to the approved conceptual design;
- missing contradictions or hidden privilege-escalation paths;
- non-enforced authority claims;
- unsafe retry semantics around uncertain external effects;
- lease/fencing ambiguity;
- reservation/exposure ambiguity;
- Task/Attempt/verification lifecycle ambiguity;
- excessive scope in MVC;
- interfaces whose semantics remain ambiguous.

After written-spec approval, the next formal steps are:

```text
Written spec approval
→ OSS landscape mapping: ADOPT / ADAPT / BUILD
→ implementation architecture decisions
→ detailed implementation plan
→ implementation
```
