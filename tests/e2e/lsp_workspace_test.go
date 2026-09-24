package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLSPProcessFiveMWorkspace(t *testing.T) {
	binary := binaryEnv(t, "LUGO_BIN")
	root := copyFixture(t)
	p := startProcess(t, binary)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p.send(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"rootUri": fileURI(root), "capabilities": map[string]any{}, "initializationOptions": map[string]any{"telemetryEnabled": false},
	}})
	init := p.response(t, ctx, 1)
	if init["error"] != nil {
		t.Fatalf("initialize failed: %v", init["error"])
	}
	p.send(t, map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}})
	uri := fileURI(filepath.Join(root, "client", "main.lua"))
	sourceBytes, err := os.ReadFile(filepath.Join(root, "client", "main.lua"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(sourceBytes)
	p.send(t, map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "lua", "version": 1, "text": source}}})
	diagnostics := p.notification(t, ctx, "textDocument/publishDiagnostics", uri)
	if diagnostics["params"] == nil {
		t.Fatalf("diagnostics notification omitted params")
	}
	p.send(t, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "textDocument/documentSymbol", "params": map[string]any{"textDocument": map[string]any{"uri": uri}}})
	response := p.response(t, ctx, 2)
	if response["error"] != nil {
		t.Fatalf("document symbols failed: %v", response["error"])
	}
	if !strings.Contains(string(mustJSON(response["result"])), "greet") {
		t.Fatalf("document symbols did not contain greet: %v", response["result"])
	}
	p.send(t, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "textDocument/definition", "params": map[string]any{"textDocument": map[string]any{"uri": uri}, "position": map[string]any{"line": 8, "character": 8}}})
	definition := p.response(t, ctx, 3)
	if definition["error"] != nil || !strings.Contains(string(mustJSON(definition["result"])), "range") {
		t.Fatalf("definition failed: %v", definition)
	}
}
