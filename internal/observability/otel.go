package observability

import (
	"context"
	"strings"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const instrumentationName = "github.com/SofiaFlux/meeseek-collective/internal/observability"

type Bridge struct {
	tracer trace.Tracer
}

type spanSpec struct {
	name       string
	attributes []attribute.KeyValue
}

func New(provider trace.TracerProvider) *Bridge {
	if provider == nil {
		provider = noop.NewTracerProvider()
	}
	return &Bridge{tracer: provider.Tracer(instrumentationName)}
}

func (b *Bridge) StartTask(ctx context.Context, taskID domain.ID) (context.Context, trace.Span) {
	return b.start(ctx, taskSpanSpec(taskID))
}

func (b *Bridge) StartAttempt(ctx context.Context, taskID, attemptID domain.ID) (context.Context, trace.Span) {
	return b.start(ctx, attemptSpanSpec(taskID, attemptID))
}

func (b *Bridge) StartCapability(ctx context.Context, attemptID domain.ID, capabilityName string) (context.Context, trace.Span) {
	return b.start(ctx, capabilitySpanSpec(attemptID, capabilityName))
}

func (b *Bridge) StartOperation(ctx context.Context, taskID, operationID domain.ID) (context.Context, trace.Span) {
	return b.start(ctx, operationSpanSpec(taskID, operationID))
}

func (b *Bridge) start(ctx context.Context, spec spanSpec) (context.Context, trace.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	if b == nil || b.tracer == nil {
		b = New(nil)
	}
	return b.tracer.Start(ctx, spec.name, trace.WithAttributes(spec.attributes...))
}

func taskSpanSpec(taskID domain.ID) spanSpec {
	return spanSpec{
		name: "meeseek.task",
		attributes: []attribute.KeyValue{
			attribute.String("meeseek.task.id", strings.TrimSpace(string(taskID))),
		},
	}
}

func attemptSpanSpec(taskID, attemptID domain.ID) spanSpec {
	return spanSpec{
		name: "meeseek.attempt",
		attributes: []attribute.KeyValue{
			attribute.String("meeseek.task.id", strings.TrimSpace(string(taskID))),
			attribute.String("meeseek.attempt.id", strings.TrimSpace(string(attemptID))),
		},
	}
}

func capabilitySpanSpec(attemptID domain.ID, capabilityName string) spanSpec {
	return spanSpec{
		name: "meeseek.capability",
		attributes: []attribute.KeyValue{
			attribute.String("meeseek.attempt.id", strings.TrimSpace(string(attemptID))),
			attribute.String("meeseek.capability.name", strings.TrimSpace(capabilityName)),
		},
	}
}

func operationSpanSpec(taskID, operationID domain.ID) spanSpec {
	return spanSpec{
		name: "meeseek.operation",
		attributes: []attribute.KeyValue{
			attribute.String("meeseek.task.id", strings.TrimSpace(string(taskID))),
			attribute.String("meeseek.operation.id", strings.TrimSpace(string(operationID))),
		},
	}
}
