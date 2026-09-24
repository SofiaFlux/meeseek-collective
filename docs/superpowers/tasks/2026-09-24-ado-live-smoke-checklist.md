# ADO PR review — live smoke checklist

The full chain is implemented and unit-tested; this checklist covers the
controlled smoke run on the operator's machine. Nothing here is automated:
it requires the real ADO org, real Copilot CLI auth, and human eyes.

## Preconditions

- [ ] `summa42-box` binary built from the current `main`.
- [ ] Node.js + `npx` on PATH; on Linux `libsecret-1-0` installed (the MCP
      server's keytar dependency).
- [ ] ADO auth session (e.g. `az login`) valid and non-interactive.
- [ ] Copilot CLI installed and authenticated as the same OS user.
- [ ] `SUMMA42_ADO_MCP_COMMAND` / `SUMMA42_ADO_ORGANIZATION` exported; the
      wrapper starts `@azure-devops/mcp <org> -a azcli` with the org as the
      sole positional argument.
- [ ] Owner Mission created with grant capabilities **and** actions including
      `ado.pr.comment` (start without `ado.pr.approve`), plus a resource
      envelope for the observer (`--envelope`).
- [ ] A target PR exists where the operator is a direct reviewer, not draft,
      not self-authored, at a known revision.

## Shadow pass (default, publishes nothing)

- [ ] `run-observer --mission <id> --reviewer-id <id> --grant-capability read
      --grant-capability ado.pr.comment --grant-action ado.pr.comment
      --envelope <id> --max-steps 4 --remaining-budget 10`
      creates exactly one case + one Work 1 review Task for the PR revision.
- [ ] `run-worker --workspace-root <path>` (with `SUMMA42_COPILOT_PATH`,
      `SUMMA42_COPILOT_MCP_SERVER`, `SUMMA42_COPILOT_TOOLS` set) executes the
      review; the ADO read tools available to Copilot are the 7 read-only ones.
- [ ] The run manifest carries objective + payload hash; the attempt evidence
      contains the canonical review JSON.
- [ ] The assessment records a decision (comments/vote proposal or hold) and a
      Work 2 `publish-decision` Task appears with the effect capability in its
      authority ceiling.
- [ ] `run-driver` materializes Work 2; with `SUMMA42_PUBLISH_MODE` unset
      (default `none`) the publisher records intents and **nothing is written
      to ADO**.

## Staged enablement (only after the shadow pass is clean)

- [ ] `SUMMA42_PUBLISH_MODE=comments` + `SUMMA42_PUBLISH_RISK_COMMENT=LOW`
      (owner policy must allow LOW with the effect authority valid): exactly
      the review's comments appear on the PR, each with its correlation
      marker; the approve action is recorded as skipped.
- [ ] `run-final-verifier --mission <id>` re-verifies each comment by
      read-back, finalizes Work 2 (`SUCCEEDED`) and closes the case
      (`CLOSED`) with a durable verification record.
- [ ] Votes stay disabled (`ado.pr.approve` absent from the grant and
      `SUMMA42_PUBLISH_MODE=comments`); enabling votes requires an explicit
      `SUMMA42_PUBLISH_RISK_APPROVE` and a reviewed grant-bound policy rule —
      not `LOW`.

## Failure drills

- [ ] Kill the worker mid-attempt; after restart the lease is recovered and no
      duplicate evidence/completion appears.
- [ ] Make the ADO MCP server fail during a driver tick; the driver aborts
      visibly, cases stay `READY_FOR_VERIFICATION`, and the next tick resumes
      without duplicate effects.
- [ ] Post an unrelated ADO PR change mid-run: the revision key changes, a
      new case opens, and the old one remains closed/ready — never merged.
