package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SofiaFlux/meeseek-collective/internal/control"
	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/identity"
	"github.com/spf13/cobra"
)

type fakeControlAPI struct {
	status       control.StatusDTO
	task         control.TaskDTO
	attempt      control.AttemptDTO
	operation    control.OperationDTO
	created      control.CreateTaskRequest
	approveCalls int
	shutdowns    int
}

func (f *fakeControlAPI) Status(context.Context) (control.StatusDTO, error) { return f.status, nil }
func (f *fakeControlAPI) CreateTask(_ context.Context, request control.CreateTaskRequest) (control.TaskDTO, error) {
	f.created = request
	return f.task, nil
}
func (f *fakeControlAPI) Task(context.Context, domain.ID) (control.TaskDTO, error) {
	return f.task, nil
}
func (f *fakeControlAPI) Approve(_ context.Context, _ domain.ID, signer identity.Signer) (control.ApprovalDTO, error) {
	f.approveCalls++
	return control.ApprovalDTO{ApprovalID: "approval-1", ApprovedBy: signer.PrincipalID(), Status: "APPROVED"}, nil
}
func (f *fakeControlAPI) Attempt(context.Context, domain.ID) (control.AttemptDTO, error) {
	return f.attempt, nil
}
func (f *fakeControlAPI) Operation(context.Context, domain.ID) (control.OperationDTO, error) {
	return f.operation, nil
}
func (f *fakeControlAPI) Shutdown(context.Context) error { f.shutdowns++; return nil }

func TestRootWiresControlCommandsAndStableJSONOutput(t *testing.T) {
	api := &fakeControlAPI{
		status:    control.StatusDTO{CollectiveID: "collective-1", State: "DORMANT"},
		task:      control.TaskDTO{ID: "task-1", State: domain.TaskEligible, Priority: 4},
		attempt:   control.AttemptDTO{ID: "attempt-1", TaskID: "task-1", State: domain.AttemptRunning},
		operation: control.OperationDTO{ID: "operation-1", TaskID: "task-1", State: domain.OperationPrepared},
	}

	root := newRootCommandWithClient(api)
	if findCommand(t, root, "status") == nil || findCommand(t, root, "task", "create") == nil || findCommand(t, root, "task", "show") == nil || findCommand(t, root, "approve") == nil || findCommand(t, root, "inspect", "attempt") == nil || findCommand(t, root, "inspect", "operation") == nil {
		t.Fatal("expected control commands are not all registered")
	}

	output := executeCommand(t, newRootCommandWithClient(api), "--json", "status")
	if !strings.Contains(output, `"collective_id":"collective-1"`) || !strings.Contains(output, `"state":"DORMANT"`) {
		t.Fatalf("status JSON is not stable DTO output: %s", output)
	}

	output = executeCommand(t, newRootCommandWithClient(api), "--json", "task", "show", "task-1")
	if !strings.Contains(output, `"id":"task-1"`) || strings.Contains(output, `"ID"`) {
		t.Fatalf("task JSON is not tagged stable DTO output: %s", output)
	}

	output = executeCommand(t, newRootCommandWithClient(api), "--json", "inspect", "attempt", "attempt-1")
	if !strings.Contains(output, `"id":"attempt-1"`) {
		t.Fatalf("attempt JSON = %s", output)
	}
	output = executeCommand(t, newRootCommandWithClient(api), "--json", "inspect", "operation", "operation-1")
	if !strings.Contains(output, `"id":"operation-1"`) {
		t.Fatalf("operation JSON = %s", output)
	}
}

func TestTaskCreateAndApproveCallControlAPI(t *testing.T) {
	api := &fakeControlAPI{task: control.TaskDTO{ID: "task-created", State: domain.TaskEligible}}
	output := executeCommand(t, newRootCommandWithClient(api), "--json", "task", "create",
		"--purpose-kind", string(domain.PurposeOwnerDirective),
		"--purpose-id", "owner-directive-1",
		"--acceptance", "verified result",
		"--enforcement", string(domain.EnforcementPartial),
		"--resource-envelope", "resource-1",
		"--priority", "9",
	)
	if !strings.Contains(output, `"id":"task-created"`) {
		t.Fatalf("task create output = %s", output)
	}
	if api.created.Priority != 9 || api.created.Purpose.Kind != domain.PurposeOwnerDirective || len(api.created.AcceptanceCriteria) != 1 {
		t.Fatalf("task create request = %+v", api.created)
	}

	keyPath := filepath.Join(t.TempDir(), "owner.pem")
	if _, err := identity.NewLocalEd25519(keyPath, "owner"); err != nil {
		t.Fatal(err)
	}
	output = executeCommand(t, newRootCommandWithClient(api), "--json", "approve", "approval-1", "--owner-key", keyPath)
	if api.approveCalls != 1 || !strings.Contains(output, `"status":"APPROVED"`) {
		t.Fatalf("approve calls=%d output=%s", api.approveCalls, output)
	}
}

func TestStatusHumanOutputAndShutdownRemainExplicit(t *testing.T) {
	api := &fakeControlAPI{status: control.StatusDTO{CollectiveID: "collective-1", State: "DORMANT"}}
	output := executeCommand(t, newRootCommandWithClient(api), "status")
	if !strings.Contains(output, "DORMANT") || !strings.Contains(output, "collective-1") {
		t.Fatalf("human status output = %q", output)
	}
	if api.shutdowns != 0 {
		t.Fatal("read-only status requested shutdown")
	}
}

func executeCommand(t *testing.T, command *cobra.Command, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	command.SetOut(&out)
	command.SetErr(&errOut)
	command.SetArgs(args)
	if err := command.Execute(); err != nil {
		t.Fatalf("execute %v: %v stderr=%s", args, err, errOut.String())
	}
	return out.String()
}

func findCommand(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()
	command, _, err := root.Find(path)
	if err != nil {
		t.Fatal(err)
	}
	return command
}
