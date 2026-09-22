# Summa42 MVC Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Minimal Viable Collective as a single-Cube semantic vertical slice that preserves the approved Task/Attempt/External Operation, authority, policy, memory, economics, verification, and failure-recovery semantics.

**Architecture:** One Go Box daemon owns authoritative semantics and a local SQLite source of truth. Executors are out-of-process and receive scoped capabilities; embedded OPA evaluates policy under a side-effect-free capability profile; consequential external effects cross a Box-owned PREPARED → DISPATCHED commitment boundary. The implementation starts local-first but keeps identifiers and contracts portable to later multi-Cube/PostgreSQL/NATS/SPIRE evolution.

**Tech Stack:** Go 1.27.1 toolchain; SQLite via `modernc.org/sqlite v1.58.0`; migrations via `github.com/pressly/goose/v3 v3.27.2`; OPA `github.com/open-policy-agent/opa v1.20.2`; MCP Go SDK `github.com/modelcontextprotocol/go-sdk v1.6.1`; Cobra `github.com/spf13/cobra v1.10.2`; Windows named-pipe support `github.com/Microsoft/go-winio v0.6.2`; OpenTelemetry Go `go.opentelemetry.io/otel v1.44.0`; Go standard library for crypto, IDs, HTTP, JSON, process control, and tests.

**Spec:** `docs/superpowers/specs/2026-09-14-summa42-design.md`

**Architecture decisions:** `docs/superpowers/architecture/2026-09-14-mvc-architecture-decisions.md`

**Architecture approval:** `docs/superpowers/architecture/2026-09-15-mvc-architecture-approval.md`

**Spike evidence:** `docs/superpowers/research/2026-09-14-mvc-architecture-spike-findings.md`

## Global Constraints

- Core runtime baseline is Go 1.27; repository toolchain is Go 1.27.1.
- MVC topology is one logical Box daemon plus out-of-process executors.
- SQLite is the sole canonical MVC state store and is local-filesystem only.
- SQLite opens with `journal_mode=WAL`, `synchronous=FULL`, `foreign_keys=ON`, and configured `busy_timeout`.
- Canonical state is relational current state plus append-oriented causal/audit events; MVC is not pure event sourcing.
- AgentLedger is not an authoritative runtime dependency.
- OPA is embedded only as a restricted, side-effect-free policy evaluator; `http.send` and side-effecting custom built-ins are forbidden.
- Any OPA compile/evaluation error, timeout, undefined/ambiguous result, or invalid `PolicyDecision` fails closed for consequential action.
- Fence equality alone never grants current Attempt authority; authoritative mutations additionally require current Attempt identity, ACTIVE/unexpired lease, and allowed Task state.
- Protected external effect identity is a durable Task-semantic effect slot. Parameters and adapter versions produce compatibility fingerprints/revisions, not new identities.
- `PREPARED` is cancelable and has not yet escaped local control. `PREPARED → DISPATCHED` is an atomic, freshly revalidated commitment boundary.
- A durable artifact/file does not prove Attempt completion. Only a canonical `AttemptCompletionRecord` transaction moves a Task to `AWAITING_VERIFICATION`.
- Task success requires independent acceptance; Attempt completion never directly unlocks success-dependent Tasks.
- Unresolved external outcome/cost remains exposure and cannot be silently released.
- A hard budget is only claimed where a technical upper bound is enforceable.
- The Box/TEB owns authoritative persistence writes, credential isolation, capability leases, external-operation dispatch/reconciliation, and budget enforcement.
- Execution paths advertise `ENFORCED`, `PARTIAL`, or `UNENFORCED`; the runtime must never overclaim containment.
- The strong reference execution profile is Linux/OCI-oriented. Native Windows/macOS may run weaker profiles honestly.
- Harnesses never receive Owner/root private keys, direct authoritative SQLite write access, or ambient protected-service credentials.
- MCP is transport/interoperability, never authority.
- OpenTelemetry is operational telemetry, not the authoritative audit log.
- No PostgreSQL, Redis, NATS, Kafka, Graphiti, AgentLedger service, Temporal, or Kubernetes is required to boot MVC.
- Domain packages must not import OPA-, MCP-, Docker-, Graphiti-, AgentLedger-, or cloud-specific types.
- TDD is mandatory for semantic behavior. Every task begins with a failing test and ends with a focused commit.

---

## Locked File Structure

The implementation should converge on the following focused structure. Do not collapse these packages into a single large runtime package.

```text
.github/workflows/ci.yml
.gitignore
Makefile
go.mod
go.sum
README.md

cmd/summa42/main.go
cmd/summa42/root.go
cmd/summa42/init_cmd.go
cmd/summa42/status_cmd.go
cmd/summa42/task_cmd.go
cmd/summa42/approve_cmd.go
cmd/summa42/inspect_cmd.go
cmd/summa42-box/main.go

internal/clock/clock.go
internal/domain/id.go
internal/domain/states.go
internal/domain/task.go
internal/domain/attempt.go
internal/domain/operation.go
internal/domain/policy.go
internal/domain/resources.go
internal/domain/memory.go
internal/domain/errors.go

internal/state/sqlite/store.go
internal/state/sqlite/migrate.go
internal/state/sqlite/migrations/00001_bootstrap.sql
internal/state/sqlite/migrations/00002_purpose.sql
internal/state/sqlite/migrations/00003_execution.sql
internal/state/sqlite/migrations/00004_evidence_acceptance.sql
internal/state/sqlite/migrations/00005_resources.sql
internal/state/sqlite/migrations/00006_operations.sql
internal/state/sqlite/migrations/00007_capabilities.sql
internal/state/sqlite/migrations/00008_memory_audit.sql

internal/identity/signer.go
internal/identity/local_ed25519.go
internal/bootstrap/service.go

internal/policy/engine.go
internal/policy/opa_engine.go
internal/policy/opa_capabilities.go
internal/policy/testdata/base.rego

internal/purpose/service.go
internal/execution/service.go
internal/evidence/store.go
internal/verification/service.go
internal/resources/service.go
internal/operations/service.go
internal/operations/provider.go
internal/operations/reconciler.go
internal/scheduler/service.go
internal/wake/service.go
internal/capabilities/registry.go
internal/capabilities/session.go
internal/teb/profile.go
internal/teb/oci.go
internal/teb/egress_proxy.go
internal/executors/executor.go
internal/executors/command.go
internal/executors/codex.go
internal/memory/service.go
internal/audit/service.go
internal/control/server.go
internal/control/client.go
internal/observability/otel.go

adapters/mcp/server.go
adapters/mcp/tools.go

internal/testutil/clock.go
internal/testutil/store.go
internal/testutil/fake_provider.go
internal/testutil/fake_executor.go

tests/acceptance/failure_semantics_test.go
tests/acceptance/vertical_slice_test.go
```

Dependency direction is:

```text
domain <- core services <- adapters/external libraries
```

---

