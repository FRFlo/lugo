package e2e

import (
	"context"
	"testing"
	"time"
)

func TestMCPProcessRejectsTraversal(t *testing.T) {
	p := startProcess(t, binaryEnv(t, "LUGO_MCP_BIN"), copyFixture(t))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	p.sendLine(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "security-e2e", "version": "1"}}})
	if response := p.responseLine(t, ctx, 1); response["error"] != nil {
		t.Fatalf("initialize: %v", response)
	}
	p.sendLine(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{}})
	for i, args := range []map[string]any{
		{"name": "lugo_diagnostics", "arguments": map[string]any{"path": "../outside.lua"}},
		{"name": "lugo_lsp_request_advanced", "arguments": map[string]any{"method": "textDocument/didOpen", "params": map[string]any{}}},
	} {
		id := i + 2
		p.sendLine(t, map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": args})
		response := p.responseLine(t, ctx, id)
		result, _ := response["result"].(map[string]any)
		if response["error"] == nil && result["isError"] != true {
			t.Errorf("unsafe tool call %v unexpectedly succeeded: %v", args["name"], response)
		}
	}
}
