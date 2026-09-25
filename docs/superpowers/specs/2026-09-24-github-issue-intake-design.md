# GitHub issue intake — design

## Intent

Let the Box notice GitHub issues opened by configured maintainers and register
each eligible issue revision as a durable workflow case with a triage Task.
This is the first sub-project of the GitHub issue-to-PR role; planning, code
writing, review and PR creation are later sub-projects.

## Position in the larger role

1. **This slice:** intake — read issues, filter, evidence, case, triage Task.
2. Follow-on: triage/planning decision; sandboxed coding executor; review and
   draft-PR effect; end-to-end orchestration.

Autonomy: draft PR, human merges. Maintainer trust: explicit ID list from
configuration, never inferred from org roles or labels.

## Architecture

New package `internal/ghissue`:

- `source.go`: `Issue` type, `IssueLister` (`ListIssues(ctx, repo, state,
  cursor) ([]Issue, string, error)`), parsing, candidate filter with reasons.
- `observe.go`: `Config`, `ObserveResult`, `ObserveOnce`, `Run`.
- GitHub transport reuses the existing `internal/feedbackgithub` credential
  source and HTTP client conventions; list/read access is added as a new
  operation in that client (or a sibling client in `ghissue` if the existing
  client's surface is feedback-specific — decide at implementation time by
  reading `client.go`; the wire contract is identical: bearer token, JSON,
  status handling).

`ObserveOnce`: list open issues (all pages; issues that are actually pull
requests are skipped — GitHub returns them in the same list) → filter →
`workflowcase.Ensure` (source `github`, object `issue#<n>`, revision
`<updated_at>` ISO-8601) → snapshot evidence (`github.issue.snapshot`,
`application/json`) → `MaterializeTask` for Work 1 (`github.issue.triage`)
with recipe objective, payload `{repo, issue, revision, title, labels}`,
acceptance "triage decision recorded", and the configured envelope. The case
Work ID is the idempotency key, so repeated polls reuse cases and Tasks;
edited issues (new `updated_at`) open a new case revision.

## Filter (with stated reasons)

- author not in the configured maintainer list → `not-maintainer`;
- closed or missing `updated_at` → not eligible (defensive);
- assignee present → `already-assigned`;
- label `box-hold` or `wontfix` → `held-by-label`;
- otherwise candidate; triage type (`bug` / `feature` / `unclassified`) is
  recorded for the follow-on triage decision, never acted on here.

## Configuration and CLI

`run-gh-intake --mission <id> --repo <owner/name> --maintainer-id <id>
(repeatable) --envelope <id> --max-steps <n> --remaining-budget <n>
--poll-interval <d>`. Missing mission/repo/maintainer/envelope is a startup
error. No external writes: intake only creates evidence, cases and Tasks.

## Error handling

- List failure or malformed page: tick error; `Run` aborts visibly for
  supervisor restart; context cancellation stops cleanly.
- Per-issue Ensure failure: exclusion with reason; case-without-Task is
  repaired on the next tick (same Find-first discipline as the ADO observer).
- Unknown issue shapes fail closed as unparseable exclusions.

## Testing

Fake issue lister + full testutil stack: only maintainer issues become cases;
each exclusion reason; re-poll reuses the case and creates no new evidence;
edited issue (new `updated_at`) opens a second case; triage Task fields land
per the recipe; `Run` cancellation. No live credentials.

## Scope boundary

No code writing, planning, PR creation, comments, labels mutation, or GitHub
writes of any kind. Trust remains an explicit configured list.

## Acceptance criteria

- One active case per (mission, github, issue#n, updated_at) revision; edits
  create new revisions; repeats are no-ops.
- Every issue ends up materialized, excluded with a reason, or failed.
- Intake performs no GitHub writes.