### Task 1: Bootstrap the Go module, binaries, CI, and deterministic test commands

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `Makefile`
- Create: `.github/workflows/ci.yml`
- Create: `cmd/summa42/main.go`
- Create: `cmd/summa42/root.go`
- Create: `cmd/summa42-box/main.go`
- Create: `internal/version/version.go`
- Test: `internal/version/version_test.go`

**Interfaces:**
- Produces: two buildable entry points, `summa42` and `summa42-box`.
- Produces: canonical commands `make test`, `make test-race`, `make vet`, `make build`.
- No dependency on later domain packages yet.

- [ ] **Step 1: Write a failing version test**

```go
package version

import "testing"

func TestVersionIsDefined(t *testing.T) {
    if Version == "" {
        t.Fatal("Version must be defined")
    }
}
```

- [ ] **Step 2: Run the test and verify RED**

Run: `go test ./internal/version`

Expected: FAIL because package/value does not exist.

- [ ] **Step 3: Create `go.mod` and minimal version package**

`go.mod` starts with:

```go
module github.com/SofiaFlux/summa42

go 1.27.0

toolchain go1.27.1
```

Create:

```go
package version

const Version = "0.0.0-dev"
```

Use Cobra for `summa42`, but keep command wiring thin:

```go
func NewRootCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "summa42",
        Short: "Control a Summa42",
    }
    return cmd
}
```

`cmd/summa42/main.go` calls `Execute()` and exits non-zero on error. `cmd/summa42-box/main.go` initially starts and exits cleanly with a placeholder-free `run(ctx)` function that later tasks extend.

- [ ] **Step 4: Add pinned bootstrap dependencies and CI**

Run:

```bash
go get github.com/spf13/cobra@v1.10.2
go mod tidy
```

CI runs on Linux with Go 1.27.1:

```yaml
steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-go@v6
    with:
      go-version: '1.27.1'
  - run: go test ./...
  - run: go vet ./...
  - run: go build ./cmd/summa42 ./cmd/summa42-box
```

- [ ] **Step 5: Verify GREEN**

Run:

```bash
go test ./...
go vet ./...
go build ./cmd/summa42 ./cmd/summa42-box
```

Expected: all commands exit 0.

- [ ] **Step 6: Commit**

```bash
git add .github .gitignore Makefile go.mod go.sum cmd internal/version
git commit -m "build: bootstrap Summa42 Go module"
```

---

### Task 2: Define stable domain primitives and state machines

**Files:**
- Create: `internal/domain/id.go`
- Create: `internal/domain/states.go`
- Create: `internal/domain/task.go`
- Create: `internal/domain/attempt.go`
- Create: `internal/domain/operation.go`
- Create: `internal/domain/policy.go`
- Create: `internal/domain/resources.go`
- Create: `internal/domain/memory.go`
- Create: `internal/domain/errors.go`
- Create: `internal/clock/clock.go`
- Test: `internal/domain/states_test.go`
- Test: `internal/domain/id_test.go`

**Interfaces:**
- Produces: `domain.ID`, `domain.NewID(prefix string)`, state enums, core record structs.
- Produces: `clock.Clock { Now() time.Time }` and `clock.System`.
- Later services consume these types; these files import only the standard library.

- [ ] **Step 1: Write failing ID/state tests**

```go
func TestNewIDHasPrefixAndEntropy(t *testing.T) {
    a := NewID("task")
    b := NewID("task")
    if a == b || !strings.HasPrefix(string(a), "task_") {
        t.Fatalf("unexpected ids: %q %q", a, b)
    }
}

func TestOperationStateTerminality(t *testing.T) {
    if !OperationConfirmedEffect.Terminal() {
        t.Fatal("confirmed effect must be terminal")
    }
    if OperationPrepared.Terminal() {
        t.Fatal("prepared is not terminal")
    }
}
```

- [ ] **Step 2: Run RED**

Run: `go test ./internal/domain`

Expected: FAIL because types do not exist.

- [ ] **Step 3: Implement IDs with standard-library CSPRNG**

```go
type ID string

func NewID(prefix string) ID {
    var b [16]byte
    if _, err := rand.Read(b[:]); err != nil {
        panic(err)
    }
    return ID(prefix + "_" + hex.EncodeToString(b[:]))
}
```

- [ ] **Step 4: Define exact state enums**

Use these names exactly:

```go
type TaskState string
const (
    TaskCreated              TaskState = "CREATED"
    TaskEligible             TaskState = "ELIGIBLE"
    TaskExecuting            TaskState = "EXECUTING"
    TaskAwaitingVerification TaskState = "AWAITING_VERIFICATION"
    TaskSucceeded            TaskState = "SUCCEEDED"
    TaskFailed               TaskState = "FAILED"
    TaskBlocked              TaskState = "BLOCKED"
    TaskCancelled            TaskState = "CANCELLED"
)

type AttemptState string
const (
    AttemptLeased    AttemptState = "LEASED"
    AttemptRunning   AttemptState = "RUNNING"
    AttemptCompleted AttemptState = "COMPLETED"
    AttemptFailed    AttemptState = "FAILED"
    AttemptCancelled AttemptState = "CANCELLED"
    AttemptExpired   AttemptState = "EXPIRED"
)

type LeaseState string
const (
    LeaseActive  LeaseState = "ACTIVE"
    LeaseRevoked LeaseState = "REVOKED"
    LeaseExpired LeaseState = "EXPIRED"
)

type OperationState string
const (
    OperationPrepared          OperationState = "PREPARED"
    OperationDispatched        OperationState = "DISPATCHED"
    OperationConfirmedEffect   OperationState = "CONFIRMED_EFFECT"
    OperationConfirmedNoEffect OperationState = "CONFIRMED_NO_EFFECT"
    OperationOutcomeUnknown    OperationState = "OUTCOME_UNKNOWN"
    OperationCancelled         OperationState = "CANCELLED"
)
```

Also define `EnforcementLevel`, `PolicyOutcome`, `ReservationState`, `ClaimStatus`, and sentinel errors `ErrStaleAttempt`, `ErrLeaseInactive`, `ErrPolicyDenied`, `ErrIntentConflict`, `ErrBudgetExceeded`, `ErrOutcomeUnknown`.

- [ ] **Step 5: Verify GREEN**

Run: `go test ./internal/domain`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/domain internal/clock
git commit -m "feat: define core Summa42 domain states"
```

---

### Task 3: Add the canonical SQLite store and migration runner

**Files:**
- Create: `internal/state/sqlite/store.go`
- Create: `internal/state/sqlite/migrate.go`
- Create: `internal/state/sqlite/migrations/00001_bootstrap.sql`
- Create: `internal/testutil/store.go`
- Test: `internal/state/sqlite/store_test.go`

**Interfaces:**
- Produces: `sqlite.Open(ctx context.Context, path string) (*Store, error)`.
- Produces: `(*Store).DB() *sql.DB` and `(*Store).WithTx(ctx, fn)` only for infrastructure/core-service packages.
- Store enforces required pragmas on every opened connection.

- [ ] **Step 1: Write failing pragma/migration tests**

```go
func TestOpenAppliesSafetyPragmas(t *testing.T) {
    s := testutil.OpenStore(t)
    assertPragma(t, s.DB(), "journal_mode", "wal")
    assertPragma(t, s.DB(), "synchronous", "2") // FULL
    assertPragma(t, s.DB(), "foreign_keys", "1")
}
```

Also assert `schema_migrations` exists after `Open`.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/state/sqlite`

