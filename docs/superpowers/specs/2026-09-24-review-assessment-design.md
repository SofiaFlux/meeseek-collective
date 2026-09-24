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
    Review    executors.ReviewResult  // parsed, in memory; driver holds it
    ReviewEvidenceID domain.ID       // blob ID of the canonical review JSON
    Payload   CopilotReviewPayload  // repo, pr, source/target commits
    Project   string                 // ADO project for pr.get/file calls
    Caller    PRCaller               // ado.pr.get, ado.build.status
    RequireCI bool                   // driver sets from grant (see below)
    CIStatus  string                 // expected value, default "succeeded"
    RemainingBudget int64
}
```

The assessor does not fetch the review JSON itself; the driver (Slice 3) reads
it via the new `evidence.Get` and passes both value and blob ID. Order inside
`AssessReview`: validate input → run gates → `Put` decision blob
(`application/json`, `ado.review.decision`) → `Assess` with
`EvidenceIDs=[ReviewEvidenceID, decisionID]` (satisfies the non-empty rule).

`RequireCI` derivation (applied by the driver, stated here): the gate is
required iff the case grant lists the `ado.build.status` capability —
capability presence is the grant's machine-readable "requires this gate" signal.

## Gates (any failure → hold)

- **Freshness:** `ado.pr.get` with `{"action":"get"}` plus project/repository/PR
  identifiers returns current source/target commits; they must equal the
  payload ones, and `reviewedCommits` must contain the source commit. Else
  `stale-review`. Response field paths are assumed from MCP/REST conventions
  and confirmed in the live smoke follow-on; missing/unparseable → hold.
- **Consistency (necessary, not sufficient):** every `findings[].path` ∈
  `reviewedFiles`; FINDINGS with empty findings is inconsistent. This does not
  prove diff coverage — that remains residual model risk.
- **CI (only when required):** `ado.build.status` with `{"action":
  "get_status"}` on the merge build referenced by the `pr.get` response must
  equal the expected value; missing ref or unreadable status → `ci-unknown`
  hold.

## Mapping

Next Work 2 for both proposal branches (full shape — `Decide` rejects blank
kinds and enforces all three subset gates):

```go
workflow.WorkProposal{
    Kind: "publish-decision",
    RequiredCapabilities: append([]string(nil), kase.NextWork.RequiredCapabilities...),
    AuthorityCeiling:     append([]string(nil), kase.NextWork.AuthorityCeiling...),
    ProposedActions:      []string{"ado.pr.approve"}, // or {"ado.pr.comment"}
}
```

Caps/ceiling copy the current case step, so the ceiling⊆grant and
required⊆ceiling checks hold by construction (validated at `Ensure` time
against the unchanged grant). Before proposing, the assessor verifies the
needed action ∈ grant actions; otherwise `Unknown` hold with reason
`grant-denies-<action>` (a `Decide` error is never used for policy outcomes).

- CLEAN + gates pass → `Continue` with the approve Next.
- FINDINGS → `Continue` with the comment Next. Findings never propose
  approval, even partially.
- CLEAN with non-empty findings → `Unknown` hold `verdict-findings-mismatch`
  (the model contradicted itself; fail closed).
- UNCERTAIN or any gate failure → `Unknown` with reason → hold (`BLOCKED`).
  UNCERTAIN findings are still recorded in the decision blob, never proposed.
- `Decide`-level `BLOCKED` (budget/steps exhausted) is a recorded terminal
  hold like any other; only `Assess` request-drift errors are true errors.

`ProgressSignature`: `ado:<ObjectID>:<RevisionID>:<verdict>` using the
`PullRequest.ObjectID()` (`repo#num`) and `RevisionID()` (`source:target`)
encodings. `RemainingBudget` passes through unchanged (`Assess` enforces
`0 <= req <= current`; real spend is enforced against the resource envelope at
schedule/lease time). Replay story (honest): `Assess` idempotency is exact request-JSON
equality, and `EvidenceIDs` embed random blob IDs — so a retry after re-`Put`
fails as "different request", and equal non-empty signatures across attempts
hit the no-progress `BLOCKED` rule. Callers retry only with byte-identical
requests (same evidence IDs, budget, signature); the Slice 3 driver is
single-pass by construction.

## Machine-readable decision

```go
ReviewDecision{Action: "comment"|"approve"|"hold",
  Comments: [{Path, Line, Body}], Vote: "approve"|"",
  Reason: string}
```

Comment bodies render `<path>[:<line>]: <explanation>` plus the evidence quote;
`line <= 0` renders without location (never guessed). The vote is a fixed
approve marker.

## evidence.Get

New `evidence.Store.Get(ctx, id) (EvidenceObject, []byte, error)`: reads blob
bytes for a stored ID; unknown ID or unreadable file errors. Added and tested
in this slice; consumed by the Slice 3 driver (the assessor itself takes the
review value in memory).

## Testing

Fake `PRCaller` + testutil stack with a directly `Ensure`d case: CLEAN→vote
proposal, FINDINGS→comments proposal (never approve), UNCERTAIN→hold, stale
commits→hold, findings-outside-reviewedFiles→hold, empty-findings
FINDINGS→hold, CLEAN-with-findings→mismatch hold, grant without the
comment/approve action→`grant-denies-*` hold, CI required green/red/missing,
decision blob content + kind, `Get` round-trip and unknown-ID error. No live
credentials.

## Scope boundary

No driver loop, no Work 2 materialization, no ADO write provider, no
reconciliation, no least-privilege narrowing. Shadow posture holds.

## Acceptance criteria

- Every completed review records exactly one durable outcome: vote proposal,
  comments proposal, limit-hold, or hold with reason.
- Gate failures hold even a CLEAN verdict; findings never yield approval;
  policy denials hold with reason instead of erroring.
- Decision blob is re-readable and sufficient for Slice 3 to publish without
  re-running the model.
