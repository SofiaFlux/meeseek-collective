# Meeseek Collective MVC Implementation Plan — Self-Review Addendum

**Status:** Normative companion to `2026-09-15-mvc-implementation-plan.md`  
**Date:** 2026-09-15

This file records corrections found by the required Superpowers plan self-review. It is normative: where this file is more specific than the main implementation plan, this file wins. Executors must read both files before Task 1.

## 1. File-structure corrections

The locked structure in the main plan additionally includes:

```text
internal/version/version.go
internal/version/version_test.go
internal/control/listen_unix.go
internal/control/listen_windows.go
internal/runtime/box.go
internal/runtime/box_test.go
```

`internal/runtime/box.go` owns Box composition. `cmd/meeseek-box/main.go` is only process/bootstrap wiring and must not become the service locator.

## 2. Exact Attempt authority API

The canonical execution-authority API is both a low-level transaction guard and a convenience transaction wrapper. Use these exact signatures:

```go
type GuardedAttempt struct {
    TaskID          domain.ID
    AttemptID       domain.ID
    FenceGeneration int64
    LeaseExpiresAt  time.Time
    TaskState       domain.TaskState
}

func (s *Service) GuardAttempt(
    ctx context.Context,
    tx *sql.Tx,
    attemptID domain.ID,
    allowedTaskStates ...domain.TaskState,
) (GuardedAttempt, error)

func (s *Service) WithGuardedAttempt(
    ctx context.Context,
    attemptID domain.ID,
    allowedTaskStates []domain.TaskState,
    fn func(*sql.Tx, GuardedAttempt) error,
) error
```

`GuardAttempt` must check in the same database snapshot:

```text
current_attempt_id == attempt_id
current_fence == attempt.fence_generation
lease_state == ACTIVE
lease_expires_at > trusted Box clock
Task state ∈ allowedTaskStates
```

`WithGuardedAttempt` starts the transaction, calls `GuardAttempt`, calls `fn`, and commits only if all succeed. Tasks 8, 10, and 12 use these APIs rather than reproducing SQL predicates.

## 3. Task challenge and failure semantics omitted from the first state list

Task 2 must also define:

```go
const (
    TaskChallenged domain.TaskState = "CHALLENGED"
    TaskExpired    domain.TaskState = "EXPIRED"
)

type ChallengeScope string
const (
    ChallengeTask              ChallengeScope = "TASK"
    ChallengeParent            ChallengeScope = "PARENT"
    ChallengeGoal              ChallengeScope = "GOAL"
    ChallengeMissionAssumption ChallengeScope = "MISSION_ASSUMPTION"
)

type FailureClass string
const (
    FailureTransient           FailureClass = "TRANSIENT"
    FailureCapability          FailureClass = "CAPABILITY"
    FailureEpistemic           FailureClass = "EPISTEMIC"
    FailurePlanning            FailureClass = "PLANNING"
    FailureResource            FailureClass = "RESOURCE"
    FailureAuthority           FailureClass = "AUTHORITY"
    FailureObjectiveImpossible FailureClass = "OBJECTIVE_IMPOSSIBLE"
    FailureExecution           FailureClass = "EXECUTION"
)
```

`00003_execution.sql` additionally contains `task_challenges` and `attempt_failures`.

Task 7 additionally produces:

```go
func (s *Service) ChallengeTask(ctx context.Context, taskID domain.ID, scope domain.ChallengeScope, reason string, evidenceIDs []domain.ID) error
func (s *Service) FailAttempt(ctx context.Context, attemptID domain.ID, class domain.FailureClass, signature string, evidenceIDs []domain.ID) error
```

A challenge may block/pause dependent work. Repeated identical failure signature must not automatically trigger another retry; scheduler receives it as a replan/diagnose signal.

## 4. Exact initial OPA side-effect-free capability profile

Task 5 must not say merely “comparison/string/collection built-ins”. The initial capability profile is built from these exact OPA `ast.Builtin` objects:

```go
var safeBuiltins = []*ast.Builtin{
    ast.Equality,
    ast.Assign,
    ast.Member,
    ast.MemberWithKey,
    ast.GreaterThan,
    ast.GreaterThanEq,
    ast.LessThan,
    ast.LessThanEq,
    ast.NotEqual,
    ast.Equal,
    ast.Plus,
    ast.Minus,
    ast.Multiply,
    ast.Divide,
    ast.Ceil,
    ast.Floor,
    ast.Round,
    ast.Abs,
    ast.Rem,
    ast.And,
    ast.Or,
    ast.Count,
    ast.Sum,
    ast.Product,
    ast.Max,
    ast.Min,
    ast.Any,
    ast.All,
    ast.ArrayConcat,
    ast.ArrayFlatten,
    ast.ArraySlice,
    ast.ArrayReverse,
    ast.ToNumber,
    ast.SetDiff,
    ast.Intersection,
    ast.Union,
    ast.Concat,
    ast.IndexOf,
    ast.IndexOfN,
    ast.Substring,
    ast.Lower,
    ast.Upper,
    ast.Contains,
    ast.StartsWith,
    ast.EndsWith,
    ast.Split,
    ast.SplitN,
    ast.Replace,
    ast.Trim,
    ast.TrimSpace,
    ast.Sprintf,
    ast.JSONMarshal,
    ast.JSONUnmarshal,
    ast.JSONIsValid,
    ast.HexEncode,
    ast.HexDecode,
    ast.ObjectUnion,
    ast.ObjectRemove,
    ast.ObjectFilter,
    ast.ObjectGet,
    ast.ObjectKeys,
    ast.Sort,
    ast.IsNumber,
    ast.IsString,
    ast.IsBoolean,
    ast.IsArray,
    ast.IsObject,
    ast.IsSet,
    ast.IsNull,
}
```

