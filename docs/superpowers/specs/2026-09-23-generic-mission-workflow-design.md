# Generic Mission workflow — design

## Intent

The Collective should turn a durable Mission and explicitly delegated authority into repeatable, observable work-and-assessment cycles. Azure DevOps pull-request review is the **first vertical slice**, not a workflow hardcoded into the Box. The same core process must later support other sources, executors, assessment rules and external effects without rewriting Task/Attempt scheduling or authorization. This does not mean a Mission's natural-language statement can invent a new credential, adapter, action or grant.

The user's first recipe is: observe active Azure Repos PRs where the authenticated user is a direct reviewer; ask the already configured Copilot CLI for a review; if findings exist, publish comments; if the review is clean and all gates pass, cast `Approve`. Copilot does not publish. The Box alone commits external effects.

## Approaches considered

1. **Hardcode the PR loop in Box:** quickest for this case, but source selection, decision and effects become inseparable. It cannot carry the Collective's broader Mission model.
2. **Delegate the entire Mission to a broad Copilot session:** resembles the manual workflow, but model-owned MCP writes bypass Task authority, budgets, fencing and protected effects.
3. **Small generic work/assessment loop with domain adapters (selected):** reuse existing Mission, Task/Attempt, Evidence, final Verification and External Operation primitives. Add intermediate assessment and next-work selection; implement ADO PR review as the first adapter/recipe.

## Core lifecycle

```text
Mission + Owner-approved domain grant
  → observation creates a case/goal and its first Work Task
  → Work Attempt → durable evidence
  → intermediate Assessment of progress, gaps, uncertainty and authority
      ├─ CONTINUE → bounded next Work Task → Assessment → ...
      ├─ BLOCKED → wait for missing capability, auth or Owner decision
      └─ READY → independent final Verification
                     ├─ rejected → bounded next Work Task or BLOCKED
                     └─ accepted → case complete; audit and next observation
```

The generic core owns stable case identity, scheduling, lease/fence, deduplication, retries, evidence, intermediate Assessment records, state transitions, budget and authority checks. It does **not** contain ADO fields, Copilot prompt text, or `Approve` logic. An observation has `source`, `object_id`, `revision_id`, timestamp and evidence; `(source, object_id, revision_id)` is the idempotency key. A recipe maps observations and prior assessments to the next Task proposal, executor input, assessment criteria and a finite set of permitted *proposed* effects. This is explicit configuration/code, not authority extracted from the Mission statement.

An Assessment is distinct from final Verification. It evaluates whether the last work produced sufficient evidence, what remains unknown, whether another bounded Task can make progress, or whether the case must be held. Its outcomes are `CONTINUE`, `READY_FOR_VERIFICATION`, and `BLOCKED`, with evidence and reasons. It may propose narrower child work under the same grant; it cannot approve its own result, expand authority or declare the Mission complete. Repetition is bounded by remaining budget, deadline, iteration limit and a no-progress check; an unresolved loop becomes `BLOCKED` rather than running forever.

Final Verification is independent of the executor and intermediate assessor. It confirms the case acceptance criteria and the durable outcome of any external effects. For a continuous Mission, completion closes the individual case/revision; the Mission remains active and the observer may later create work for a new revision or object. Dormancy between observations is legitimate.

The Box already implements much of the later lifecycle, but has no production worker loop, no usable CLI path for allocating a resource envelope to new Tasks, and no semantic Task objective/payload for the executor. The implementation must close those gaps generically rather than inserting PR-specific branches into `cmd/summa42-box`.

## Authority and model boundary

The Mission supplies purpose, never permission. The Owner issues a revocable, signed standing grant limited by domain, resource scope, action types, expiry, budget and required enforcement. The Collective may narrow or revoke it, never enlarge it. Observe and Work Tasks inherit subsets of this grant. Existing policy defaults deny non-LOW operations; approval must receive a reviewed, exact grant-bound policy rule, not be misclassified as LOW.

An executor may read only the capability set assigned to its Attempt and return evidence or a proposed decision. It cannot dispatch consequential writes. Intermediate Assessment may select the next Task or propose a typed effect, but the core validates current grant, lease, revision, budget, policy and recipe-specific gates immediately before preparing and dispatching any External Operation. An effect is itself work whose observed result is assessed before final verification. Unknown external outcomes are reconciled, never blindly repeated. Authentication expiry or missing capability produces a visible held state, not a clean verdict.

