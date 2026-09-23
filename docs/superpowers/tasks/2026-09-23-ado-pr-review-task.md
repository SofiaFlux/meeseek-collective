# Generic Mission workflow with ADO PR review — design checklist

- [x] Inspect current Mission, Task, capability, executor, resource and External Operation boundaries.
- [x] Verify current Copilot CLI and Azure DevOps MCP/REST behavior against official documentation.
- [x] Confirm user intent: continuously review ADO PRs assigned to the current reviewer; Copilot analyzes, Box alone publishes comments or Approve.
- [x] Compare thin CLI automation, native Copilot review, and Box-governed orchestration.
- [x] Draft a generic Box-governed lifecycle and ADO review as its first recipe.
- [x] Obtain review of the written design from the user.
- [x] Create the first implementation plan after design approval: [generic workflow kernel](../plans/2026-09-23-generic-mission-workflow-kernel.md).
- [x] Implement and verify the generic workflow kernel slice in bounded steps; see the [kernel implementation plan](../plans/2026-09-23-generic-mission-workflow-kernel.md).
- [x] Implement the durable case/assessment ledger; see the [durable workflow cases plan](../plans/2026-09-23-durable-workflow-cases.md).
- [x] Verify the complete repository after the durable ledger: `go test ./... -count=1` and `go vet ./...` passed; loopback permission was needed for tests. `graphify update .` was attempted but failed with `Operation not permitted`; its generated cache file was removed.
- [ ] Implement Task scheduling and the ADO observation and protected-effect adapters as follow-on work described in the kernel plan's scope boundary and [self-review](../plans/2026-09-23-generic-mission-workflow-kernel.md#self-review-against-the-approved-design).
- [ ] Run live ADO integration and smoke checks on the user's test machine after the follow-on scheduling and adapter work is complete.