Expected: FAIL because store does not exist.

- [ ] **Step 3: Add pinned persistence dependencies**

```bash
go get modernc.org/sqlite@v1.58.0
go get github.com/pressly/goose/v3@v3.27.2
go mod tidy
```

- [ ] **Step 4: Implement `Open` with required DSN/PRAGMA behavior**

Core shape:

```go
func Open(ctx context.Context, path string) (*Store, error) {
    db, err := sql.Open("sqlite", path)
    if err != nil { return nil, err }
    db.SetMaxOpenConns(1)
    pragmas := []string{
        "PRAGMA journal_mode=WAL",
        "PRAGMA synchronous=FULL",
        "PRAGMA foreign_keys=ON",
        "PRAGMA busy_timeout=5000",
    }
    for _, q := range pragmas {
        if _, err := db.ExecContext(ctx, q); err != nil { db.Close(); return nil, err }
    }
    if err := migrate(ctx, db); err != nil { db.Close(); return nil, err }
    return &Store{db: db}, nil
}
```

Use `//go:embed migrations/*.sql` and goose's provider/API to apply embedded migrations. Do not shell out to a goose CLI.

- [ ] **Step 5: Verify GREEN and race baseline**

Run:

```bash
go test ./internal/state/sqlite -count=1
go test -race ./internal/state/sqlite -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/state internal/testutil/store.go
git commit -m "feat: add canonical SQLite state store"
```

---

### Task 4: Bootstrap Owner identity, Constitution, and Collective metadata

**Files:**
- Create: `internal/identity/signer.go`
- Create: `internal/identity/local_ed25519.go`
- Create: `internal/bootstrap/service.go`
- Modify: `internal/state/sqlite/migrations/00001_bootstrap.sql`
- Create: `cmd/summa42/init_cmd.go`
- Test: `internal/identity/local_ed25519_test.go`
- Test: `internal/bootstrap/service_test.go`

**Interfaces:**
- Produces: `identity.Signer` with `PrincipalID()`, `PublicKey()`, `Sign()`.
- Produces: `bootstrap.Service.Init(ctx, InitRequest) (InitResult, error)`.
- Persists Collective id, Owner principal, Cube principal, Constitution hash/version, and custody profile.

- [ ] **Step 1: Write failing signer/bootstrap tests**

```go
func TestLocalSignerRoundTrip(t *testing.T) {
    path := filepath.Join(t.TempDir(), "owner.key")
    s, err := NewLocalEd25519(path, "owner")
    if err != nil { t.Fatal(err) }
    sig, err := s.Sign([]byte("hello"))
    if err != nil { t.Fatal(err) }
    if !ed25519.Verify(s.PublicKey(), []byte("hello"), sig) { t.Fatal("bad signature") }
}
```

Bootstrap test must assert a second `Init` refuses to silently replace the Owner/Constitution.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/identity ./internal/bootstrap`

Expected: FAIL.

- [ ] **Step 3: Implement signer and custody semantics**

Use standard-library Ed25519. Serialize private keys as PKCS#8 PEM with filesystem mode `0600` on Unix. Mark the profile explicitly as `LOCAL_DEV_FILE`; never label it hardware-backed. Constitutional root material is generated/read only by explicit ceremony functions and is never loaded by normal Box startup.

```go
type Signer interface {
    PrincipalID() domain.ID
    PublicKey() ed25519.PublicKey
    Sign(message []byte) ([]byte, error)
    CustodyProfile() string
}
```

- [ ] **Step 4: Implement bootstrap transaction**

One transaction creates:

```text
collective_metadata
principals(owner)
principals(cube)
constitutions(version=1, hash, active=1)
```

Store canonical Constitution content hash and signature. Do not put private keys in SQLite.

- [ ] **Step 5: Wire `summa42 init`**

Command accepts `--home`, writes config under that directory, creates DB/evidence directories, generates Owner/Cube keys, and prints created principal ids. Re-running against initialized state must fail with a clear error unless an explicit future recovery command is used.

- [ ] **Step 6: Verify GREEN**

Run:

```bash
go test ./internal/identity ./internal/bootstrap
go run ./cmd/summa42 init --home "$(mktemp -d)"
```

Expected: tests PASS and init exits 0 once.

- [ ] **Step 7: Commit**

```bash
git add internal/identity internal/bootstrap internal/state/sqlite/migrations cmd/summa42
git commit -m "feat: bootstrap Collective identity and constitution"
```

---

### Task 5: Implement restricted, fail-closed embedded OPA policy evaluation

**Files:**
- Create: `internal/policy/engine.go`
- Create: `internal/policy/opa_engine.go`
- Create: `internal/policy/opa_capabilities.go`
- Create: `internal/policy/testdata/base.rego`
- Modify: `internal/state/sqlite/migrations/00001_bootstrap.sql`
- Test: `internal/policy/opa_engine_test.go`

**Interfaces:**
- Produces: `PolicyEngine.Evaluate(ctx context.Context, in PolicyInput) (domain.PolicyDecision, error)`.
- `PolicyDecision` always records policy set id/hash, capability profile hash, input digest, evaluated timestamp.
- Consequential callers treat any returned error as deny/fail-closed.

- [ ] **Step 1: Write the security tests first**

```go
func TestOPARejectsHTTPSend(t *testing.T) {
    module := `package summa42
    decision := http.send({"method":"get","url":"https://example.com"})`
    _, err := newTestEngine(t, module).Evaluate(context.Background(), testInput())
    if err == nil { t.Fatal("http.send must be unavailable") }
}

func TestOPATimeoutFailsClosed(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
    defer cancel()
    _, err := engine.Evaluate(ctx, testInput())
    if err == nil { t.Fatal("timeout must fail closed") }
}
```

Also test undefined output, multiple/ambiguous results, malformed outcome, and valid ALLOW.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/policy`

Expected: FAIL.

- [ ] **Step 3: Add OPA dependency**

```bash
go get github.com/open-policy-agent/opa@v1.20.2
go mod tidy
```

- [ ] **Step 4: Implement an explicit capabilities allowlist**

Build `ast.Capabilities` by filtering OPA's current capabilities to an exact `safeBuiltinNames` set checked into `opa_capabilities.go`. Initial MVC policies only need comparison, boolean, numeric, string, collection, JSON, and deterministic encoding/hash operations. Explicitly omit at least:

```text
http.send
net.lookup_ip_addr
opa.runtime
time.now_ns
rand.intn
```

Do not register custom built-ins in MVC.

Add a unit test that iterates the resulting capability list and asserts the forbidden names are absent.

- [ ] **Step 5: Implement bounded strict evaluation**

Use `rego.Capabilities(caps)`, `rego.StrictBuiltinErrors(true)`, a context deadline (default 250 ms), controlled `rego.Time(in.Now)` rather than ambient time, and JSON shape validation before constructing `PolicyDecision`.

