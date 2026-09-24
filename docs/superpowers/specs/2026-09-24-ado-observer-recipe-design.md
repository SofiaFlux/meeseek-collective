# ADO observer and review Work 1 — design

## Intent

Turn reviewer-assigned Azure DevOps pull requests into durable review Tasks:
periodic discovery through the read-only PR capabilities, one workflow case per
(mission, source, object, revision), and a materialized Work 1 review Task.
Assessment of review output and Work 2 effects are later slices.

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
- New `workflowcase.Find(missionID, source, objectID, revisionID)` lookup over
  the existing `UNIQUE (mission_id, source, object_id, revision_id)` index.
- CLI `run-observer` in `cmd/summa42-box` reuses `buildADOProviderFromEnv` and
  mirrors `runWorker` store wiring.

## Data flow

```text
Run(ctx, cfg, interval) → ObserveOnce(ctx, cfg) per tick
  ado.pr.org_active (nil or filter map, NEVER an action key)
    or ado.pr.list {"action":"list", project?, repository?, ...} when
    --project/--repository are set; page loop until empty page/absent token
  parse → exclude (draft/self/group-only/unclear, each with reason)
  workflowcase.Find(mission, "ado", repo#n, sourceCommit:targetCommit)
    hit  → reuse case, no new evidence row; Materialize (repairs case-no-Task)
    miss → Put canonical PR JSON (application/json, ado.pr.snapshot)
           → Ensure → MaterializeTask
  → ObserveResult{Ensured, Materialized, Excluded[{PR, Reason}], Failed[{PR, Error}]}
```

Find-first is load-bearing: `Ensure` compares the full initial request
including `EvidenceID`, so a fresh `Put` per tick would turn re-polls into
conflict errors instead of idempotent reuse. `Failed` covers Ensure-ok /
Materialize-failed (case exists, Task missing) for retry next tick.

## Review Work 1 contract

`--work-capability` (repeatable, default: grant capabilities) plus the grant
populate `Observation.FirstWork{Kind: "ado.pr.review", RequiredCapabilities,
AuthorityCeiling}`, with `ProposedActions` nil (Work 1 proposes no actions).
This must pass the `Decide` subset gates: ceiling ⊆ grant capabilities,
required ⊆ ceiling, actions ⊆ grant actions.

`MaterializeTask(ctx, execSvc, case.ID, case.CurrentWorkID, TaskRequest{
Objective, PayloadJSON, AcceptanceCriteria, ResourceEnvelopeID, ...})`.
The case overwrites Purpose, TaskClass (= NextWork.Kind), RequiredCapabilities,
AuthorityCeiling and IdempotencyKey (= CurrentWorkID) — do not set those five
on the template. Template preserves objective `Review ADO PR <repo>#<n>`,
payload `{repo, pr, sourceCommit, targetCommit}`, acceptance
`review evidence recorded for <rev>`, and the envelope.

## Flags

| Flag | Maps to |
|---|---|
| `--mission` (required) | `Observation.MissionID` (must satisfy `ValidatePurposeTx`) |
| `--reviewer-id` (required) | self/group-only filter identity |
| `--grant-capability/--grant-action` (repeatable) | `Grant{Capabilities, Actions}` |
| `--work-capability` (repeatable, default grant caps) | `FirstWork.RequiredCapabilities` |
| `--envelope` (required) | Task `ResourceEnvelopeID` |
| `--max-steps`, `--remaining-budget` (positive, required) | `Observation` limits + `Decide` |
| `--project`, `--repository` (optional pair) | `ado.pr.list` instead of `ado.pr.org_active` |
| `--poll-interval` (positive, required) | `Run` tick |

## Parser and paging

- `PullRequest{Repository, Number, Title, IsDraft, AuthorID, Reviewers[{ID,
  IsGroup}], SourceCommit, TargetCommit}`. Required: repository, number, both
  commits; missing → exclusion reason `unparseable-pr` (fail closed, visible).
- Canonical blob example (what gets Put, not the MCP wire shape):
  `{"repo":"shop","pr":42,"sourceCommit":"abc","targetCommit":"def",
  "draft":false,"author":"u1","reviewers":["u2"]}`.
- MCP wire-field mapping is plan-level detail; shapes follow the MCP TOOLSET
  and are confirmed in the live smoke follow-on. Unknown wire fields ignored.
- Paging: repeat the list call with identical filters plus the continuation
  token from the previous response while present; stop on empty page. Exact
  token field names confirmed live; the loop shape is fixed.

## Error handling

- List-call or evidence-store failure: tick error, `Run` aborts visibly
  (supervisor restarts, same as the worker loop); cancellation stops cleanly.
- Per-PR `Ensure` failure: exclusion with reason (PR identity known).
- `Ensure`-ok / `Materialize`-failed: `Failed` entry, retried next tick via
  Find-first. No partial state is silent.
- No `operations.Service` dispatches; the observer only reads ADO and writes
  evidence/cases/Tasks.

## Testing

Fake `PRCaller` plus testutil stack: discovery maps PRs to cases; revision
change opens a new case; re-poll reuses the case with no new evidence row;
partial case gains its Task on re-poll; each exclusion class yields its reason;
template/authority fields land per the contract above; tick error aborts `Run`;
cancellation stops it. Full `go test ./...` and `go vet` pass, no credentials.

## Scope boundary

No assessment of review output, no Work 2 (comments/votes), no Copilot
executor, no durable exclusion ledger (reasons live in the result), no
least-privilege narrowing beyond work-vs-grant capabilities. Shadow posture
holds: nothing publishes. Executor registration for `ado.pr.review` (needed
before the worker loop can lease these Tasks unattended) belongs to the
Copilot executor slice, not this one.

## Acceptance criteria

- One active case per (mission, source, object, revision); re-poll reuses it
  with zero new evidence rows; new commits open a new case.
- Every discovered PR ends materialized, excluded with reason, or failed with
  error — none silent.
- Materialized Task is `ELIGIBLE` with recipe objective/payload/acceptance/
  envelope and case-derived purpose/class/capabilities/authority.
- `Run` polls until cancelled and never dispatches external effects.
