# Publish executor (Work 2) — design

## Intent

Execute approved publication decisions as a worker-loop executor: read the
decision blob, run each intent through `Prepare`/`Dispatch`, and return
per-effect evidence. Staged modes keep shadow safe by default.

## Approaches considered

1. **Publisher executor kind `ado-publish` (selected):** Work 2 stays inside
   task machinery (lease, evidence, completion); modes gate dispatch.
2. **Driver-side direct dispatch:** bypasses attempt guards and completion
   provenance. Rejected.
3. **One intent per task:** multiplies tasks and slots without benefit; the
   decision is atomic per review. Rejected.

## Contract

```go
PublishConfig{Mode PublishMode /* none|comments|all */,
  Operations PublishOps, Comment *adoeffects.CommentProvider,
  Vote *adoeffects.VoteProvider, OwnerApprovals []domain.ID,
  RiskComment string, RiskApprove string}
PublishOps interface {
    Prepare(ctx, operations.PrepareRequest) (domain.ExternalOperation, error)
    Dispatch(ctx, operationID, attemptID domain.ID) (domain.ExternalOperation, error)
}
```

No `SettleOutcome` here: service `Dispatch` already settles internally, and
the private reconciler owns unknowns (re-`Dispatch` path). The executor calls
`Prepare` → `Dispatch` per intent; an `UNKNOWN`/error outcome is recorded in
evidence and stops the sequence (later reconcile, never blind re-dispatch in
this slice).

Work 2 payload (all required, fail before any `Prepare` if blank):
`{decision, caseID, workID, project, repo, pr, revision}`. `decision` is the
evidence ID of the `adoreview.ReviewDecision` JSON blob (Slice 2); the
executor `Get`s and validates it (`action` ∈ comment/approve/hold; a `hold`
decision records only, never prepares — hold reviews must not become Work 2
tasks, enforced by the Slice 3c driver, defended here).

Marker `[summa42:<caseID>:<workID>]`; slot keys
`ado.pr.comment:<project>/<repo>#<pr>:<revision>:<index>` and
`ado.pr.approve:<project>/<repo>#<pr>:<revision>`.

## Modes and risk (staged enablement)

- `none`: record intents to evidence, no `Prepare`. Default. Shadow.
- `comments`: dispatch comment intents; approve intents recorded as skipped.
- `all`: dispatch both.
- Risk is operator policy vocabulary, never defaulted down: comments default
  `LOW`; approve risk is REQUIRED explicit config (startup error in `all`
  mode without it) — per the parent rule, approval must rest on a reviewed
  grant-bound rule, not a `LOW` misclassification.

Skipped/recorded-only intents are explicit evidence entries, never silent.

## Evidence

`AGENT_MESSAGE` entries with one JSON line per intent outcome
`{slot, operation, state, reference|skipped|recorded-only}`. No other kinds.

## Cross-slice fix (assessor caps)

`Prepare` requires the provider capability in the task authority ceiling, so
Work 2 caps must carry the effect capability. `AssessReview` first checks the
effect cap ∈ grant capabilities (else new `grant-denies-capability:ado.pr.*`
hold), then appends it to both `RequiredCapabilities` and `AuthorityCeiling`
of Next Work 2 (subset gates hold by construction; `ProposedActions` check
unchanged). Publish-capable missions must therefore grant `ado.pr.comment` /
`ado.pr.approve` as *capabilities*, not just actions — observer grant docs
amended accordingly. Slice 2 code, spec and plan change in this slice with
tests.

## Registration

Builder from env (`SUMMA42_PUBLISH_MODE` default `none`;
`SUMMA42_PUBLISH_APPROVERS` csv owner IDs; `SUMMA42_PUBLISH_RISK_COMMENT`
default `LOW`; `SUMMA42_PUBLISH_RISK_APPROVE` required in `all` mode; ADO
command/org env reused) wired only into `runWorker`:
build both adoeffects providers (with `adomcp`-backed read funcs) →
`runtime.Config.OperationProviders` → `box.Operations` into
`PublishConfig.Operations` → kind `ado-publish`. Missing ADO env → providers
absent → kind unregistered with startup notice.

Executor selection among registry kinds stays scheduler baseline/preference
(single-purpose deployment registers exactly the needed executors; a
taskclass→kind routing table is explicit follow-on, not this slice).

## Testing

Fake `PublishOps` + fake MCP (adoeffects seam): mode none records without
Prepare; comments mode dispatches comments + skips approve; all mode
dispatches both; UNKNOWN recorded + stops; bad payload/decision/hold fails or
records before any Prepare; slot keys exact; assessor cap-append + grant-
denies-capability tests. No live credentials.

## Scope boundary

No driver loop, no Work 2 materialization, no Assessment 2 (its owner is the
Slice 3c driver, which also owns UNKNOWN reconciliation), no approval
creation (approver IDs from config; approvals lifecycle is existing
machinery). Shadow default `none` publishes nothing.

## Acceptance criteria

- Each decision intent ends dispatched, recorded-only, or skipped — all
  visible in attempt evidence; UNKNOWN never re-dispatched here.
- Staged modes + explicit approve risk gate real writes; default publishes
  nothing.
- Work 2 Tasks carry ceilings covering their effect capabilities
  (grant-denies otherwise, as holds).
