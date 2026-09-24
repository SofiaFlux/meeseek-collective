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

## Invocation

```text
copilot -p <prompt> -s --no-ask-user
  --available-tools=<csv ADO read tools>
  --allow-tool=<same csv>
  --deny-tool=shell,write
  [--model <model>]
```

- No freeform-args field exists anywhere: `--allow-all*` cannot be constructed.
- Same OS user, non-interactive (`-p`, `--no-ask-user`), stdout is the whole
  response (`-s`). Working directory = attempt workspace.
- `AllowedTools` holds named ADO *read* tool identifiers only (e.g. MCP server
  read tools); constructor rejects blank entries. Shell, file writes, URLs,
  memory writes and ADO write tools are never granted.

## Prompt and result contract

Prompt carries task/attempt IDs, objective, payload (repo, PR, source/target
commits), acceptance criteria, and one instruction: respond with ONLY this JSON
document, no prose:

```json
{"verdict": "CLEAN|FINDINGS|UNCERTAIN", "reviewedCommits": ["..."],
 "reviewedFiles": ["..."], "findings": [{"path": "...", "line": 0,
 "explanation": "...", "evidence": "..."}], "reason": "..."}
```

Validation (fail closed → executor error → worker `FailAttempt`):
- trimmed stdout parses as exactly this document;
- `verdict` ∈ {CLEAN, FINDINGS, UNCERTAIN};
- `reviewedCommits` and `reviewedFiles` non-empty;
- every finding has non-blank `path` and `explanation` (`line` optional).

File coverage and commit freshness are the assessor's job (it owns the payload
revision), not the executor's.

## Environment

Allowlist: `PATH`, `COPILOT_HOME`, `COPILOT_MODEL`, `COPILOT_GITHUB_TOKEN`,
`GH_TOKEN`, `GITHUB_TOKEN`, `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`,
`SSL_CERT_FILE`, `SSL_CERT_DIR`, `TMPDIR`, `TMP`, `TEMP`. Protected (constructor
rejects, codex pattern): `SUMMA42_OWNER_PRIVATE_KEY`, `AZURE_CLIENT_SECRET`,
`AWS_SECRET_ACCESS_KEY`. Auth tokens stay allowed; owner/box secrets never pass.

## Evidence

`AGENT_MESSAGE` with the canonical result JSON, plus `STDOUT`/`STDERR`
entries (codex pattern). Token usage recorded when reported by the CLI;
absence is tolerated (no usage block is required in the contract).

## Registration

`cmd/summa42-box` builds the executor from env (`SUMMA42_COPILOT_PATH`
required opt-in; `SUMMA42_COPILOT_MODEL`, `SUMMA42_COPILOT_TOOLS` csv,
`SUMMA42_COPILOT_TIMEOUT`) and registers kind `"copilot"` in
`runtime.Config.Executors`. Absent path = kind unregistered, stated at startup;
the worker then cannot lease review Tasks instead of silently mis-executing.

## Testing

Fake `copilot` executable (script echoing canned stdout): CLEAN/FINDINGS/
UNCERTAIN parse; rejection of non-JSON, bad verdict, empty commits/files,
finding without path; timeout error; argv contains no shell/write/allow-all;
protected env rejected at construction. No real CLI or credentials.

## Scope boundary

No assessment, no coverage/freshness verdicts, no comments/votes, no model
fallback chains, no session transcript persistence. Shadow posture holds: the
executor returns evidence only.

## Acceptance criteria

- Valid review JSON → typed result evidence; any deviation → executor error.
- Invocation never grants shell/write/allow-all (asserted on argv).
- `run-worker`/`run-observer` compositions can lease `ado.pr.review` Tasks to
  kind `copilot` once the env is set; without it the kind is absent, loudly.
