# ADO Write Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish ADO review comments and approval votes through effect slots with read-back reconciliation.

**Architecture:** New `internal/adoeffects` package with `CommentProvider`/`VoteProvider` implementing `operations.Provider`; MCP dial seam injected for fakes; reads for lookup via injected `ReadFunc`. No changes to `adomcp` or the operations service.

**Tech Stack:** Go 1.27, `operations.Provider` contract, fake MCP session in tests; no live credentials.

## Global Constraints

- No `ado.pr.vote` name anywhere: vote capability/intent is `ado.pr.approve`.
- Vote value is exactly 10; no rejection votes, merges, edits, reassignments, or work-item mutations exist.
- Dispatch/LookupOutcome return only CONFIRMED_EFFECT, CONFIRMED_NO_EFFECT, OUTCOME_UNKNOWN.
- `adomcp` untouched.
- TDD: failing test first for every behavior, then minimal implementation.
- Package suite before each task commit; full `go test ./... -count=1` and `go vet ./...` in Task 2 before the final commit.

---

## File structure

- Create `internal/adoeffects/effects.go`: `Config`, intents, both providers, dial seam, `ReadFunc`.
- Create `internal/adoeffects/effects_test.go`: fake session, intent/dispatch/lookup tests.

### Task 1: Intents, canonical form, dispatch

**Files:**
- Create: `internal/adoeffects/effects.go`
- Create: `internal/adoeffects/effects_test.go`

**Interfaces:**
- Consumes: `operations.Provider/IntentDescriptor/ProviderOutcome`, `domain` states, `resources.Enforceability` (follow `internal/fieldfeedback/provider.go` as the exact template — read it first).
- Produces: `CommentIntent`, `VoteIntent`, `CommentProvider`, `VoteProvider`, `Config`, `ReadFunc` used by Task 2.

- [ ] **Step 1: Write the failing tests.** In `effects_test.go` (`package adoeffects`):

```go
type fakeDial struct {
    calls []mcpCall
    pages []map[string]any
    err   error
}

type mcpCall struct {
    tool string
    args map[string]any
}

func (f *fakeDial) dial(context.Context) (callToolFunc, error) {
    return func(_ context.Context, tool string, args map[string]any) (map[string]any, error) {
        f.calls = append(f.calls, mcpCall{tool, args})
        if f.err != nil {
            return nil, f.err
        }
        if len(f.pages) == 0 {
            return map[string]any{"ok": true}, nil
        }
        page := f.pages[0]
        f.pages = f.pages[1:]
        return page, nil
    }, nil
}
```

The seam is the `callToolFunc` func type (plain maps, no MCP SDK imports in
tests); production code adapts the real session to it. `context` and `testing`
imports required.

