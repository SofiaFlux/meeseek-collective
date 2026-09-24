# Workflow driver (Work 2 orchestration) — design

## Intent

Close the orchestration gap between Work 1 assessment and Work 2 publication:
materialize the publish Task when a case carries a publish decision, and assess
the completed publication (Assessment 2) so the case either becomes ready for
final verification or holds.

## Approaches considered

1. **Driver step in `adoreview` (selected):** one bounded pass per tick over
   active cases; idempotent materialization via the case Work ID; Assessment 2
   through the existing `Assess` contract.
2. **Assessment/verification worker (final design):** a generic assessor
   process — larger, and Work 2 must exist first. This is the concrete ADO
   driver; generic extraction is follow-on.
3. **Publisher executor self-assesses:** executors do not write case state;
   assessment is a separate durable decision. Rejected.

## APIs added

- `workflowcase.ListActive(ctx) ([]Case, error)` — ACTIVE cases ordered by ID.
- `execution.FindByIdempotencyKey(ctx, key) (Task, bool, error)` — Work-ID
  lookup; the driver uses it to find the current Work Task without a
  second index.

Both are thin, tested, and mirror the existing store-scan patterns.

## StepOnce flow

For every active case with `NextWork.Kind == "publish-decision"`:

1. **Guard:** the Work 1 assessment must propose a publish decision. The case
   only reaches this state via the assessor, so a hold (UNKNOWN) never lands
   here; a defensive check skips non-publish kinds and cases with
   `CurrentWorkID == ""`.
2. **Project resolution:** the Work 1 Task payload carries repo/PR/commits; if
   `project` is absent the driver calls `ado.pr.get` once and records it in the
   Work 2 payload (fail-closed on error).
3. **Work 2 Task:** `MaterializeTask(case.ID, case.CurrentWorkID, template)`
   with the Work 2 payload and recipe objective/acceptance/envelope. The Work
   ID is the idempotency key, so repeated ticks never duplicate the Task.
4. **Assessment 2:** when the Work 2 Task is `AWAITING_VERIFICATION`, read its
   completion evidence (completion manifest → `evidence.Get`, kind
   `AGENT_MESSAGE` from the publisher). All entries confirmed →
   `Assess(Ready)` (case becomes `READY_FOR_VERIFICATION` for final
   verification). Any `unknown`/`skipped`/`recorded-only` → `Assess(Unknown)`
   with a reason (hold). Unreadable evidence → hold.
5. **Budget:** `RemainingBudget` passes the case budget through unchanged;
   real spend is enforced by the resource envelope at schedule/lease time.

## Loop and CLI

`Driver.Run(ctx, interval)` ticks `StepOnce`; context cancellation stops
cleanly (mid-step context errors are suppressed like worker/observer). A step
error aborts visibly for supervisor restart. `run-driver` in
`cmd/summa42-box` wires `--mission`, `--envelope`, `--budget`, `--interval`
and optional `--project` fallback through the existing runtime composition.

## Testing

Full testutil stack with fakes: Work 2 materialization from a published case;
idempotent re-tick (same Task ID, one evidence); Assessment 2 ready path
(all confirmed) and hold paths (unknown entry, unreadable evidence); project
resolution via `ado.pr.get`; guard against non-publish cases; Run cancellation.
No live credentials.

## Scope boundary

No final verification (next slice), no UNKNOWN reconciliation loop (operations
service owns it), no ADO-specific branches outside `adoreview`. Shadow posture
holds: the driver only orchestrates; publishing is gated by the publisher mode.

## Acceptance criteria

- A published Work 1 decision produces exactly one Work 2 Task, and repeated
  driver ticks are idempotent.
- Completed publication moves the case to `READY_FOR_VERIFICATION` only when
  every effect confirmed; anything else holds with a reason.
- Driver never publishes by itself and never runs effects.
