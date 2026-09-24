# ADO write provider — design

## Intent

Publish review comments and approval votes through the existing protected-effect
machinery (`operations.Prepare/Dispatch/Settle` + effect slots + approval
gate + reconciler). No new dispatch machinery; only the ADO-specific provider.

## Approaches considered

1. **Separate `adoeffects` package (selected):** `adomcp` stays read-only by
   construction; writes live behind the `operations.Provider` contract with
   explicit intents, canonical fingerprints and read-back lookups.
2. **Extending `adomcp` with write capabilities:** breaks the read-only
   boundary the deploy relies on. Rejected.
3. **Direct MCP writes from the driver:** bypasses policy, approval, slots and
   reconciliation. Rejected.

## Intents

```go
CommentIntent{PR int64, Repository, Project string, Path string,
  Line int64, Body string, Marker string}
VoteIntent{PR int64, Repository, Project string, Vote int}
```

- Comment → MCP `repo_pull_request_thread_write`, action `create`, new thread
  carrying the durable correlation marker. No reply action in this slice.
- Vote → MCP `repo_pull_request_write`, action `vote`, Vote ∈ {10} (approve
  only — no rejection votes, enforced at intent construction).
- Never: merge, code edits, reviewer reassignment, work-item mutation — no
  such intents exist.
- `Capability()`: `ado.pr.comment` / `ado.pr.vote`, matching assessment
  `ProposedActions`. `CanonicalIntent`: compact JSON; slot fingerprint derives
  from it. `CostProfile`: low exposure, enforced envelope.

## Transport and lookup

Own minimal MCP dial (exec + CommandTransport + the adomcp env allowlist,
duplicated with a provenance comment). Reads for `LookupOutcome` arrive via
injected `ReadFunc(ctx, capability, request) (any, error)` (production:
`adomcp.Provider.Call`; tests: fake):

- Comment lookup: `repo_pull_request_thread` `list` → marker found →
  `CONFIRMED_EFFECT` with thread reference; absent → `OUTCOME_UNKNOWN`
  (reconciler re-checks, never blindly re-dispatches — slots own that).
- Vote lookup: current reviewer vote read-back → matches intent →
  `CONFIRMED_EFFECT`; otherwise `OUTCOME_UNKNOWN`.

Unknown descriptor types are rejected, never interpreted.

## Testing

Fake MCP dial seam (adomcp pattern): canonical intent stability, Dispatch tool
+ args per intent, marker/vote lookups both ways, unknown-intent rejection,
transport errors. No credentials.

## Scope boundary

No publisher executor, no driver loop, no reply/update/status thread actions,
no vote values besides approve. Shadow posture holds: nothing dispatches
without `operations` approval machinery.

## Acceptance criteria

- Both intents round-trip through Prepare→Dispatch→Settle in tests with slot
  dedup (same fingerprint never dispatches twice).
- Unknown outcomes reconcile via lookup, never via repeated Dispatch.
- `adomcp` untouched and still read-only.
