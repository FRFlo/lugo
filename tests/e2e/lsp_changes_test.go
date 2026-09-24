package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLSPProcessDocumentChangesReplaceSymbols(t *testing.T) {
	root := copyFixture(t)
	p := startProcess(t, binaryEnv(t, "LUGO_BIN"))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	p.send(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"rootUri": fileURI(root), "capabilities": map[string]any{}}})
	if response := p.response(t, ctx, 1); response["error"] != nil {
		t.Fatalf("initialize: %v", response)
	}
	p.send(t, map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}})
	uri := fileURI(filepath.Join(root, "client", "main.lua"))
	initial, err := os.ReadFile(filepath.Join(root, "client", "main.lua"))
	if err != nil {
		t.Fatal(err)
	}
	p.send(t, map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "lua", "version": 1, "text": string(initial)}}})
	for _, step := range []struct {
		name, source, want, absent string
		version                    int
	}{
		{"initial", "", "greet", "changedGreeting", 1},
		{"changed", "local function changedGreeting() return 42 end\nreturn changedGreeting\n", "changedGreeting", "greet", 2},
	} {
		t.Run(step.name, func(t *testing.T) {
			if step.version > 1 {
				p.send(t, map[string]any{"jsonrpc": "2.0", "method": "textDocument/didChange", "params": map[string]any{"textDocument": map[string]any{"uri": uri, "version": step.version}, "contentChanges": []map[string]any{{"text": step.source}}}})
			}
			p.send(t, map[string]any{"jsonrpc": "2.0", "id": step.version + 1, "method": "textDocument/documentSymbol", "params": map[string]any{"textDocument": map[string]any{"uri": uri}}})
			response := p.response(t, ctx, step.version+1)
			if response["error"] != nil {
				t.Fatalf("documentSymbol: %v", response)
			}
			symbols := string(mustJSON(response["result"]))
			if !strings.Contains(symbols, `"name":"`+step.want+`"`) || strings.Contains(symbols, `"name":"`+step.absent+`"`) {
				t.Fatalf("symbols after %s = %s", step.name, symbols)
			}
		})
	}
}
