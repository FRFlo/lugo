# End-to-end tests

`go test ./tests/e2e -v` builds the Lugo LSP and MCP binaries once, starts each as
a subprocess, and exercises their real stdio protocols against the fixtures in
`fixtures/fivem-static/`. The tests are part of ordinary `go test ./...` and do
not require an FXServer, network access, or a configured FiveM installation.

The fixture has separate provider and consumer resources, a client/server/shared
resource, NUI assets, and intentionally unresolved interactions. These tests
verify **static analysis and protocol behavior**; they do not claim to execute
FiveM events, exports, or natives in a live game server.

To reuse prebuilt binaries, set `LUGO_BIN` and `LUGO_MCP_BIN` to executable
paths. Without those variables, the test package builds temporary binaries and
removes them after completion. Telemetry is disabled in subprocesses. Process
output is treated as protocol data; diagnostics are captured separately from
stderr. Each scenario uses a temporary copy of the fixture workspace.

Coverage layers:

- `token/`, `lexer/`, `parser/`, `ast/`, `semantic/`: focused invariants,
  malformed input, reusable state, and zero-allocation gates.
- `lsp/`, `cmd/lugo-mcp/`: in-process integration and public request contracts.
- `tests/e2e/*.go`: one file per scenario for spawned-process LSP/MCP smoke,
  document changes, navigation, semantic tokens, resource contracts, security
  boundaries, and the headless `--ci` binary entry point. Shared stdio and
  fixture helpers live in `tests/e2e/harness_test.go`.

VS Code extension tests in `vscode/` are JS unit/contract tests, not editor E2E.

Add a regression test at the narrowest useful layer first; add a process E2E
when framing, lifecycle, initialization, workspace state, or cross-boundary
behavior matters. Never put a real or simulated FXServer in this suite.
