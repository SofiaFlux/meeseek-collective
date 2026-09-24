# ADO PR Read Capabilities Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add six read-only PR/file/thread/build capabilities to `internal/adomcp` with per-capability action binding.

**Architecture:** Extend the existing `toolFor`/`Call`/`Advertise`/`Probe` structure. Actions bind per capability via an `allowedActions` map (the global work-item-only `readAction` is removed, never extended — `ado.pr.list` and `ado.pr.get` share one MCP tool, so a global list would break scoping). New capabilities append after the existing two so current probe indices keep working.

**Tech Stack:** Go 1.27, existing `mcpSession` fake in `provider_test.go`; no live credentials.

## Global Constraints

- Read-only only: no `*_write` MCP tool is ever mapped; unknown capabilities and unlisted actions fail before any subprocess dial (zero dials).
- Per-capability action binding via `allowedActions`; do not extend the global `readAction` (it is deleted).
- Every `Call` branch replicates the `tool`-override rejection.
- TDD: failing test first for every behavior, then minimal implementation.
- `go test ./internal/adomcp -count=1` must pass before each task commit; full `go test ./... -count=1` and `go vet ./...` must pass in Task 2 before the final commit.

---

## File structure

- Modify `internal/adomcp/provider.go` (~215 lines): `allowedActions` map replacing `readAction`; `toolFor` additions; `Advertise` appends; `Call` branches per capability. One responsibility preserved: explicit read-only MCP subset.
- Modify `internal/adomcp/provider_test.go` (~117 lines, `package adomcp`, existing `fakeSession` with `tools`/`called`/`args`/`result`): extend advertise/reject/forward/probe tests. Existing probe test indexes `definitions[0]`/`[1]` stay valid because new capabilities append after the existing two.

### Task 1: Per-capability binding plus PR list and get

**Files:**
- Modify: `internal/adomcp/provider.go`
- Modify: `internal/adomcp/provider_test.go`

**Interfaces:**
- Consumes: `Config`, `Provider.dial`, `fakeSession`, `capabilities.Definition` (all existing).
- Produces: `allowedActions` map, `ado.pr.list` / `ado.pr.get` capabilities used by Task 2 as the established branch pattern.

- [ ] **Step 1: Write the failing tests.** Append to `provider_test.go`:

```go
func TestProviderAdvertisesPRListAndGet(t *testing.T) {
    p, err := New(Config{Command: "/bin/true", Organization: "Contoso"})
    if err != nil {
        t.Fatal(err)
    }
    definitions, err := p.Advertise(t.Context())
    if err != nil {
        t.Fatal(err)
    }
    if len(definitions) != 4 {
        t.Fatalf("got %d definitions, want 4", len(definitions))
    }
    want := []string{"ado.projects.list", "ado.work_item.read", "ado.pr.list", "ado.pr.get"}
    for i, name := range want {
        if definitions[i].Skill.Name != name {
            t.Fatalf("definition[%d] = %q, want %q", i, definitions[i].Skill.Name, name)
        }
    }
}

func TestProviderEnforcesPerCapabilityActions(t *testing.T) {
    p, _ := New(Config{Command: "/bin/true", Organization: "Contoso"})
    dials := 0
    session := &fakeSession{tools: []string{"repo_pull_request"}}
    p.dial = func(context.Context) (mcpSession, error) { dials++; return session, nil }
    for _, tc := range []struct {
        name    string
        request any
    }{
        {"ado.pr.get", map[string]any{"action": "list"}},
        {"ado.pr.list", map[string]any{"action": "get"}},
        {"ado.pr.list", map[string]any{"action": "update"}},
        {"ado.pr.get", map[string]any{"action": "get", "tool": "repo_pull_request_write"}},
    } {
        if _, err := p.Call(t.Context(), tc.name, tc.request); err == nil {
            t.Fatalf("accepted %s: %#v", tc.name, tc.request)
        }
    }
    if dials != 0 {
        t.Fatalf("dialed %d times for rejected calls", dials)
    }
    if _, err := p.Call(t.Context(), "ado.pr.get", map[string]any{"action": "get", "pullRequestId": 42}); err != nil {
        t.Fatal(err)
    }
    if _, err := p.Call(t.Context(), "ado.pr.list", map[string]any{"action": "list_by_commits"}); err != nil {
        t.Fatal(err)
    }
    if len(session.called) != 2 || session.called[0] != "repo_pull_request" || session.called[1] != "repo_pull_request" {
        t.Fatalf("called tools: %v", session.called)
    }
}
```

