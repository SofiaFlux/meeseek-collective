# ADO observer and review Work 1 — design

## Intent

Turn reviewer-assigned Azure DevOps pull requests into durable review Tasks:
periodic discovery through the new read-only PR capabilities, one workflow case
per PR revision, and a materialized Work 1 review Task the worker loop can
execute. Assessment of review output and Work 2 effects are later slices.

## Approaches considered

1. **adoreview library + ticker + CLI (selected):** `ObserveOnce` runs the full
   pass, `Run` loops it, `run-observer` wires flags. Fake-able source, no
   credentials in tests.
2. **Observer as worker-loop executor:** executors return evidence, not Tasks;
   forcing discovery through attempt machinery needs fake scaffolding. Rejected.
3. **Hook into wake STRATEGIC_PULSE:** wake is generic timer infrastructure;
   recipe logic inside it breaks the generic boundary. Rejected.

## Architecture

New package `internal/adoreview` (adomcp untouched):
- `source.go`: `PullRequest` type, listing via injected `PRCaller`
  (`Call(ctx, capability, request) (any, error)`, satisfied by `adomcp.Provider`),
  defensive parsing, exclusion filter with reasons.
- `observe.go`: `Config`, `ObserveOnce`, `Run`, `ObserveResult`.
- CLI `run-observer` in `cmd/summa42-box` reuses `buildADOProviderFromEnv`.

## Data flow

```text
Run(ctx, cfg, interval) → ObserveOnce(ctx, cfg) per tick
  pr.org_active (or pr.list with --project/--repository)
  parse → exclude (draft/self/group-only/unclear, each with reason)
  per-PR canonical JSON blob → evidence ID
  workflowcase.Ensure(source=ado, object=repo#n, revision=sourceCommit:targetCommit)
  workflowcase.MaterializeTask(review template)
  → ObserveResult{Ensured, Materialized, Excluded[{PR, Reason}]}
```

Parser requires PR identity and both commits; unknown fields ignored. Result
shapes follow the MCP TOOLSET; live validation happens in the smoke follow-on.
Unclear reviewer assignment excludes conservatively with reason
`cannot-confirm-direct-assignment`.

## Review Work 1 template

`FirstWork{Kind: "ado.pr.review"}`; required capabilities from
`--work-capability` (default: grant capabilities), ceiling from the grant.
Task template: objective `Review ADO PR <repo>#<n>`, payload
`{repo, pr, sourceCommit, targetCommit}`, acceptance
`review evidence recorded for <rev>`, envelope from `--envelope`.
Mission, grant, limits and reviewer come from CLI flags.

## Error handling

- `Run` aborts visibly on `ObserveOnce` error (supervisor restarts, same as the
  worker loop); cancellation stops cleanly with no new poll.
- Per-PR failures (parse, evidence, Ensure, Materialize) are collected into the
  result as exclusions with reasons where the PR is known, otherwise returned
  as the tick error. No partial case without evidence.
- No `operations.Service` dispatches; the observer only reads ADO and writes
  evidence/cases/Tasks.

## Testing

Fake `PRCaller` plus testutil stack: discovery maps PRs to cases, revision
change creates a new case, same revision reuses it (idempotent), each exclusion
class yields its reason, template fields land on the materialized Task, tick
error aborts `Run`, cancellation stops it. Full `go test ./...` and `go vet`
pass with no live credentials.

## Scope boundary

No assessment of review output, no Work 2 (comments/votes), no Copilot
executor, no durable exclusion ledger (reasons live in the result for now), no
least-privilege narrowing beyond work-vs-grant capabilities. Shadow posture
holds: nothing publishes.

## Acceptance criteria

- One active case per PR revision; re-poll reuses it; new commits open a new one.
- Every discovered PR is either materialized or excluded with a stated reason.
- Materialized Task carries objective/payload/acceptance/envelope from the recipe
  and authority from the case step; worker loop can lease it unattended.
- `Run` polls until cancelled and never dispatches external effects.
