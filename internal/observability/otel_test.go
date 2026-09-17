package observability

import (
	"context"
	"testing"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
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
			wantName:  "meeseek.task",
			wantAttrs: map[string]string{"meeseek.task.id": "task-1"},
		},
		{
			name:      "attempt",
			spec:      attemptSpanSpec(domain.ID("task-1"), domain.ID("attempt-1")),
			wantName:  "meeseek.attempt",
			wantAttrs: map[string]string{"meeseek.task.id": "task-1", "meeseek.attempt.id": "attempt-1"},
		},
		{
			name:      "capability",
			spec:      capabilitySpanSpec(domain.ID("attempt-1"), "github.write"),
			wantName:  "meeseek.capability",
			wantAttrs: map[string]string{"meeseek.attempt.id": "attempt-1", "meeseek.capability.name": "github.write"},
		},
		{
			name:      "operation",
			spec:      operationSpanSpec(domain.ID("task-1"), domain.ID("operation-1")),
			wantName:  "meeseek.operation",
			wantAttrs: map[string]string{"meeseek.task.id": "task-1", "meeseek.operation.id": "operation-1"},
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
	}
	for _, start := range starts {
		ctx = start(ctx)
	}
}
