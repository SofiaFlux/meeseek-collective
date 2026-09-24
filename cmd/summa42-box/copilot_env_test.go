package main

import (
	"testing"
)

func TestBuildCopilotExecutorAbsentWithoutPath(t *testing.T) {
	t.Setenv("SUMMA42_COPILOT_PATH", "")
	executors, err := buildCopilotExecutorFromEnv()
	if err != nil || executors != nil {
		t.Fatalf("result = %v, %v; want nil, nil", executors, err)
	}
}

func TestBuildCopilotExecutorRejectsBadConfig(t *testing.T) {
	t.Setenv("SUMMA42_COPILOT_PATH", "/bin/true")
	t.Setenv("SUMMA42_COPILOT_TOOLS", "shell")
	if _, err := buildCopilotExecutorFromEnv(); err == nil {
		t.Fatal("accepted shell tool")
	}
}

func TestBuildCopilotExecutorDefaultsReadTools(t *testing.T) {
	t.Setenv("SUMMA42_COPILOT_PATH", "/bin/true")
	t.Setenv("SUMMA42_COPILOT_TOOLS", "")
	t.Setenv("SUMMA42_COPILOT_MCP_SERVER", "ado")
	t.Setenv("SUMMA42_COPILOT_TIMEOUT", "")
	built, err := buildCopilotExecutorFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := built["copilot"]; !ok {
		t.Fatalf("kinds = %v, want copilot", built)
	}
}
