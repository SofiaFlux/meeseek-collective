# Workflow Driver Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Materialize Work 2 from publish decisions and assess publication with verified effects.

**Architecture:** `adomcp.Call` decodes SDK results at the boundary; three small read APIs (`workflowcase.ListActive/ListAssessments`, `execution.FindByIdempotencyKey`); `adoreview.Driver` orchestrates one bounded pass per tick with Assessment 2 doing read-back verification through adoeffects lookups.

**Tech Stack:** Go 1.27, existing services, fakes; no live credentials.

## Global Constraints

- Driver never dispatches: Assessment 2 verifies via `LookupOutcome` only.
- Case filters: configured mission + `Source == "ado"` + `NextWork.Kind == "publish-decision"`.
- No SettleOutcome/redispatch anywhere in the driver.
- TDD: failing test first; package suite before each commit; full `go test ./... -count=1` + `go vet ./...` in Task 3.

---

## File structure

- Modify `internal/adomcp/provider.go`: decode `*mcp.CallToolResult` → `map[string]any` in `Call` (structured content first, text-JSON fallback; fail closed).
- Modify `internal/adomcp/provider_test.go`: decoder tests (structured, text fallback, garbage).
- Modify `internal/workflowcase/service.go`: `ListActive`, `ListAssessments`, `AssessmentRecord` type; tests.
- Modify `internal/execution/read.go`: `FindByIdempotencyKey`; tests.
- Create `internal/adoreview/driver.go`: `Driver`, `DriverConfig`, `DriverResult`, `StepOnce`, `Run`.
- Create `internal/adoreview/driver_test.go`: full-stack tests.
- Modify `cmd/summa42-box/main.go`: `run-driver` dispatch + flags; tests.

### Task 1: adomcp result decoding

**Files:**
- Modify: `internal/adomcp/provider.go`, `provider_test.go`

- [ ] **Step 1: Write the failing test.** In `provider_test.go` (internal package, existing `fakeSession`):

```go
func TestCallDecodesStructuredAndTextResults(t *testing.T) {
    p, _ := New(Config{Command: "/bin/true", Organization: "Contoso"})
    session := &fakeSession{tools: []string{"repo_pull_request"}}
    p.dial = func(context.Context) (mcpSession, error) { return session, nil }
    // check the exact mcp.CallToolResult shape for structured content first
    // (read internal/adomcp/provider.go Call currently: returns result directly)
    session.result = structuredCallResult(map[string]any{"status": "active"})
    got, err := p.Call(t.Context(), "ado.pr.get", map[string]any{"action": "get"})
    if err != nil { t.Fatal(err) }
    m, ok := got.(map[string]any)
    if !ok || m["status"] != "active" { t.Fatalf("got %#v", got) }
    session.result = textCallResult(`{"status":"text"}`)
    got, err = p.Call(t.Context(), "ado.pr.get", map[string]any{"action": "get"})
    if err != nil { t.Fatal(err) }
    m, _ = got.(map[string]any)
    if m == nil || m["status"] != "text" { t.Fatalf("got %#v", got) }
    session.result = garbageCallResult()
    if _, err := p.Call(t.Context(), "ado.pr.get", map[string]any{"action": "get"}); err == nil {
        t.Fatal("accepted undecodable result")
    }
}
```

The three helper constructors build `*mcp.CallToolResult` with the exact SDK field names — read the SDK types via the existing import path (`github.com/modelcontextprotocol/go-sdk/mcp`) and the current `fakeSession.result` usage in the file; if a helper cannot be built from exported fields, construct the result via a tiny in-test `mcpServer` round-trip alternative and adjust (report the deviation).

