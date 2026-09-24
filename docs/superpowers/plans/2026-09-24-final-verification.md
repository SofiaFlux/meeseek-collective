# Final Verification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Independently re-verify published effects, finalize the Work 2 Task, and close the case durably.

**Architecture:** Migration `00015` adds `workflow_verifications` and the `CLOSED` case state (table rebuild); `workflowcase` gains `ListReadyForVerification`, `Close`, `Reject`; `adoreview.FinalVerifier` binds Work 2 identity, re-verifies via `LookupOutcome`, finalizes the task, and closes.

**Tech Stack:** Go 1.27, SQLite/goose, existing services, fakes; no live credentials.

## Global Constraints

- Verifier never dispatches; closure requires independent read-back confirmation.
- Transient `OUTCOME_UNKNOWN` keeps the case ready; deterministic failures `Reject` to `BLOCKED`.
- Closure never completes/deactivates the Mission.
- TDD first; package suite per commit; full suite + vet in the final task.

---

## File structure

- Create `internal/state/sqlite/migrations/00015_workflow_verifications.sql` (+ migration round-trip test in `internal/state/sqlite` following existing migration test patterns — read one first).
- Modify `internal/workflowcase/service.go`: `Closed`, `VerificationRequest/Record`, `Close`, `Reject`, `ListReadyForVerification`; tests.
- Create `internal/adoreview/verify.go`: `FinalVerifier`, `NewFinalVerifier`, `StepOnce`, `Run`; tests.
- Modify `cmd/summa42-box/main.go`: `run-final-verifier`; tests.
- Modify `docs/superpowers/tasks/2026-09-23-ado-pr-review-task.md`: checklist refresh.

### Task 1: Migration + case close/reject/list APIs