The stable policy fixture returns exactly one object:

```rego
package summa42

default decision := {"outcome": "DENY", "reason_codes": ["default_deny"]}

decision := {"outcome": "ALLOW", "reason_codes": ["low_risk"]} if {
  input.risk == "LOW"
  input.authority_valid == true
}
```

- [ ] **Step 6: Verify GREEN**

Run:

```bash
go test ./internal/policy -count=1
go test -race ./internal/policy -count=1
```

Expected: all policy safety tests PASS.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/policy internal/state/sqlite/migrations
git commit -m "feat: add restricted embedded OPA policy engine"
```

---

### Task 6: Add Mission, Goal, obligation, and authorized-purpose lineage

**Files:**
- Create: `internal/state/sqlite/migrations/00002_purpose.sql`
- Create: `internal/purpose/service.go`
- Modify: `internal/domain/task.go`
- Test: `internal/purpose/service_test.go`

**Interfaces:**
- Produces: `PurposeRef{Kind, ID}` and `purpose.Service` methods `CreateMission`, `CreateGoal`, `CreateObligation`, `ValidatePurpose`.
- A Task cannot be created without a valid purpose in one of the approved classes: MISSION, OBLIGATION, COLLECTIVE_MAINTENANCE, GOVERNANCE, STRATEGIC_PULSE, RECOVERY, OWNER_DIRECTIVE.

- [ ] **Step 1: Write failing lineage tests**

```go
func TestTaskPurposeRejectsUnknownReference(t *testing.T) {
    err := svc.ValidatePurpose(ctx, domain.PurposeRef{Kind: domain.PurposeMission, ID: "mission_missing"})
    if !errors.Is(err, domain.ErrInvalidPurpose) { t.Fatalf("got %v", err) }
}
```

Also test a goal traces to an active Mission and an obligation survives Mission deactivation.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/purpose`

- [ ] **Step 3: Add purpose schema**

Migration creates `missions`, `goals`, `obligations`, and indexes for active Mission and parent Goal relationships. Enforce at most one active Mission with a partial unique index.

- [ ] **Step 4: Implement service methods with transactions**

`CreateGoal` requires either an active Mission or another authorized non-Mission purpose depending on goal class. `DeactivateMission` never deletes obligations.

- [ ] **Step 5: Verify GREEN and commit**

Run: `go test ./internal/purpose -count=1`

```bash
git add internal/purpose internal/domain/task.go internal/state/sqlite/migrations
git commit -m "feat: add authorized purpose lineage"
```

---

### Task 7: Implement Task creation, child inheritance, Attempts, leases, and fencing

**Files:**
- Create: `internal/state/sqlite/migrations/00003_execution.sql`
- Create: `internal/execution/service.go`
- Create: `internal/testutil/clock.go`
- Test: `internal/execution/service_test.go`

**Interfaces:**
- Produces: `execution.Service.CreateTask`, `CreateChildTask`, `StartAttempt`, `RenewLease`, `RevokeLease`, `GuardAttempt`.
- `GuardAttempt(ctx, tx, attemptID, allowedStates...)` is the one reusable authoritative guard for later services.

- [ ] **Step 1: Write the mandatory stale-lease test first**

```go
func TestGuardRejectsExpiredLeaseEvenWhenFenceMatches(t *testing.T) {
    task, attempt := seedActiveAttempt(t, svc, fakeClock)
    fakeClock.Advance(2 * time.Minute)
    err := svc.WithGuardedAttempt(ctx, attempt.ID, []domain.TaskState{domain.TaskExecuting}, func(tx *sql.Tx) error { return nil })
    if !errors.Is(err, domain.ErrLeaseInactive) { t.Fatalf("got %v for task %s", err, task.ID) }
}
```

Also test revoked lease, non-current Attempt, mismatched fence, wrong Task state, and valid current lease.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/execution`

- [ ] **Step 3: Add execution schema**

Tables include `tasks`, `task_dependencies`, `attempts`. Required Attempt columns:

```text
attempt_id PK
task_id FK
fence_generation
lease_state
lease_expires_at
started_at
completed_at
executor_kind
```

Task has `current_attempt_id`, `current_fence`, purpose reference, parent Task, acceptance criteria JSON, required capabilities JSON, priority/time fields, and resource-envelope id.

- [ ] **Step 4: Implement StartAttempt atomically**

Within one transaction:

```text
verify Task ELIGIBLE
increment current_fence
create Attempt LEASED + ACTIVE expiry
set Task EXECUTING/current_attempt_id
append execution event
```

- [ ] **Step 5: Implement the authoritative guard exactly once**

The SQL predicate must verify current Attempt id, fence, `lease_state='ACTIVE'`, `lease_expires_at > clock.Now()`, and allowed Task state. Later operations/completion/capability code must call this guard rather than reimplement weaker checks.

- [ ] **Step 6: Implement child inheritance**

`CreateChildTask` inherits purpose lineage, authority ceiling, and parent resource envelope. It may narrow them but never increase authority or budget. Unit test a child request for greater budget/authority fails.

- [ ] **Step 7: Verify GREEN**

Run:

```bash
go test ./internal/execution -count=1
go test -race ./internal/execution -count=1
```

- [ ] **Step 8: Commit**

```bash
git add internal/execution internal/testutil/clock.go internal/state/sqlite/migrations
git commit -m "feat: add durable tasks attempts leases and fencing"
```

---

### Task 8: Implement content-addressed Evidence, CompletionRecord, and independent acceptance

**Files:**
- Create: `internal/state/sqlite/migrations/00004_evidence_acceptance.sql`
- Create: `internal/evidence/store.go`
- Create: `internal/verification/service.go`
- Test: `internal/evidence/store_test.go`
- Test: `internal/verification/service_test.go`

**Interfaces:**
- Produces: `EvidenceStore.Put(ctx, io.Reader, Metadata) (EvidenceObject, error)`.
- Produces: `verification.Service.CompleteAttempt` and `AcceptTask`.
- CompletionRecord transaction is the sole authoritative discriminator for completed Attempt output.

- [ ] **Step 1: Write the completion-gap crash tests**

```go
func TestStagedBlobDoesNotImplyAttemptCompletion(t *testing.T) {
    ev := putEvidence(t, evidenceStore, []byte("result"))
    restartStore(t)
    got := loadTask(t, taskID)
    if got.State == domain.TaskAwaitingVerification { t.Fatal("orphan evidence inferred completion") }
    _ = ev
}

