package domain

import (
	"strings"
	"testing"
)

func TestNewIDHasPrefixAndEntropy(t *testing.T) {
	a := NewID("task")
	b := NewID("task")
	if a == b || !strings.HasPrefix(string(a), "task_") {
		t.Fatalf("unexpected ids: %q %q", a, b)
	}
}