Also update `TestProviderAdvertisesOnlyExplicitReadCapabilities`: change `len(definitions) != 2` to `!= 4` and extend the name assertion with `"ado.pr.list"` and `"ado.pr.get"` (keep indices 0–1 first). `context` is already imported in the test file.

- [ ] **Step 2: Run them, expect FAIL.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adomcp -run 'TestProvider(AdvertisesPRListAndGet|EnforcesPerCapabilityActions|AdvertisesOnlyExplicitReadCapabilities)$' -count=1`. Expected: FAIL (`undefined` capabilities / wrong count).
- [ ] **Step 3: Implement.** In `provider.go`, replace `readAction` with:

```go
var allowedActions = map[string]map[string]bool{
    "ado.work_item.read": {
        "get": true, "get_batch": true, "list_comments": true, "my": true,
        "list_revisions": true, "list_for_iteration": true, "get_type": true,
    },
    "ado.pr.list": {"list": true, "list_by_commits": true},
    "ado.pr.get":  {"get": true},
}

func allowedAction(capability, action string) bool {
    return allowedActions[capability][action]
}
```

Delete the `readAction` function. Change the `ado.work_item.read` branch to use it:

```go
    case "ado.work_item.read":
        m, ok := request.(map[string]any)
        if !ok {
            return nil, errors.New("work item request must be an object")
        }
        action, ok := m["action"].(string)
        if !ok || !allowedAction(capability, action) {
            return nil, fmt.Errorf("work item action %q is not read-only", action)
        }
```

Add branches (inside the same `switch capability`, before `callCtx` creation):

```go
    case "ado.pr.list", "ado.pr.get":
        m, ok := request.(map[string]any)
        if !ok {
            return nil, errors.New("pull request request must be an object")
        }
        action, ok := m["action"].(string)
        if !ok || !allowedAction(capability, action) {
            return nil, fmt.Errorf("pull request action %q is not allowed for %q", action, capability)
        }
        if _, bypass := m["tool"]; bypass {
            return nil, errors.New("MCP tool override is forbidden")
        }
        for key, value := range m {
            args[key] = value
        }
```

Extend `toolFor`:

```go
    case "ado.pr.list", "ado.pr.get":
        return "repo_pull_request", nil
```

Extend `Advertise` (append after the existing two):

```go
    return []capabilities.Definition{
        p.definition("ado.projects.list", "core_list_projects"),
        p.definition("ado.work_item.read", "wit_work_item"),
        p.definition("ado.pr.list", "repo_pull_request"),
        p.definition("ado.pr.get", "repo_pull_request"),
    }, nil