func TestCompletionRecordResumesVerificationAfterRestart(t *testing.T) {
    completeAttempt(t, svc, attemptID, manifest)
    restartStore(t)
    got := loadTask(t, taskID)
    if got.State != domain.TaskAwaitingVerification { t.Fatalf("got %s", got.State) }
}
```

- [ ] **Step 2: Run RED**

Run: `go test ./internal/evidence ./internal/verification`

- [ ] **Step 3: Implement content-addressed blob writes**

Protocol:

```text
write temp file
fsync temp
SHA-256 verify
atomic rename to blobs/sha256/<prefix>/<hash>
fsync parent directory where supported
insert metadata/reference transactionally when linked
```

An unlinked blob is safe orphan evidence and can be garbage-collected later.

- [ ] **Step 4: Implement `CompleteAttempt` as one guarded DB transaction**

Call `execution.GuardAttempt`; insert unique `attempt_completion_records(attempt_id, manifest_hash, ...)`; set Attempt `COMPLETED`; set Task `AWAITING_VERIFICATION`; create durable verification work/state; append audit event.

- [ ] **Step 5: Implement independent `AcceptTask`**

Acceptance records verifier identity/type, criteria result, evidence ids, timestamp. Only this transaction marks Task `SUCCEEDED` and unlocks success-dependent Tasks.

- [ ] **Step 6: Verify GREEN and commit**

Run: `go test ./internal/evidence ./internal/verification -count=1`

```bash
git add internal/evidence internal/verification internal/state/sqlite/migrations
git commit -m "feat: add durable evidence completion and acceptance"
```

---

### Task 9: Implement Resource Ledger, reservations, unresolved exposure, and hard-cap honesty

**Files:**
- Create: `internal/state/sqlite/migrations/00005_resources.sql`
- Create: `internal/resources/service.go`
- Test: `internal/resources/service_test.go`

**Interfaces:**
- Produces: `Reserve`, `Settle`, `MarkUnresolved`, `Release`, `Available`.
- `Reserve` accepts an enforceability descriptor; hard-budget-required work fails if no technical ceiling exists.

- [ ] **Step 1: Write failing unresolved-exposure tests**

```go
func TestUnresolvedExposureStillConsumesBudget(t *testing.T) {
    r := reserve(t, svc, 70)
    markUnresolved(t, svc, r.ID)
    if _, err := svc.Reserve(ctx, envelopeID, 40, hardCapped()); !errors.Is(err, domain.ErrBudgetExceeded) {
        t.Fatalf("expected budget exceeded, got %v", err)
    }
}
```

Also test that estimated-only cost control cannot satisfy `RequireHardCap=true`.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/resources`

- [ ] **Step 3: Add ledger schema and transactional arithmetic**

Store settled cost, HELD reservations, UNRESOLVED exposure, hard limit, and cost-quality source separately. `Available` computes:

```text
hard_limit - settled - held - unresolved
```

Never infer release from executor timeout alone.

- [ ] **Step 4: Implement hard-cap metadata**

Use exact enum:

```go
type CostControl string
const (
    CostTechnicallyCapped CostControl = "TECHNICALLY_CAPPED"
    CostPrepaidQuota      CostControl = "PREPAID_QUOTA"
    CostVerifiedStop      CostControl = "VERIFIED_STOP"
    CostEstimatedOnly     CostControl = "ESTIMATED_ONLY"
    CostPotentiallyOpen   CostControl = "POTENTIALLY_UNBOUNDED"
)
```

- [ ] **Step 5: Verify GREEN and commit**

Run: `go test ./internal/resources -count=1`

```bash
git add internal/resources internal/state/sqlite/migrations
git commit -m "feat: add resource ledger and budget exposure"
```

---

### Task 10: Implement durable effect slots, External Operations, dispatch commitment, and reconciliation

**Files:**
- Create: `internal/state/sqlite/migrations/00006_operations.sql`
- Create: `internal/operations/provider.go`
- Create: `internal/operations/service.go`
- Create: `internal/operations/reconciler.go`
- Create: `internal/testutil/fake_provider.go`
- Test: `internal/operations/service_test.go`
- Test: `internal/operations/reconciler_test.go`

**Interfaces:**
- Produces: `ResolveEffectSlot`, `Prepare`, `Dispatch`, `SettleOutcome`, `Reconcile`.
- Provider contract separates `Dispatch` from `LookupOutcome`.
- This task integrates execution guard + policy + resources in the same trusted path.

- [ ] **Step 1: Write the effect-slot drift test**

```go
func TestChangedIntentDoesNotMintSecondEffectSlot(t *testing.T) {
    first := preparePurchase(t, svc, taskID, "purchase-primary", qty(1))
    second, err := svc.Prepare(ctx, request(taskID, attempt2, "purchase-primary", qty(2)))
    if !errors.Is(err, domain.ErrIntentConflict) { t.Fatalf("got %v", err) }
    if second.EffectSlotID != "" && second.EffectSlotID != first.EffectSlotID { t.Fatal("minted second slot") }
}
```

- [ ] **Step 2: Write the PREPARED-revocation test**

Prepare under ALLOW, then change active policy/authority to DENY, call `Dispatch`, and assert provider call count remains zero and operation never becomes DISPATCHED.

- [ ] **Step 3: Write unknown-outcome/retry tests**

Fake provider simulates: effect applied, acknowledgement lost. Assert operation becomes `OUTCOME_UNKNOWN`, reservation becomes unresolved exposure, replacement Attempt calls `LookupOutcome` before any new dispatch, and confirmed result is reused.

- [ ] **Step 4: Run RED**

Run: `go test ./internal/operations`

- [ ] **Step 5: Add operation schema**

Create durable `effect_slots` with unique `(collective_id, task_id, trusted_slot_key)`. Store current `intent_fingerprint` and revision separately. `external_operations` references the slot and stores PREPARED/DISPATCHED/outcome state, policy decision ids, dispatcher claim, provider reference, and reservation id.

- [ ] **Step 6: Implement canonical intent fingerprinting**

Use canonical JSON built from a provider-owned typed descriptor, not raw LLM JSON. Hash SHA-256 over semantically material fields. Adapter version is recorded separately and only affects compatibility checks when provider marks it semantically relevant.

- [ ] **Step 7: Implement PREPARE transaction**

In one transaction: guard Attempt/lease/fence; validate policy decision freshness; resolve effect slot; compare intent fingerprint/revision; reserve bounded exposure; insert/bind PREPARED operation; append Decision/Event.

- [ ] **Step 8: Implement atomic DISPATCH claim**

Before external I/O, evaluate policy again and atomically revalidate current authority, policy/profile hash, Attempt/lease/fence where applicable, intent, budget reservation, enforcement path, cancellation flag, and exclusive dispatcher claim. Only after transaction commits DISPATCHED may provider I/O run.

- [ ] **Step 9: Implement recovery/reconciliation**

On Box startup, DISPATCHED-without-outcome is treated conservatively as `OUTCOME_UNKNOWN` and queued for provider lookup. PREPARED is revalidated; it is never blindly sent.

- [ ] **Step 10: Verify GREEN**

Run:

```bash
go test ./internal/operations -count=1
go test -race ./internal/operations -count=1
```

- [ ] **Step 11: Commit**

```bash
git add internal/operations internal/testutil/fake_provider.go internal/state/sqlite/migrations
git commit -m "feat: add durable external operation commit boundary"
```

---

### Task 11: Implement deterministic scheduler, wake engine, dormancy, and minimal Strategic Pulse

