package executors_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/executors"
)

func TestCommandExecutorPreservesArgvCapturesEvidenceAndDropsAmbientEnvironment(t *testing.T) {
	workspace := t.TempDir()
	marker := filepath.Join(workspace, "should-not-exist")
	dangerous := "literal;touch " + marker
	t.Setenv("AWS_SECRET_ACCESS_KEY", "must-not-leak")

	executor, err := executors.NewCommandExecutor(executors.CommandConfig{
		Path: os.Args[0],
		Args: []string{"-test.run=TestCommandExecutorHelperProcess", "--", dangerous},
		Environment: map[string]string{
			"SUMMA42_HELPER_PROCESS": "1",
		},
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := executor.Start(context.Background(), executors.AttemptEnvelope{
		TaskID:    domain.ID("task-command"),
		AttemptID: domain.ID("attempt-command"),
		Workspace: workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", result.ExitCode, result.Stderr)
	}
	if strings.TrimSpace(result.Stdout) != dangerous {
		t.Fatalf("stdout = %q, want literal argv %q", result.Stdout, dangerous)
	}
	if !strings.Contains(result.Stderr, "AWS_SECRET_ACCESS_KEY=") || strings.Contains(result.Stderr, "must-not-leak") {
		t.Fatalf("ambient secret leaked or helper output missing: %q", result.Stderr)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shell metacharacters were interpreted; marker stat err=%v", err)
	}
	if !hasEvidence(result.Evidence, executors.EvidenceStdout, dangerous) {
		t.Fatalf("stdout evidence missing: %#v", result.Evidence)
	}
	if !hasEvidence(result.Evidence, executors.EvidenceStderr, "AWS_SECRET_ACCESS_KEY=") {
		t.Fatalf("stderr evidence missing: %#v", result.Evidence)
	}
	if result.Usage.WallTime <= 0 {
		t.Fatalf("wall time = %s, want positive observation", result.Usage.WallTime)
	}
}

func TestCommandExecutorEnforcesWallClockTimeout(t *testing.T) {
	executor, err := executors.NewCommandExecutor(executors.CommandConfig{
		Path: os.Args[0],
		Args: []string{"-test.run=TestCommandExecutorHelperProcess"},
		Environment: map[string]string{
			"SUMMA42_HELPER_PROCESS": "1",
			"SUMMA42_HELPER_SLEEP":   "5s",
		},
		Timeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = executor.Start(context.Background(), executors.AttemptEnvelope{
		TaskID:    domain.ID("task-timeout"),
		AttemptID: domain.ID("attempt-timeout"),
		Workspace: t.TempDir(),
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v, want context deadline exceeded", err)
	}
}

func TestCommandExecutorHelperProcess(t *testing.T) {
	if os.Getenv("SUMMA42_HELPER_PROCESS") != "1" {
		return
	}
	if raw := os.Getenv("SUMMA42_HELPER_SLEEP"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			os.Exit(3)
		}
		time.Sleep(d)
	}

	literal := ""
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			literal = os.Args[i+1]
			break
		}
	}
	_, _ = os.Stdout.WriteString(literal)
	_, _ = os.Stderr.WriteString("AWS_SECRET_ACCESS_KEY=" + os.Getenv("AWS_SECRET_ACCESS_KEY"))
	os.Exit(0)
}

func hasEvidence(evidence []executors.Evidence, kind executors.EvidenceKind, contains string) bool {
	for _, item := range evidence {
		if item.Kind == kind && strings.Contains(item.Content, contains) {
			return true
		}
	}
	return false
}
