# Final verification and case closure — design

## Intent

Close the lifecycle independently: a case in `READY_FOR_VERIFICATION` is
re-verified by read-back (trusting no intermediate verdict), its Work 2 Task
is finalized, and the case is closed with a durable verification record.

## Approaches considered

1. **Independent re-verification in `adoreview` (selected):** the verifier
   re-runs effect read-backs itself and closes the case.
2. **Trust the driver's assessment:** not independent. Rejected.
3. **`AcceptTask` as the closure mechanism:** it finalizes a task, not the case.
   Used here as the task-finalization step, not the case closer. Rejected as
   sole mechanism.

## Schema and state

Migration `00015_workflow_verifications.sql`:

- rebuild `workflow_cases` with a four-state CHECK
  (`ACTIVE|BLOCKED|READY_FOR_VERIFICATION|CLOSED`), preserving rows,
  uniqueness (`UNIQUE (mission_id, source, object_id, revision_id)`,
  `UNIQUE (case_id, work_id)`), indexes, and the `workflow_assessments` FK;
- create `workflow_verifications` (verification_id, case_id UNIQUE,
  verifier_id, verifier_type, snapshot_hash, snapshot_json, evidence_ids_json,
  created_at);
- Down reverses both steps.

`workflowcase.Closed` joins the other `State` constants (case states live in
`workflowcase`, not `domain/states.go`). On close: `current_work_id=''`,
`next_work_json='{}'` (both NOT NULL).

`Close(ctx, VerificationRequest{CaseID, VerifierID, VerifierType, Snapshot
VersionedSnapshot, EvidenceIDs})`: allowed only from
`READY_FOR_VERIFICATION`; one transaction: require every cited evidence row
to exist (mirror `AcceptTask`'s check), insert the verification (UNIQUE
case_id), flip the case. Replay: identical `snapshot_hash` + identical
normalized evidence IDs returns the existing record; material drift errors.
The snapshot is deterministic (no timestamps/random IDs) so concurrent
identical verifications hash equal despite fresh evidence rows.

`Reject(ctx, caseID, reason, evidenceIDs)`: `READY_FOR_VERIFICATION →
BLOCKED` for deterministic rejections (tampered evidence, missing/unexpected
slots, invalid payload identity). Transient `OUTCOME_UNKNOWN` never rejects —
the case stays ready for the next tick.

`ListReadyForVerification(ctx, missionID)`: same filter/order conventions as
`ListActive`.

## FinalVerifier flow

Per mission's ready cases with `Source == "ado"`:

1. Locate Work 2 via the producing assessment's Work ID
   (`FindByIdempotencyKey`), decode its `PublishPayload` strictly, and bind
   `Decision/CaseID/WorkID/Revision/repo/PR` against the loaded decision and
   case — mismatch → `Reject` with evidence.
2. Rebuild the expected intents with the existing `buildPublishIntents` and
   decode publication evidence with `decodePublisherEntries`; require exact
   slot equality, no duplicate/skipped/recorded-only entries.
3. Verify each expected intent read-only through the matching adoeffects
   `LookupOutcome`. All `CONFIRMED_EFFECT` → proceed; deterministic mismatch
   → `Reject`; `OUTCOME_UNKNOWN` → leave ready.
4. Finalize the Task: if `AWAITING_VERIFICATION` call
   `verification.AcceptTask` with a distinct verifier identity and the
   completion evidence; if already `SUCCEEDED` (crash-window replay) continue;
   any other state → leave ready.
5. `Close` with a snapshot `{caseID, revision, workID, slots[], verdicts}`
   plus evidence IDs (decision, completion, re-verification JSON blob of kind
   `ado.workflow.verification`).

Closure never completes or deactivates the Mission; new revisions remain
observable under the same active Mission.

## Production path

`run-final-verifier` command in `cmd/summa42-box` (`--mission`,
`--poll-interval`, optional `--project`), composed like `run-driver`:
providers from the ADO env, verifier from Box services, loop with the same
cancellation semantics.

## Documentation and defaults

ADO PR checklist: replace the stale omnibus item with checked entries for
scheduler, observer, review executor, protected effects, driver, and verifier;
leave live smoke unchecked. Defaults: the worker's `SUMMA42_PUBLISH_MODE`
stays `none` (shadow), and `run-observer` never publishes.

## Testing

Fake `LookupProvider` + full stack: close happy path (Task SUCCEEDED, case
CLOSED, record, replay idempotent, drift error); close refuses non-ready
states; reject path (tampered decision, slot mismatch) blocks the case;
unknown outcome leaves the case ready; AcceptTask crash-window replay; mission
remains active. No live credentials.

## Acceptance criteria

- A case closes only after independent read-back confirms every effect and the
  Work 2 Task is finalized.
- Deterministic failures block the case with a reason; transient unknowns keep
  it ready.
- Closure is durable, replay-safe, and never completes the Mission.
