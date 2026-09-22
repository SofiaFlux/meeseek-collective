package control

import "testing"

func TestApprovalSigningMessageBindsExactDigestAndAction(t *testing.T) {
	approve := string(ApprovalSigningMessage("challenge-1", "digest-1", "APPROVE"))
	reject := string(ApprovalSigningMessage("challenge-1", "digest-1", "REJECT"))
	if approve == reject {
		t.Fatal("approval and rejection signing messages are identical")
	}
	wantApprove := "meeseek-owner-approval-v2\nchallenge:challenge-1\nrequest-digest:digest-1\naction:APPROVE\n"
	if approve != wantApprove {
		t.Fatalf("approve signing message = %q, want %q", approve, wantApprove)
	}
}
