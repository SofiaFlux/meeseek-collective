# Idempotent Workflow Task Materialization Plan

> For agentic workers: use superpowers:subagent-driven-development task by task. Track steps with checkboxes.

**Goal:** Turn a case's current generic work proposal into one durable, replay-safe execution Task with a semantic objective and structured input.

**Architecture:** Extend existing execution Tasks with objective, JSON payload and an optional idempotency key. The execution service hashes the normalized request so retries return the same Task while conflicting reuse fails. workflowcase.MaterializeTask derives purpose, task class, capabilities and authority from the active case step; the recipe supplies objective, payload, acceptance criteria, enforcement and resource envelope. The stable workflow Work ID becomes the idempotency key.

**Architecture diagram:**

    Workflow Case + Recipe Task Template
                  |
                  v
        workflowcase.MaterializeTask
                  |
                  v
           execution.CreateTask
                  |
                  v
       SQLite Task + idempotency key
                  |
                  v
       Scheduler candidate for worker

**Tech stack:** Go 1.27, SQLite/goose, existing execution, scheduler and workflowcase packages.

**Scope:** This slice prepares existing Tasks for execution and retry. It does not start a worker loop, call providers, assess executor output or publish ADO effects.

### Task 1: Persist semantic Task input and request idempotency

**Files:** create migration 00014_task_intent.sql and internal/execution/task_intent_test.go; modify internal/domain/task.go, internal/execution/service.go, internal/execution/read.go and internal/scheduler/service.go.

- [ ] Write TestTaskIntentRoundTrips for objective plus JSON payload through CreateTask and Task read.
- [ ] Write TestCreateTaskIdempotencyReplaysAndRejectsRequestDrift: same key and request returns same Task and one row; changing payload under that key errors; empty keys still create distinct Tasks.
- [ ] Run: GOCACHE=/tmp/summa42-full-go-cache go test ./internal/execution -run 'Test(TaskIntentRoundTrips|CreateTaskIdempotencyReplaysAndRejectsRequestDrift)$' -count=1. Expect a compile failure for missing fields.
- [ ] Add migration columns objective TEXT NOT NULL DEFAULT '', payload_json TEXT NOT NULL DEFAULT '{}', idempotency_key TEXT, request_hash TEXT; add a partial unique index where idempotency_key IS NOT NULL. Down drops index and columns in reverse order.
- [ ] Add Objective string, PayloadJSON json.RawMessage and IdempotencyKey string to domain.Task and execution.TaskRequest. Normalize absent payload to {}, reject invalid JSON, trim objective/key and hash all normalized request fields. In the CreateTask transaction, validate Purpose, insert Task plus key/hash; on key conflict load and return the existing Task only when hashes match, else return conflict. Empty keys skip deduplication. Update task insert/read scans and scheduler eligible-task scan for the columns.
- [ ] Run: GOCACHE=/tmp/summa42-full-go-cache go test ./internal/execution ./internal/scheduler -count=1. Expect PASS. Commit migration, domain, execution, scheduler and tests as feat: persist idempotent task inputs.

### Task 2: Materialize one case step as an existing Task

**Files:** create internal/workflowcase/materialize.go and materialize_test.go; modify internal/workflowcase/service.go.

- [ ] Write TestMaterializeUsesCaseAuthorityAndStableWorkKey. Given a case whose NextWork is read-only review, materialize a Task from a template containing objective, payload, acceptance criteria, enforcement and resource envelope. Assert Mission purpose, task class from NextWork.Kind, capabilities and authority from NextWork, request key from CurrentWorkID, and objective/payload from template. Repeating identical input returns the same Task ID; changing the objective under the same Work key errors; after assessment advances to a new Work ID, materialization creates a distinct Task.
- [ ] Run: GOCACHE=/tmp/summa42-full-go-cache go test ./internal/workflowcase -run TestMaterialize -count=1. Expect missing method failure.
- [ ] Implement Service.MaterializeTask(ctx, executionSvc, caseID, workID, template). Load Case with Get; require ACTIVE and matching CurrentWorkID; require nonblank objective, acceptance criteria and ResourceEnvelopeID. Override template Purpose to the case Mission, TaskClass to NextWork.Kind, RequiredCapabilities and AuthorityCeiling to the validated NextWork values, and IdempotencyKey to CurrentWorkID. Preserve recipe resource, acceptance, enforcement, objective and payload. Call execution.CreateTask; dispatch nothing.
- [ ] Run targeted tests and commit as feat: materialize workflow steps as idempotent tasks.

### Task 3: Carry Task intent into executor envelope and provenance

**Files:** modify internal/executors/executor.go and internal/runmanifest/service.go; create or extend their tests.

- [ ] Test that AttemptEnvelope exposes Task objective and payload.
- [ ] Test that RunManifest records objective and a stable SHA-256 payload hash, that changing payload changes the manifest hash, and raw payload does not appear in manifest JSON.
- [ ] Run targeted tests and confirm red before implementation.
- [ ] Add Objective and PayloadJSON to AttemptEnvelope. Add TaskObjective and TaskPayloadHash to runmanifest.Manifest; compute hash from domain.Task.PayloadJSON in RecordAttemptStartInTx. Never serialize raw payload in the manifest.
- [ ] Run focused executor and runmanifest tests; commit as feat: record task intent in attempt provenance.

### Task 4: Document and verify

**Files:** update internal/workflowcase/doc.go and docs/superpowers/tasks/2026-09-23-ado-pr-review-task.md.

- [ ] Document that MaterializeTask reuses a durable Task by case Work ID, the recipe supplies objective/payload/envelope, and the Scheduler still has no production worker loop.
- [ ] Run gofmt, go test ./... -count=1 with loopback access if needed, go vet ./..., git diff --check, inspect SQL scan order, and attempt graphify update .; record permission errors and remove generated untracked cache files.
- [ ] Commit docs as docs: document workflow Task materialization.

**Self-review:** This stage connects case steps to existing Task, Scheduler and Attempt provenance while keeping the process generic. It still does not create the production worker loop or the ADO observation/review/effect adapters; those remain follow-on work under the approved design.