**Files:**
- Create: `internal/state/sqlite/migrations/00015_workflow_verifications.sql`
- Modify: `internal/state/sqlite/*_test.go` (migration round-trip; follow the file's existing up/down test pattern)
- Modify: `internal/workflowcase/service.go` + tests (new `close_test.go` following `find_test.go` style)

- [ ] **Step 1: Write the failing tests.** Migration: existing-case survives up/down round-trip with assessments intact; `workflow_verifications` row rejected for duplicate case_id. `workflowcase`: `Close` from ready state → case CLOSED + record; replay same snapshot returns record; drift errors; `Close` from ACTIVE/BLOCKED errors; `Reject` ready→BLOCKED only; `ListReadyForVerification` filters mission/state/order.
- [ ] **Step 2: Run, expect failures** — `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/state/sqlite ./internal/workflowcase -run 'TestMigration00015|TestClose|TestReject|TestListReady' -count=1`.
- [ ] **Step 3: Implement.** Migration: rebuild `workflow_cases` with 4-state CHECK (preserve rows, both UNIQUE constraints, indexes, `workflow_assessments` FK — read `00013_workflow_cases.sql` for the exact schema), create `workflow_verifications` (UNIQUE case_id, snapshot_hash/json, evidence_ids_json), Down reverses. Service: `Closed State = "CLOSED"`; `VerificationRequest{CaseID, VerifierID, VerifierType, SnapshotHash, SnapshotJSON, EvidenceIDs}`; `Close` = one transaction: validate purpose (mirror `Assess`), require case READY_FOR_VERIFICATION, require cited evidence rows exist (mirror `verification.requireEvidence` query), insert-or-replay by case_id (same hash + normalized IDs → return existing; else `ErrIntentConflict`), update case state/current_work_id=''/next_work_json='{}'/updated_at; `Reject(ctx, caseID, reason, evidenceIDs)` = transaction persisting a reason evidence blob row? — keep durable reason in `workflow_verifications`? NO: add a `reject` row? Simplest durable place: `Reject` writes the reason into the case's `initial_request_json`? That corrupts Ensure's replay equality. Decision: `Reject` inserts a row into `workflow_verifications` with `verifier_type='REJECT'`, `snapshot_json={"reason":...}` and flips the case to BLOCKED (one record either way). `ListReadyForVerification` mirrors `ListActive`'s query with state=READY_FOR_VERIFICATION.
- [ ] **Step 4: Run package tests, expect PASS** — same command without `-run` filter.
- [ ] **Step 5: Commit** — `git add internal/state/sqlite internal/workflowcase && git commit -m "feat: add case closure and rejection transitions"`. (Stage exact files.)

### Task 2: FinalVerifier

**Files:**
- Create: `internal/adoreview/verify.go`, `verify_test.go`

**Interfaces:**
- Consumes: Task 1 APIs; driver's `buildPublishIntents`/`decodePublisherEntries` (same package — reuse directly, do not duplicate); `LookupProvider` (driver.go); `verification.AcceptTask`; `evidence.Put/Get`.
- Produces: `FinalVerifier` for Task 3 wiring.

- [ ] **Step 1: Write the failing tests.** Full-stack (reuse driver_test.go fixtures/helpers — the Work 2 completed case): `TestFinalVerifierClosesConfirmedCase` (Task → SUCCEEDED, case CLOSED, record, replay idempotent, Mission still ACTIVE); `TestFinalVerifierRejectsTamperedDecision` (mutated decision blob → case BLOCKED with reason); `TestFinalVerifierHoldsOnUnknownOutcome` (LookupOutcome UNKNOWN → case stays READY_FOR_VERIFICATION); `TestFinalVerifierRejectsSlotMismatch` (extra/missing slot → BLOCKED); `TestFinalVerifierHandlesAcceptedTaskReplay` (Task already SUCCEEDED → closes without re-accepting).
- [ ] **Step 2: Run, expect compile failure** — `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoreview -run TestFinalVerifier -count=1`.
- [ ] **Step 3: Implement.** `FinalVerifierConfig{MissionID, Comment/Vote LookupProvider, VerifierID, VerifierType}` + service deps (cases/execution/evidence/verification). `StepOnce`: `ListReadyForVerification` → filter Source ado → per case: locate producing assessment + Work 2 task (same discovery helpers the driver uses — extract to a shared unexported helper in the same package if duplicated), bind payload identity (Decision/CaseID/WorkID/Revision/repo/PR), rebuild expected intents via `buildPublishIntents`, strict `decodePublisherEntries` + exact slot equality, per-intent `LookupOutcome` (confirmed → continue; UNKNOWN → leave ready; mismatch → `Reject`), finalize task (`AcceptTask` when AWAITING_VERIFICATION, verifier identity distinct from publisher, evidence = completion evidence; SUCCEEDED → proceed; other → leave ready), persist `ado.workflow.verification` evidence blob, `Close` with deterministic snapshot `{caseID, revision, workID, slots[], verdicts[]}` (sorted, no timestamps). `Run(ctx, interval)` mirrors driver cancellation semantics.
- [ ] **Step 4: Run package tests, expect PASS** — `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoreview -count=1`.
- [ ] **Step 5: Commit** — `git add internal/adoreview/verify.go internal/adoreview/verify_test.go && git commit -m "feat: independently verify and close ADO workflow cases"`.

### Task 3: run-final-verifier CLI, docs, full verification

**Files:**
- Modify: `cmd/summa42-box/main.go` (+ tests), `docs/superpowers/tasks/2026-09-23-ado-pr-review-task.md`

- [ ] **Step 1: Write the failing test** — flag parsing for `run-final-verifier` (`--mission` required, `--poll-interval` default 30s, optional `--project`, `--verifier-id`/`--verifier-type` defaults documented in code comments) mirroring worker/observer flag test style.
- [ ] **Step 2: Run, expect compile failure** — `GOCACHE=/tmp/summa42-full-go-cache go test ./cmd/summa42-box -run 'TestParseFinalVerifierFlags' -count=1`.
- [ ] **Step 3: Implement** — dispatch + `runFinalVerifier` mirroring `runDriver` (providers from ADO env builder, verifier from Box services, Run until cancel). Docs: replace the stale omnibus checklist item with checked scheduler/observer/review/effects/driver/verifier entries linking plans; leave live smoke unchecked; state the shadow default accurately (`SUMMA42_PUBLISH_MODE` default none; `run-observer` never publishes).
- [ ] **Step 4: Package tests PASS** — `GOCACHE=/tmp/summa42-full-go-cache go test ./cmd/... -count=1`.
- [ ] **Step 5: Full verification** — `GOCACHE=/tmp/summa42-full-go-cache go test ./... -count=1`, `go vet ./...`, `git diff --check`, `gofmt -l` touched files.
- [ ] **Step 6: Commit** — `git add cmd/summa42-box/main.go <exact-test-file> docs/superpowers/tasks/2026-09-23-ado-pr-review-task.md && git commit -m "feat: add run-final-verifier and refresh ADO checklist"`.

## Self-review

- Spec coverage: migration+states → Task 1; Close/Reject/ListReady → Task 1; verifier flow (bind, rebuild, verify, finalize, close) → Task 2; production path → Task 3; docs/defaults → Task 3; tests → all; acceptance → Task 2 tests.
- No placeholders: exact files, code-level contracts, commands; the Reject-record decision is pinned explicitly instead of left open.
- Type consistency: `Closed/VerificationRequest/Close/Reject/ListReadyForVerification/FinalVerifierConfig/NewFinalVerifier` fixed across tasks.
