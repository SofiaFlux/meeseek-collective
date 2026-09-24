# Copilot Review Executor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run locked-down Copilot CLI review invocations returning strictly validated typed results, registered as kind `copilot` for the worker loop.

**Architecture:** New `CopilotExecutor` in `internal/executors` mirroring `codex.go` (closed argv, exclusive env allowlist, codex completion precedence). `cmd/summa42-box` builds it from env via `buildCopilotExecutorFromEnv` (nil-nil-absent mirror of the ADO provider builder) into the `runWorker` runtime config only.

**Tech Stack:** Go 1.27, stdlib `os/exec`, fake shell scripts in tests; no real CLI or credentials.

## Global Constraints

- Closed argv set only (`-p`, `-s`, `--no-ask-user`, `--available-tools=`, `--allow-tool=`, `--deny-tool=shell,write,url,memory`, `--model=`); `--allow-all*`, `--add-dir`, `--allow-url`, `--agent`, `--share*`, `--fleet` never emitted.
- Tool config restricted to the 7 read-only MCP tools; hostile configs fail at construction.
- TDD: failing test first for every behavior, then minimal implementation.
- Package suite before each task commit; full `go test ./... -count=1` and `go vet ./...` in Task 2 before the final commit.

---

## File structure

- Create `internal/executors/copilot.go`: `CopilotConfig`, `CopilotExecutor`, `NewCopilotExecutor`, `Start`, `ReviewResult`, prompt/argv/parse helpers.
- Create `internal/executors/copilot_test.go`: fake `copilot` scripts, contract tests.
- Modify `cmd/summa42-box/main.go`: `buildCopilotExecutorFromEnv`, `runWorker` merge into `runtime.Config.Executors["copilot"]`, startup notice when absent.
- Modify `cmd/summa42-box/main_test.go` or create `copilot_env_test.go` (check which fits existing style first): env builder tests.

### Task 1: Executor with closed contract

**Files:**
- Create: `internal/executors/copilot.go`
- Create: `internal/executors/copilot_test.go`

**Interfaces:**
- Consumes: `AttemptEnvelope`, `ExecutionResult`, `Evidence`, `Usage` (existing); `environmentList`/`cloneEnvironment`/`exitCode` helpers (existing, same package).
- Produces: `NewCopilotExecutor`, `CopilotExecutor.Start` (satisfies `Executor`), `ReviewResult` used by Task 2 only for wiring (no signature dependency).

- [ ] **Step 1: Write the failing tests.** In `copilot_test.go` (`package executors_test` if that matches `command_test.go`; check first — codex_test.go is `package executors` internal? Verify the package clause of both test files before writing and follow `command_test.go`):

```go
func writeFakeCopilot(t *testing.T, script string) string {
    t.Helper()
    path := filepath.Join(t.TempDir(), "copilot")
    if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
        t.Fatal(err)
    }
    return path
}

func copilotConfig(path string) executors.CopilotConfig {
    return executors.CopilotConfig{
        Path: path, MCPServer: "ado",
        AllowedTools: []string{"repo_pull_request"},
        Timeout:      time.Minute,
    }
}

func TestCopilotParsesCleanResult(t *testing.T) {
    path := writeFakeCopilot(t, "#!/bin/sh\necho '{\"verdict\":\"CLEAN\",\"reviewedCommits\":[\"abc\"],\"reviewedFiles\":[\"main.go\"],\"findings\":[]}'\n")
    executor, err := executors.NewCopilotExecutor(copilotConfig(path))
    if err != nil {
        t.Fatal(err)
    }
    result, err := executor.Start(context.Background(), executors.AttemptEnvelope{
        TaskID: "task-1", AttemptID: "attempt-1", Workspace: t.TempDir(),
        Objective: "review", PayloadJSON: json.RawMessage(`{"repo":"shop","pr":1,"sourceCommit":"a","targetCommit":"b"}`),
    })
    if err != nil {
        t.Fatal(err)
    }
    if result.ExitCode != 0 {
        t.Fatalf("exit = %d, want 0", result.ExitCode)
    }
    var decoded map[string]any
    if err := json.Unmarshal([]byte(result.Stdout), &decoded); err != nil {
        t.Fatal(err)
    }
    if decoded["verdict"] != "CLEAN" {
        t.Fatalf("verdict = %v", decoded["verdict"])
    }
}
```

