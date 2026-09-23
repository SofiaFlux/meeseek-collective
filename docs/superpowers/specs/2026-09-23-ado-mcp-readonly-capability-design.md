# Explicit read-only Azure DevOps MCP capabilities

## Scope

The Box does not discover ambient MCP servers. When the Owner explicitly configures a local Azure DevOps MCP command and organization, startup constructs one provider and assesses its two supported read capabilities. Without that configuration, startup and capability state stay unchanged. This slice does not add task execution or implicit authority.

## Contract

- The provider advertises `ado.projects.list` and `ado.work_item.read` with stable IDs, version `v1`, provider `ado-mcp`, organization-specific access context, self-named authority requirements, and minimum `PARTIAL` enforcement.
- Assessment connects to the configured MCP server, checks exact required tools, and records evidence, cost metadata, health and availability. Missing tools are assessed unavailable; transport failures fail startup when ADO is enabled.
- Calls accept only the two semantic capabilities. `ado.projects.list` accepts no request parameters and calls `core_list_projects`. `ado.work_item.read` accepts only an object with an allowed read action (`get`, `get_batch`, `list_comments`, `my`, `list_revisions`, `list_for_iteration`, `get_type`), then calls `wit_work_item`. No write-named tools or other actions can pass through this adapter.
- Each call uses the existing capability Session checks for task authority, assessment, active lease and visibility. MCP tool errors become Go errors. No credentials are stored in repo or local config; the child process uses the caller's existing authentication context.
- Box wiring is opt-in via environment variables for executable path and organization, with a bounded connect/call timeout. Both are required together. It assesses at startup before serving control requests. No auto-registration of arbitrary advertised tools.

## Boundary and follow-up

This is a read-only integration primitive, not an autonomous mission worker. A Task can declare these semantic capability names and matching authority ceiling, but an executor/worker must still open a Session to call them. The separate worker design must address workspace, resource envelope, lease renewal, evidence, verification and protected external writes before activation.
