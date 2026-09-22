# Meeseek Collective

Meeseek Collective is an experimental local-first agent orchestration runtime. The current **Minimal Viable Collective (MVC)** is intentionally one Cube: one trusted Box daemon owns canonical state, policy, authority checks, scheduling semantics, external-operation commit boundaries, verification, memory, audit and resource accounting.

> **Experimental status:** Meeseek Collective is local-first, single-Cube software under active development. It is not production-ready and must not be used for unmanaged consequential workloads. Run it only where you can review its authority, policy, evidence, and containment boundaries.

## Prerequisites

- Go 1.27
- Linux and Docker only when running the optional OCI enforcement gate

## Build and start locally

Build the two binaries explicitly:

```bash
mkdir -p ./bin
go build -o ./bin/meeseek ./cmd/meeseek
go build -o ./bin/meeseek-box ./cmd/meeseek-box
```

Initialize the local Collective once, start the Box in one terminal, then query it from another:

```bash
./bin/meeseek init
./bin/meeseek-box
./bin/meeseek status
```

| Command | Purpose |
| --- | --- |
| `./bin/meeseek init` | Create the local Collective home and initial state. |
| `./bin/meeseek-box` | Start the single local Box daemon. |
| `./bin/meeseek status` | Query the local Box status. |
| `go test ./... -count=1` | Run the normal Go test suite. |

`meeseek init` creates `~/.meeseek` by default, including the local SQLite database, evidence directory, local-development Owner/Cube keys and a `config.json` file with mode `0600`. The config contains a randomly generated 256-bit bearer token used only for the local control transport. Set `MEESEEK_HOME` to select another Collective home. `MEESEEK_CONTROL_ENDPOINT` and `MEESEEK_CONTROL_TOKEN` can override the automatically resolved endpoint/token.

The Box never loads the Owner private key during normal startup. Owner signing material is used by the CLI only when an explicit approval operation is requested.

## What is authoritative

SQLite is the sole MVC source of truth and must live on a **local filesystem**. It runs in WAL mode with `synchronous=FULL`, foreign keys enabled and a busy timeout. The database is not supported on NFS/SMB or as shared multi-Cube state.

Canonical semantics distinguish:

- Task from Attempt;
- Attempt completion from independent Task acceptance;
- PREPARED external operations from the DISPATCHED commitment boundary;
- durable effect-slot identity from mutable intent fingerprints;
- settled cost from unresolved exposure;
- operational telemetry from the canonical audit trail.

The Box rejects stale/revoked/expired Attempts even when their fence value still matches. Unknown external outcomes are reconciled rather than blindly retried, and unresolved exposure continues to consume budget.

## Policy and authority

Bootstrap installs one active, persisted OPA policy profile. Its safe capability profile has no network access and excludes `http.send`. The default policy is deliberately conservative: only LOW-risk consequential actions with valid authority are allowed; everything else is denied. Policy compile/evaluation errors and provenance mismatches fail closed.

The local control API verifies Owner approval signatures with the public key stored in canonical state. Durable approval requests are persisted and exact-bound to their subject and digest. Multi-principal approvals use AND semantics, and current policy/authority is rechecked immediately before consequential dispatch.

## Enforcement levels

Every execution path must describe what it can actually enforce:

- **ENFORCED** — the declared security guarantees are technically enforced. The strong reference profile is Linux/OCI with read-only root, non-root execution, dropped capabilities, no-new-privileges, process/memory/CPU limits, network isolation, workspace-only writes, no ambient credentials and no container socket.
- **PARTIAL** — some controls are technically enforced but the full strong profile is not proven.
- **UNENFORCED** — orchestration semantics still apply, but containment is not claimed.

The native local Box defaults to `UNENFORCED` containment. The Codex adapter defaults to `PARTIAL` and may advertise `ENFORCED` only when its model-egress assessment proves the required mediated egress guarantees.

## Attempt run provenance

