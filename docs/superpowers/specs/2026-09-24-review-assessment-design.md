# Review assessment (Work 1) — design

## Intent

Turn a completed Copilot review attempt into a durable, gated assessment
decision: propose Work 2 publication (comments or vote), or hold. No external
writes happen here; a later driver materializes Work 2 and dispatches effects.

## Approaches considered

1. **Assessor library in `adoreview` (selected):** pure mapping plus live gates,
   persisted through existing `workflowcase.Assess`. Fully testable on fakes;
   the driver loop belongs to the Work 2 slice.
2. **Assessor plus driver in one slice:** drags in Work 2 materialization —
   half of the next slice. Rejected.
3. **Gates inside the Copilot executor:** the executor returns results, it does
   not judge them; gates need the payload revision and live ADO. Rejected.

## Input

```go
ReviewInput{
    Case      workflowcase.Case
    WorkID    domain.ID
    Review    executors.ReviewResult
    Payload   CopilotReviewPayload  // repo, pr, source/target commits
    Caller    PRCaller              // ado.pr.get, ado.build.status
    RequireCI bool
    CIStatus  string // expected value, default "succeeded"
    RemainingBudget int64
}
```

## Gates (any failure → hold)

- **Freshness:** `ado.pr.get` current source/target commits equal the payload
  ones, and `reviewedCommits` contains the source commit. Else `stale-review`.
- **Consistency:** every `findings[].path` ∈ `reviewedFiles`; FINDINGS with
  empty findings is inconsistent. Full diff coverage is not checkable without
  a changed-files source — explicit residual model risk, accepted by the
  parent design.
- **CI (only when required):** `ado.build.status` equals the expected value;
  missing/unreadable → hold. Shapes follow the MCP TOOLSET, confirmed live
  later; parser fails closed.

## Mapping

- CLEAN + gates pass → `Continue`, Next Work 2 `{Kind: "ado.pr.publish",
  RequiredCapabilities/AuthorityCeiling from the current case step,
  ProposedActions: ["ado.pr.approve"]}`.
- FINDINGS → `Continue`, Next with `ProposedActions: ["ado.pr.comment"]`.
  Findings never propose approval, even partially.
- UNCERTAIN or any gate failure → `Unknown` with reason → hold (`BLOCKED`).
  The assessment performs no external write.

`ProgressSignature`: `ado:<object>:<revision>:<verdict>` (stable, replay-safe
through `Assess` idempotency). `RemainingBudget` passes through (attempt spend
is tracked separately against the resource envelope).

## Machine-readable decision

```go
ReviewDecision{Action: "comment"|"approve"|"hold",
  Comments: [{Path, Line, Body}], Vote: "approve"|"",
  Reason: string}
```

Persisted as a canonical JSON evidence blob (`application/json`,
`ado.review.decision`); its ID joins `Assessment.EvidenceIDs` alongside the
review blob. Comment bodies render deterministically from findings
(`<path>[:<line>]: <explanation>` plus evidence quote); the vote is a fixed
approve marker, never free text. The Slice 3 driver reads it back.

## evidence.Get

New `evidence.Store.Get(ctx, id) (EvidenceObject, []byte, error)`: reads the
blob bytes for a stored ID; unknown ID or unreadable file errors. Needed by
the assessor (review JSON) and later by the driver (decision JSON).

## Testing

Fake `PRCaller` + testutil stack with a directly `Ensure`d case: CLEAN→vote
proposal, FINDINGS→comments proposal (never approve), UNCERTAIN→hold, stale
commits→hold, findings-outside-reviewedFiles→hold, empty-findings FINDINGS→
hold, CI required green/red, decision blob content + kind, `Get` round-trip
and unknown-ID error. No live credentials.

## Scope boundary

No driver loop, no Work 2 materialization, no ADO write provider, no
reconciliation, no least-privilege narrowing. Shadow posture holds.

## Acceptance criteria

- Every completed review maps to exactly one of: vote proposal, comments
  proposal, hold with reason — recorded durably via `Assess`.
- Gate failures hold even a CLEAN verdict; findings never yield approval.
- Decision blob is re-readable and sufficient for Slice 3 to publish without
  re-running the model.
