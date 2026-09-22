package observability

import (
	"context"
	"testing"

	"github.com/SofiaFlux/summa42/internal/domain"
)

func TestSpanSpecsReferenceCanonicalIDs(t *testing.T) {
	tests := []struct {
		name      string
		spec      spanSpec
		wantName  string
		wantAttrs map[string]string
	}{
		{
			name:      "task",
			spec:      taskSpanSpec(domain.ID("task-1")),
			wantName:  "summa42.task",
			wantAttrs: map[string]string{"summa42.task.id": "task-1"},
		},
		{
			name:      "attempt",
			spec:      attemptSpanSpec(domain.ID("task-1"), domain.ID("attempt-1")),
			wantName:  "summa42.attempt",
			wantAttrs: map[string]string{"summa42.task.id": "task-1", "summa42.attempt.id": "attempt-1"},
		},
		{
			name:      "capability",
			spec:      capabilitySpanSpec(domain.ID("attempt-1"), "github.write"),
			wantName:  "summa42.capability",
			wantAttrs: map[string]string{"summa42.attempt.id": "attempt-1", "summa42.capability.name": "github.write"},
		},
		{
			name:      "operation",
			spec:      operationSpanSpec(domain.ID("task-1"), domain.ID("operation-1")),
			wantName:  "summa42.operation",
			wantAttrs: map[string]string{"summa42.task.id": "task-1", "summa42.operation.id": "operation-1"},
		},
		{
			name: "feedback", spec: feedbackSpanSpec("feedback-1", "emit"), wantName: "summa42.feedback",
			wantAttrs: map[string]string{"summa42.feedback.id":"feedback-1","summa42.feedback.phase":"emit"},
		},
		{
			name: "sanitization", spec: sanitizationSpanSpec("candidate-1"), wantName: "summa42.sanitization",
			wantAttrs: map[string]string{"summa42.feedback.candidate_id":"candidate-1"},
		},
		{
			name: "experience", spec: experienceRuleSpanSpec("rule-1", "promote"), wantName: "summa42.experience_rule",
			wantAttrs: map[string]string{"summa42.experience.rule_id":"rule-1","summa42.experience.phase":"promote"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.spec.name != tt.wantName {
				t.Fatalf("span name = %q, want %q", tt.spec.name, tt.wantName)
			}
			got := map[string]string{}
			for _, attr := range tt.spec.attributes {
				got[string(attr.Key)] = attr.Value.AsString()
			}
			if len(got) != len(tt.wantAttrs) {
				t.Fatalf("attributes = %v, want %v", got, tt.wantAttrs)
			}
			for key, value := range tt.wantAttrs {
				if got[key] != value {
					t.Fatalf("attribute %s = %q, want %q", key, got[key], value)
				}
			}
		})
	}
}

func TestDefaultBridgeIsNoopButUsable(t *testing.T) {
	bridge := New(nil)
	ctx := context.Background()

	starts := []func(context.Context) context.Context{
		func(ctx context.Context) context.Context {
			ctx, span := bridge.StartTask(ctx, "task-1")
			defer span.End()
			if span.IsRecording() {
				t.Fatal("default task span unexpectedly records without configured exporter/provider")
			}
			return ctx
		},
		func(ctx context.Context) context.Context {
			ctx, span := bridge.StartAttempt(ctx, "task-1", "attempt-1")
			defer span.End()
			return ctx
		},
		func(ctx context.Context) context.Context {
			ctx, span := bridge.StartCapability(ctx, "attempt-1", "github.write")
			defer span.End()
			return ctx
		},
		func(ctx context.Context) context.Context {
			ctx, span := bridge.StartOperation(ctx, "task-1", "operation-1")
			defer span.End()
			return ctx
		},
		func(ctx context.Context) context.Context {
			ctx, span := bridge.StartFeedback(ctx, "feedback-1", "emit")
			defer span.End()
			return ctx
		},
		func(ctx context.Context) context.Context {
			ctx, span := bridge.StartSanitization(ctx, "candidate-1")
			defer span.End()
			return ctx
		},
		func(ctx context.Context) context.Context {
			ctx, span := bridge.StartExperienceRule(ctx, "rule-1", "promote")
			defer span.End()
			return ctx
		},
	}
	for _, start := range starts {
		ctx = start(ctx)
	}
}
