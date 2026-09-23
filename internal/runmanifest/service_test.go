package runmanifest_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/purpose"
	"github.com/SofiaFlux/summa42/internal/runmanifest"
	state "github.com/SofiaFlux/summa42/internal/state/sqlite"
	"github.com/SofiaFlux/summa42/internal/teb"
	"github.com/SofiaFlux/summa42/internal/testutil"
)

func newManifestHarness(t *testing.T) (context.Context, *state.Store, *runmanifest.Service, *execution.Service, domain.ID) {
	t.Helper()
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC))
	profile := teb.PartialProfile("oci-test", teb.GuaranteeNonRoot, teb.GuaranteeWorkspaceOnlyWrite)
	manifests := runmanifest.New(store, runmanifest.StaticContext{
		Build: runmanifest.BuildMetadata{RuntimeVersion: "v0.1.0", RuntimeCommit: "abc123"},
		Policy: runmanifest.PolicySnapshot{
			PolicySetID: "policy_test", PolicySetHash: "policy-hash", PolicyCapabilitiesHash: "caps-hash",
		},
		TEBProfile: profile,
	})
	purposes := purpose.New(store, clk)
	missionID, err := purposes.CreateMission(ctx, "Exercise attempt run provenance")
	if err != nil {
		t.Fatal(err)
	}
	execSvc := execution.New(store, clk, purposes, manifests)
	return ctx, store, manifests, execSvc, missionID
}

