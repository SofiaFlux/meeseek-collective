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
  `feedbackgithub.FileCredentialSource` pattern for tokens (bearer, token
  file/env), but is a SEPARATE client exposing GET operations only
  (`listIssues`, `getIssue`) — no POST/PATCH/DELETE methods exist on the type,
  so the write path is structurally unreachable. Pagination follows the
  GitHub `Link: rel="next"` header; a missing link ends the loop; a repeated
  URL guard prevents loops.

## Flow (exact order — evidence before Ensure)

`Find(mission, "github", objectID, revision)` → on miss: `Put` canonical
snapshot (`application/json`, kind `github.issue.snapshot`) → `Ensure` with
that evidence ID → `MaterializeTask`; on hit: `MaterializeTask` only (repairs
a case without a Task, no new evidence). A fresh `Put` per poll would break
`Ensure`'s exact-request replay, hence Find-first.

- `objectID` = `github:<owner>/<name>#<n>` (repository-qualified: plain
  `issue#n` collides across repositories under the UNIQUE
  (mission, source, object, revision) key).
- `revision` = `updated_at` parsed to UTC RFC3339Nano (single canonical form).
- `FirstWork{Kind: "github.issue.triage", RequiredCapabilities:
  ["github.issue.read"], AuthorityCeiling: from the configured grant}` —
  non-empty and subset-checked by `Decide` inside `Ensure`.

## Wire mapping and canonical snapshot

Required per issue: `number`, `title`, `body`, `state`, `user.login`,
`assignees[]`, `labels[].name`, `updated_at`, `html_url`; `pull_request`
present → skipped as a PR (counted, not a candidate). Canonical snapshot JSON
keys (stable, sorted labels): `{repo, issue, title, body, author,
assignees[], labels[], url, updatedAt}`.

Triage classification (deterministic, recorded for the follow-on slice, never
acted on here): label `bug`/`defect` or title prefix `[bug]` → `bug`;
`enhancement`/`feature` or `[feat]`/`[feature]` → `feature`; else
`unclassified`. Classification lands in the snapshot and the Task payload.

Task payload: `{repo, issue, revision, title, url, author, labels[],
triage, issueSnapshot: <evidenceID>}`; objective `Triage GitHub issue
<owner>/<name>#<n>`; acceptance `triage decision recorded for <revision>`.

## Filter (reason tokens, in order)

1. `not-maintainer` — author login ∉ configured maintainer list.
2. `already-assigned` — `assignees` non-empty.
3. `held-by-label` — label `box-hold` or `wontfix` (exact, case-insensitive).
4. `unparseable-issue` — missing required fields.
5. otherwise candidate (recorded with its triage class).

## Executor boundary (why nothing runs)

`github.issue.triage` requires capability `github.issue.read`, which no
registered executor advertises (`workerCapacity` advertises executor-kind
capabilities only). With no triage executor in this slice, the Task stays
`ELIGIBLE` but is never selected — enforced structurally and covered by a
scheduler test (eligible-but-unclaimed). The follow-on triage executor will
register the capability.

## Configuration and CLI

`run-gh-intake --mission --repo <owner/name> --maintainer-id <id>
(repeatable) --grant-capability <id> (repeatable; must include
`github.issue.read`) --work-capability <id> (default: grant caps) --envelope
--max-steps --remaining-budget --poll-interval`. Missing mission/repo/
maintainer/envelope, empty grants, or non-positive limits are startup errors.

Composition is read-only: feedback forced to local-only/disabled, no
feedback sink, no operation providers, no write-capable executor registered
(contrast `runObserver`). Covered by a test asserting no feedback emitter or
GitHub write provider is present.

## Error handling

- List/page/transport failure: tick error; `Run` aborts visibly; context
  cancellation stops cleanly (mid-step context errors suppressed, as in the
  ADO observer).
- Per-issue: `Ensure` error → exclusion with reason; Ensure-ok /
  Materialize-failed → `Failed` entry retried next tick.
- Case found in `BLOCKED`/`READY_FOR_VERIFICATION`/`CLOSED` for the same
  revision → skipped with reason (only `ACTIVE` cases can materialize).

## Testing

Fake lister + full testutil stack (only-maintainer cases, each reason, PR
skips, revision change opens a second case, re-poll no-op with unchanged
snapshot count, Find-first evidence discipline, case-state skip, eligible-
but-unclaimed scheduler assertion); fake-HTTP client tests (GET-only, Link
pagination, auth header, non-2xx, timeouts); CLI read-only composition test.

## Scope boundary

No code writing, planning, PRs, comments, label mutation, or any GitHub
write. No triage decision (follow-on slice).

## Acceptance criteria

- One active case per (mission, `github`, repo-qualified object, revision);
  edits create new revisions; repeats are no-ops without new evidence.
- Every issue ends materialized, excluded with a reason, or failed; PRs are
  skipped explicitly.
- The Task cannot be leased by any registered executor in this slice, and the
  intake path performs no GitHub writes.
