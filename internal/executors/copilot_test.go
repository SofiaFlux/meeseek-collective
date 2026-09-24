package executors_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/executors"
)

func writeFakeCopilot(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "copilot")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func copilotConfig(path string) executors.CopilotConfig {
	return executors.CopilotConfig{
		Path: path, MCPServer: "ado",
		AllowedTools: []string{"repo_pull_request"},
		Timeout:      time.Minute,
	}
}

func TestCopilotParsesCleanResult(t *testing.T) {
	path := writeFakeCopilot(t, "#!/bin/sh\necho '{\"verdict\":\"CLEAN\",\"reviewedCommits\":[\"abc\"],\"reviewedFiles\":[\"main.go\"],\"findings\":[]}'\n")
	executor, err := executors.NewCopilotExecutor(copilotConfig(path))
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Start(context.Background(), executors.AttemptEnvelope{
		TaskID: "task-1", AttemptID: "attempt-1", Workspace: t.TempDir(),
		Objective: "review", PayloadJSON: json.RawMessage(`{"repo":"shop","pr":1,"sourceCommit":"a","targetCommit":"b"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["verdict"] != "CLEAN" {
		t.Fatalf("verdict = %v", decoded["verdict"])
	}
}

func TestCopilotRejectsHostileToolConfigs(t *testing.T) {
	base := copilotConfig(writeFakeCopilot(t, "#!/bin/sh\nexit 0\n"))
	cases := map[string]func(executors.CopilotConfig) executors.CopilotConfig{
		"shell fragment": func(c executors.CopilotConfig) executors.CopilotConfig {
			c.AllowedTools = []string{"shell"}
			return c
		},
		"write with parens": func(c executors.CopilotConfig) executors.CopilotConfig {
			c.AllowedTools = []string{"write(README.md)"}
			return c
		},
		"allow-all flag": func(c executors.CopilotConfig) executors.CopilotConfig {
			c.AllowedTools = []string{"--allow-all-tools"}
			return c
		},
		"qualified tool config": func(c executors.CopilotConfig) executors.CopilotConfig {
			c.AllowedTools = []string{"ado(pr_write)"}
			return c
		},
		"blank tool": func(c executors.CopilotConfig) executors.CopilotConfig {
			c.AllowedTools = []string{"   "}
			return c
		},
		"empty list": func(c executors.CopilotConfig) executors.CopilotConfig {
			c.AllowedTools = nil
			return c
		},
		"blank server": func(c executors.CopilotConfig) executors.CopilotConfig {
			c.MCPServer = "   "
			return c
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := executors.NewCopilotExecutor(mutate(base)); err == nil {
				t.Fatalf("expected NewCopilotExecutor error for %s", name)
			}
		})
	}
}

func TestCopilotRejectsInvalidResult(t *testing.T) {
	cases := map[string]string{
		"non-JSON stdout":      "#!/bin/sh\necho 'not json'\n",
		"bad verdict":          "#!/bin/sh\necho '{\"verdict\":\"BOGUS\",\"reviewedCommits\":[\"abc\"],\"reviewedFiles\":[\"main.go\"],\"findings\":[]}'\n",
		"empty commits":        "#!/bin/sh\necho '{\"verdict\":\"CLEAN\",\"reviewedCommits\":[],\"reviewedFiles\":[\"main.go\"],\"findings\":[]}'\n",
		"finding without path": "#!/bin/sh\necho '{\"verdict\":\"FINDINGS\",\"reviewedCommits\":[\"abc\"],\"reviewedFiles\":[\"main.go\"],\"findings\":[{\"line\":1,\"explanation\":\"x\",\"evidence\":\"y\"}]}'\n",
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			path := writeFakeCopilot(t, script)
			executor, err := executors.NewCopilotExecutor(copilotConfig(path))
			if err != nil {
				t.Fatal(err)
			}
			_, err = executor.Start(context.Background(), executors.AttemptEnvelope{
				TaskID: "task-1", AttemptID: "attempt-1", Workspace: t.TempDir(),
				Objective: "review", PayloadJSON: json.RawMessage(`{"repo":"shop","pr":1,"sourceCommit":"a","targetCommit":"b"}`),
			})
			if err == nil {
				t.Fatalf("expected Start error for %s", name)
			}
		})
	}
}

func TestCopilotRejectsBadPayload(t *testing.T) {
	path := writeFakeCopilot(t, "#!/bin/sh\necho '{\"verdict\":\"CLEAN\",\"reviewedCommits\":[\"abc\"],\"reviewedFiles\":[\"main.go\"],\"findings\":[]}'\n")
	executor, err := executors.NewCopilotExecutor(copilotConfig(path))
	if err != nil {
		t.Fatal(err)
	}
	_, err = executor.Start(context.Background(), executors.AttemptEnvelope{
		TaskID: "task-1", AttemptID: "attempt-1", Workspace: t.TempDir(),
		Objective: "review", PayloadJSON: json.RawMessage(`{"repo":"shop","pr":1,"sourceCommit":"a"}`),
	})
	if err == nil {
		t.Fatal("expected Start error for payload missing targetCommit")
	}
}

func TestCopilotArgvIsClosed(t *testing.T) {
	path := writeFakeCopilot(t, "#!/bin/sh\necho \"$@\" > \"$(pwd)/argv.txt\"\necho '{\"verdict\":\"CLEAN\",\"reviewedCommits\":[\"abc\"],\"reviewedFiles\":[\"main.go\"],\"findings\":[]}'\n")
	executor, err := executors.NewCopilotExecutor(copilotConfig(path))
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	_, err = executor.Start(context.Background(), executors.AttemptEnvelope{
		TaskID: "task-1", AttemptID: "attempt-1", Workspace: workspace,
		Objective: "review", PayloadJSON: json.RawMessage(`{"repo":"shop","pr":1,"sourceCommit":"a","targetCommit":"b"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(workspace, "argv.txt"))
	if err != nil {
		t.Fatal(err)
	}
	dump := string(raw)
	if !strings.Contains(dump, "--deny-tool=shell,write,url,memory") {
		t.Fatalf("argv dump missing deny-tool list: %q", dump)
	}
	for _, forbidden := range []string{"allow-all", "add-dir", "--agent"} {
		if strings.Contains(dump, forbidden) {
			t.Fatalf("argv dump contains forbidden %q: %q", forbidden, dump)
		}
	}
}

func TestCopilotTimeoutAndExitCode(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		path := writeFakeCopilot(t, "#!/bin/sh\nsleep 2\necho '{\"verdict\":\"CLEAN\",\"reviewedCommits\":[\"abc\"],\"reviewedFiles\":[\"main.go\"],\"findings\":[]}'\n")
		cfg := copilotConfig(path)
		cfg.Timeout = 50 * time.Millisecond
		executor, err := executors.NewCopilotExecutor(cfg)
		if err != nil {
			t.Fatal(err)
		}
		_, err = executor.Start(context.Background(), executors.AttemptEnvelope{
			TaskID: "task-1", AttemptID: "attempt-1", Workspace: t.TempDir(),
			Objective: "review", PayloadJSON: json.RawMessage(`{"repo":"shop","pr":1,"sourceCommit":"a","targetCommit":"b"}`),
		})
		if err == nil {
			t.Fatal("expected timeout error")
		}
	})
	t.Run("exit code", func(t *testing.T) {
		path := writeFakeCopilot(t, "#!/bin/sh\necho '{\"verdict\":\"CLEAN\",\"reviewedCommits\":[\"abc\"],\"reviewedFiles\":[\"main.go\"],\"findings\":[]}'\nexit 3\n")
		executor, err := executors.NewCopilotExecutor(copilotConfig(path))
		if err != nil {
			t.Fatal(err)
		}
		_, err = executor.Start(context.Background(), executors.AttemptEnvelope{
			TaskID: "task-1", AttemptID: "attempt-1", Workspace: t.TempDir(),
			Objective: "review", PayloadJSON: json.RawMessage(`{"repo":"shop","pr":1,"sourceCommit":"a","targetCommit":"b"}`),
		})
		if err == nil {
			t.Fatal("expected exit-code error")
		}
	})
}

func TestCopilotRejectsProtectedEnv(t *testing.T) {
	cfg := copilotConfig(writeFakeCopilot(t, "#!/bin/sh\nexit 0\n"))
	cfg.Environment = map[string]string{"SUMMA42_OWNER_PRIVATE_KEY": "must-not-pass"}
	if _, err := executors.NewCopilotExecutor(cfg); err == nil {
		t.Fatal("expected constructor error for protected env")
	}
}