func createManifestTask(t *testing.T, ctx context.Context, execSvc *execution.Service, missionID domain.ID) domain.Task {
	t.Helper()
	task, err := execSvc.CreateTask(ctx, execution.TaskRequest{
		Purpose:              domain.PurposeRef{Kind: domain.PurposeMission, ID: missionID},
		AcceptanceCriteria:   []string{"verified output"},
		RequiredCapabilities: []string{"cap-b", "cap-a"},
		RequiredEnforcement:  domain.EnforcementPartial,
		AuthorityCeiling:     []string{"cap-a", "cap-b"},
		ResourceEnvelopeID:   "env_manifest",
	})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestStartAttemptPersistsImmutableCanonicalRunManifest(t *testing.T) {
	ctx, store, manifests, execSvc, missionID := newManifestHarness(t)
	task := createManifestTask(t, ctx, execSvc, missionID)
	seedCapability(t, store, "cap_a_id", "cap-a", "v2", "provider-a", "assessment-a")

	attempt, err := execSvc.StartAttempt(ctx, task.ID, "codex", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manifests.Manifest(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}

	if record.Manifest.Version != 1 || record.Manifest.TaskID != task.ID || record.Manifest.AttemptID != attempt.ID {
		t.Fatalf("manifest identity = %#v", record.Manifest)
	}
	if record.Manifest.FenceGeneration != attempt.FenceGeneration || record.Manifest.ExecutorKind != "codex" {
		t.Fatalf("manifest fence/executor = %d/%q", record.Manifest.FenceGeneration, record.Manifest.ExecutorKind)
	}
	if record.Manifest.Runtime.RuntimeVersion != "v0.1.0" || record.Manifest.Runtime.RuntimeCommit != "abc123" {
		t.Fatalf("runtime snapshot = %#v", record.Manifest.Runtime)
	}
	if record.Manifest.Policy.PolicySetHash != "policy-hash" || record.Manifest.Policy.PolicyCapabilitiesHash != "caps-hash" {
		t.Fatalf("policy snapshot = %#v", record.Manifest.Policy)
	}
	if record.Manifest.TEB.Name != "oci-test" || record.Manifest.TEB.Level != domain.EnforcementPartial {
		t.Fatalf("TEB snapshot = %#v", record.Manifest.TEB)
	}
	if len(record.Manifest.TEB.Guarantees) != 2 ||
		record.Manifest.TEB.Guarantees[0] != string(teb.GuaranteeNonRoot) ||
		record.Manifest.TEB.Guarantees[1] != string(teb.GuaranteeWorkspaceOnlyWrite) {
		t.Fatalf("TEB guarantees = %v", record.Manifest.TEB.Guarantees)
	}
	if record.Manifest.ResourceEnvelopeID != "env_manifest" {
		t.Fatalf("resource envelope = %s", record.Manifest.ResourceEnvelopeID)
	}
	if len(record.Manifest.VisibleCapabilities) != 2 ||
		record.Manifest.VisibleCapabilities[0].SemanticName != "cap-a" ||
		record.Manifest.VisibleCapabilities[1].SemanticName != "cap-b" {
		t.Fatalf("capability snapshot = %#v", record.Manifest.VisibleCapabilities)
	}
	capA := record.Manifest.VisibleCapabilities[0]
	if capA.CapabilityID != "cap_a_id" || capA.SemanticVersion != "v2" || capA.AssessmentID != "assessment-a" {
		t.Fatalf("cap-a snapshot = %#v", capA)
	}
	if record.Manifest.VisibleCapabilities[1].CapabilityID != "" {
		t.Fatalf("unregistered capability fabricated id: %#v", record.Manifest.VisibleCapabilities[1])
	}

	digest := sha256.Sum256([]byte(record.ManifestJSON))
	if record.ManifestHash != hex.EncodeToString(digest[:]) {
		t.Fatalf("manifest hash = %s, want %s", record.ManifestHash, hex.EncodeToString(digest[:]))
	}
	for _, forbidden := range []string{"authority_ceiling", "credential", "control_token", "workspace"} {
		if contains(record.ManifestJSON, forbidden) {
			t.Fatalf("manifest leaked forbidden field %q: %s", forbidden, record.ManifestJSON)
		}
	}

	if _, err := store.DB().ExecContext(ctx,
		"UPDATE attempt_run_manifests SET manifest_hash='changed' WHERE attempt_id=?", attempt.ID,
	); err == nil {
		t.Fatal("immutable manifest UPDATE succeeded")
	}
	if _, err := store.DB().ExecContext(ctx,
		"DELETE FROM attempt_run_manifests WHERE attempt_id=?", attempt.ID,
	); err == nil {
		t.Fatal("immutable manifest DELETE succeeded")
	}
}

func TestReplacementAttemptGetsDistinctManifestAndFence(t *testing.T) {
	ctx, _, manifests, execSvc, missionID := newManifestHarness(t)
	task := createManifestTask(t, ctx, execSvc, missionID)
	first, err := execSvc.StartAttempt(ctx, task.ID, "codex", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := execSvc.RevokeLease(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := execSvc.StartAttempt(ctx, task.ID, "claude", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	firstRecord, err := manifests.Manifest(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondRecord, err := manifests.Manifest(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstRecord.ManifestHash == secondRecord.ManifestHash || firstRecord.Manifest.AttemptID == secondRecord.Manifest.AttemptID {
		t.Fatalf("replacement reused manifest identity: first=%#v second=%#v", firstRecord, secondRecord)
	}
	if secondRecord.Manifest.FenceGeneration <= firstRecord.Manifest.FenceGeneration {
		t.Fatalf("replacement fence %d <= first %d", secondRecord.Manifest.FenceGeneration, firstRecord.Manifest.FenceGeneration)
	}
}

func TestManifestRecordsTaskIntentWithoutRawPayload(t *testing.T) {
	ctx, _, manifests, execSvc, missionID := newManifestHarness(t)
	createTask := func(payload json.RawMessage) domain.Task {
		t.Helper()
		task, err := execSvc.CreateTask(ctx, execution.TaskRequest{
			Purpose:             domain.PurposeRef{Kind: domain.PurposeMission, ID: missionID},
			Objective:           "publish a summary",
			PayloadJSON:         payload,
			AcceptanceCriteria:  []string{"summary exists"},
			RequiredEnforcement: domain.EnforcementPartial,
			ResourceEnvelopeID:  "env_manifest",
		})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}
	firstPayload := json.RawMessage(`{"private_marker":"sensitive-alpha"}`)
	firstTask := createTask(firstPayload)
	firstAttempt, err := execSvc.StartAttempt(ctx, firstTask.ID, "codex", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	first, err := manifests.Manifest(ctx, firstAttempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	firstDigest := sha256.Sum256(firstTask.PayloadJSON)
	if first.Manifest.TaskObjective != firstTask.Objective {
		t.Fatalf("task objective = %q, want %q", first.Manifest.TaskObjective, firstTask.Objective)
	}
	if first.Manifest.TaskPayloadHash != hex.EncodeToString(firstDigest[:]) {
		t.Fatalf("task payload hash = %q, want SHA-256 of persisted payload", first.Manifest.TaskPayloadHash)
	}
	if strings.Contains(first.ManifestJSON, "sensitive-alpha") || strings.Contains(first.ManifestJSON, "private_marker") {
		t.Fatalf("manifest contains raw payload: %s", first.ManifestJSON)
	}
	if err := execSvc.RevokeLease(ctx, firstAttempt.ID); err != nil {
		t.Fatal(err)
	}
	retryAttempt, err := execSvc.StartAttempt(ctx, firstTask.ID, "codex", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := manifests.Manifest(ctx, retryAttempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Manifest.TaskPayloadHash != first.Manifest.TaskPayloadHash {
		t.Fatal("same task payload produced a different payload hash")
	}

	secondTask := createTask(json.RawMessage(`{"private_marker":"sensitive-beta"}`))
	secondAttempt, err := execSvc.StartAttempt(ctx, secondTask.ID, "codex", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manifests.Manifest(ctx, secondAttempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.Manifest.TaskPayloadHash == first.Manifest.TaskPayloadHash {
		t.Fatal("changing task payload did not change payload hash")
	}
	if second.ManifestHash == first.ManifestHash {
		t.Fatal("changing task payload did not change manifest hash")
	}
	changedPayloadOnly := first.Manifest
	changedPayloadOnly.TaskPayloadHash = second.Manifest.TaskPayloadHash
	changedJSON, err := json.Marshal(changedPayloadOnly)
	if err != nil {
		t.Fatal(err)
	}
	changedDigest := sha256.Sum256(changedJSON)
	if hex.EncodeToString(changedDigest[:]) == first.ManifestHash {
		t.Fatal("payload hash does not affect manifest hash")
	}
}

func TestProvenanceJoinsLaterEvidenceAndSettledExternalCostWithoutMutatingManifest(t *testing.T) {
	ctx, store, manifests, execSvc, missionID := newManifestHarness(t)
	task := createManifestTask(t, ctx, execSvc, missionID)
	attempt, err := execSvc.StartAttempt(ctx, task.ID, "codex", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	before, err := manifests.Manifest(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 21, 9, 1, 0, 0, time.UTC).Format(time.RFC3339Nano)
	db := store.DB()
	if _, err := db.ExecContext(ctx,
		"INSERT INTO evidence_objects(evidence_id,content_hash,media_type,kind,size_bytes,created_at) VALUES('e1','hash-e1','text/plain','TEST',1,?)", now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO attempt_completion_records(completion_id,attempt_id,task_id,manifest_hash,manifest_json,completed_at) VALUES('completion-1',?,?, 'completion-hash', ?, ?)",
		attempt.ID, task.ID, `{"version":1,"evidence_ids":["e1"]}`, now,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(ctx,
		"INSERT INTO policy_sets(policy_set_id,version,module_name,module,policy_hash,capabilities_hash,active,created_at) VALUES('policy_test',1,'test.rego','package test','policy-hash','caps-hash',1,?)", now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO resource_envelopes(envelope_id,hard_limit,created_at) VALUES('env_manifest',100,?)", now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO resource_reservations(reservation_id,envelope_id,state,reserved_amount,settled_amount,cost_control,cost_source,require_hard_cap,created_at,updated_at) VALUES('res-1','env_manifest','SETTLED',10,7,'TECHNICALLY_CAPPED','test',0,?,?)", now, now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO effect_slots(effect_slot_id,collective_id,task_id,trusted_slot_key,provider,descriptor_type,intent_fingerprint,intent_revision,canonical_intent_json,adapter_version,adapter_version_semantic,created_at,updated_at) VALUES('slot-1','collective',?,'key','provider','test','fp',1,'{}','v1',0,?,?)",
		task.ID, now, now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO external_operations(operation_id,task_id,attempt_id,effect_slot_id,operation_sequence,state,intent_fingerprint,intent_revision,provider,adapter_version,reservation_id,risk,prepare_policy_decision_id,prepare_policy_set_id,prepare_policy_set_hash,prepare_policy_capabilities_hash,provider_reference,actual_cost,created_at,settled_at) VALUES('op-1',?,?,'slot-1',1,'CONFIRMED_EFFECT','fp',1,'provider','v1','res-1','LOW','decision-1','policy_test','policy-hash','caps-hash','ref',7,?,?)",
		task.ID, attempt.ID, now, now,
	); err != nil {
		t.Fatal(err)
	}

	provenance, err := manifests.Provenance(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(provenance.OutputEvidence) != 1 || provenance.OutputEvidence[0].ID != "e1" || provenance.OutputEvidence[0].ContentHash != "hash-e1" {
		t.Fatalf("output evidence = %#v", provenance.OutputEvidence)
	}
	if provenance.SettledExternalCost == nil || *provenance.SettledExternalCost != 7 {
		t.Fatalf("settled external cost = %#v", provenance.SettledExternalCost)
	}
	after, err := manifests.Manifest(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ManifestHash != before.ManifestHash || after.ManifestJSON != before.ManifestJSON {
		t.Fatal("derived provenance mutated immutable run manifest")
	}
}

func seedCapability(t *testing.T, store *state.Store, id, name, version, provider, assessmentID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 9, 21, 8, 59, 0, 0, time.UTC).Format(time.RFC3339Nano)
	db := store.DB()
	if _, err := db.ExecContext(ctx,
		"INSERT INTO capability_definitions(capability_id,semantic_name,semantic_version,provider,access_context,authority_requirements_json,minimum_enforcement,active,created_at,updated_at) VALUES(?,?,?,?,?,'[\""+name+"\"]','PARTIAL',1,?,?)",
		id, name, version, provider, "local", now, now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO capability_assessments(assessment_id,capability_id,provider,enforcement_level,access_context,assessed_at,evidence_json,cost_metadata_json,health,available) VALUES(?,?,?,'ENFORCED','local',?,'[\"ok\"]','{}','HEALTHY',1)",
		assessmentID, id, provider, now,
	); err != nil {
		t.Fatal(err)
	}
}

func contains(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
