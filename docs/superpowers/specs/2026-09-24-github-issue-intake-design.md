# GitHub issue intake — design

## Intent

Let the Box notice GitHub issues opened by configured maintainers and register
each eligible issue revision as a durable workflow case with a triage Task.
First sub-project of the GitHub issue-to-PR role; planning, code writing,
review and PR creation follow. Draft PR, human merges. Maintainer trust is an
explicit configured login list — never inferred from roles or labels.

## Architecture

New package `internal/ghissue` (mirrors `adoreview`):

- `source.go`: `Issue`, `IssueLister`, `Unparseable`, parse + filter with
  reason tokens, canonical snapshot.
- `results.go`: `ExcludedIssue` (shared by source filtering and the observer).
- `observe.go`: `ObserveConfig` (distinct name — `Config` belongs to the
  client in client.go), `ObserveResult`, `FailedIssue`, `ObserveOnce`, `Run`.
- `client.go`: read-only GitHub transport. Reuses the exported
  `feedbackgithub.FileCredentialSource` pattern for tokens, but as a SEPARATE
  client exposing GET operations only.

  `ghissue.Config{BaseURL, Repository, TokenFile, Timeout, MaxResponseBytes}`.
  Env contract: `SUMMA42_GITHUB_TOKEN_FILE` (token file path; required —
  mirroring the feedback file-only credential, NOT an inline token) and
  `SUMMA42_GITHUB_REPOSITORY` (`owner/name`). `BaseURL` defaults to
  `https://api.github.com`; the host must be `api.github.com` over HTTPS, or
  an explicit loopback endpoint for tests — any other host or scheme is a
  startup error, so bearer credentials can never be aimed elsewhere. The base
  URL carries no path, query or fragment; redirects are never followed, so the
  token stays pinned to the configured origin; Link resolution is
  same-origin only.

  Exported surface is exactly:
  `New(Config) (*Client, error)`, `(*Client).ListIssues(ctx, cursor string)
  ([]any, []Unparseable, string, error)` and `(*Client).Name() string`. No
  generic request method, no POST/PATCH/DELETE exists on the type, so writes
  are structurally unreachable. Each page: `GET /repos/{repo}/issues?state=open&per_page=100[&page=N]`;
  response body capped at `MaxResponseBytes`; 401/403/5xx → sanitized errors
  (no token echo); timeout via context. Pagination: the `Link: rel="next"`
  header; no link ends the loop; a malformed link, a cross-origin link, or a
  repeated `next` URL is an error (fail closed).

  `Name()` returns the configured repository in `owner/name` form; the
  collector uses it to qualify object IDs. `ListIssues` splits each page: an
  item missing a required wire field becomes one `Unparseable` (processing
  continues), the remaining items are returned raw. The required-field check
  is the single shared helper `classifyWireItem` in source.go, reused by
  `ParseIssue`, so client and parser cannot disagree. Pagination loops and
  repeated-cursor detection live in the collector (`CollectIssues`), keeping
  the client stateless.

## Flow (exact order — evidence before Ensure)

`Find(mission, "github", objectID, revision)` →
- **miss:** `Put` canonical snapshot (`application/json`,
  `github.issue.snapshot`) → atomic `EnsureAndMaterialize` (new
  `workflowcase` method: case insert + Task creation in ONE transaction, so a
  Task failure rolls the case back and leaves no unrepairable partial state);
- **hit:** `MaterializeTask` only, with payload `issueSnapshot` =
  `existing.ObservationEvidenceID` (the case's durable pointer — the hit path
  creates NO new evidence, so this is the only correct source);
- **hit but case state ≠ `ACTIVE`:** skip with reason `case-not-active`
  (only ACTIVE cases can materialize).

`EnsureAndMaterialize(ctx, executionSvc, observation, template)` lives in
`workflowcase` and reuses a new transaction-aware execution primitive
`CreateTaskWithGuardInTx(ctx, tx, request, guard)` inside the SAME transaction
as the case insert. `CreateTaskWithGuard` keeps its own-transaction behavior
by wrapping `CreateTaskWithGuardInTx` in `store.WithTx` — SQLite runs a single
connection (`db.SetMaxOpenConns(1)`), so a nested `WithTx` would deadlock
instead of composing. The ADO observer's miss path switches to
`EnsureAndMaterialize` too, closing the same partial-case window there.

