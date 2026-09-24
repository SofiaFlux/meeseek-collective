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
CommentIntent{Project, Repository string, PR int64, Path string,
  Line int64, Body string, Marker string}
VoteIntent{Project, Repository string, PR int64, Vote int}
```

- Comment → MCP `repo_pull_request_thread_write`, action `create`
  (TOOLSET repos table). No reply/update actions in this slice.
- Vote → MCP `repo_pull_request_write`, action `vote`. `Vote` ∈ {10}
  (approve only), enforced in `CanonicalIntent` validation.
- No reviewer identity field: the write executes as the server's authenticated
  identity (azcli session), same as reads. No merge/edit/reassign/work-item
  intents exist, ever.
- Marker: `CommentIntent.Marker` (driver-built from case/work IDs, e.g.
  `[summa42:<case>:<work>]`) appended as the final body line by the provider
  (provider owns placement, driver owns content).

Argument field sets beyond tool+action follow the MCP tool schemas as
documented assumptions, verified in the live smoke follow-on; transport and
tool errors fail closed (error, never silent success).

## Provider values (fieldfeedback mirror)

- Comment: `Name() "ado-pr-comment"`, `Capability() "ado.pr.comment"`.
- Vote: `Name() "ado-pr-vote"`, `Capability() "ado.pr.approve"` (matches
  assessment `ProposedActions`; there is no `ado.pr.vote` anywhere).
- Both: `EnforcementLevel() Enforced`, `AdapterVersion() "ado-effects-v1"`,
  `AdapterVersionSemanticallyRelevant() false`.
- `DescriptorType()` returns the capability string; unknown descriptor types
  (including pointer/value mismatches handled explicitly like the template)
  are rejected, never interpreted.
- `CanonicalIntent`: trim strings, validate (non-blank project/repo/marker,
  PR > 0, non-blank body, vote == 10), then compact `json.Marshal`. Slot
  fingerprinting hashes provider + descriptor + intent (adapter version
  excluded while irrelevant, per the service formula).
- `CostProfile`: `MaxExposure 1`, `TechnicallyCapped`, no hard-cap
  requirement, source `"ado pr comment"` / `"ado pr vote"`.
- `Dispatch`/`LookupOutcome` return only `CONFIRMED_EFFECT`,
  `CONFIRMED_NO_EFFECT`, `OUTCOME_UNKNOWN` (the service enforces this).

## Transport and lookup

Config `{Command, Organization, Timeout}` injected (adomcp.Config mirror).
Own minimal MCP dial; the env allowlist duplicates adomcp's with a provenance
comment (exporting a shared helper is an explicit follow-on — drift risk
acknowledged). Reads for `LookupOutcome` arrive via injected `ReadFunc(ctx,
capability, request) (any, error)` (production: `adomcp.Provider.Call`):

- Comment lookup: existing `ado.pr.threads` (`list`) scans for the marker →
  found = `CONFIRMED_EFFECT` with thread reference; absent =
  `OUTCOME_UNKNOWN` (reconciler re-checks, never blindly re-dispatches).
- Vote lookup: existing `ado.pr.get` reviewers array → reviewer vote equals
  intent → `CONFIRMED_EFFECT`, else `OUTCOME_UNKNOWN`. Reviewer/vote response
  paths are assumed from REST conventions and confirmed live; if the shape
  lacks votes, the lookup stays unknown (safe) and an explicit read capability
  becomes follow-on work. `adomcp` itself is untouched.

## Slot and approval binding (driver-owned, stated here)

- Comment slot key: `ado.pr.comment:<repo>#<pr>:<revision>:<index>`;
  vote slot key: `ado.pr.approve:<repo>#<pr>:<revision>`. Trusted (driver-
  derived from case/work/decision, never model-authored).
- `Prepare` risk/attributes/required-approvers and the authority-ceiling
  entries covering `ado.pr.comment`/`ado.pr.approve` are the Slice 3b driver
  contract (owner approvals). This slice defines intents + provider only.

## Shadow posture (corrected)

Shadow means the driver records intents and never calls
`Prepare`/`Dispatch` — approval machinery alone is not shadow. Staged
enablement (`none` → `comments` → `all` including approve) is driver config
in Slice 3b. This slice's acceptance round-trips intents through the
operations service test harness only (following
`internal/operations/service_test.go` patterns); production shadow publishes
nothing.

## Testing

Fake MCP dial seam (adomcp pattern): canonical intent stability, Dispatch tool
+ action per intent, marker/vote lookups both ways, unknown-intent rejection,
transport errors. No credentials.

## Acceptance criteria

- Both intents round-trip Prepare→Dispatch→Settle in harness tests with slot
  dedup (same fingerprint never dispatches twice).
- Unknown outcomes reconcile via lookup, never via repeated Dispatch.
- `adomcp` untouched and still read-only.
