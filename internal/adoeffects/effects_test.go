package adoeffects

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/operations"
)

type fakeDial struct {
	calls []mcpCall
	pages []map[string]any
	err   error
}

type mcpCall struct {
	tool string
	args map[string]any
}

func (f *fakeDial) dial(context.Context) (callToolFunc, error) {
	return func(_ context.Context, tool string, args map[string]any) (map[string]any, error) {
		f.calls = append(f.calls, mcpCall{tool, args})
		if f.err != nil {
			return nil, f.err
		}
		if len(f.pages) == 0 {
			return map[string]any{"ok": true}, nil
		}
		page := f.pages[0]
		f.pages = f.pages[1:]
		return page, nil
	}, nil
}

func TestCommentIntentCanonicalStable(t *testing.T) {
	p := &CommentProvider{}
	a, err := p.CanonicalIntent(CommentIntent{Project: "proj", Repository: "shop", PR: 7, Path: "main.go", Line: 3, Body: "nit", Marker: "[summa42:c:w]"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.CanonicalIntent(CommentIntent{Project: "proj", Repository: "shop", PR: 7, Path: "main.go", Line: 3, Body: "nit", Marker: "[summa42:c:w]"})
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("canonical unstable:\n%s\n%s", a, b)
	}
	var decoded map[string]any
	if err := json.Unmarshal(a, &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestProvidersRejectForeignIntents(t *testing.T) {
	comment := &CommentProvider{}
	if _, err := comment.CanonicalIntent(VoteIntent{PR: 1, Vote: 10}); err == nil {
		t.Fatal("comment provider accepted vote intent")
	}
	vote := &VoteProvider{}
	if _, err := vote.CanonicalIntent(CommentIntent{Body: "x"}); err == nil {
		t.Fatal("vote provider accepted comment intent")
	}
	if _, err := vote.CanonicalIntent(VoteIntent{PR: 1, Vote: -10}); err == nil {
		t.Fatal("vote provider accepted rejection vote")
	}
	if vote.Capability() != "ado.pr.approve" || comment.Capability() != "ado.pr.comment" {
		t.Fatalf("capabilities = %q %q", vote.Capability(), comment.Capability())
	}
}

func stubRead(context.Context, string, any) (any, error) {
	return nil, errors.New("no reads in dispatch test")
}

func TestDispatchCallsWriteTools(t *testing.T) {
	commentDial := &fakeDial{pages: []map[string]any{{"threadId": 9}}}
	comment := &CommentProvider{
		config: Config{Command: "/bin/true", Organization: "Contoso", Timeout: time.Minute},
		dial:   commentDial.dial,
		read:   stubRead,
	}
	canonical, err := comment.CanonicalIntent(CommentIntent{Project: "proj", Repository: "shop", PR: 7, Path: "main.go", Line: 3, Body: "nit", Marker: "[summa42:c:w]"})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := comment.Dispatch(context.Background(), operations.ProviderDispatchRequest{
		OperationID: "op-1", TaskID: "task-1", EffectSlotID: "slot-1",
		IntentFingerprint: "fp", IntentRevision: 1, CanonicalIntent: canonical,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != domain.OperationConfirmedEffect && outcome.State != domain.OperationOutcomeUnknown {
		t.Fatalf("state = %q", outcome.State)
	}
	if len(commentDial.calls) != 1 {
		t.Fatalf("calls = %v, want exactly 1", commentDial.calls)
	}
	call := commentDial.calls[0]
	if call.tool != "repo_pull_request_thread_write" || call.args["action"] != "create" {
		t.Fatalf("call = %+v", call)
	}
	voteDial := &fakeDial{pages: []map[string]any{{"vote": 10}}}
	vote := &VoteProvider{
		config: Config{Command: "/bin/true", Organization: "Contoso", Timeout: time.Minute},
		dial:   voteDial.dial,
		read:   stubRead,
	}
	voteCanonical, err := vote.CanonicalIntent(VoteIntent{Project: "proj", Repository: "shop", PR: 7, Vote: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vote.Dispatch(context.Background(), operations.ProviderDispatchRequest{
		OperationID: "op-2", TaskID: "task-1", EffectSlotID: "slot-2",
		IntentFingerprint: "fp2", IntentRevision: 1, CanonicalIntent: voteCanonical,
	}); err != nil {
		t.Fatal(err)
	}
	if len(voteDial.calls) != 1 || voteDial.calls[0].tool != "repo_pull_request_write" || voteDial.calls[0].args["action"] != "vote" {
		t.Fatalf("calls = %+v", voteDial.calls)
	}
}