A fresh `Put` per poll would break `Ensure`'s exact-request replay, hence
Find-first.

- `objectID` = `github:<owner>/<name>#<n>` (repository-qualified: plain
  `issue#n` collides across repositories under the UNIQUE
  (mission, source, object, revision) key).
- `revision` = `updated_at` parsed to UTC RFC3339Nano (single canonical form).
- `FirstWork{Kind: "github.issue.triage", RequiredCapabilities:
  ["github.issue.read"] merged with any extra `--work-capability` values,
  AuthorityCeiling: the configured grant capabilities, ProposedActions: nil}`.
  `--work-capability` is ADDITIONAL only; `github.issue.read` is always
  included. STARTUP VALIDATION: every effective work capability (the merged
  set) must be listed in the grant, and the grant must contain
  `github.issue.read` — a violation is a startup error, never a per-issue
  `Decide`/`Ensure` failure at runtime. Maintainer logins, grant capabilities,
  grant actions and work capabilities are trimmed and deduplicated once
  before validation, so the grant ceiling and the required capabilities passed
  to `workflow.Decide` always compare identical tokens. The lister's
  `Name()` must match the configured repository (case-insensitively) or the
  tick fails before any write.

## Wire mapping and canonical snapshot

Required per issue: `number` (positive integer), `title`, `state`,
`user.login`, `updated_at` (RFC3339), `html_url` — each must be present and
non-blank, otherwise the item is `unparseable`. `body`, `assignees` and
`labels` may be missing or `null` and normalize to `""`/`[]`/`[]`; when
present they must have the right shape (string, array of objects, array of
objects with the expected string field) or the item is `unparseable` — a
malformed field is never allowed to abort a page. `state` must be
`open` — closed issues are excluded (token `not-open`) before any other
maintainer filter. Items carrying `pull_request` are skipped as PRs
(counted in `ObserveResult.PullRequestsSkipped`; a PR is not a candidate and
produces no exclusion row). `assignees`/`labels` are
normalized to `[]`, deduplicated and sorted case-insensitively; `body`
defaults to `""`; `updatedAt` in the snapshot is the canonical UTC revision
string (identical to the case revision).

Triage classification (deterministic; recorded for the follow-on slice, never
acted on here): labels/prefixes compared trimmed and case-insensitively; label
`bug` or `defect`, or title prefix `[bug]` → `bug`; label `enhancement` or
`feature`, or title prefix `[feat]`/`[feature]` → `feature`; bug wins on a tie
(checked first); else `unclassified`.

Canonical snapshot JSON (exact keys, all present, `updatedAt` is the canonical
UTC revision string identical to the case revision):
`{repo, issue, title, body, author, assignees, labels, url, updatedAt, triage}`.
Labels and assignees sorted case-insensitively and deduplicated; null
`assignees`/`labels`/`body` normalized to `[]`/`[]`/`""`.

Task payload: `{repo, issue, revision, title, url, author, labels, triage,
issueSnapshot}` where `issueSnapshot` is the `github.issue.snapshot` evidence
ID (new blob on miss, `Case.ObservationEvidenceID` on hit) — the executor
resolves the body and full detail through that evidence, so the payload does
not inline `body`. Objective `Triage GitHub issue <owner>/<name>#<n>`;
acceptance `triage decision recorded for <revision>`.

## Filter (reason tokens, in this precedence)

1. item carries `pull_request` — counted in `ObserveResult.PullRequestsSkipped`
   (never a candidate, no exclusion row).
2. `unparseable-issue` — missing required fields (one exclusion per bad item;
   parsing continues for the rest of the page — mirror the ADO
   `Unparseable` split).
3. `not-open` — `state != "open"` (belt-and-braces: the client already asks
   for open issues).
4. `not-maintainer` — author login ∉ configured maintainer logins (exact,
   case-insensitive).
