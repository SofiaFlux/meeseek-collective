package control

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/execution"
)

type fakeStatusProvider struct {
	calls int
	value StatusDTO
}

func (f *fakeStatusProvider) Status(context.Context) (StatusDTO, error) {
	f.calls++
	return f.value, nil
}

type fakeTaskService struct {
	created int
	last    execution.TaskRequest
	task    domain.Task
}

func (f *fakeTaskService) CreateTask(_ context.Context, request execution.TaskRequest) (domain.Task, error) {
	f.created++
	f.last = request
	return f.task, nil
}

func (f *fakeTaskService) Task(context.Context, domain.ID) (domain.Task, error) {
	return f.task, nil
}

type fakeApprovalService struct {
	calls int
	id    domain.ID
	actor domain.ID
}

func (f *fakeApprovalService) Approve(_ context.Context, id, actor domain.ID) error {
	f.calls++
	f.id = id
	f.actor = actor
	return nil
}

type fakeAttemptReader struct{ attempt domain.Attempt }

func (f *fakeAttemptReader) Attempt(context.Context, domain.ID) (domain.Attempt, error) {
	return f.attempt, nil
}

type fakeOperationReader struct{ operation domain.ExternalOperation }

func (f *fakeOperationReader) Operation(context.Context, domain.ID) (domain.ExternalOperation, error) {
	return f.operation, nil
}

type fakeShutdownService struct{ calls int }

func (f *fakeShutdownService) RequestShutdown(context.Context) error {
	f.calls++
	return nil
}

func TestControlAPIRequiresBearerAuthAndStatusIsReadOnly(t *testing.T) {
	server, status, _, _, _, _, _ := newTestServer(t)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	response, err := http.Get(httpServer.URL + "/status")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
	if status.calls != 0 {
		t.Fatalf("status provider called before authentication: %d", status.calls)
	}

	response = doRequest(t, http.MethodGet, httpServer.URL+"/status", "control-secret", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authenticated status = %d, want 200", response.StatusCode)
	}
	var got StatusDTO
	decodeJSON(t, response, &got)
	if got.State != "DORMANT" || got.CollectiveID != "collective-test" {
		t.Fatalf("status = %+v", got)
	}

	response = doRequest(t, http.MethodPost, httpServer.URL+"/status", "control-secret", bytes.NewBufferString("{}"))
	response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /status = %d, want 405", response.StatusCode)
	}
	if status.calls != 1 {
		t.Fatalf("status provider calls = %d, want exactly one read", status.calls)
	}
}

func TestApprovalRequiresOwnerSignatureOverChallengeAndDigest(t *testing.T) {
	server, _, _, approvals, _, _, _ := newTestServer(t)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	response := doRequest(t, http.MethodPost, httpServer.URL+"/approvals/approval-1", "control-secret", bytes.NewBufferString(`{}`))
	response.Body.Close()
	if response.StatusCode == http.StatusOK || approvals.calls != 0 {
		t.Fatalf("authenticated transport acted as Owner: status=%d calls=%d", response.StatusCode, approvals.calls)
	}

	challengeResponse := doRequest(t, http.MethodGet, httpServer.URL+"/approvals/approval-1/challenge", "control-secret", nil)
	if challengeResponse.StatusCode != http.StatusOK {
		t.Fatalf("challenge status = %d", challengeResponse.StatusCode)
	}
	var challenge ApprovalChallengeDTO
	decodeJSON(t, challengeResponse, &challenge)
	if challenge.Challenge == "" || challenge.RequestDigest == "" {
		t.Fatalf("empty approval challenge: %+v", challenge)
	}

	_, wrongPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	wrongSignature := base64.StdEncoding.EncodeToString(ed25519.Sign(wrongPrivate, ApprovalSigningMessage(challenge.Challenge, challenge.RequestDigest)))
	response = postApproval(t, httpServer.URL, challenge, wrongSignature)
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden || approvals.calls != 0 {
		t.Fatalf("wrong Owner signature status=%d calls=%d", response.StatusCode, approvals.calls)
	}

	ownerSignature := base64.StdEncoding.EncodeToString(ed25519.Sign(testOwnerPrivateKey(t), ApprovalSigningMessage(challenge.Challenge, challenge.RequestDigest)))
	response = postApproval(t, httpServer.URL, challenge, ownerSignature)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid Owner approval status = %d, want 200", response.StatusCode)
	}
	if approvals.calls != 1 || approvals.id != "approval-1" || approvals.actor != "owner-test" {
		t.Fatalf("approval call = calls:%d id:%s actor:%s", approvals.calls, approvals.id, approvals.actor)
	}

	response = postApproval(t, httpServer.URL, challenge, ownerSignature)
	response.Body.Close()
	if response.StatusCode == http.StatusOK || approvals.calls != 1 {
		t.Fatalf("approval challenge replay accepted: status=%d calls=%d", response.StatusCode, approvals.calls)
	}
}

