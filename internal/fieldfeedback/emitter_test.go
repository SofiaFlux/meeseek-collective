package fieldfeedback

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/execution"
	"github.com/SofiaFlux/meeseek-collective/internal/localconfig"
	"github.com/SofiaFlux/meeseek-collective/internal/purpose"
)

func TestEmitLocalOnlyNeverCreatesTask(t *testing.T) {
	feedback, artifact := emissionArtifact(t)
	exec := execution.New(feedback.store, feedback.clock, purpose.New(feedback.store, feedback.clock))
	if err := feedback.ConfigureEmission(exec, localconfig.FieldFeedbackConfig{
		Enabled: true, Mode: localconfig.FeedbackModeLocalOnly,
	}, "owner_1"); err != nil {
		t.Fatal(err)
	}
	if err := feedback.OnSanitized(context.Background(), artifact.ID); err != nil {
		t.Fatal(err)
	}
	assertEmissionTaskCount(t, feedback, 0)
	candidate, err := feedback.Candidate(context.Background(), artifact.CandidateID)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.State != domain.FeedbackStateLocalOnly {
		t.Fatalf("candidate state = %s, want LOCAL_ONLY", candidate.State)
	}
}

func TestEmitCreatesOneGovernedMaintenanceTask(t *testing.T) {
	feedback, artifact := emissionArtifact(t)
	exec := execution.New(feedback.store, feedback.clock, purpose.New(feedback.store, feedback.clock))
	cfg := localconfig.FieldFeedbackConfig{
		Enabled: true, Mode: localconfig.FeedbackModeAutoIfAllowed,
		Provider: "github", Destination: "SofiaFlux/meeseek-collective",
		MaintenanceEnvelopeID: "env_feedback", RequiredEnforcement: domain.EnforcementEnforced,
	}
	if err := feedback.ConfigureEmission(exec, cfg, "owner_1"); err != nil {
		t.Fatal(err)
	}
	if err := feedback.OnSanitized(context.Background(), artifact.ID); err != nil {
		t.Fatal(err)
	}
	task, err := feedback.RequestEmit(context.Background(), artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := feedback.RequestEmit(context.Background(), artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != again.ID {
		t.Fatalf("repeat emit minted tasks %s and %s", task.ID, again.ID)
	}
	if task.Purpose.Kind != domain.PurposeCollectiveMaintenance || task.Purpose.ID != artifact.ID {
		t.Fatalf("task purpose = %+v", task.Purpose)
	}
	if task.TaskClass != "collective.feedback.emit" {
		t.Fatalf("task class = %q", task.TaskClass)
	}
	if len(task.RequiredCapabilities) != 1 || task.RequiredCapabilities[0] != "feedback.github.issue.create" {
		t.Fatalf("required capabilities = %v", task.RequiredCapabilities)
	}
	if len(task.AuthorityCeiling) != 1 || task.AuthorityCeiling[0] != "feedback.github.issue.create" {
		t.Fatalf("authority ceiling = %v", task.AuthorityCeiling)
	}
	if task.RequiredEnforcement != domain.EnforcementEnforced || task.ResourceEnvelopeID != "env_feedback" {
		t.Fatalf("task enforcement/envelope = %s/%s", task.RequiredEnforcement, task.ResourceEnvelopeID)
	}
	assertEmissionTaskCount(t, feedback, 1)
}

func TestRequireApprovalEmissionPersistsOwnerTightening(t *testing.T) {
	feedback, artifact := emissionArtifact(t)
	exec := execution.New(feedback.store, feedback.clock, purpose.New(feedback.store, feedback.clock))
	cfg := localconfig.FieldFeedbackConfig{
		Enabled: true, Mode: localconfig.FeedbackModeRequireApproval,
		Provider: "github", Destination: "SofiaFlux/meeseek-collective",
		MaintenanceEnvelopeID: "env_feedback", RequiredEnforcement: domain.EnforcementEnforced,
	}
	if err := feedback.ConfigureEmission(exec, cfg, "owner_1"); err != nil {
		t.Fatal(err)
	}
	if err := feedback.OnSanitized(context.Background(), artifact.ID); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := feedback.store.DB().QueryRowContext(context.Background(),
		"SELECT required_approvers_json FROM feedback_emissions WHERE feedback_id = ?", artifact.ID,
	).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var ids []domain.ID
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "owner_1" {
		t.Fatalf("required approvers = %v", ids)
	}
}

func TestNoMaintenanceBudgetLeavesArtifactExportReadyWithoutTask(t *testing.T) {
	feedback, artifact := emissionArtifact(t)
	exec := execution.New(feedback.store, feedback.clock, purpose.New(feedback.store, feedback.clock))
	cfg := localconfig.FieldFeedbackConfig{
		Enabled: true, Mode: localconfig.FeedbackModeAutoIfAllowed,
		Provider: "github", Destination: "SofiaFlux/meeseek-collective",
		MaintenanceEnvelopeID: "missing-envelope", RequiredEnforcement: domain.EnforcementEnforced,
	}
	if err := feedback.ConfigureEmission(exec, cfg, "owner_1"); err != nil {
		t.Fatal(err)
	}
	err := feedback.OnSanitized(context.Background(), artifact.ID)
	if !errors.Is(err, ErrNoMaintenanceBudget) {
		t.Fatalf("OnSanitized error = %v, want ErrNoMaintenanceBudget", err)
	}
	assertEmissionTaskCount(t, feedback, 0)
	candidate, err := feedback.Candidate(context.Background(), artifact.CandidateID)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.State != domain.FeedbackStateExportReady {
		t.Fatalf("candidate state = %s, want EXPORT_READY", candidate.State)
	}
}

func emissionArtifact(t *testing.T) (*Feedback, domain.SanitizedFeedback) {
	t.Helper()
	feedback, candidate := sanitizerCandidate(t, "Generic safe field observation")
	sanitizer, err := NewDeterministicSanitizer(feedback.store, feedback.clock, feedback, DeterministicSanitizerConfig{
		Version: "emit-test-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact, result, err := sanitizer.Sanitize(context.Background(), candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != domain.SanitizationPass {
		t.Fatalf("sanitization outcome = %s", result.Outcome)
	}
	return feedback, artifact
}

func assertEmissionTaskCount(t *testing.T, feedback *Feedback, want int) {
	t.Helper()
	var count int
	if err := feedback.store.DB().QueryRowContext(context.Background(),
		"SELECT count(*) FROM tasks WHERE task_class = 'collective.feedback.emit'",
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("feedback emit task count = %d, want %d", count, want)
	}
}
