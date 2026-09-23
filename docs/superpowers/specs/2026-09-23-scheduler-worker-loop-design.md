# Scheduler worker loop — design

## Intent

Close the "no production worker loop" gap: a generic, unattended daemon that turns
`ELIGIBLE` Tasks into completed or failed Attempts using registered executors.
No recipe-specific branches; ADO/Copilot specifics stay in later slices.

This slice intentionally stops at attempt completion. Successful tasks accumulate in
`AWAITING_VERIFICATION` with `PENDING` verification work; intermediate Assessment
and final Acceptance belong to a later orchestrator slice. The worker performs zero
`operations.Service` dispatches (evidence, completion and failure records only),
so the shadow posture from the approved workflow design is preserved.

## Approaches considered

1. **Sequential polling daemon with `StepOnce` + `Run` (selected):** one iteration
   per tick (`Next → ChooseExecutor → Lease → execute → evidence →
   Complete/Fail`); `Run` loops until context cancellation. Deterministic under
   SQLite and lease/fence; fully testable with fake executors and no credentials.
2. **Cron-driven single step:** no long-lived process, but gaps between runs race
   lease expiry and push ops burden onto the user. Rejected.
3. **Concurrent worker pool:** higher throughput, but SQLite write contention and
   lease races make correctness harder to prove. Deferred (YAGNI).

## Architecture

New `internal/scheduler/worker.go` (+ `worker_test.go`). `Worker` composes existing
services and adds no new tables (it writes evidence, completion and failure rows
only through those services):

- `Scheduler` for `Next` / `ChooseExecutor` / `Lease`
- executor registry `map[string]executors.Executor` (eligible kinds = registry keys;
  in production built from `Box.Executors`)
- `evidence.Store` for persisting executor output blobs
- `verification.Service` for `CompleteAttempt` on success
- `execution.Service` for `FailAttempt` on executor failure

`StepOnce(ctx, capacity)` performs one iteration and returns an outcome
(`Idle` / `Completed` / `Failed`) without leasing when there is nothing to do.
`Run(ctx, capacity, interval)` invokes `StepOnce` per tick until `ctx` is done.
`capacity` is rebuilt per tick from static worker config; there is no live
capability probing in this slice, so a task whose `RequiredCapabilities` are absent
from the map is skipped every tick (exact config shape pinned in the plan).

`ChooseExecutor` runs before any lease, so an unknown kind fails without an orphan
attempt. Worker tests construct the `Scheduler` with a nil preference for
determinism; with the default Box wiring the experience-service preference may
override the sorted baseline.

## Data flow

```text
Run(ctx, capacity, interval) → StepOnce(ctx, capacity) per tick
  Next(ctx, capacity) → (nil, nil): Idle outcome, no lease
  ChooseExecutor(task, registry keys) → Lease(task.ID, kind)
  AttemptEnvelope{TaskID, AttemptID, Objective, PayloadJSON,
                  AcceptanceCriteria, Workspace, VisibleCapabilities,
                  ResourceEnvelopeID}
    VisibleCapabilities = task.RequiredCapabilities
    ResourceEnvelopeID  = task.ResourceEnvelopeID
    Workspace           = <WorkspaceRoot>/<attemptID>, created per step
  executor.Start(ctx-with-timeout=leaseDuration, envelope)
  ExitCode == 0 → success path below; ExitCode != 0 or error → failure path
  success → Put blobs → verification.CompleteAttempt(manifest = evidence IDs)
  failure → execution.FailAttempt (mapping table below)
```

## Evidence mapping

- `Stdout` (if non-empty) → `Put` as `text/plain` / kind `STDOUT`
- `Stderr` (if non-empty) → `Put` as `text/plain` / kind `STDERR`
- each `result.Evidence[i]` → `Put` as `text/plain` / kind = its `EvidenceKind`
- IDs collected in deterministic order: stdout, stderr, then `Evidence` in order
- empty output (no blobs): the success path is impossible because `CompleteAttempt`
  requires non-empty evidence, so the worker takes the failure path with empty
  `evidenceIDs` instead

## Failure mapping

| Trigger | `FailureClass` | `signature` | `evidenceIDs` |
|---|---|---|---|
| executor error, non-zero exit, empty output on success path | `FailureExecution` | `worker:<kind>:<taskID>` | IDs of blobs already `Put`, possibly empty |
| executor panic (recovered, loop survives) | `FailureExecution` | `worker:panic:<kind>:<taskID>` | as above |

A repeated signature moves the task to `BLOCKED` per `FailAttempt` semantics, which
bounds retries without a separate counter.

## Error handling

- Lease lost mid-execution: the guarded completion/failure rejects with a
  stale/lease error; the worker logs the attempt's evidence IDs and moves on.
  Already-`Put` blobs stay unreferenced — orphan blobs are expected and tolerated.
  No retry in the same tick, no blind repeat. No `RenewLease` in this slice.
- Context cancelled: the current step finishes, no new lease is taken, `Run` returns.
- The worker dispatches no `operations.Service` effects and grants executors only
  `VisibleCapabilities = task.RequiredCapabilities` (subset of the authority ceiling
  already enforced by scheduler eligibility).

## CLI

`main()` currently ignores `os.Args` and always serves the control plane.
`run-worker [--poll-interval] [--lease-duration]` adds `os.Args` dispatch: it opens
the Box through the shared `Open` (skipping the control server), builds the
registry from `Box.Executors`, and runs `Worker.Run` until `SIGINT`/`SIGTERM`.
Defaults pinned in the implementation plan.

## Testing

Fake `Executor` plus testutil SQLite: idle on `(nil, nil)`, success records an
evidence manifest via `CompleteAttempt`, executor error yields `FailAttempt` with
the mapped class/signature, empty registry errors without leasing, and cancellation
stops the loop without a new lease. Full `go test ./...` and `go vet` must pass;
no live credentials required.

## Scope boundary

This slice does not assess executor output, adds no concurrency, no ADO/Copilot
specifics, no lease renewal, and no CLI resource-envelope allocation. Tasks rest in
`AWAITING_VERIFICATION` until the Assessment/Verification slice arrives. Shadow-mode
rollout and live-machine smoke tests from the approved workflow design remain
follow-on work.

## Acceptance criteria

- `Run` drives `ELIGIBLE` tasks unattended: success → task `AWAITING_VERIFICATION`
  with `PENDING` verification work; failure → `ELIGIBLE` (retryable) or `BLOCKED`
  (repeat signature). The loop never produces `SUCCEEDED` (no `AcceptTask`).
- Completion is at-most-once and guarded: a retry after success surfaces a
  stale/lease error, never a second completion record. Blob bytes are
  content-addressed; evidence DB rows are not deduped.
- A held or failed task never dispatches consequential writes; executors only
  return evidence, and the worker touches no `operations.Service`.
- Cancellation stops the loop cleanly with no new lease.
