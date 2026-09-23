# Read-only ADO MCP capability implementation plan

1. Test-first provider: prove exact whitelist, action validation, tool errors, and assessment of present/missing tools through an injected MCP transport. Implement `internal/adomcp` with the existing Go MCP SDK.
2. Test-first Box startup wiring: opt-in environment configuration, fail-closed partial configuration, construct provider, pass it to `runtime.Open`, call `AssessProvider` before listening. Keep disabled startup unchanged.
3. Document the configuration and Task capability/authority names; explicitly state that no worker runs tasks yet and that local MCP authentication is external.
4. Run focused tests, full `go test ./...`, `go vet ./...`, review diff and update graphify index.