**Files:**
- Create: `internal/scheduler/service.go`
- Create: `internal/wake/service.go`
- Test: `internal/scheduler/service_test.go`
- Test: `internal/wake/service_test.go`

**Interfaces:**
- Produces: `Scheduler.Next(ctx, CapacitySnapshot) (*TaskCandidate, error)` and `Lease(ctx, taskID, executorKind)`.
- Produces: `WakeService.NextDeadline`, `Due`, and minimal `StrategicPulse` scheduling.

- [ ] **Step 1: Write failing eligibility tests**

Test scheduler rejects Tasks with unmet dependencies, missing capability, insufficient enforcement level, invalid purpose, insufficient budget, or `earliest_start` in the future. Test a deadline-bound eligible Task beats lower-urgency work without using a single magic scalar.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/scheduler ./internal/wake`

- [ ] **Step 3: Implement deterministic eligibility pipeline**

Order:

```text
purpose valid
→ dependency ready
→ time window
→ capability/access/enforcement eligible
→ authority/policy prerequisites
→ resource envelope
→ rank by urgency/cost-of-delay/dependency unlock/priority/resource fit
```

Ranking uses deterministic tuple/comparators, not an opaque LLM score.

- [ ] **Step 4: Implement durable wakeups and lease expiry recovery**

Wakeups live in SQLite. Startup reads due wakeups and expired ACTIVE leases, marks expired leases, and re-evaluates Tasks before scheduling replacements.

- [ ] **Step 5: Implement minimal Strategic Pulse**

Pulse is a cheap scheduled internal task that checks: due obligations, blocked Tasks requiring re-evaluation, Known Unknown verification opportunities, stale capability assessments, and whether any authorized work exists. If none exists, Box remains DORMANT and does not spawn cognition.

- [ ] **Step 6: Verify GREEN and commit**

Run: `go test ./internal/scheduler ./internal/wake -count=1`

```bash
git add internal/scheduler internal/wake
git commit -m "feat: add deterministic scheduler and wake engine"
```

---

### Task 12: Implement Capability Registry, assessment, and revocable Attempt-scoped sessions

**Files:**
- Create: `internal/state/sqlite/migrations/00007_capabilities.sql`
- Create: `internal/capabilities/registry.go`
- Create: `internal/capabilities/session.go`
- Test: `internal/capabilities/registry_test.go`
- Test: `internal/capabilities/session_test.go`

**Interfaces:**
- Produces: capability definitions separate `Skill`, `Access`, `AuthorityRequirement`, `EnvironmentRequirement`.
- Produces: `Session.Call(ctx, name, request)` that always re-checks current Attempt + lease before protected calls.

- [ ] **Step 1: Write revocation/session tests**

Create session under active Attempt, revoke lease without creating replacement Attempt, then call a protected capability. Assert `ErrLeaseInactive` and provider call count zero.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/capabilities`

- [ ] **Step 3: Add registry/assessment schema**

Store capability id, semantic name/version, provider, enforcement level, access context, assessed-at, assessment evidence, cost metadata, health, and availability.

- [ ] **Step 4: Implement assessment as evidence-backed state**

A provider advertises primitive capabilities; assessor executes safe probes and records actual observed capability. README/tool descriptions are inputs, not verification.

- [ ] **Step 5: Implement sessions**

Session token is random, stored hashed, bound to collective/task/attempt/fence/lease expiry and visible capabilities. Every call resolves token then calls `execution.GuardAttempt` before authority-sensitive work.

- [ ] **Step 6: Verify GREEN and commit**

Run: `go test ./internal/capabilities -count=1`

```bash
git add internal/capabilities internal/state/sqlite/migrations
git commit -m "feat: add capability registry and scoped sessions"
```

---

### Task 13: Implement TEB enforcement profiles, CommandExecutor, and reproducible OCI bypass tests

**Files:**
- Create: `internal/teb/profile.go`
- Create: `internal/teb/oci.go`
- Create: `internal/teb/egress_proxy.go`
- Create: `internal/executors/executor.go`
- Create: `internal/executors/command.go`
- Create: `internal/testutil/fake_executor.go`
- Test: `internal/teb/oci_test.go`
- Test: `internal/executors/command_test.go`

**Interfaces:**
- Produces: `Executor.Start(ctx, AttemptEnvelope) (ExecutionResult, error)`.
- Produces: TEB profiles `ENFORCED`, `PARTIAL`, `UNENFORCED` with machine-readable guarantees.
- OCI backend uses Docker CLI first; Podman support is a later adapter, not a requirement for this task.

- [ ] **Step 1: Write executor isolation tests that skip only when Docker is absent**

Tests must attempt to:

```text
write outside workspace
read an unmapped host secret fixture
reach a disallowed network target
access /var/run/docker.sock
run as root / gain extra capability
```

For `ENFORCED` profile all attempts must fail. A skipped Docker test is not an MVC acceptance pass; it is only acceptable during development on hosts without Docker.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/teb ./internal/executors -count=1`

Expected: non-Docker unit tests fail until implementation; Docker integration tests may SKIP if engine absent.

- [ ] **Step 3: Implement CommandExecutor without shell concatenation**

Use argv arrays with `exec.CommandContext`; never concatenate untrusted strings into `sh -c`. Capture stdout/stderr as Evidence, enforce wall-clock timeout, and report observed resource usage where available.

- [ ] **Step 4: Implement baseline OCI profile**

Docker invocation must include equivalent guarantees to:

```text
--read-only
--cap-drop=ALL
--security-opt=no-new-privileges
--user=<non-root uid:gid>
--pids-limit
--memory
--cpus
--network=none          # offline strong profile
-v <workspace>:/workspace:rw
```

Never mount Docker socket, home directory, SSH config, cloud credentials, or authoritative DB path.

- [ ] **Step 5: Implement model-egress proxy primitive**

Add a Box-owned CONNECT proxy with exact host allowlist. For testability, proxy policy is pure:

```go
type EgressPolicy struct { AllowedHosts map[string]struct{} }
func (p EgressPolicy) AllowConnect(hostport string) bool
```

Integration profile uses an internal OCI network where executor has no direct internet route; only the proxy is dual-homed to the provider network. This is the basis for future `ENFORCED` model-control egress.

- [ ] **Step 6: Verify unit and available integration tests**

Run:

```bash
go test ./internal/teb ./internal/executors -count=1
go test -race ./internal/executors -count=1
```

On a Docker-capable Linux runner additionally require: `go test -tags=oci ./internal/teb -run TestOCIBypass -count=1` PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/teb internal/executors internal/testutil/fake_executor.go
git commit -m "feat: add trusted execution boundary and command executor"
```

---

### Task 14: Expose Attempt-scoped capabilities over MCP without granting authority

**Files:**
- Create: `adapters/mcp/server.go`
- Create: `adapters/mcp/tools.go`
- Test: `adapters/mcp/server_test.go`

**Interfaces:**
- Consumes: capability sessions from Task 12.
- Produces: per-Attempt MCP server/transport exposing only visible tools.
- Pin stable MCP SDK `v1.6.1`; do not adopt pre-release v1.7 protocol features in MVC unless a later reviewed compatibility need requires them.