Plus (same file): `TestCopilotRejectsHostileToolConfigs` (table: `shell`, `write(README.md)`, `--allow-all-tools`, `ado(pr_write)`, blank, empty list, blank server → all `NewCopilotExecutor` errors); `TestCopilotRejectsInvalidResult` (table: non-JSON stdout, bad verdict, empty commits, finding without path → `Start` errors); `TestCopilotRejectsBadPayload` (missing `targetCommit` → error return before spawn; no marker file needed — the error itself proves no spawn); `TestCopilotArgvIsClosed` (fake script writes `"$@"` to `"$(pwd)/argv.txt"` — the attempt workspace, which the executor sets as working directory — then emits valid CLEAN JSON; assert the dump file contains `--deny-tool=shell,write,url,memory` and contains no `allow-all`, no `add-dir`, no `--agent`); `TestCopilotTimeoutAndExitCode` (sleep script + 50ms timeout → error; `exit 3` script with valid JSON → error); `TestCopilotRejectsProtectedEnv` (`SUMMA42_OWNER_PRIVATE_KEY` → constructor error).

Imports: `context`, `encoding/json`, `os`, `path/filepath`, `strings`, `testing`, `time`, `executors`.

- [ ] **Step 2: Run them, expect compile failure.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/executors -run 'TestCopilot' -count=1`. Expected: FAIL (undefined `CopilotConfig`/`NewCopilotExecutor`).
- [ ] **Step 3: Implement `copilot.go`.** (`package executors`)

```go
type CopilotConfig struct {
    Path         string
    Model        string
    MCPServer    string
    AllowedTools []string
    Timeout      time.Duration
    Environment  map[string]string
}

type CopilotExecutor struct {
    path        string
    model       string
    server      string
    tools       []string
    timeout     time.Duration
    environment map[string]string
}

type ReviewVerdict string

const (
    ReviewClean     ReviewVerdict = "CLEAN"
    ReviewFindings  ReviewVerdict = "FINDINGS"
    ReviewUncertain ReviewVerdict = "UNCERTAIN"
)

type ReviewFinding struct {
    Path        string `json:"path"`
    Line        int64  `json:"line"`
    Explanation string `json:"explanation"`
    Evidence    string `json:"evidence"`
}

type ReviewResult struct {
    Verdict         ReviewVerdict   `json:"verdict"`
    ReviewedCommits []string        `json:"reviewedCommits"`
    ReviewedFiles   []string        `json:"reviewedFiles"`
    Findings        []ReviewFinding `json:"findings"`
    Reason          string          `json:"reason"`
}

type CopilotReviewPayload struct {
    Repo         string `json:"repo"`
    PR           int64  `json:"pr"`
    SourceCommit string `json:"sourceCommit"`
    TargetCommit string `json:"targetCommit"`
}

var copilotReadTools = map[string]struct{}{
    "repo_pull_request": {}, "repo_pull_request_org": {}, "repo_pull_request_thread": {},
    "repo_file": {}, "pipelines_build": {}, "core_list_projects": {}, "wit_work_item": {},
}

var copilotForbiddenToolFragments = []string{"shell", "write", "url", "memory", "allow-all"}

var copilotEnvironmentAllowlist = map[string]struct{}{
    "PATH": {}, "COPILOT_MODEL": {}, "COPILOT_GITHUB_TOKEN": {}, "GH_TOKEN": {}, "GITHUB_TOKEN": {},
    "HTTP_PROXY": {}, "HTTPS_PROXY": {}, "NO_PROXY": {},
    "SSL_CERT_FILE": {}, "SSL_CERT_DIR": {}, "TMPDIR": {}, "TMP": {}, "TEMP": {},
}