Construct capabilities from those objects plus the approved Rego language/features for OPA v1.20.2. Do not start from unrestricted capabilities and subtract a blacklist.

The following must fail compilation under the profile:

```text
http.send
net.lookup_ip_addr
opa.runtime
time.now_ns
rand.intn
```

If a later policy genuinely needs another pure built-in, adding it is a reviewed policy-capability-profile change: add a unit test proving it is side-effect-free for Meeseek's threat model, then change the profile hash/version. Do not silently widen capabilities at runtime.

## 5. Exact Box runtime composition location

Task 18 creates `internal/runtime/box.go` with explicit dependencies:

```go
type Box struct {
    Store        *sqlite.Store
    Clock        clock.Clock
    Policy       policy.PolicyEngine
    Purpose      *purpose.Service
    Execution    *execution.Service
    Evidence     *evidence.Store
    Verification *verification.Service
    Resources    *resources.Service
    Operations   *operations.Service
    Reconciler   *operations.Reconciler
    Capabilities *capabilities.Registry
    Scheduler    *scheduler.Service
    Wake         *wake.Service
    Memory       *memory.Service
    Audit        *audit.Service
    Control      *control.Server
}

func NewBox(cfg Config) (*Box, error)
func (b *Box) Run(ctx context.Context) error
func (b *Box) Close() error
```

`cmd/meeseek-box/main.go` loads configuration, creates `runtime.NewBox`, handles OS signals, and calls `Run`. It contains no semantic SQL or policy logic.

## 6. Exact control transport files

Task 17 always creates both transport implementations; “if needed” in the main plan is removed.

```text
internal/control/listen_unix.go      //go:build !windows
internal/control/listen_windows.go   //go:build windows
```

Unix uses `net.Listen("unix", socketPath)`. Windows uses `winio.ListenPipe(pipePath, nil)`. Both feed the same `http.Server` handler tree.

## 7. TEB model-egress topology contract

Task 13's model-egress support uses this semantic interface:

```go
type EgressPolicy struct {
    AllowedHosts map[string]struct{}
}

func (p EgressPolicy) AllowConnect(hostport string) bool

type EgressProxy interface {
    Start(ctx context.Context, policy EgressPolicy) (ProxyEndpoint, error)
    Close() error
}
```

For a claimed `ENFORCED` online-agent profile the executor namespace/container must have no direct external route. The only path to model-provider network is the Box-controlled allowlisting proxy. A configuration that merely sets `HTTPS_PROXY` while leaving direct egress available is `PARTIAL`, not `ENFORCED`.

Acceptance tests use local provider and disallowed-target fixtures so CI does not depend on public internet:

```text
executor internal network ── proxy ── provider fixture network
       │
       └── direct route to provider/disallowed fixture: absent
```

Test all three:

1. direct provider connection fails;
2. proxy connection to allowlisted provider succeeds;
3. proxy connection to non-allowlisted target fails.

## 8. Spec-coverage matrix

The self-review maps approved MVC requirements to implementation tasks:

| Approved requirement | Plan task(s) |
|---|---|
| Owner / Constitution / identity | 4 |
| Restricted deterministic policy | 5 |
| Mission / authorized purpose / obligations | 6 |
| Durable Task graph / child work | 7 |
| Leases / fencing / failure classes / challenge | 7 + this addendum |
| Evidence / AttemptCompletionRecord / acceptance | 8 |
| Economics / reservations / unresolved exposure | 9 |
| Effect slots / External Operations / unknown outcome | 10 |
| Deterministic scheduler / dormancy / wake / Strategic Pulse | 11 |
| Capability registry / assessment / scoped sessions | 12 |
| TEB / enforcement profiles / deterministic executor | 13 |
| MCP interoperability | 14 |
| First real agentic harness | 15 |
| Canonical Evidence/Event/Claim memory | 16 |
| Causal Decision/Audit records | 16 |
| Operational observability | 16 |
| Human/local control plane and approvals | 17 |
| All reviewed failure scenarios | 18 |
| Single-Cube useful vertical slice | 18 |

## 9. Placeholder and consistency review result

Checked plan and addendum for `TODO`, `TBD`, and `FIXME`: none are permitted.

The canonical names after self-review are:

```text
PolicyEngine.Evaluate
execution.Service.GuardAttempt
execution.Service.WithGuardedAttempt
EvidenceStore.Put
verification.Service.CompleteAttempt
verification.Service.AcceptTask
resources.Service.Reserve / Settle / MarkUnresolved / Release / Available
operations.Service.ResolveEffectSlot / Prepare / Dispatch / SettleOutcome
operations.Reconciler.Reconcile
scheduler.Service.Next / Lease
capabilities.Session.Call
runtime.NewBox / Run / Close
```

Implementation must use these names unless a reviewer-approved plan amendment changes them before the dependent task starts.

## 10. Task 16 OpenTelemetry dependency compatibility correction

The main plan's Task 16 command pins `go.opentelemetry.io/otel@v1.44.0`. That version cannot coexist with the already normative OPA `v1.20.2` pin: OPA `v1.20.2` declares `go.opentelemetry.io/otel v1.46.0` and `go.opentelemetry.io/otel/trace v1.46.0` as module requirements, and Go rejects an explicit simultaneous request for OPA `v1.20.2` plus OTel `v1.44.0`.

Task 16 therefore uses OTel `v1.46.0` while preserving OPA `v1.20.2`. This compatibility correction supersedes only the `v1.44.0` version literal in Task 16; it does not change the observability semantics: export remains optional/no-op by default, telemetry references canonical ids, and SQLite/EvidenceStore audit remains authoritative.
