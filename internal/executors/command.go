package executors

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type CommandConfig struct {
	Path        string
	Args        []string
	Environment map[string]string
	Timeout     time.Duration
}

type CommandExecutor struct {
	config CommandConfig
}

func NewCommandExecutor(config CommandConfig) (*CommandExecutor, error) {
	config.Path = strings.TrimSpace(config.Path)
	if config.Path == "" {
		return nil, errors.New("command path is required")
	}
	if config.Timeout <= 0 {
		return nil, errors.New("command timeout must be positive")
	}
	config.Args = append([]string(nil), config.Args...)
	config.Environment = cloneEnvironment(config.Environment)
	return &CommandExecutor{config: config}, nil
}

func (e *CommandExecutor) Start(ctx context.Context, envelope AttemptEnvelope) (ExecutionResult, error) {
	if e == nil {
		return ExecutionResult{}, errors.New("command executor is not configured")
	}
	if strings.TrimSpace(envelope.Workspace) == "" {
		return ExecutionResult{}, errors.New("attempt workspace is required")
	}

	runCtx, cancel := context.WithTimeout(ctx, e.config.Timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, e.config.Path, e.config.Args...)
	cmd.Dir = envelope.Workspace
	cmd.Env = environmentList(e.config.Environment)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	started := time.Now()
	err := cmd.Run()
	wall := time.Since(started)
	result := ExecutionResult{
		ExitCode: exitCode(err),
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Evidence: []Evidence{
			{Kind: EvidenceStdout, Content: stdout.String()},
			{Kind: EvidenceStderr, Content: stderr.String()},
		},
		Usage: Usage{WallTime: wall},
	}
	if runCtx.Err() != nil {
		return result, runCtx.Err()
	}
	if err != nil {
		return result, fmt.Errorf("command execution failed: %w", err)
	}
	return result, nil
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func environmentList(environment map[string]string) []string {
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+environment[key])
	}
	return result
}

func cloneEnvironment(source map[string]string) map[string]string {
	if source == nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
