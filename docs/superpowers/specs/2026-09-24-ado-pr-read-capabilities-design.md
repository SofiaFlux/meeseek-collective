# ADO PR read capabilities — design

## Intent

Extend `internal/adomcp` with the read-only pull-request, file, thread and
build-status operations the ADO PR review recipe needs for discovery and review.
No observer, no recipe, no Task creation in this slice.

Deploy feedback (live `RB-Group` org) confirms the current provider works with
2 healthy capabilities (`ado.projects.list`, `ado.work_item.read`). This slice
adds PR discovery; the observer/recipe slice follows.

## Capability table

| Capability | MCP tool | Allowed `action` values |
|---|---|---|
| `ado.pr.list` | `repo_pull_request` | `list`, `list_by_commits` |
| `ado.pr.get` | `repo_pull_request` | `get` |
| `ado.pr.org_active` | `repo_pull_request_org` | none (filter args only) |
| `ado.pr.threads` | `repo_pull_request_thread` | `list`, `list_comments` |
| `ado.pr.file` | `repo_file` | `get_content`, `list_directory` |
| `ado.build.status` | `pipelines_build` | `get_status` |

Notes:
- The current server dispatches by tool + action (like `wit_work_item`), not by
  per-action tool names. The older `repo_list_pull_requests_by_project` name from
  deploy feedback does not exist in the current TOOLSET and is not used.
- `ado.pr.get` returns source/target commits for the recipe's revision key.
  Linked work items resolve through the existing `ado.work_item.read`.
- `ado.pr.org_active` lists the authenticated user's active PRs organization-wide;
  `ado.pr.list` is the precise per-project/repository query (e.g. reviewer filter).
  Exact filter field names are validated live later; the provider forwards them.

## Read-only enforcement (existing pattern, unchanged)

- `toolFor` maps capability → MCP tool; anything else errors before dial.
- `Call` validates `action` against the table above, rejects a `tool` override
  field, and forwards remaining args untouched. No `*_write` tool is ever mapped.
- `Advertise`/`Probe` gain the six capabilities; the existing two are untouched.

## Testing

Existing `fakeSession` pattern, no live credentials:
- advertise count and names (2 → 8), each definition scoped to its own capability
- unknown capability and mutating/wrong actions rejected with zero dials
- per-capability forwarding: correct MCP tool name and args passthrough
- tool error propagation; probe unavailable for absent tools

## Scope boundary

No observer, no case/Task creation, no Copilot executor, no votes/comments, no CI
gating decisions. Live validation against the real MCP server (tool presence,
filter fields, org scoping) happens on the user's machine as recipe follow-on.

## Acceptance criteria

- 8 advertised capabilities, all read-only; `Probe` reports health per MCP tool.
- Unknown capabilities and non-listed actions fail before any subprocess dial.
- `go test ./...` and `go vet ./...` pass; no live credentials required.
