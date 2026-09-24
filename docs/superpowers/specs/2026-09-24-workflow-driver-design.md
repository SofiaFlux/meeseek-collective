# Workflow driver (Work 2 orchestration) — design

## Intent

Close the orchestration gap between Work 1 assessment and Work 2 publication:
materialize the publish Task for cases carrying a publish decision, then assess
the completed publication (Assessment 2) with verified effects, so the case
either becomes ready for final verification or holds.

## Approaches considered

1. **Driver step in `adoreview` (selected):** one bounded pass per tick over
   active ADO cases; idempotent materialization via the case Work ID;
   Assessment 2 through the existing `Assess` contract.
2. **Generic assessor process (final design direction):** right long-term shape,
   but Work 2 must exist first; extraction is follow-on.
3. **Publisher executor self-assesses:** executors do not write case state.
   Rejected.

## Cross-cutting fix: ADO result decoding

`adomcp.Provider.Call` returns the raw `*mcp.CallToolResult`, but every
consumer (assessor gates, driver, adoeffects read funcs) asserts
`map[string]any`. Fix at the boundary: `Call` decodes the SDK result to
`map[string]any` (structured content first, text-JSON fallback), and errors
fail closed when neither carries an object. One decoder, all callers.

## APIs added

- `workflowcase.ListActive(ctx, missionID) ([]Case, error)` — ACTIVE cases of
  one mission, ordered by ID.
- `workflowcase.ListAssessments(ctx, caseID) ([]AssessmentRecord, error)` with
  `{ID, WorkID, RequestJSON, ResultJSON, CreatedAt}`; the driver selects the
  assessment whose stored result's `Case.CurrentWorkID` equals the case's
  current Work ID (the Continue that produced it) and extracts the
  `ado.review.decision` evidence ID from its request.
- `execution.FindByIdempotencyKey(ctx, key) (Task, bool, error)`.
- `workflowcase.Assessment(ctx, workID) (CompletionEvidence...)` — no; Work
  completion evidence is read through `runmanifest.Provenance(attemptID)`
  (existing: completion manifest → evidence IDs), not a new API.

## StepOnce flow (per active case, `Source == "ado"`, `NextWork.Kind ==
"publish-decision"`)

1. **Decision discovery:** find the producing assessment (above); require its
   decision evidence object of kind `ado.review.decision`; skip (not error)
   when absent — the case simply is not ready for Work 2 yet.
2. **Ordering guard:** the assessed Work 1 Task must be
   `AWAITING_VERIFICATION` and its completion manifest (via
   `runmanifest.Provenance(attemptID).OutputEvidence`) must contain the
   review evidence cited by the assessment. Otherwise skip.
3. **Project resolution:** Work 1 payload carries repo/PR/commits; missing
   project → one `ado.pr.get` call, fail closed on error.
4. **Materialize Work 2:** `MaterializeTask(case.ID, case.CurrentWorkID, …)`
   with payload `{decision, caseID, workID, project, repo, pr, revision}` —
   Work ID idempotency makes repeated ticks no-ops.
5. **Task-state matrix for the Work 2 Task:**
   - `ELIGIBLE`/`EXECUTING` → wait (skip).
   - `AWAITING_VERIFICATION` → Assessment 2.
   - `BLOCKED`/`CHALLENGED`/`CANCELLED`/`EXPIRED` → `Assess(Unknown, "work2
     <state>")` hold; a `FAILED` attempt with the Task back in `ELIGIBLE`
     repeats the same signature until the Task blocks.
6. **Assessment 2 (verified effects):** read the Work 2 completion evidence;
   each publisher entry is re-verified read-only through the matching
   adoeffects provider's `LookupOutcome` (never `Dispatch`). All intents
   confirmed AND the expected slot set matches the decision exactly →
   `Assess(Ready)` (case → `READY_FOR_VERIFICATION`). Any missing slot,
   unknown, skipped, or recorded-only entry → `Assess(Unknown)` with reason.
   `Assess` replays idempotently for identical requests.
7. **Budget:** case budget passes through unchanged; effect exposure is
   reserved by `operations.Prepare` (not by the scheduler).

## Loop and CLI

`Driver.Run(ctx, interval)` ticks `StepOnce`; cancellation stops cleanly
(mid-step context errors suppressed like worker/observer); step errors abort
visibly. `run-driver` wires `--mission`, `--envelope`, `--poll-interval`
(plus optional `--project` fallback) and uses the ADO provider readers for
verification lookups.

## Testing

Full testutil stack with fakes: decision discovery; ordering guard (early
assessment, unbound evidence); materialization idempotence; project
resolution; task-state matrix (wait, ready, blocked hold); Assessment 2
verified-effect paths (all-confirmed ready; unknown/skip hold; extra/missing
slot hold); Run cancellation; adomcp decoder (structured + text fallback).
No live credentials.

## Scope boundary

No final verification (next slice), no UNKNOWN reconciliation loop
(operations service owns it), no ADO branches outside `adoreview`/`adomcp`.
Shadow posture holds: the driver orchestrates; publishing stays mode-gated.

## Acceptance criteria

- A published Work 1 decision produces exactly one Work 2 Task; repeated ticks
  are idempotent.
- The case reaches `READY_FOR_VERIFICATION` only when every effect verifies
  by read-back; every other path holds with a reason.
- The driver never dispatches and never publishes by itself.
