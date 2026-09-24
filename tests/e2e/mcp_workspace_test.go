package e2e

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMCPProcessFiveMWorkspace(t *testing.T) {
	binary := binaryEnv(t, "LUGO_MCP_BIN")
	root := copyFixture(t)
	p := startProcess(t, binary, root)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p.sendLine(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "lugo-e2e", "version": "1"},
	}})
	init := p.responseLine(t, ctx, 1)
	if init["error"] != nil {
		t.Fatalf("MCP initialize failed: %v", init["error"])
	}
	p.sendLine(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{}})
	p.sendLine(t, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "lugo_workspace", "arguments": map[string]any{}}})
	result := p.responseLine(t, ctx, 2)
	if result["error"] != nil {
		t.Fatalf("MCP workspace call failed: %v", result["error"])
	}
	if !strings.Contains(string(mustJSON(result["result"])), "fxmanifest.lua") {
		t.Fatalf("workspace result omitted manifest: %s", mustJSON(result["result"]))
	}
	p.sendLine(t, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "lugo_fivem_contracts", "arguments": map[string]any{}}})
	contracts := p.responseLine(t, ctx, 3)
	contractJSON := string(mustJSON(contracts["result"]))
	if !strings.Contains(contractJSON, "provider") || !strings.Contains(contractJSON, "GetGreeting") {
		t.Fatalf("contracts omitted provider export: %s", contractJSON)
	}
	if !strings.Contains(contractJSON, "consumer") {
		t.Fatalf("contracts omitted consumer resource: %s", contractJSON)
	}
}