var copilotProtectedEnvironment = map[string]struct{}{
    "SUMMA42_OWNER_PRIVATE_KEY": {}, "AZURE_CLIENT_SECRET": {}, "AWS_SECRET_ACCESS_KEY": {},
}
```

`NewCopilotExecutor`: trim/require Path; Timeout must be > 0; MCPServer must match `^[A-Za-z0-9_-]+$`; AllowedTools non-empty; each tool: non-blank, no `(`, exact member of `copilotReadTools`, and must not contain any forbidden fragment (case-insensitive); Environment keys: reject protected, reject anything outside allowlist (exclusive, codex pattern). Store defensive copies; precompute qualified `server(tool)` list.

`Start`: nil/configured guard; workspace and objective required (codex pattern); decode+validate payload (`repo`, `pr > 0`, both commits non-blank) before spawn; build argv:

```go
args := []string{"-p", prompt, "-s", "--no-ask-user",
    "--available-tools=" + strings.Join(qualified, ","),
    "--allow-tool=" + strings.Join(qualified, ","),
    "--deny-tool=shell,write,url,memory"}
if e.model != "" {
    args = append(args, "--model="+e.model)
}
```

Run with timeout ctx, `cmd.Dir = workspace`, exclusive env list. Completion precedence exactly: build `ExecutionResult{ExitCode: exitCode(runErr), Stdout, Stderr, Evidence, Usage{WallTime}}` where Evidence = `AGENT_MESSAGE(canonical)` + `STDERR` always + `STDOUT` only if canonical non-empty; then `runCtx.Err()` → return result+err; `runErr != nil` → result + fmt error; parse/validate → result + err. Canonical = `json.Marshal(validated ReviewResult)` compact.

Prompt builder: IDs/objective/payload fields/acceptance (codexPrompt shape) + `Respond with ONLY the review JSON document, no prose.` + the schema literal.

- [ ] **Step 4: Run package tests, expect PASS.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/executors -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** `git add internal/executors/copilot.go internal/executors/copilot_test.go && git commit -m "feat: add Copilot review executor"`.

### Task 2: Env registration in runWorker, full verification

**Files:**
- Modify: `cmd/summa42-box/main.go`
- Create or modify test file for env builder (check `main_test.go` style first)
- (No changes to `run()` control-plane path or `runObserver`.)

**Interfaces:**
- Consumes: `NewCopilotExecutor`, `CopilotConfig` from Task 1; `buildADOProviderFromEnv` pattern; `runtime.Config.Executors`.
- Produces: kind `"copilot"` registered for worker runs; nothing downstream in this plan.

- [ ] **Step 1: Write the failing tests.** Using `t.Setenv` (precedent in `main_test.go`):

```go
func TestBuildCopilotExecutorAbsentWithoutPath(t *testing.T) {
    t.Setenv("SUMMA42_COPILOT_PATH", "")
    executors, err := buildCopilotExecutorFromEnv()
    if err != nil || executors != nil {
        t.Fatalf("result = %v, %v; want nil, nil", executors, err)
    }
}

func TestBuildCopilotExecutorRejectsBadConfig(t *testing.T) {
    t.Setenv("SUMMA42_COPILOT_PATH", "/bin/true")
    t.Setenv("SUMMA42_COPILOT_TOOLS", "shell")
    if _, err := buildCopilotExecutorFromEnv(); err == nil {
        t.Fatal("accepted shell tool")
    }
}

func TestBuildCopilotExecutorDefaultsReadTools(t *testing.T) {
    t.Setenv("SUMMA42_COPILOT_PATH", "/bin/true")
    t.Setenv("SUMMA42_COPILOT_TOOLS", "")
    t.Setenv("SUMMA42_COPILOT_MCP_SERVER", "ado")
    t.Setenv("SUMMA42_COPILOT_TIMEOUT", "")
    built, err := buildCopilotExecutorFromEnv()
    if err != nil {
        t.Fatal(err)
    }
    if _, ok := built["copilot"]; !ok {
        t.Fatalf("kinds = %v, want copilot", built)
    }
}
```

- [ ] **Step 2: Run them, expect compile failure.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./cmd/summa42-box -run 'TestBuildCopilotExecutor' -count=1`. Expected: FAIL (undefined `buildCopilotExecutorFromEnv`).
- [ ] **Step 3: Implement.** In `main.go` (read `buildADOProviderFromEnv` and the `runWorker` Open-config construction first and mirror them):

```go
func buildCopilotExecutorFromEnv() (map[string]executors.Executor, error) {
    path := strings.TrimSpace(os.Getenv("SUMMA42_COPILOT_PATH"))
    if path == "" {
        return nil, nil
    }
    server := strings.TrimSpace(os.Getenv("SUMMA42_COPILOT_MCP_SERVER"))
    tools := splitCSV(os.Getenv("SUMMA42_COPILOT_TOOLS"))
    if len(tools) == 0 {
        tools = []string{"repo_pull_request", "repo_pull_request_org", "repo_pull_request_thread", "repo_file", "pipelines_build", "core_list_projects", "wit_work_item"}
    }
    timeout := 5 * time.Minute
    if raw := strings.TrimSpace(os.Getenv("SUMMA42_COPILOT_TIMEOUT")); raw != "" {
        parsed, err := time.ParseDuration(raw)
        if err != nil || parsed <= 0 {
            return nil, fmt.Errorf("invalid SUMMA42_COPILOT_TIMEOUT %q", raw)
        }
        timeout = parsed
    }
    executor, err := executors.NewCopilotExecutor(executors.CopilotConfig{
        Path: path, Model: strings.TrimSpace(os.Getenv("SUMMA42_COPILOT_MODEL")),
        MCPServer: server, AllowedTools: tools, Timeout: timeout,
    })
    if err != nil {
        return nil, err
    }
    return map[string]executors.Executor{"copilot": executor}, nil
}
```

`splitCSV`: split on comma, trim, drop empties (write it; check whether a helper already exists in main.go first and reuse if so). In `runWorker`, before `Open`: call the builder, merge into `runtime.Config.Executors` (create the map if nil), and when the result is nil print a startup notice to stderr that kind `copilot` is not registered (mirror existing startup messaging style in the file). `run()` and `runObserver` untouched.

- [ ] **Step 4: Run package tests, expect PASS.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./cmd/summa42-box -count=1`. Expected: PASS.
- [ ] **Step 5: Run full verification.** In order, all must pass:
  - `GOCACHE=/tmp/summa42-full-go-cache go test ./... -count=1`
  - `go vet ./...`, `git diff --check`, `gofmt -l` on touched files
- [ ] **Step 6: Commit.** `git add internal/executors/copilot.go internal/executors/copilot_test.go cmd/summa42-box/main.go cmd/summa42-box/*test*.go && git commit -m "feat: register copilot executor from env"`. (List the exact test filenames from `git status`; never `git add .`. Task 1 files are already committed — if the glob matches them harmlessly, prefer naming exact files.)

## Self-review

- Spec coverage: closed argv → Task 1 args + argv test; tool allowlist + hostile configs → validation + test; payload schema + fail-closed → decode guard + test; workspace containment → Dir set, no read tools; deny list → argv test; completion precedence → Start tail + timeout/exit tests; exclusive env + protected → constructor + test; evidence mirror → Evidence block; registration (nil-nil-absent, runWorker-only, absent notice) → Task 2; testing section → all tests; acceptance → Task 2 wiring.
- No placeholders: exact files, code, commands, expected outputs; two "check first" notes name the exact files/patterns to confirm.
- Type consistency: `CopilotConfig`/`ReviewResult`/`ReviewFinding`/`CopilotReviewPayload`/`NewCopilotExecutor`/`buildCopilotExecutorFromEnv` identical across tasks; `SUMMA42_COPILOT_*` names fixed in Task 2.