5. `already-assigned` — `assignees` non-empty.
6. `held-by-label` — label `box-hold` or `wontfix` (exact, case-insensitive).
7. otherwise candidate (recorded with its triage class).

## Executor boundary (why nothing runs)

`github.issue.triage` requires `github.issue.read`; no executor in this slice
advertises it. `workerCapacity` derives its capability map from registered
executor-kind keys only, and `scheduler.Next` selects only tasks whose
`RequiredCapabilities` are present in that map. In the `run-gh-intake`
composition no executor is registered at all, so the Task is `ELIGIBLE` but
never selected. Scope of the guarantee: this CLI composition (the scheduler
itself trusts the supplied snapshot — a hypothetical capacity map containing
`github.issue.read` could lease it). The follow-on triage executor will
advertise that capability explicitly.

## Configuration and CLI

`run-gh-intake --mission --repo <owner/name> --maintainer <login>
(repeatable) --grant-capability <id> (repeatable; must include
`github.issue.read`) --work-capability <id> (additional caps only) --envelope
--max-steps --remaining-budget --poll-interval`. Startup errors: missing
mission/repo/maintainer/envelope, empty grant, grant without
`github.issue.read`, any work capability not grant-listed, non-positive
limits/interval. Repository precedence: `--repo` wins when both it and
`SUMMA42_GITHUB_REPOSITORY` are set. ADO env is NOT required.

Composition is read-only: feedback forced to local-only/disabled, no
feedback sink, no operation providers, no executor registered (contrast
`runObserver`). Test asserts the opened Box has no feedback emitter/provider
and an empty executor map.

## Error handling

| Condition | Result |
|---|---|
| list/page/transport/malformed-Link/repeated-cursor error | tick-level error: fatal on the first tick, absorbed behind the bounded backoff afterwards |
| unparseable item | exclusion `unparseable-issue`, processing continues |
| `EnsureAndMaterialize` error (atomic: no case, no Task; the snapshot blob may exist as an orphan, which is tolerated) | exclusion `ensure-failed` |
| hit path `MaterializeTask` error | per-issue error: `Failed` entry, reported every tick and retried on the next tick while the issue is still eligible — never fatal |
| `Find` error (store) | per-issue error: reported every tick, backed off and retried on the next tick — never fatal |
| `Put` error (disk) | per-issue error: reported every tick, backed off and retried on the next tick — never fatal; no case without evidence |
| hit, case state ≠ `ACTIVE` | skip `case-not-active` |
| context cancelled mid-tick | Run returns nil |



## Testing

Fake lister + full testutil stack: only maintainer issues become cases; each
reason token (`unparseable-issue`, `not-open`,
`not-maintainer`, `already-assigned`, `held-by-label`); mixed page with
parsed + unparseable items (both handled, processing continues); revision
change (`+00:00` → `Z` offset) opens a second case and proves UTC
normalization; re-poll is a no-op with unchanged snapshot count and identical
payload (`issueSnapshot` from `Case.ObservationEvidenceID`); cross-repository
collision test (`owner-a/repo` and `owner-b/repo` issue #7 are distinct
cases); grant
validation errors; `case-not-active` for BLOCKED/READY/CLOSED; a scheduler
test proving an unrelated executor with capacity lacking
`github.issue.read` never claims the Task (eligible-but-unclaimed); CLI
composition test asserting no feedback emitter/provider and an empty executor
map. Fake-HTTP client tests: GET-only surface (compile-level: no write
methods), `state=open` query parameter, Link pagination, repeated/cross-
origin/malformed Link errors, auth header present, non-2xx sanitized,
response cap.

## Scope boundary

No code writing, planning, PRs, comments, label mutation, or any GitHub
write. No triage decision (follow-on slice).

## Acceptance criteria

- One active case per (mission, `github`, repo-qualified object, revision);
  edits create new revisions; repeats are no-ops without new evidence.
- Every issue ends materialized, excluded with a reason, or failed; PRs are
  skipped explicitly.
- The Task cannot be leased by any executor registered in the `run-gh-intake`
  composition, and the intake path performs no GitHub writes.