- [ ] **Step 1: Write scope/revocation tests**

Test Attempt A sees only tools A; Attempt B cannot reuse A's session; revoked/expired lease causes tool call failure even while MCP transport remains open.

- [ ] **Step 2: Run RED**

Run: `go test ./adapters/mcp`

- [ ] **Step 3: Add SDK**

```bash
go get github.com/modelcontextprotocol/go-sdk@v1.6.1
go mod tidy
```

- [ ] **Step 4: Implement thin MCP translation**

Tool handler does only:

```text
parse MCP request
→ map to semantic capability call
→ Session.Call
→ convert result/evidence references to MCP result
```

No policy, authority, budget, or operation semantics are reimplemented in the MCP adapter.

- [ ] **Step 5: Verify GREEN and commit**

Run: `go test ./adapters/mcp -count=1`

```bash
git add adapters/mcp go.mod go.sum
git commit -m "feat: expose scoped capabilities over MCP"
```

---

### Task 15: Add the first real agentic harness adapter using Codex CLI

**Files:**
- Create: `internal/executors/codex.go`
- Create: `internal/executors/testdata/fake_codex.sh`
- Test: `internal/executors/codex_test.go`
- Modify: `internal/capabilities/registry.go`

**Interfaces:**
- Produces: `CodexExecutor` implementing `Executor`.
- Uses stable non-interactive `codex exec --ephemeral --json` and a configured workspace.
- Adapter is replaceable; Codex-specific events never enter domain types directly.

- [ ] **Step 1: Write parser/process tests against a fake Codex JSONL fixture**

Fixture emits `thread.started`, command/file-change events, final agent message, and `turn.completed` usage. Test adapter returns normalized `ExecutionResult` with evidence and usage, not Codex structs.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/executors -run Codex -count=1`

- [ ] **Step 3: Implement command invocation**

Build argv equivalent to:

```text
codex exec --ephemeral --json --cd /workspace -
```

Prompt is written over stdin, not shell interpolated. Set environment from an explicit allowlist. Never pass GitHub/cloud/Owner credentials.

- [ ] **Step 4: Define enforcement honesty**

Default Codex profile is `PARTIAL` until the exact model-control endpoint set for the active authentication/provider profile is verified behind the model-egress proxy. The adapter may only advertise `ENFORCED` when runtime startup performs a successful egress assessment proving direct general egress is blocked and all required model endpoints traverse the allowlisted proxy.

Protected GitHub/cloud/email effects therefore remain Box capabilities even when Codex itself has network access.

- [ ] **Step 5: Add optional real smoke test**

If `codex` is installed and `SUMMA42_CODEX_SMOKE=1`, run an ephemeral low-risk prompt in a temporary workspace and require clean JSONL completion. CI does not require external credentials for ordinary test runs.

- [ ] **Step 6: Verify GREEN and commit**

Run:

```bash
go test ./internal/executors -run Codex -count=1
```

```bash
git add internal/executors internal/capabilities/registry.go
git commit -m "feat: add Codex agentic executor adapter"
```

---

### Task 16: Implement canonical Memory, Decision/Audit records, and OpenTelemetry bridge

**Files:**
- Create: `internal/state/sqlite/migrations/00008_memory_audit.sql`
- Create: `internal/memory/service.go`
- Create: `internal/audit/service.go`
- Create: `internal/observability/otel.go`
- Test: `internal/memory/service_test.go`
- Test: `internal/audit/service_test.go`

**Interfaces:**
- Produces: canonical Evidence/Event/Claim/KnowledgeDelta writes and point-in-time queries.
- Produces: `DecisionRecord` capture at decision time.
- OTel spans reference ids but never replace canonical audit.

- [ ] **Step 1: Write memory provenance/temporal tests**

Test a Claim can be SUPERSEDED without deleting prior validity, conflicting same-period claim creates contradiction, and a query `BeliefsAt(t)` returns only evidence available by `t`.

- [ ] **Step 2: Write audit immutability tests**

DecisionRecord must include trigger, alternatives considered, basis class, evidence ids, policy/authority ids, expected outcome/confidence/resource envelope, actor, and timestamp. Test later updates cannot overwrite the original record.

- [ ] **Step 3: Run RED**

Run: `go test ./internal/memory ./internal/audit`

- [ ] **Step 4: Implement canonical memory tables and FTS**

Use SQLite relational tables plus FTS virtual table for textual recall. Graph/vector projection does not enter this task.

- [ ] **Step 5: Implement KnowledgeDelta ingestion validation**

A Summa42 submits observations/claims/evidence links; service validates schema/provenance, deduplicates entities/claims conservatively, records contradiction/supersession, and never upgrades a claim to VERIFIED solely because several agents repeat it.

- [ ] **Step 6: Add OTel dependency and bridge**

```bash
go get go.opentelemetry.io/otel@v1.44.0
go mod tidy
```

Instrument task/attempt/capability/operation spans. Export is optional/no-op by default. Audit data remains in SQLite even if telemetry is disabled.

- [ ] **Step 7: Verify GREEN and commit**

Run: `go test ./internal/memory ./internal/audit -count=1`

```bash
git add internal/memory internal/audit internal/observability internal/state/sqlite/migrations go.mod go.sum
git commit -m "feat: add canonical memory audit and telemetry bridge"
```

---

### Task 17: Implement local control plane and CLI operations

**Files:**
- Create: `internal/control/server.go`
- Create: `internal/control/client.go`
- Create: `cmd/summa42/status_cmd.go`
- Create: `cmd/summa42/task_cmd.go`
- Create: `cmd/summa42/approve_cmd.go`
- Create: `cmd/summa42/inspect_cmd.go`
- Modify: `cmd/summa42/root.go`
- Modify: `cmd/summa42-box/main.go`
- Test: `internal/control/server_test.go`
- Test: `cmd/summa42/root_test.go`

**Interfaces:**
- Produces: authenticated local control API used by CLI.
- Unix: HTTP over Unix domain socket.
- Windows: HTTP over named pipe using `go-winio v0.6.2` behind build-tagged transport file.
- Loopback TCP only via explicit config fallback and never as proof of Owner identity.

- [ ] **Step 1: Write local-auth/control tests**

Test unauthenticated request is rejected; status is read-only; approval request requires Owner signature over challenge + request digest; transport peer identity alone does not count as Owner approval.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/control ./cmd/summa42`

- [ ] **Step 3: Add Windows pipe dependency and transport abstraction**

```bash
go get github.com/Microsoft/go-winio@v0.6.2
go mod tidy
```

Use build-tagged `listen_unix.go` and `listen_windows.go` if needed, even if the logical server remains `net/http`.

- [ ] **Step 4: Implement API methods**

Minimum endpoints/RPCs:

```text
GET status
POST tasks
GET tasks/{id}
POST approvals/{id}
GET inspect/operations/{id}
GET inspect/attempts/{id}
POST shutdown
```

Mutating commands call core services; handlers never issue direct SQL for semantic transitions.

