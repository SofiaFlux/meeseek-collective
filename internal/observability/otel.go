package observability

import (
	"context"
	"strings"

	"github.com/SofiaFlux/summa42/internal/domain"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const instrumentationName = "github.com/SofiaFlux/summa42/internal/observability"

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

func (b *Bridge) StartFeedback(ctx context.Context, feedbackID domain.ID, phase string) (context.Context, trace.Span) {
	return b.start(ctx, feedbackSpanSpec(feedbackID, phase))
}

func (b *Bridge) StartSanitization(ctx context.Context, candidateID domain.ID) (context.Context, trace.Span) {
	return b.start(ctx, sanitizationSpanSpec(candidateID))
}

func (b *Bridge) StartExperienceRule(ctx context.Context, ruleID domain.ID, phase string) (context.Context, trace.Span) {
	return b.start(ctx, experienceRuleSpanSpec(ruleID, phase))
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
		name: "summa42.task",
		attributes: []attribute.KeyValue{
			attribute.String("summa42.task.id", strings.TrimSpace(string(taskID))),
		},
	}
}

func attemptSpanSpec(taskID, attemptID domain.ID) spanSpec {
	return spanSpec{
		name: "summa42.attempt",
		attributes: []attribute.KeyValue{
			attribute.String("summa42.task.id", strings.TrimSpace(string(taskID))),
			attribute.String("summa42.attempt.id", strings.TrimSpace(string(attemptID))),
		},
	}
}

func capabilitySpanSpec(attemptID domain.ID, capabilityName string) spanSpec {
	return spanSpec{
		name: "summa42.capability",
		attributes: []attribute.KeyValue{
			attribute.String("summa42.attempt.id", strings.TrimSpace(string(attemptID))),
			attribute.String("summa42.capability.name", strings.TrimSpace(capabilityName)),
		},
	}
}

func operationSpanSpec(taskID, operationID domain.ID) spanSpec {
	return spanSpec{
		name: "summa42.operation",
		attributes: []attribute.KeyValue{
			attribute.String("summa42.task.id", strings.TrimSpace(string(taskID))),
			attribute.String("summa42.operation.id", strings.TrimSpace(string(operationID))),
		},
	}
}


func feedbackSpanSpec(feedbackID domain.ID, phase string) spanSpec {
	return spanSpec{
		name: "summa42.feedback",
		attributes: []attribute.KeyValue{
			attribute.String("summa42.feedback.id", strings.TrimSpace(string(feedbackID))),
			attribute.String("summa42.feedback.phase", strings.TrimSpace(phase)),
		},
	}
}

func sanitizationSpanSpec(candidateID domain.ID) spanSpec {
	return spanSpec{
		name: "summa42.sanitization",
		attributes: []attribute.KeyValue{
			attribute.String("summa42.feedback.candidate_id", strings.TrimSpace(string(candidateID))),
		},
	}
}

func experienceRuleSpanSpec(ruleID domain.ID, phase string) spanSpec {
	return spanSpec{
		name: "summa42.experience_rule",
		attributes: []attribute.KeyValue{
			attribute.String("summa42.experience.rule_id", strings.TrimSpace(string(ruleID))),
			attribute.String("summa42.experience.phase", strings.TrimSpace(phase)),
		},
	}
}