## First recipe: ADO PR review

An ADO source scans every page of active directly assigned PRs in one configured organization. Draft, self-authored and group-only assignments are initially excluded with visible reasons. Its revision key includes repository/PR identity and source **and target** commit IDs, so either changing forces a fresh review. The ADO read adapter must gain exact read-only PR, file, thread, linked-work-item and optional CI-status operations; the current provider only offers project and work-item reads.

The Copilot executor runs the already authenticated CLI as the same OS user in non-interactive mode. A dedicated invocation restricts available and approved tools to named ADO *read* tools; it grants no shell, file writes, ADO write tools or `--allow-all`. It returns a strict structured result: `CLEAN`, `FINDINGS` or `UNCERTAIN`, reviewed commits and files, and findings with path, line where known, explanation and evidence. Copilot output is untrusted; the Box checks format, fetched-file coverage and source/target freshness. These checks cannot prove that a model found every defect, so automatic approval retains explicit delegated model risk.

The ADO recipe has two ordered Work steps when the review can proceed: **Work 1 = Copilot review → Assessment 1 = choose comments/Approve/hold → Work 2 = Box publishes the chosen comments or vote → Assessment 2 = inspect/reconcile the publication → independent final Verification = close the case**. Discovery creates the case before Work 1. Assessment 1 records a decision but performs no external write; a hold ends this run before Work 2. A `FINDINGS` assessment proposes comments and never an approval. A complete `CLEAN` assessment may propose a vote only after all gates pass. The generic effect boundary receives typed `ado.pr.comment` and `ado.pr.approve` intents. A dedicated ADO operation provider performs the actual write and outcome lookup. Comments carry a durable correlation marker; vote reconciliation reads the current reviewer vote. No merge, code edit, reviewer reassignment or work-item mutation is allowed. CI must pass only if the Owner's grant requires that gate. An ambiguous finding location is held rather than guessed.

## Rollout and verification

Default mode is shadow: discover, review, record the proposed effects, publish nothing. The Owner can separately enable comments, then automatic votes under the scoped grant after testing on the actual ADO machine. The Box never asks for interactive reauthentication while unattended; expired daily auth pauses the affected work until the user signs in again.

Core tests use fake Source, Executor, Assessor, Verifier and Effect adapters to show the same iterative lifecycle works for at least two distinct work types. ADO-specific tests cover paginated reviewer discovery, PR revision changes, clean/finding/uncertain Copilot results, auth expiry, and comment/vote reconciliation. No live ADO organization is available here; stages that publish require a controlled smoke test on the user's test machine.

## Acceptance criteria

- The generic engine can run two different recipes without source/action-specific conditions in its core.
- Each observation revision yields at most one active case. Work 1 and Work 2 are distinct, ordered Tasks; a held case never starts Work 2. A new revision creates a new case.
- A Work Attempt is followed by intermediate Assessment; `CONTINUE` can create only bounded narrower work, while no-progress and exhausted budget become `BLOCKED`.
- Final Verification is separate from Assessment; a rejected result cannot be silently labeled complete.
- An executor's proposed effect cannot dispatch without inherited authority and fresh deterministic verification.
- Clean ADO review with all gates yields one `Approve`; findings yield comments and no vote; uncertain or incomplete output yields no effect.
- Restart, duplicate poll and unknown dispatch outcome do not duplicate comments or votes.
- Mission/grant revocation stops new work and effects immediately; auth expiry creates a visible hold.

## Sources

- [Azure DevOps MCP toolset](https://github.com/microsoft/azure-devops-mcp/blob/main/docs/TOOLSET.md)
- [Azure DevOps PR review behavior](https://learn.microsoft.com/en-us/azure/devops/repos/git/review-pull-requests?view=azure-devops)
- [Azure DevOps PR reviewer API](https://learn.microsoft.com/en-us/rest/api/azure/devops/git/pull-request-reviewers?view=azure-devops-rest-7.1)
- [Copilot CLI programmatic permissions](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-programmatic-reference)
