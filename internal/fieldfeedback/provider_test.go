package fieldfeedback

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/operations"
)

type fakeFeedbackReader struct {
	artifact domain.SanitizedFeedback
}

func (r fakeFeedbackReader) SanitizedFeedback(context.Context, domain.ID) (domain.SanitizedFeedback, error) {
	return r.artifact, nil
}

type wrongEmitDescriptor struct{}
func (wrongEmitDescriptor) DescriptorType() string { return "wrong" }

func TestProviderCanonicalIntentAcceptsOnlyEmitIntentAndCarriesNoContent(t *testing.T) {
	artifact := domain.SanitizedFeedback{
		ID: "feedback-1", ContentJSON: `{"category":"TEST","observed_behavior":"safe"}`,
		Fingerprint: "abc123",
	}
	sink := newFakeSink()
	provider, err := NewProvider("github", "owner/repo", fakeFeedbackReader{artifact: artifact}, sink)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.CanonicalIntent(wrongEmitDescriptor{}); err == nil {
		t.Fatal("provider accepted wrong descriptor type")
	}
	canonical, err := provider.CanonicalIntent(EmitIntent{
		SanitizedFeedbackID: artifact.ID, Destination: "owner/repo",
	})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(canonical, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields["sanitized_feedback_id"] != string(artifact.ID) || fields["destination"] != "owner/repo" {
		t.Fatalf("canonical intent = %s", canonical)
	}
	if string(canonical) == artifact.ContentJSON {
		t.Fatal("canonical intent contains artifact content instead of identity")
	}
}

func TestProviderRequiresReaderAndRendersOnlyImmutableArtifact(t *testing.T) {
	if _, err := NewProvider("github", "owner/repo", nil, newFakeSink()); err == nil {
		t.Fatal("provider constructed without feedback reader")
	}
	artifact := domain.SanitizedFeedback{
		ID: "feedback-1",
		ContentJSON: `{"category":"RECOVERY_FRICTION","observed_behavior":"generic safe behavior"}`,
		Fingerprint: "f00baa",
	}
	sink := newFakeSink()
	provider, err := NewProvider("github", "owner/repo", fakeFeedbackReader{artifact: artifact}, sink)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := provider.CanonicalIntent(EmitIntent{SanitizedFeedbackID: artifact.ID, Destination: "owner/repo"})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := provider.Dispatch(context.Background(), operations.ProviderDispatchRequest{CanonicalIntent: canonical})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != domain.OperationConfirmedEffect {
		t.Fatalf("dispatch outcome = %s", outcome.State)
	}
	payload := sink.lastPayload()
	if payload.Body != artifact.ContentJSON {
		t.Fatalf("payload body = %q, want exact sanitized content", payload.Body)
	}
	if payload.Marker != "meeseek-feedback:"+artifact.Fingerprint {
		t.Fatalf("marker = %q", payload.Marker)
	}
}

func TestProviderLookupUsesMarkerWithoutCreate(t *testing.T) {
	artifact := domain.SanitizedFeedback{
		ID: "feedback-1", ContentJSON: `{"category":"TEST"}`, Fingerprint: "fingerprint-1",
	}
	sink := newFakeSink()
	sink.created["meeseek-feedback:"+artifact.Fingerprint] = "issue-existing"
	provider, err := NewProvider("github", "owner/repo", fakeFeedbackReader{artifact: artifact}, sink)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := provider.CanonicalIntent(EmitIntent{SanitizedFeedbackID: artifact.ID, Destination: "owner/repo"})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := provider.LookupOutcome(context.Background(), operations.ProviderDispatchRequest{CanonicalIntent: canonical})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != domain.OperationConfirmedEffect || outcome.ProviderReference != "issue-existing" {
		t.Fatalf("lookup outcome = %+v", outcome)
	}
	createCount, lookupCount := sink.counts()
	if createCount != 0 || lookupCount != 1 {
		t.Fatalf("sink counts create=%d lookup=%d", createCount, lookupCount)
	}
}