Wait — the real MCP `CallTool` signature comes from `mcp.CommandTransport` client session (`*mcp.CallToolParams` / `*mcp.CallToolResult` per adomcp's `mcpSession` interface). To avoid importing the MCP SDK with exact param types in the seam, define the seam as `callToolFunc func(ctx context.Context, tool string, args map[string]any) (map[string]any, error)` — plain maps, no SDK types in our interface. The production adapter wraps the real session; fakes implement the func directly. Simpler and SDK-free. Use this func seam (not a session interface) everywhere below.

Tests:

```go
func TestCommentIntentCanonicalStable(t *testing.T) {
    p := &CommentProvider{}
    a, err := p.CanonicalIntent(CommentIntent{Project: "proj", Repository: "shop", PR: 7, Path: "main.go", Line: 3, Body: "nit", Marker: "[summa42:c:w]"})
    if err != nil {
        t.Fatal(err)
    }
    b, err := p.CanonicalIntent(CommentIntent{Project: "proj", Repository: "shop", PR: 7, Path: "main.go", Line: 3, Body: "nit", Marker: "[summa42:c:w]"})
    if err != nil {
        t.Fatal(err)
    }
    if string(a) != string(b) {
        t.Fatalf("canonical unstable:\n%s\n%s", a, b)
    }
    var decoded map[string]any
    if err := json.Unmarshal(a, &decoded); err != nil {
        t.Fatal(err)
    }
}

func TestProvidersRejectForeignIntents(t *testing.T) {
    comment := &CommentProvider{}
    if _, err := comment.CanonicalIntent(VoteIntent{PR: 1, Vote: 10}); err == nil {
        t.Fatal("comment provider accepted vote intent")
    }
    vote := &VoteProvider{}
    if _, err := vote.CanonicalIntent(CommentIntent{Body: "x"}); err == nil {
        t.Fatal("vote provider accepted comment intent")
    }
    if _, err := vote.CanonicalIntent(VoteIntent{PR: 1, Vote: -10}); err == nil {
        t.Fatal("vote provider accepted rejection vote")
    }
    if vote.Capability() != "ado.pr.approve" || comment.Capability() != "ado.pr.comment" {
        t.Fatalf("capabilities = %q %q", vote.Capability(), comment.Capability())
    }
}

func stubRead(context.Context, string, any) (any, error) {
    return nil, errors.New("no reads in dispatch test")
}

func TestDispatchCallsWriteTools(t *testing.T) {
    commentDial := &fakeDial{pages: []map[string]any{{"threadId": 9}}}
    comment := &CommentProvider{
        config: Config{Command: "/bin/true", Organization: "Contoso", Timeout: time.Minute},
        dial:   commentDial.dial,
        read:   stubRead,
    }
    canonical, err := comment.CanonicalIntent(CommentIntent{Project: "proj", Repository: "shop", PR: 7, Path: "main.go", Line: 3, Body: "nit", Marker: "[summa42:c:w]"})
    if err != nil {
        t.Fatal(err)
    }
    outcome, err := comment.Dispatch(context.Background(), operations.ProviderDispatchRequest{
        OperationID: "op-1", TaskID: "task-1", EffectSlotID: "slot-1",
        IntentFingerprint: "fp", IntentRevision: 1, CanonicalIntent: canonical,
    })
    if err != nil {
        t.Fatal(err)
    }
    if outcome.State != domain.OperationConfirmedEffect && outcome.State != domain.OperationOutcomeUnknown {
        t.Fatalf("state = %q", outcome.State)
    }
    if len(commentDial.calls) != 1 {
        t.Fatalf("calls = %v, want exactly 1", commentDial.calls)
    }
    call := commentDial.calls[0]
    if call.tool != "repo_pull_request_thread_write" || call.args["action"] != "create" {
        t.Fatalf("call = %+v", call)
    }
    voteDial := &fakeDial{pages: []map[string]any{{"vote": 10}}}
    vote := &VoteProvider{
        config: Config{Command: "/bin/true", Organization: "Contoso", Timeout: time.Minute},
        dial:   voteDial.dial,
        read:   stubRead,
    }
    voteCanonical, err := vote.CanonicalIntent(VoteIntent{Project: "proj", Repository: "shop", PR: 7, Vote: 10})
    if err != nil {
        t.Fatal(err)
    }
    if _, err := vote.Dispatch(context.Background(), operations.ProviderDispatchRequest{
        OperationID: "op-2", TaskID: "task-1", EffectSlotID: "slot-2",
        IntentFingerprint: "fp2", IntentRevision: 1, CanonicalIntent: voteCanonical,
    }); err != nil {
        t.Fatal(err)
    }
    if len(voteDial.calls) != 1 || voteDial.calls[0].tool != "repo_pull_request_write" || voteDial.calls[0].args["action"] != "vote" {
        t.Fatalf("calls = %+v", voteDial.calls)
    }
}
```

Test files use `package adoeffects` (internal) so struct-literal construction with
the fake seam compiles. `context`, `encoding/json`, `errors`, `testing`,
`time`, `domain`, `operations` imports as needed.

- [ ] **Step 2: Run them, expect compile failure.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoeffects -count=1`. Expected: FAIL (undefined `CommentProvider`, `VoteIntent`, etc.).
- [ ] **Step 3: Implement `effects.go`.** (`package adoeffects`)

```go
type Config struct {
    Command      string
    Organization string
    Timeout      time.Duration
}

type callToolFunc func(ctx context.Context, tool string, args map[string]any) (map[string]any, error)

type ReadFunc func(ctx context.Context, capability string, request any) (any, error)

type CommentIntent struct {
    Project    string `json:"project"`
    Repository string `json:"repository"`
    PR         int64  `json:"pr"`
    Path       string `json:"path"`
    Line       int64  `json:"line"`
    Body       string `json:"body"`
    Marker     string `json:"marker"`
}

type VoteIntent struct {
    Project    string `json:"project"`
    Repository string `json:"repository"`
    PR         int64  `json:"pr"`
    Vote       int64  `json:"vote"`
}

func (CommentIntent) DescriptorType() string { return "ado.pr.comment" }
func (VoteIntent) DescriptorType() string    { return "ado.pr.approve" }
```

Providers (follow `fieldfeedback/provider.go` method shapes exactly):

```go
type CommentProvider struct {
    config Config
    dial   func(context.Context) (callToolFunc, error)
    read   ReadFunc
}

func NewCommentProvider(config Config, read ReadFunc) (*CommentProvider, error) {
    // trim/require Command + Organization (mirror adomcp.New validation incl. timeout default 15s)
    // read may be nil only if... no: require non-nil (lookup is mandatory)
}

type VoteProvider struct {
    config Config
    dial   func(context.Context) (callToolFunc, error)
    read   ReadFunc
}

func NewVoteProvider(config Config, read ReadFunc) (*VoteProvider, error) {
    // same validation as NewCommentProvider
}

func (p *CommentProvider) Name() string { return "ado-pr-comment" }
func (p *CommentProvider) Capability() string { return "ado.pr.comment" }
func (p *CommentProvider) EnforcementLevel() domain.EnforcementLevel { return domain.EnforcementEnforced }
func (p *CommentProvider) AdapterVersion() string { return "ado-effects-v1" }
func (p *CommentProvider) AdapterVersionSemanticallyRelevant() bool { return false }

func (p *CommentProvider) CanonicalIntent(descriptor operations.IntentDescriptor) ([]byte, error) {
    intent, ok := descriptor.(CommentIntent)
    if !ok {
        if pointed, ok := descriptor.(*CommentIntent); ok && pointed != nil {
            intent = *pointed
        } else {
            return nil, fmt.Errorf("comment provider rejects descriptor type %T", descriptor)
        }
    }
    intent.Project = strings.TrimSpace(intent.Project)
    // ... trim all strings; require project, repository, PR > 0, body, marker non-blank
    return json.Marshal(intent)
}

func (p *CommentProvider) CostProfile(descriptor operations.IntentDescriptor) (operations.CostProfile, error) {
    if _, err := p.CanonicalIntent(descriptor); err != nil {
        return operations.CostProfile{}, err
    }
    return operations.CostProfile{MaxExposure: 1, Enforceability: resources.Enforceability{
        CostControl: resources.CostTechnicallyCapped, RequireHardCap: false, Source: "ado pr comment",
    }}, nil
}

func (p *CommentProvider) Dispatch(ctx context.Context, request operations.ProviderDispatchRequest) (operations.ProviderOutcome, error) {
    var intent CommentIntent
    if err := json.Unmarshal(request.CanonicalIntent, &intent); err != nil {
        return operations.ProviderOutcome{}, err
    }
    // re-validate via CanonicalIntent(intent) to fail closed on tampered canonical bytes
    call, err := p.dial(ctx)  // with timeout like adomcp.Call
    if err != nil {
        return operations.ProviderOutcome{}, err
    }
    body := intent.Body + "\n" + intent.Marker
    _, err = call(ctx, "repo_pull_request_thread_write", map[string]any{
        "action": "create", "project": intent.Project, "repository": intent.Repository,
        "pullRequestId": intent.PR,
        "thread": map[string]any{"comments": []any{map[string]any{"content": body}}, "status": "active"},
    })
    if err != nil {
        return operations.ProviderOutcome{}, err
    }
    return operations.ProviderOutcome{State: domain.OperationDispatched}, nil
}
```

Hmm — what State does Dispatch return? Check `operations` Dispatch flow: service.Dispatch calls provider.Dispatch then interprets outcome... `ProviderOutcome{State...}` with confirmed states enforced (service.go:458-464). Does Dispatch expect DISPATCHED (not a defined state — states are PREPARED/DISPATCHED? No: states list has no DISPATCHED! PREPARED, CONFIRMED_EFFECT, CONFIRMED_NO_EFFECT, OUTCOME_UNKNOWN, CANCELLED). So provider Dispatch must return one of the confirmed/unknown states. For a fire-and-forget write with no read-back in Dispatch: return...? Look at fieldfeedback Dispatch resolve: it returns what? The implementer must read `internal/fieldfeedback/provider.go` Dispatch/LookupOutcome + `operations/service.go:284-330` (Dispatch) to mirror exact outcome semantics (likely: attempt lookup-inside-dispatch then CONFIRMED_EFFECT or OUTCOME_UNKNOWN). Do NOT guess: read those ranges first, then implement to match. If Dispatch-after-write immediately looks up the marker/vote and confirms, Dispatch returns CONFIRMED_EFFECT / OUTCOME_UNKNOWN — elegant (write+verify in one step). Pin that: Dispatch writes, then runs the same lookup as LookupOutcome and returns its state.

`VoteProvider` mirrors with `repo_pull_request_write` + `{"action":"vote", ..., "vote": 10}`, Vote != 10 rejected in CanonicalIntent.

Production dial: `exec.CommandContext` + MCP client `Connect` like `adomcp.connect`, wrapped to `callToolFunc` translating to/from `mcp.CallToolParams`/`CallToolResult` (check adomcp's exact `mcp` SDK call shape first). Env allowlist: duplicate adomcp's 12-entry map with provenance comment `// mirrored from internal/adomcp adoEnvironment; export a shared helper as follow-on`.

- [ ] **Step 4: Run package tests, expect PASS.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoeffects -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** `git add internal/adoeffects/effects.go internal/adoeffects/effects_test.go && git commit -m "feat: add ADO comment and approve providers"`.

### Task 2: Lookup outcomes, harness round-trip, full verification

**Files:**
- Modify: `internal/adoeffects/effects.go` (lookup methods)
- Modify: `internal/adoeffects/effects_test.go` (lookup + round-trip tests)

**Interfaces:**
- Consumes: providers + intents from Task 1; `operations.Service` test harness patterns in `internal/operations/service_test.go` (read the fake-provider + approvals setup first and mirror it).
- Produces: complete write provider; Slice 3b driver consumes `CommentProvider`/`VoteProvider` + slot keys.

- [ ] **Step 1: Write the failing tests.**

```go
func TestCommentLookupFindsMarker(t *testing.T) {
    // provider with dial (unused) + read fake returning threads payload:
    // map[string]any{"threads": []any{map[string]any{"comments": []any{map[string]any{"content": "nit\n[summa42:c:w]"}}}}}
    // LookupOutcome with canonical of CommentIntent{..., Marker: "[summa42:c:w]"}
    // assert State == CONFIRMED_EFFECT
    // second lookup with threads lacking the marker → OUTCOME_UNKNOWN
}

func TestVoteLookupConfirmsMatchingVote(t *testing.T) {
    // read fake returning map[string]any{"reviewers": []any{map[string]any{"vote": float64(10)}}}
    // LookupOutcome canonical VoteIntent{Vote: 10} → CONFIRMED_EFFECT
    // reviewers vote 0 → OUTCOME_UNKNOWN
}
```

Lookup request parsing: threads payload shape `{"threads": [...]}` with nested `comments[].content` (provisional wire assumption, consistent with plan's documented-assumption rule); reviewers `{"reviewers": [{"vote": N}]}` matching any reviewer with equal vote (server-identity-agnostic: any 10 counts — the write executes as one identity; conservative: match ANY reviewer vote == intent).

Harness round-trip test: build a minimal `operations.Service` using the `service_test.go` harness pattern (read that file first for the constructor args: store/clock/execution/policy/resources/approvals/collectiveID + fake provider registration) with the real `CommentProvider` (fake dial returning success + read fake returning the marker) registered, then `Prepare` (with a real task+attempt fixture? This needs execution fixtures — follow service_test.go fixture builders exactly) → `Dispatch` → assert terminal CONFIRMED_EFFECT and that a second `Prepare`+`Dispatch` with the same slot key does not re-dial (slot dedup: dial call count == 1). If the harness requires approval plumbing beyond a small setup, use the test file's fake approval path verbatim — do not invent approvals wiring.

- [ ] **Step 2: Run them, expect compile failure.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoeffects -run 'TestCommentLookupFindsMarker|TestVoteLookupConfirmsMatchingVote' -count=1`. Expected: FAIL (undefined `LookupOutcome`).
- [ ] **Step 3: Implement lookups.**

```go
func (p *CommentProvider) LookupOutcome(ctx context.Context, request operations.ProviderDispatchRequest) (operations.ProviderOutcome, error) {
    var intent CommentIntent
    if err := json.Unmarshal(request.CanonicalIntent, &intent); err != nil {
        return operations.ProviderOutcome{}, err
    }
    raw, err := p.read(ctx, "ado.pr.threads", map[string]any{"action": "list", "project": intent.Project, "repository": intent.Repository, "pullRequestId": intent.PR})
    if err != nil {
        return operations.ProviderOutcome{State: domain.OperationOutcomeUnknown}, nil
    }
    if threadContainsMarker(raw, intent.Marker) {
        return operations.ProviderOutcome{State: domain.OperationConfirmedEffect, ProviderReference: ...}, nil
    }
    return operations.ProviderOutcome{State: domain.OperationOutcomeUnknown}, nil
}
```

Read errors → OUTCOME_UNKNOWN with nil error (reconciler retries lookup; transport failure is not a negative). `threadContainsMarker` walks `threads[].comments[].content` substring match. Vote mirror with reviewers[].vote numeric compare (float64/int64/json.Number tolerant helper).

- [ ] **Step 4: Run package tests, expect PASS.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoeffects -count=1`. Expected: PASS.
- [ ] **Step 5: Run full verification.** In order: `GOCACHE=/tmp/summa42-full-go-cache go test ./... -count=1`, `go vet ./...`, `git diff --check`, `gofmt -l` on touched files. All must pass.
- [ ] **Step 6: Commit.** `git add internal/adoeffects/effects.go internal/adoeffects/effects_test.go && git commit -m "feat: reconcile ADO comment and vote outcomes"`.

## Self-review

- Spec coverage: intents + vote-10 rule → Task 1 structs/validation; provider values (names/capabilities/version/cost/descriptor) → Task 1 methods; canonical/fingerprint → stable-bytes test; transport/config → dial + Config; lookup rules (marker/vote, unknown-not-negative) → Task 2; testing list → all; acceptance (harness round-trip + slot dedup + adomcp untouched) → Task 2 test + `git status` scope.
- No placeholders: exact files, code, commands, outputs; the two "read first" notes name exact files/ranges.
- Type consistency: `CommentIntent`/`VoteIntent`/`CommentProvider`/`VoteProvider`/`Config`/`ReadFunc`/`callToolFunc` identical across tasks; `ado.pr.comment`/`ado.pr.approve`, `ado-effects-v1`, thread/vote tool+action literals fixed.
