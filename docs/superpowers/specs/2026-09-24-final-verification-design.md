# Final verification and case closure — design

## Intent

Close the lifecycle: a case in `READY_FOR_VERIFICATION` is independently
re-verified (durable effects confirmed by read-back, not by trusting the
driver) and then closed with a durable verification record.

## Approaches considered

1. **Independent re-verification in `adoreview` (selected):** the verifier
   re-runs the effect read-backs itself (same code path as Assessment 2,
   trusting no intermediate verdict) and closes the case.
2. **Trust the driver's assessment:** not independent; a single bug could
   close cases on unverified effects. Rejected.
3. **Reuse `verification.AcceptTask` only:** it verifies task acceptance
   criteria, not the case outcome/effects; no case-closure state exists.
   Rejected as the closure mechanism.

## State and schema

New case state `CLOSED`. Migration creates `workflow_verifications`
(verification_id, case_id, verifier_id, verifier_type, criteria_json,
evidence_ids_json, created_at) and a unique index on case_id. `Close(ctx,
caseID, VerificationRecord)` transitions only from `READY_FOR_VERIFICATION`:
writes the verification row, sets state `CLOSED`, clears
`current_work_id`/`next_work_json`, bumps `updated_at` — one transaction,
idempotent by case_id (replay returns the existing record, drift errors).

## FinalVerifier

`FinalVerifierStepOnce`: cases of the configured mission in
`READY_FOR_VERIFICATION`; for each, rebuild every expected effect intent from
the stored decision and the Work 2 payload, verify each through the matching
`adoeffects` provider's `LookupOutcome` (read-only), and require:

- every expected slot present and `CONFIRMED_EFFECT`;
- no unexpected slots;
- the decision action matches the slots recorded at publication.

Satisfied → `Close` with a verification record citing the decision evidence,
the Work 2 completion evidence, and the re-verification evidence blob (the
verifier's own JSON snapshot, kind `ado.workflow.verification`). Anything else
→ skip (the case waits; operations reconciliation owns unresolved outcomes).

## Documentation and defaults

The ADO PR review checklist records the implemented chain (discover → review →
assess → publish (staged) → assess publication → verify → close) and leaves
live smoke as the remaining item. Shadow defaults stay: `run-observer` mode
`none` publishes nothing; without env no executors register.

## Testing

Fake `LookupProvider` + full stack: close happy path (case CLOSED, record
written, replay idempotent, drift error); close refuses non-READY states;
verifier holds on unknown/unexpected slots; verifier re-verification is
independent (a tampered decision blob causes hold, not closure). No live
credentials.

## Scope boundary

No live smoke automation, no UI, no re-verification retry loop (the verifier
re-checks on every tick). No case reopening in this slice.

## Acceptance criteria

- A case closes only after independent read-back confirms every effect.
- Closure is durable (record + state) and replay-safe.
- Holds never close a case.
