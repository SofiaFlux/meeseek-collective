# Copilot review executor — design

## Intent

Give the worker loop an executor that runs the authenticated Copilot CLI in a
locked-down, non-interactive invocation and returns a strictly validated,
typed review result. This unblocks leasing `ado.pr.review` Tasks; assessment
of the result and Work 2 effects are later slices.

## Approaches considered

1. **Dedicated `CopilotExecutor` (selected):** owns argv construction, prompt,
   strict JSON parsing and evidence mapping; fail-closed on any deviation.
2. **Bare `CommandExecutor` with canned flags:** safety lives in CLI config,
   not code. Rejected.
3. **Parsing `--output-format=json` JSONL:** event schema unknown/unstable.
   Rejected in favor of `-s` text with whole-output JSON validation.

## Invocation (closed argv set)

```text
copilot -p <prompt> -s --no-ask-user
  --available-tools=<csv> --allow-tool=<csv> --deny-tool=shell,write,url,memory
  [--model=MODEL]
```

This exact set is all that is ever emitted. Never emitted: `--allow-all`,
`--allow-all-tools`, `--allow-all-paths`, `--allow-all-urls`, `--add-dir`,
`--allow-url`, `--agent`, `--share`, `--share-gist`, `--fleet`. Same OS user,
non-interactive, `cmd.Dir` = attempt workspace (containment only).

Local file reads (`read`/`glob`) are deliberately NOT granted: review inputs
arrive only through MCP reads, so the workspace directory is never readable by
the agent. This resolves the workspace question explicitly — containment
without read access, matching the parent's "named ADO read tools" rule.

## Tool allowlist (closed)

`CopilotConfig.AllowedTools` holds bare MCP tool names, each validated against
this explicit read-only set (from the Azure DevOps MCP TOOLSET):

```text
repo_pull_request, repo_pull_request_org, repo_pull_request_thread,
repo_file, pipelines_build, core_list_projects, wit_work_item
```

Constructor rejects: blank entries, anything containing `(`, `shell`, `write`,
`url`, `memory`, `read`, `allow-all` in any form. The executor qualifies each
as `<server>(<tool>)` using `CopilotConfig.MCPServer` (syntactic check:
`^[A-Za-z0-9_-]+$`), emitted as the `--available-tools`/`--allow-tool` CSVs.
Empty tools or blank server → constructor error (explicit opt-in, no silent
no-tool run).

## Prompt and payload contract

`AttemptEnvelope.PayloadJSON` must decode as:

```go
type CopilotReviewPayload struct {
    Repo         string `json:"repo"`
    PR           int64  `json:"pr"`
    SourceCommit string `json:"sourceCommit"`
    TargetCommit string `json:"targetCommit"`
}
```

All four required, PR > 0; anything else → executor error before spawning
(fail closed). Prompt renders IDs/objective/payload/acceptance (codex
`codexPrompt` shape) plus one instruction: respond with ONLY this JSON
document, no prose:

```json
{"verdict": "CLEAN|FINDINGS|UNCERTAIN", "reviewedCommits": ["..."],
 "reviewedFiles": ["..."], "findings": [{"path": "...", "line": 0,
 "explanation": "...", "evidence": "..."}], "reason": "..."}
```

Validation: trimmed stdout parses exactly; verdict in set; commits/files
non-empty; every finding has non-blank `path` and `explanation`. File coverage
and commit freshness are the assessor's job (it owns the payload revision).

## Completion precedence (codex mirror)

Always return `ExecutionResult{ExitCode, Stdout, Stderr, Evidence,
Usage{WallTime}}`, then: context timeout → error; non-zero exit → error even
with valid-looking JSON; else JSON validation failure → error. Stderr is
evidence only, never a failure by itself.

## Environment (exclusive allowlist)

Forwarded (built from scratch, like codex/command — unlisted vars never reach
the child, so `COPILOT_ALLOW_ALL`/`COPILOT_AUTO_UPDATE` can never pass):
`PATH`, `COPILOT_MODEL`, `COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`,
`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`, `SSL_CERT_FILE`, `SSL_CERT_DIR`,
`TMPDIR`, `TMP`, `TEMP`. `GITHUB_TOKEN` is allowed deliberately (CLI auth;
output redaction for it is on by default) — this differs from codex on purpose
and is stated here, not inherited silently.

Protected (constructor rejects): `SUMMA42_OWNER_PRIVATE_KEY`,
`AZURE_CLIENT_SECRET`, `AWS_SECRET_ACCESS_KEY`. `COPILOT_HOME` is NOT
forwarded; ambient operator `~/.copilot` config is trusted exactly as far as
the operator-installed `copilot` binary itself (operator trust boundary, not
an agent bypass).

`--model=MODEL` flag form (docs). Precedence: `CopilotConfig.Model` flag wins
when set, else `COPILOT_MODEL` env passthrough. `Timeout` required positive
(CopilotConfig, default 5 minutes via `SUMMA42_COPILOT_TIMEOUT` Go duration).

## Evidence (codex mirror)

- `Stdout` field = canonical result JSON (compact re-encode of the validated
  struct — "canonical" means `json.Marshal` output, not the raw stdout).
- `Evidence`: `AGENT_MESSAGE` with the canonical JSON, `STDERR` always,
  `STDOUT` only when non-empty.
- `Usage` always present with `WallTime`; token counters stay zero (the CLI
  text contract reports none).

## Registration

`buildCopilotExecutorFromEnv() (map[string]executors.Executor, error)` in
`cmd/summa42-box`, mirroring `buildADOProviderFromEnv` nil-nil-absent:
missing `SUMMA42_COPILOT_PATH` → `(nil, nil)`. Called only in the `runWorker`
composition (observer never executes; control-plane `run()` unchanged). Kind
`"copilot"`. Absent kind is logged at startup; with no review-capable executor
registered the worker cannot lease `ado.pr.review` Tasks instead of silently
mis-executing. No taskclass→kind routing table in this slice: deployment
registers copilot as the review executor and scheduler baseline/preference
selects among registered kinds.

## Testing

Fake `copilot` executable (script echoing canned stdout): CLEAN/FINDINGS/
UNCERTAIN parse; rejection of non-JSON, bad verdict, empty commits/files,
finding without path; timeout and non-zero exit errors despite valid JSON;
argv assertions (exact closed set, no shell/write/allow-all, deny list
present); hostile tool configs rejected (`shell`, `write(READ.md)`,
`--allow-all-tools`, `ado(pr_write)`, blank, empty list); payload missing
field fails before spawn; protected env rejected. No real CLI or credentials.

## Scope boundary

No assessment, no coverage/freshness verdicts, no comments/votes, no model
fallback chains, no transcript persistence. Shadow posture holds: evidence only.

## Acceptance criteria

- Valid review JSON → typed result evidence; any deviation → executor error.
- Invocation matches the closed argv set; tool config outside the read-only
  seven fails closed at construction.
- With env set, `runWorker` registers kind `copilot` and review Tasks become
  leasable; without it the kind is absent and startup says so.
