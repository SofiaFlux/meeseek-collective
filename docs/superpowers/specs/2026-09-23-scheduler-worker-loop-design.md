# Scheduler worker loop — design

## Intent

Close the "no production worker loop" gap: a generic, unattended daemon that turns
`ELIGIBLE` Tasks into completed or failed Attempts using registered executors.
No recipe-specific branches; ADO/Copilot specifics stay in later slices.

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
services and adds no storage:

- `Scheduler` for `Next` / `ChooseExecutor` / `Lease`
- executor registry `map[string]executors.Executor` (eligible kinds = registry keys)
- `evidence.Store` for persisting executor output blobs
- `verification.Service` for `CompleteAttempt` on success
- `execution.Service` for `FailAttempt` on executor failure

A thin `run-worker` subcommand in `cmd/summa42-box` wires poll interval and lease
duration through the existing bootstrap. No PR-specific branches in the CLI.

## Data flow

```text
Run(ctx, interval) → StepOnce(ctx, capacity) per tick
  Next(capacity) → none: Idle
  ChooseExecutor(task, registry keys) → Lease(task, kind)
  AttemptEnvelope{task/attempt IDs, objective, payload, criteria,
                  capabilities, resource envelope}
  executor.Start(ctx-with-timeout=lease, envelope)
  success → Put(stdout/stderr/evidence blobs) → verification.CompleteAttempt
  executor error → execution.FailAttempt (class derived from error type)
```

## Error handling

- Lease lost mid-execution: the guarded completion rejects; the worker records and
  moves on. No blind retry — reconciliation is a later slice.
- Executor panic: recovered and converted to `FailAttempt`, never crashing the loop.
- Context cancelled: the current step finishes, no new lease is taken, `Run` returns.
- Unknown executor kind: `ChooseExecutor` fails before any lease, so no orphan attempt.

## Testing

Fake `Executor` plus testutil SQLite: idle when no candidate, success path records an
evidence manifest via `CompleteAttempt`, executor error yields `FailAttempt`, and an
empty registry yields an error without leasing. Full `go test ./...` and `go vet`
must pass; no live credentials required.

## Scope boundary

This slice does not assess executor output (a separate orchestrator slice owns
Assessment → next work), adds no concurrency, no ADO/Copilot specifics, and no CLI
resource-envelope allocation. Shadow-mode rollout and live-machine smoke tests from
the approved workflow design remain follow-on work.

## Acceptance criteria

- `Run` drives `ELIGIBLE` tasks to terminal attempt states unattended and stops
  cleanly on cancellation.
- A held or failed task never dispatches consequential writes by itself; executors
  only return evidence.
- Restarting the loop never duplicates evidence blobs (content-addressed) or
  completions (guarded, idempotent).