- [ ] **Step 2: Run, expect FAIL** (current Call returns the raw result). `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adomcp -run TestCallDecodes -count=1`.
- [ ] **Step 3: Implement decoding.** In `Call`, replace `return result, nil` with `return decodeToolResult(result)`. `decodeToolResult` walks `result.Content`: collect `*mcp.TextContent` text blocks, try `json.Unmarshal` into `map[string]any`, prefer a block whose text decodes to an object (structured blocks may expose `.Content` as a map — check SDK types; if a `*mcp.CallToolResult` has a structured-content field use it first), else error `ADO MCP <tool> returned an undecodable result`. Tool-error check (`result.IsError`) stays first.
- [ ] **Step 4: Run package tests, expect PASS** — `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adomcp -count=1`.
- [ ] **Step 5: Commit** — `git add internal/adomcp/provider.go internal/adomcp/provider_test.go && git commit -m "fix: decode ADO MCP results at the provider boundary"`.

### Task 2: Read APIs and driver core

**Files:**
- Modify: `internal/workflowcase/service.go` + tests (`service_test.go` or new `list_test.go` — check existing file layout)
- Modify: `internal/execution/read.go` + tests
- Create: `internal/adoreview/driver.go`, `driver_test.go`

**Interfaces:**
- Consumes: Task 1 decoding; `AssessReview`/`ReviewDecision`; `PublishPayload`; `adoeffects` providers (`LookupOutcome`); `runmanifest.Provenance` (read its exact method/signature first — `internal/runmanifest/service.go:241-300`).
- Produces: `Driver` for Task 3 wiring.

- [ ] **Step 1: Write the failing tests.** `workflowcase`: `ListActive` returns only that mission's ACTIVE cases ordered by ID; `ListAssessments` returns stored records (request/result JSON round-trip). `execution`: `FindByIdempotencyKey` hits and misses. `driver_test.go` (package adoreview, full testutil stack + fake `PRCaller` + fake providers with `LookupOutcome` returning confirmed/unknown):

```go
func TestDriverMaterializesWork2Idempotently(t *testing.T) {
    // stack: purpose/execution/evidence/verification/runmanifest/cases
    // case ACTIVE with NextWork{Kind: publish-decision}, CurrentWorkID set,
    // one stored assessment whose result Case.CurrentWorkID == current work
    // and whose request EvidenceIDs contains a decision blob (kind ado.review.decision)
    // Work 1 task: created + completed (reuse acceptance/execution helpers from
    // existing tests where possible; otherwise create task, StartAttempt,
    // CompleteAttempt with an evidence blob so the ordering guard passes)
    // driver StepOnce twice
    // assert: one Work 2 task (FindByIdempotencyKey(current work)), same ID both times
    // assert Work 2 payload contains decision/caseID/workID/project/repo/pr/revision
}

func TestDriverAssessesVerifiedPublication(t *testing.T) {
    // Work 2 task completed with publisher-shaped AGENT_MESSAGE evidence lines
    // fake provider LookupOutcome → CONFIRMED_EFFECT
    // StepOnce → case state READY_FOR_VERIFICATION (cases.Get)
}

func TestDriverHoldsUnverifiedPublication(t *testing.T) {
    // table: LookupOutcome unknown; entry skipped; missing slot → case BLOCKED
}

func TestDriverHoldsBlockedWork2Task(t *testing.T) {
    // Work 2 task state BLOCKED (FailAttempt twice or direct state) → case BLOCKED with reason
}

func TestDriverSkipsNonPublishCases(t *testing.T) { /* other mission / source / kind untouched */ }
```

