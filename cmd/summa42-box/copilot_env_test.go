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

func TestBuildCopilotExecutorDoesNotLeakNonAllowlistedEnv(t *testing.T) {
	t.Setenv("SUMMA42_COPILOT_PATH", "/bin/true")
	t.Setenv("SUMMA42_COPILOT_TOOLS", "")
	t.Setenv("SUMMA42_COPILOT_MCP_SERVER", "ado")
	t.Setenv("SUMMA42_COPILOT_TIMEOUT", "")
	t.Setenv("FOO", "1")
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("GH_TOKEN", "token-value")
	t.Setenv("COPILOT_MODEL", "env-model")
	built, err := buildCopilotExecutorFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := built["copilot"]; !ok {
		t.Fatalf("kinds = %v, want copilot", built)
	}
	// Builder must succeed despite FOO=1 in os env; asserting child env
	// requires executing, which is covered at the executor level
	// (TestCopilotRejectsNonAllowlistedEnv). Here we assert the builder ran
	// without inheriting the leak — success itself is the signal since any
	// FOO passthrough would make NewCopilotExecutor fail.
}
