# MVC Review Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make PR #7 buildable and close the release-blocking MVC review findings without expanding into post-MVC issues.

**Architecture:** Restore the corrupted sanitizer source first, then enforce effect identity and Task/Attempt linkage at the service boundaries. Strengthen immutable storage and provenance selection in SQLite, with focused regression tests for every security/property invariant.

**Tech Stack:** Go 1.27.1, modernc SQLite, Goose migrations, Go test.

**Spec:** `docs/superpowers/specs/2026-09-18-field-dogfooding-feedback-design.md`

## Global Constraints

- Keep Issues #2, #4, and #5 out of MVC scope.
- Feedback export remains fail-closed and must not bypass governed Task, Attempt, approval, or External Operation semantics.
- Attempt Run Manifests are immutable lease-time records and grant no authority.
- Unknown external outcomes cannot be converted to no-effect without authoritative evidence.

## Review Focus

- Equivalent sanitized artifacts for one sink must reuse one durable logical effect slot; test distinct artifact IDs and concurrent creation.
- An Attempt must belong to the envelope Task before it can emit feedback; test cross-task rejection.
- A missing remote marker after an uncertain dispatch must remain `OUTCOME_UNKNOWN`; test no-effect is never inferred from absence alone.
- `INSERT OR REPLACE` must not replace an immutable manifest; test the actual modernc store after migrations.
- Capability assessment metadata must match the active provider/access context; test re-registration before leasing.

---

### Task 1: Restore compilation and immutable manifest guard

**Files:**
- Modify: `internal/fieldfeedback/sanitizer.go:38-75`
- Modify: `internal/state/sqlite/migrations/00011_attempt_run_manifest.sql`
- Modify: `internal/state/sqlite/store_test.go`

- [ ] Add a failing store test executing `INSERT OR REPLACE` for an existing `attempt_run_manifests.attempt_id` and asserting an immutability error.
- [ ] Run the targeted store test and observe failure against the current trigger-only schema.
- [ ] Restore `safeExportMetadataToken` to `regexp.MustCompile(\`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$\`)`, remove duplicated source text, and add an insert trigger that rejects any existing manifest key.
- [ ] Run sanitizer, store, and package tests; commit the focused repair.

### Task 2: Bind feedback effects to canonical Task/Attempt identity

**Files:**
- Modify: `internal/fieldfeedback/emitter.go`
- Modify: `internal/fieldfeedback/executor.go`
- Modify: `internal/fieldfeedback/*_test.go`

- [ ] Add a failing test in the emit executor that gives artifact Task A and a live Attempt for Task B and expects rejection before `Prepare`.
- [ ] Add a failing test that separately creates equivalent sanitized artifacts for one provider/destination and asserts only one governed task/effect slot exists.
- [ ] Implement canonical Task/Attempt validation and fingerprint-based logical effect identity; preserve existing retry idempotency.
- [ ] Run focused fieldfeedback tests and commit.

### Task 3: Preserve uncertain remote effects and capability-provenance binding

**Files:**
- Modify: `internal/fieldfeedback/provider.go`
- Modify: `internal/fieldfeedback/provider_test.go`
- Modify: `internal/runmanifest/service.go`
- Modify: `internal/runmanifest/service_test.go`

- [ ] Add a failing reconciliation test asserting a missing marker returns `OUTCOME_UNKNOWN`, not `CONFIRMED_NO_EFFECT`.
- [ ] Add a failing run-manifest test that re-registers a capability with a changed provider/access context and asserts the old assessment is not included.
- [ ] Implement the conservative reconciliation result and provider/context-filtered assessment lookup.
- [ ] Run focused provider and runmanifest tests and commit.

### Task 4: Verification and Sol review

**Files:**
- Modify: `docs/superpowers/plans/2026-09-21-mvc-review-remediation.md`

- [ ] Run `go test ./... -count=1`, `go test -race ./... -count=1`, `go vet ./...`, `go build ./cmd/summa42 ./cmd/summa42-box`, the SQLite spike, and OCI acceptance gate.
- [ ] Request read-only GPT-5.6 Sol review of `origin/main..HEAD`; resolve every Critical or Important finding through a new red-green cycle.
- [ ] Push the corrected branch, confirm GitHub checks or document quota evidence, request review approval, and merge #7 only after all local gates and review are green.
