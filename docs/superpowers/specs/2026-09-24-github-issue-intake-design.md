# GitHub issue intake — design

## Intent

Let the Box notice GitHub issues opened by configured maintainers and register
each eligible issue revision as a durable workflow case with a triage Task.
First sub-project of the GitHub issue-to-PR role; planning, code writing,
review and PR creation follow. Draft PR, human merges. Maintainer trust is an
explicit configured ID list — never inferred from roles or labels.

## Architecture

New package `internal/ghissue` (mirrors `adoreview`):

- `source.go`: `Issue`, `IssueLister`, `Unparseable`, `ExcludedIssue`,
  parse + filter with reason tokens, canonical snapshot.
- `observe.go`: `Config`, `ObserveResult`, `ExcludedIssue`/`FailedIssue`,
  `ObserveOnce`, `Run`.
- `client.go`: read-only GitHub transport. Reuses the exported
  `feedbackgithub.FileCredentialSource` pattern for tokens, but as a SEPARATE
  client exposing GET operations only.

  `ghissue.Config{BaseURL, Repository, TokenFile, Timeout, MaxResponseBytes}`.
  Env contract: `SUMMA42_GITHUB_TOKEN_FILE` (token file path; required —
  mirroring the feedback file-only credential, NOT an inline token) and
  `SUMMA42_GITHUB_REPOSITORY` (`owner/name`). `BaseURL` defaults to
  `https://api.github.com`; any other scheme/host is a startup error
  (HTTPS enforced, same-origin Link resolution only).

  Exported surface is exactly:
  `New(Config) (*Client, error)`, `(*Client).ListIssues(ctx, cursor string)
  ([]any, []Unparseable, string, error)` and `(*Client).Name() string`. No
  generic request method, no POST/PATCH/DELETE exists on the type, so writes
  are structurally unreachable. Each page: `GET /repos/{repo}/issues?state=open&per_page=100[&page=N]`;
  response body capped at `MaxResponseBytes`; 401/403/5xx → sanitized errors
  (no token echo); timeout via context. Pagination: the `Link: rel="next"`
  header; no link ends the loop; a malformed link, a cross-origin link, or a
  repeated `next` URL is an error (fail closed).

## Flow (exact order — evidence before Ensure)

`Find(mission, "github", objectID, revision)` →
- **miss:** `Put` canonical snapshot (`application/json`,
  `github.issue.snapshot`) → `Ensure` with that evidence ID →
  `MaterializeTask` (payload `issueSnapshot` = the new blob ID);
- **hit:** `MaterializeTask` only, with payload `issueSnapshot` =
  `existing.ObservationEvidenceID` (the case's durable pointer — the hit path
  creates NO new evidence, so this is the only correct source);
- **hit but case state ≠ `ACTIVE`:** skip with reason `case-not-active`
  (only ACTIVE cases can materialize).

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
  included. The grant must contain `github.issue.read` — validated at
  startup (empty/missing grant is a startup error, never a per-issue
  failure).

## Wire mapping and canonical snapshot

Required per issue: `number`, `title`, `body`, `state`, `user.login`,
`assignees[]`, `labels[].name`, `updated_at`, `html_url`. `state` must be
`open` — closed issues are excluded (token `not-open`) before any other
maintainer filter. Items carrying `pull_request` are skipped as PRs
(reason `is-pull-request`, counted in the result). `assignees`/`labels` are
nil-normalized to `[]`, deduplicated and sorted case-insensitively; `body`
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

1. `is-pull-request` — item carries `pull_request` (counted separately in the
   result; never a candidate).
2. `unparseable-issue` — missing required fields (one exclusion per bad item;
   parsing continues for the rest of the page — mirror the ADO
   `Unparseable` split).
3. `not-open` — `state != "open"` (belt-and-braces: the client already asks
   for open issues).
4. `not-maintainer` — author login ∉ configured maintainer list (exact,
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

`run-gh-intake --mission --repo <owner/name> --maintainer-id <id>
(repeatable) --grant-capability <id> (repeatable; must include
`github.issue.read`) --work-capability <id> (additional caps only) --envelope
--max-steps --remaining-budget --poll-interval`. Startup errors: missing
mission/repo/maintainer/envelope, empty grant, grant without
`github.issue.read`, non-positive limits/interval. ADO env is NOT required.

Composition is read-only: feedback forced to local-only/disabled, no
feedback sink, no operation providers, no executor registered (contrast
`runObserver`). Test asserts the opened Box has no feedback emitter/provider
and an empty executor map.

## Error handling

| Condition | Result |
|---|---|
| list/page/transport/malformed-Link error | tick error (Run aborts) |
| unparseable item | exclusion `unparseable-issue`, processing continues |
| `Ensure` error (mission invalid, etc.) | exclusion `ensure-failed` |
| Ensure ok, `MaterializeTask` error | `Failed` entry, retried next tick |
| `Find` error (store) | tick error |
| `Put` error (disk) | tick error — no case without evidence |
| hit, case state ≠ `ACTIVE` | skip `case-not-active` |
| context cancelled mid-tick | Run returns nil |



## Testing

Fake lister + full testutil stack: only maintainer issues become cases; each
reason token (`is-pull-request`, `unparseable-issue`, `not-open`,
`not-maintainer`, `already-assigned`, `held-by-label`); mixed page with
parsed + unparseable items (both handled, processing continues); revision
change (`+00:00` → `Z` offset) opens a second case and proves UTC
normalization; re-poll is a no-op with unchanged snapshot count and identical
payload (`issueSnapshot` from `Case.ObservationEvidenceID`); cross-repository
collision test (`repo-a` and `repo-b` issue #1 are distinct cases); grant
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