func TestControlRoutesUseCoreServiceInterfaces(t *testing.T) {
	server, _, tasks, _, attempts, operations, shutdown := newTestServer(t)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	create := CreateTaskRequest{
		Purpose:             domain.PurposeRef{Kind: domain.PurposeOwnerDirective, ID: "owner-directive-1"},
		AcceptanceCriteria:  []string{"result is verified"},
		RequiredEnforcement: domain.EnforcementPartial,
		ResourceEnvelopeID:  "resource-1",
		Priority:            7,
	}
	payload, err := json.Marshal(create)
	if err != nil {
		t.Fatal(err)
	}
	response := doRequest(t, http.MethodPost, httpServer.URL+"/tasks", "control-secret", bytes.NewReader(payload))
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("POST /tasks = %d", response.StatusCode)
	}
	var task TaskDTO
	decodeJSON(t, response, &task)
	if task.ID != "task-1" || tasks.created != 1 || tasks.last.Priority != 7 {
		t.Fatalf("task route did not use task service: dto=%+v calls=%d request=%+v", task, tasks.created, tasks.last)
	}

	response = doRequest(t, http.MethodGet, httpServer.URL+"/tasks/task-1", "control-secret", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET task = %d", response.StatusCode)
	}
	decodeJSON(t, response, &task)
	if task.ID != "task-1" {
		t.Fatalf("task = %+v", task)
	}

	response = doRequest(t, http.MethodGet, httpServer.URL+"/inspect/attempts/attempt-1", "control-secret", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET attempt = %d", response.StatusCode)
	}
	var attempt AttemptDTO
	decodeJSON(t, response, &attempt)
	if attempt.ID != attempts.attempt.ID {
		t.Fatalf("attempt = %+v", attempt)
	}

	response = doRequest(t, http.MethodGet, httpServer.URL+"/inspect/operations/operation-1", "control-secret", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET operation = %d", response.StatusCode)
	}
	var operation OperationDTO
	decodeJSON(t, response, &operation)
	if operation.ID != operations.operation.ID {
		t.Fatalf("operation = %+v", operation)
	}

	response = doRequest(t, http.MethodPost, httpServer.URL+"/shutdown", "control-secret", nil)
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted || shutdown.calls != 1 {
		t.Fatalf("shutdown status=%d calls=%d", response.StatusCode, shutdown.calls)
	}
}

func TestListenLocalUsesPlatformLocalTransport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix network assertion is covered on non-Windows CI; Windows is compile-gated separately")
	}
	listener, err := ListenLocal(filepath.Join(t.TempDir(), "control.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if got := listener.Addr().Network(); got != "unix" {
		t.Fatalf("listener network = %q, want unix", got)
	}
}

var ownerPrivate ed25519.PrivateKey

func newTestServer(t *testing.T) (*Server, *fakeStatusProvider, *fakeTaskService, *fakeApprovalService, *fakeAttemptReader, *fakeOperationReader, *fakeShutdownService) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ownerPrivate = private
	status := &fakeStatusProvider{value: StatusDTO{CollectiveID: "collective-test", State: "DORMANT"}}
	tasks := &fakeTaskService{task: domain.Task{ID: "task-1", State: domain.TaskEligible, Priority: 7}}
	approvals := &fakeApprovalService{}
	attempts := &fakeAttemptReader{attempt: domain.Attempt{ID: "attempt-1", TaskID: "task-1", State: domain.AttemptRunning}}
	operations := &fakeOperationReader{operation: domain.ExternalOperation{ID: "operation-1", TaskID: "task-1", AttemptID: "attempt-1", State: domain.OperationPrepared}}
	shutdown := &fakeShutdownService{}
	server, err := NewServer(ServerConfig{
		AuthToken:        "control-secret",
		OwnerPrincipalID: "owner-test",
		OwnerPublicKey:   public,
		ChallengeTTL:     time.Minute,
	}, Dependencies{
		Status: status, Tasks: tasks, Approvals: approvals, Attempts: attempts, Operations: operations, Shutdown: shutdown,
	})
	if err != nil {
		t.Fatal(err)
	}
	return server, status, tasks, approvals, attempts, operations, shutdown
}

func testOwnerPrivateKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	if len(ownerPrivate) != ed25519.PrivateKeySize {
		t.Fatal("Owner private key fixture not initialized")
	}
	return ownerPrivate
}

func doRequest(t *testing.T, method, url, token string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func postApproval(t *testing.T, baseURL string, challenge ApprovalChallengeDTO, signature string) *http.Response {
	t.Helper()
	payload, err := json.Marshal(ApprovalRequest{Challenge: challenge.Challenge, Signature: signature})
	if err != nil {
		t.Fatal(err)
	}
	return doRequest(t, http.MethodPost, baseURL+"/approvals/approval-1", "control-secret", bytes.NewReader(payload))
}

func decodeJSON(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("content type = %q, want application/json", response.Header.Get("Content-Type"))
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