```

`Probe` needs no change (it resolves via `toolFor`).

- [ ] **Step 4: Run package tests, expect PASS.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adomcp -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** `git add internal/adomcp/provider.go internal/adomcp/provider_test.go && git commit -m "feat: add ADO PR list and get capabilities"`.

### Task 2: Org PRs, threads, files, build status, full verification

**Files:**
- Modify: `internal/adomcp/provider.go`
- Modify: `internal/adomcp/provider_test.go`

**Interfaces:**
- Consumes: `allowedActions` map, branch pattern, `fakeSession` from Task 1.
- Produces: complete 8-capability provider; nothing depends on it yet (observer slice follows).

- [ ] **Step 1: Write the failing tests.** Append to `provider_test.go`:

```go
func TestProviderAdvertisesAllEightCapabilities(t *testing.T) {
    p, err := New(Config{Command: "/bin/true", Organization: "Contoso"})
    if err != nil {
        t.Fatal(err)
    }
    definitions, err := p.Advertise(t.Context())
    if err != nil {
        t.Fatal(err)
    }
    want := []string{"ado.projects.list", "ado.work_item.read", "ado.pr.list", "ado.pr.get", "ado.pr.org_active", "ado.pr.threads", "ado.pr.file", "ado.build.status"}
    if len(definitions) != len(want) {
        t.Fatalf("got %d definitions, want %d", len(definitions), len(want))
    }
    for i, name := range want {
        if definitions[i].Skill.Name != name {
            t.Fatalf("definition[%d] = %q, want %q", i, definitions[i].Skill.Name, name)
        }
        if definitions[i].Access.Provider != p.Name() || len(definitions[i].Authority.Capabilities) != 1 || definitions[i].Authority.Capabilities[0] != name {
            t.Fatalf("unscoped definition: %+v", definitions[i])
        }
    }
}

func TestProviderOrgActiveForwardsFiltersWithoutAction(t *testing.T) {
    p, _ := New(Config{Command: "/bin/true", Organization: "Contoso"})
    dials := 0
    session := &fakeSession{tools: []string{"repo_pull_request_org"}}
    p.dial = func(context.Context) (mcpSession, error) { dials++; return session, nil }
    if _, err := p.Call(t.Context(), "ado.pr.org_active", nil); err != nil {
        t.Fatal(err)
    }
    if _, err := p.Call(t.Context(), "ado.pr.org_active", map[string]any{"status": "active"}); err != nil {
        t.Fatal(err)
    }
    for _, bad := range []any{
        map[string]any{"action": "list"},
        map[string]any{"tool": "repo_pull_request"},
    } {
        if _, err := p.Call(t.Context(), "ado.pr.org_active", bad); err == nil {
            t.Fatalf("accepted org_active: %#v", bad)
        }
    }
    if dials != 2 {
        t.Fatalf("dialed %d times, want 2", dials)
    }
    if len(session.called) != 2 || session.called[0] != "repo_pull_request_org" {
        t.Fatalf("called tools: %v", session.called)
    }
}

func TestProviderForwardsThreadFileAndBuildCalls(t *testing.T) {
    p, _ := New(Config{Command: "/bin/true", Organization: "Contoso"})
    session := &fakeSession{tools: []string{"repo_pull_request_thread", "repo_file", "pipelines_build"}}
    p.dial = func(context.Context) (mcpSession, error) { return session, nil }
    calls := []struct {
        capability string
        request    map[string]any
        tool       string
    }{
        {"ado.pr.threads", map[string]any{"action": "list_comments", "pullRequestId": 7, "threadId": 3}, "repo_pull_request_thread"},
        {"ado.pr.file", map[string]any{"action": "get_content", "path": "main.go"}, "repo_file"},
        {"ado.build.status", map[string]any{"action": "get_status", "buildId": 9}, "pipelines_build"},
    }
    for _, call := range calls {
        if _, err := p.Call(t.Context(), call.capability, call.request); err != nil {
            t.Fatalf("%s: %v", call.capability, err)
        }
    }
    if len(session.called) != len(calls) {
        t.Fatalf("called tools: %v", session.called)
    }
    for i, call := range calls {
        if session.called[i] != call.tool {
            t.Fatalf("call[%d] tool = %q, want %q", i, session.called[i], call.tool)
        }
    }
    for _, bad := range []struct {
        capability string
        request    map[string]any
    }{
        {"ado.pr.threads", map[string]any{"action": "create"}},
        {"ado.pr.file", map[string]any{"action": "delete"}},
        {"ado.build.status", map[string]any{"action": "list"}},
        {"ado.build.status", map[string]any{"action": "get_status", "tool": "pipelines_write"}},
    } {
        if _, err := p.Call(t.Context(), bad.capability, bad.request); err == nil {
            t.Fatalf("accepted %s: %#v", bad.capability, bad.request)
        }
    }
}
```

Update `TestProviderAdvertisesOnlyExplicitReadCapabilities` (or the Task 1 renamed equivalent) to expect 8 definitions with the full name order.

- [ ] **Step 2: Run them, expect FAIL.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adomcp -run 'TestProvider(AdvertisesAllEightCapabilities|OrgActiveForwardsFiltersWithoutAction|ForwardsThreadFileAndBuildCalls)$' -count=1`. Expected: FAIL (unsupported capabilities).
- [ ] **Step 3: Implement.** In `provider.go`:

