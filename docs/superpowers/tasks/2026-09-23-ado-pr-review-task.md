# Generic Mission workflow with ADO PR review — design checklist

- [x] Inspect current Mission, Task, capability, executor, resource and External Operation boundaries.
- [x] Verify current Copilot CLI and Azure DevOps MCP/REST behavior against official documentation.
- [x] Confirm user intent: continuously review ADO PRs assigned to the current reviewer; Copilot analyzes, Box alone publishes comments or Approve.
- [x] Compare thin CLI automation, native Copilot review, and Box-governed orchestration.
- [x] Draft a generic Box-governed lifecycle and ADO review as its first recipe.
- [ ] Obtain review of the written design from the user.
- [ ] Create the implementation plan after design approval.
- [ ] Implement and verify in bounded steps; live ADO testing remains on the user's test machine.