- [ ] **Step 2: Run, expect compile failure** — `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/workflowcase ./internal/execution ./internal/adoreview -run 'TestDriver|TestListActive|TestFindByIdempotency' -count=1`.
- [ ] **Step 3: Implement.** `ListActive(ctx, missionID)`: single query `WHERE state = ? AND mission_id = ?` reusing the shared SELECT column list; `ListAssessments(ctx, caseID)`: `SELECT assessment_id, work_id, request_json, result_json, created_at FROM workflow_assessments WHERE case_id = ? ORDER BY created_at, assessment_id` → `AssessmentRecord{ID, WorkID, RequestJSON, ResultJSON string, CreatedAt time.Time}`. `FindByIdempotencyKey`: `SELECT task_id FROM tasks WHERE idempotency_key = ?` → loadTask.
  `driver.go`: `DriverConfig{MissionID, ResourceEnvelopeID domain.ID, Comment LookupProvider, Vote LookupProvider, Caller PRCaller, Project string}` where `LookupProvider interface { Name() string; LookupOutcome(ctx, operations.ProviderDispatchRequest) (operations.ProviderOutcome, error) }` (both adoeffects providers satisfy it). `StepOnce`: ListActive → filter source/kind → per case: producing assessment (`result.Case.CurrentWorkID == c.CurrentWorkID`) → decision evidence ID from request JSON (`Assessment.EvidenceIDs` containing an object of kind `ado.review.decision` — verify via evidence store metadata by Get) → ordering guard (Work 1 Task by `FindByIdempotencyKey(assessedWorkID)`; require `AWAITING_VERIFICATION`; `runmanifest.Provenance(attemptID).OutputEvidence` contains the review evidence ID) → project from Work 1 payload else `ado.pr.get` → `MaterializeTask` → state matrix → Assessment 2 (re-verify each publisher evidence line through `LookupOutcome` with a canonical intent rebuilt from the decision; all confirmed + exact slot set → `Assess(Ready)`; else `Assess(Unknown, reason)`). `Run(ctx, interval)` mirrors `adoreview.Run` cancellation semantics.
- [ ] **Step 4: Run package tests, expect PASS** — `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoreview ./internal/workflowcase ./internal/execution -count=1`.
- [ ] **Step 5: Commit** — `git add internal/workflowcase/service.go internal/workflowcase/*_test.go internal/execution/read.go internal/execution/*_test.go internal/adoreview/driver.go internal/adoreview/driver_test.go && git commit -m "feat: drive ADO publish workflow"`. (Stage exact files; never `git add .`.)

### Task 3: run-driver CLI, full verification

**Files:**
- Modify: `cmd/summa42-box/main.go` (+ test file)
- (No changes to run()/runObserver/runWorker.)

- [ ] **Step 1: Write the failing test** — `run-driver` flag parsing: missing `--mission`/`--envelope` errors, defaults for interval, unknown flag errors (mirror `worker_flags_test.go` style).
- [ ] **Step 2: Run, expect compile failure** — `GOCACHE=/tmp/summa42-full-go-cache go test ./cmd/summa42-box -run 'TestParseDriverFlags' -count=1`.
- [ ] **Step 3: Implement** — `parseDriverFlags` (`--mission`, `--envelope`, `--poll-interval` default 30s, optional `--project`); `runDriver(ctx, args)`: load config, provider, `Open` (OperationProviders from `buildAdoEffectProviders` if ADO env present — reuse the publisher Task 2 builder; build `LookupProvider`s from the same providers), construct `adoreview.NewDriver(...)` with `box.Store/Execution/Evidence/RunManifests`-derived services (confirm exact `Box` field names), `Run` until cancellation. Dispatch in `main` next to `run-worker`/`run-observer`.
- [ ] **Step 4: Run package tests, expect PASS** — `GOCACHE=/tmp/summa42-full-go-cache go test ./cmd/summa42-box -count=1`.
- [ ] **Step 5: Full verification** — `GOCACHE=/tmp/summa42-full-go-cache go test ./... -count=1`, `go vet ./...`, `git diff --check`, `gofmt -l` on touched files.
- [ ] **Step 6: Commit** — `git add cmd/summa42-box/main.go <exact-test-file> && git commit -m "feat: add run-driver command"`.

## Self-review

- Spec coverage: adomcp decoding → Task 1; read APIs → Task 2; discovery/ordering/materialize/state-matrix/verified-Assessment-2 → Task 2; Run+CLI → Task 2/3; tests → all; acceptance → Task 2 tests.
- No placeholders: exact files, code sketches, commands, outputs; "read first" notes name exact files/ranges.
- Type consistency: `ListActive/ListAssessments/AssessmentRecord/FindByIdempotencyKey/LookupProvider/DriverConfig/Driver` signatures fixed across tasks.