- [ ] **Step 5: Wire CLI commands**

`summa42 status`, `task create`, `task show`, `approve`, `inspect attempt`, `inspect operation`. Output defaults human-readable; `--json` returns stable JSON DTOs.

- [ ] **Step 6: Verify GREEN and commit**

Run:

```bash
go test ./internal/control ./cmd/summa42 -count=1
go build ./cmd/summa42 ./cmd/summa42-box
```

```bash
git add internal/control cmd go.mod go.sum
git commit -m "feat: add local control plane and CLI"
```

---

### Task 18: Assemble the vertical slice and enforce the full failure acceptance suite

**Files:**
- Create: `tests/acceptance/failure_semantics_test.go`
- Create: `tests/acceptance/vertical_slice_test.go`
- Modify: `README.md`
- Modify: `cmd/summa42-box/main.go`
- Modify: `.github/workflows/ci.yml`

**Interfaces:**
- Exercises the real service graph, SQLite DB, Box runtime, fake external providers/executors, and optionally OCI integration profile.
- This task does not invent new semantics. It proves the reviewed semantics are composed correctly.

- [ ] **Step 1: Write one table-driven suite for all reviewed failure gates**

Required named subtests:

```text
unknown_external_result_reconciles_without_duplicate
unresolved_billing_exposure_blocks_overcommit
stale_attempt_rejected
expired_lease_with_matching_fence_rejected
crash_before_completion_record_does_not_fake_completion
completion_record_resumes_verification
capability_bypass_blocked_in_enforced_profile
constraint_precedence_blocks_illegal_goal_path
hard_budget_requires_enforceable_ceiling
confirmed_effect_survives_attempt_replacement
effect_slot_parameter_drift_conflicts
prepared_revocation_prevents_dispatch
opa_http_send_rejected
opa_error_timeout_invalid_output_fail_closed
```

Each subtest must assert both state and provider/executor call counts where duplicate effects are the risk.

- [ ] **Step 2: Run suite and verify RED for any missing integration wiring**

Run: `go test ./tests/acceptance -count=1 -v`

Expected at first: FAIL until Box composition is complete.

- [ ] **Step 3: Compose `Box` explicitly**

Create a constructor in `cmd/summa42-box` or a focused runtime package if necessary, wiring:

```text
SQLite Store
Clock
Identity
PolicyEngine
Purpose Service
Execution Service
Evidence Store
Verification Service
Resource Ledger
Operations Service/Reconciler
Capability Registry/Sessions
Scheduler/Wake
Executors/TEB
Memory/Audit
Control Server
Observability
```

Avoid a service-locator map. Dependencies are explicit constructor fields.

- [ ] **Step 4: Write the end-to-end useful-work test**

Scenario:

```text
initialize Collective
→ create Mission/Owner purpose
→ create Goal
→ create Task: repair a fixture repository failure
→ scheduler leases Attempt
→ fake agentic executor proposes a child diagnostic Task
→ deterministic command executor runs test fixture
→ Attempt completes
→ verifier checks acceptance command
→ Task succeeds
→ KnowledgeDelta + evidence + DecisionRecords persist
→ costs/usage aggregate
→ Box has no remaining work and returns DORMANT
```

Assert every Task traces to authorized purpose and every success dependency unlock occurs only after acceptance.

- [ ] **Step 5: Add OCI acceptance gate to Linux CI where Docker is available**

Add a separate job/tag:

```bash
go test -tags=oci ./internal/teb ./tests/acceptance -run 'Bypass|Capability' -count=1 -v
```

If GitHub-hosted Docker cannot prove the intended internal-network/proxy setup reliably, run this gate on a documented Linux runner profile; do not silently downgrade it to a unit test.

- [ ] **Step 6: Document local bootstrap and honest guarantees**

README must state:

```text
summa42 init
summa42-box
summa42 status
```

and clearly describe `ENFORCED/PARTIAL/UNENFORCED`, current single-Cube scope, SQLite location restrictions, Codex adapter enforcement default, and deferred multi-Cube/Graphiti/Postgres/NATS features.

- [ ] **Step 7: Run the full verification matrix fresh**

Run:

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./cmd/summa42 ./cmd/summa42-box
python3 docs/superpowers/research/spikes/2026-09-14-sqlite-persistence-spike.py
```

On Docker-capable Linux also run:

```bash
go test -tags=oci ./internal/teb ./tests/acceptance -count=1 -v
```

Expected: zero test failures, zero vet errors, binaries build, SQLite spike prints all PASS lines, OCI gate passes on the reference environment.

- [ ] **Step 8: Commit**

```bash
git add tests README.md cmd/summa42-box .github/workflows/ci.yml
git commit -m "feat: complete Minimal Viable Collective vertical slice"
```

---

## Mandatory Review Checkpoints During Execution

A reviewer gate is required after Tasks 5, 7, 10, 13, and 18 because these establish security/reliability boundaries that later work assumes:

1. **Task 5 — Policy boundary:** verify side-effect-free OPA and fail-closed behavior.
2. **Task 7 — Execution authority:** verify lease + fence + current Attempt semantics.
3. **Task 10 — External effect boundary:** verify effect-slot identity, PREPARED/DISPATCHED, unknown outcome, and budget coupling.
4. **Task 13 — TEB:** verify claimed enforcement profile against executable bypass tests.
5. **Task 18 — MVC acceptance:** verify all reviewed failure scenarios and the semantic vertical slice.

A failed checkpoint stops downstream implementation until corrected. Do not compensate by weakening an acceptance criterion.

## Explicitly Deferred After MVC

Do not add these during this plan unless an approved MVC acceptance criterion becomes impossible without one:

```text
PostgreSQL shared state
NATS/JetStream
SPIFFE/SPIRE
multi-Cube partition islands/reconciliation
Graphiti/Neo4j/FalkorDB/Kuzu projection
full Goal Market
full Earned Autonomy
federation between Collectives
Swarms as resource domains
mobile app/web UI
Kubernetes
Firecracker/E2B
self-modifying runtime rollout
```

The code must leave adapter boundaries for them, but YAGNI applies to implementations.

## Definition of MVC Complete

MVC is complete only when all of the following are demonstrated by fresh evidence:

- one local Collective initializes with Owner/Cube/Constitution state;
- Box can remain running with zero active Summa42s and negligible active cognition;
- authorized purpose → Goal → Task → Attempt → verification → accepted success works end-to-end;
- deterministic and agentic executor abstractions both exist;
- Task survives executor/Box restart;
- stale/expired/revoked execution authority cannot mutate canonical state;
- External Operations cannot duplicate a protected effect through retries or intent drift;
- PREPARED can be revoked; DISPATCHED is treated as potentially escaped;
- unknown external result and unresolved cost remain conservative state/exposure;
- policy evaluation cannot perform direct external I/O and fails closed;
- protected capabilities are technically mediated under the claimed enforcement profile;
- canonical memory, evidence provenance, audit, and resource usage are queryable;
- all mandatory failure/contract tests pass;
- no deferred distributed dependency is required for startup.
