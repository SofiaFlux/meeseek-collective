# Publish executor (Work 2) — design

## Intent

Execute approved publication decisions as a worker-loop executor: read the
decision blob, run each intent through the protected-effect machinery, and
return per-effect evidence. Staged modes keep shadow safe by default.

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
  Vote *adoeffects.VoteProvider, OwnerApprovals []domain.ID, Risk string}
PublishOps interface {
    Prepare(ctx, operations.PrepareRequest) (domain.ExternalOperation, error)
    Dispatch(ctx, operationID, attemptID domain.ID) (domain.ExternalOperation, error)
    SettleOutcome(ctx, operationID domain.ID, outcome operations.ProviderOutcome) (domain.ExternalOperation, error)
}
```

Work 2 payload: `{decision, caseID, workID, repo, pr, revision}` (all
required). Marker `[summa42:<caseID>:<workID>]`; slot keys
`ado.pr.comment:<repo>#<pr>:<revision>:<index>` and
`ado.pr.approve:<repo>#<pr>:<revision>`.

Per intent: `Prepare(AttemptID=envelope.AttemptID, TrustedSlotKey, Intent,
Risk default "LOW", Attributes{repo,pr,revision}, RequiredApprovals)` →
`Dispatch` → provider `LookupOutcome` → `SettleOutcome`. UNKNOWN stays
UNKNOWN for later reconciliation; never blind re-dispatch in this slice.

Modes: `none` records intents to evidence without `Prepare`; `comments`
dispatches comments, records approve intents as skipped; `all` dispatches
both. Skipped/recorded-only intents are explicit evidence entries, never
silent drops.

## Cross-slice fix (assessor caps)

`Prepare` requires the provider capability in the task authority ceiling, so
Work 2 caps must include the effect capability. `AssessReview` appends
`ado.pr.comment` (comment branch) or `ado.pr.approve` (approve branch) to both
`RequiredCapabilities` and `AuthorityCeiling` of Next Work 2 — still ⊆ grant
(verified first, else the existing `grant-denies-*` hold). Slice 2 code, spec
and plan are amended accordingly in this slice with tests.

## Registration

Builder from env (`SUMMA42_PUBLISH_MODE` default `none`;
`SUMMA42_PUBLISH_APPROVERS` csv owner IDs; `SUMMA42_PUBLISH_RISK` default
`LOW`; adoeffects providers from ADO env) wired only into `runWorker` as kind
`ado-publish`. Missing ADO env → kind unregistered with startup notice.

## Testing

Fake `PublishOps` + fake MCP (adoeffects seam): mode none records without
Prepare; comments mode dispatches comments + skips approve; all mode
dispatches both; UNKNOWN settle path; bad payload/decision fails before any
Prepare; slot keys exact; grant-missing effect cap holds in assessor amend
tests. No live credentials.

## Scope boundary

No driver loop, no Work 2 materialization, no Assessment 2, no approval
creation (owner approvers come from config; approvals lifecycle is existing
machinery). Shadow default `none` publishes nothing.

## Acceptance criteria

- Each decision intent ends dispatched+settled, recorded-only, or skipped —
  all visible in attempt evidence.
- Staged modes gate real writes; default publishes nothing.
- Work 2 Tasks carry ceilings covering their effect capabilities.
