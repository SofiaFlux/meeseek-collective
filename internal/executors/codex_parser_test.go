package executors

import (
	"strings"
	"testing"
)

func TestCodexParserPreservesErrorItemMessage(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread-error-test"}`,
		`{"type":"item.completed","item":{"id":"error-1","type":"error","message":"fixture exploded"}}`,
	}, "\n") + "\n"

	_, _, _, err := parseCodexJSONL([]byte(stream))
	if err == nil {
		t.Fatal("Codex error item was accepted")
	}
	if !strings.Contains(err.Error(), "fixture exploded") {
		t.Fatalf("error = %q, want original Codex error-item message", err)
	}
}