Every Attempt leased through the Box runtime gets one immutable, hash-bound Run Manifest in the same SQLite transaction that creates the lease. The manifest is descriptive only: copying it does not recreate authority, extend a lease, mint a capability session or change canonical Task/Attempt state.

For the MVC it snapshots only facts that are actually known at lease time: Task/Attempt identity and fence, executor kind, runtime build metadata when available, policy-set provenance when exposed by the policy engine, the effective TEB profile, the Task's `RequiredCapabilities`, which the current MVC treats as the declared executor-visible capability set at lease time, with any durable capability assessment metadata, and the resource envelope. There is no Context Projection subsystem yet, so no projection hash is invented.

Later output Evidence and settled External Operation cost remain canonical in their existing stores. `runmanifest.Provenance` joins those records for inspection without mutating the immutable lease-time manifest. Executor usage/model/version fields remain empty unless a future canonical source can supply them; the MVC does not fabricate zero/default measurements.

## Field dogfooding and local experience

Field dogfooding is **disabled and LOCAL_ONLY by default**. Raw `FieldObservation` records and candidate free text may contain workload-sensitive context and remain local. The MVC deterministic sanitizer exports a structural allowlist only (closed categories/classes/state tokens, numeric metrics, human-intervention and enforcement flags); free-form behavior text, customer/repository/project identifiers, correlation hashes, and caller-controlled runtime/executor metadata are not serialized into outbound feedback. Only a deterministic, fail-closed sanitizer can produce an immutable `SanitizedFeedback` export artifact; sanitization and authorization are separate gates.

Outbound GitHub feedback is normal governed work: a `COLLECTIVE_MAINTENANCE` Task is leased to the deterministic `feedback-emitter`, which uses the protected External Operation PREPARED → DISPATCHED → reconciliation boundary. It never sends directly from the CLI. GitHub credentials are not stored in `config.json`; set `MEESEEK_FEEDBACK_GITHUB_TOKEN_FILE` to a private regular file (0600 on Unix) only when GitHub export is explicitly enabled.

Local process adaptation requires an explicit, durable, Owner-approved `AdaptationGrant`. Experience may reorder already-eligible executors only; it cannot expand capability, authority, egress, enforcement, policy, approval requirements, or resource ceilings. Promotion and rollback are based on canonical verified outcomes.

This feature does **not** implement source-code self-modification or the future Maintainer Collective. Evidence may create new work; discovery does not authorize a change.

## Current MVC scope

Implemented now: single-Cube local runtime composition, SQLite persistence, Ed25519 principals, Constitution bootstrap, embedded restricted OPA, Task/Attempt fencing and leases, immutable Attempt Run Manifests, evidence and independent verification, resource ledger, protected external-operation lifecycle/reconciliation, durable approvals, capability registry/session primitives, deterministic scheduler/wake semantics, OCI TEB primitives, deterministic and Codex executor adapters, field observations/sanitized governed feedback, evidence-driven local experience, memory/audit, MCP adapter, and authenticated local control/CLI.

Deferred by design: multi-Cube coordination, PostgreSQL, NATS, SPIFFE/SPIRE, Graphiti projection, distributed scheduling, Kubernetes and other infrastructure that is unnecessary for the one-Cube MVC.

## Verification

The normal verification matrix is:

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./cmd/meeseek ./cmd/meeseek-box
python3 docs/superpowers/research/spikes/2026-09-14-sqlite-persistence-spike.py
```

The Linux OCI gate additionally runs the bypass/capability acceptance profile against Docker. CI must not replace that check with a unit-test-only approximation.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for local setup, validation, and pull-request expectations.

## Security

Do not report suspected vulnerabilities in public Issues. See [SECURITY.md](SECURITY.md) for the private reporting route.

## Support

See [SUPPORT.md](SUPPORT.md) for reproducible-defect, security, and conduct-reporting routes.

## License

Meeseek Collective is licensed under the [Apache License 2.0](LICENSE).