Add to `allowedActions`:

```go
    "ado.pr.threads":    {"list": true, "list_comments": true},
    "ado.pr.file":       {"get_content": true, "list_directory": true},
    "ado.build.status":  {"get_status": true},
```

(`ado.pr.org_active` needs no entry — it accepts no action. `allowedAction` on a missing capability returns false, which is correct for any action value.)

Extend `toolFor`:

```go
    case "ado.pr.org_active":
        return "repo_pull_request_org", nil
    case "ado.pr.threads":
        return "repo_pull_request_thread", nil
    case "ado.pr.file":
        return "repo_file", nil
    case "ado.build.status":
        return "pipelines_build", nil
```

Extend `Advertise` (append after `ado.pr.get`):

```go
        p.definition("ado.pr.org_active", "repo_pull_request_org"),
        p.definition("ado.pr.threads", "repo_pull_request_thread"),
        p.definition("ado.pr.file", "repo_file"),
        p.definition("ado.build.status", "pipelines_build"),
```

Add `Call` branches (same switch, before `callCtx`):

```go
    case "ado.pr.org_active":
        if request != nil {
            m, ok := request.(map[string]any)
            if !ok {
                return nil, errors.New("org PR request must be an object")
            }
            if _, has := m["action"]; has {
                return nil, errors.New("org PR listing accepts no action")
            }
            if _, bypass := m["tool"]; bypass {
                return nil, errors.New("MCP tool override is forbidden")
            }
            for key, value := range m {
                args[key] = value
            }
        }
    case "ado.pr.threads", "ado.pr.file", "ado.build.status":
        m, ok := request.(map[string]any)
        if !ok {
            return nil, errors.New("request must be an object")
        }
        action, ok := m["action"].(string)
        if !ok || !allowedAction(capability, action) {
            return nil, fmt.Errorf("action %q is not allowed for %q", action, capability)
        }
        if _, bypass := m["tool"]; bypass {
            return nil, errors.New("MCP tool override is forbidden")
        }
        for key, value := range m {
            args[key] = value
        }
```

- [ ] **Step 4: Run package tests, expect PASS.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adomcp -count=1`. Expected: PASS.
- [ ] **Step 5: Run full verification.** Run in order, all must pass:
  - `GOCACHE=/tmp/summa42-full-go-cache go test ./... -count=1` (expect all `ok`)
  - `go vet ./...` (expect exit 0), `git diff --check` (clean), `gofmt -l` on touched files (no output)
- [ ] **Step 6: Commit.** `git add internal/adomcp/provider.go internal/adomcp/provider_test.go && git commit -m "feat: add ADO PR thread, file, org and build capabilities"`.

## Self-review

- Spec coverage: capability table → Tasks 1–2 branches; per-capability binding → `allowedActions` + deleted `readAction`; org_active rule → Task 2 branch; per-branch tool guard → every new branch; paging note → spec-only (observer slice); get_status-only → table + non-goal; advertise 8 + scoping → tests; unknown/mutating pre-dial → reject tests with dial counters; live validation deferred → scope boundary, no live code.
- No placeholders: exact files, code, commands, expected outputs throughout.
- Type consistency: `allowedActions`/`allowedAction` names identical across tasks; `toolFor`/`Advertise`/`Call`/`Probe` use existing signatures; test names match run commands.
