package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLSPProcessFiveMNavigationAndTokens(t *testing.T) {
	root := copyFixture(t)
	p := startProcess(t, binaryEnv(t, "LUGO_BIN"))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	p.send(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"rootUri": fileURI(root), "capabilities": map[string]any{}}})
	if response := p.response(t, ctx, 1); response["error"] != nil {
		t.Fatalf("initialize: %v", response)
	}
	p.send(t, map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}})
	uri := fileURI(filepath.Join(root, "provider", "server.lua"))
	source, err := os.ReadFile(filepath.Join(root, "provider", "server.lua"))
	if err != nil {
		t.Fatal(err)
	}
	p.send(t, map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "lua", "version": 1, "text": string(source)}}})
	request := func(id int, method string, params any) map[string]any {
		t.Helper()
		p.send(t, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		response := p.response(t, ctx, id)
		if response["error"] != nil {
			t.Fatalf("%s: %v", method, response)
		}
		return response
	}
	position := map[string]any{"line": 3, "character": strings.LastIndex(strings.Split(string(source), "\n")[3], "GetGreeting") + 2}
	params := map[string]any{"textDocument": map[string]any{"uri": uri}, "position": position}
	definition := request(2, "textDocument/definition", params)
	if !strings.Contains(string(mustJSON(definition["result"])), "server.lua") {
		t.Fatalf("export declaration did not resolve to provider/server.lua: %v", definition)
	}
	tokens := request(3, "textDocument/semanticTokens/full", map[string]any{"textDocument": map[string]any{"uri": uri}})
	result, _ := tokens["result"].(map[string]any)
	data, _ := result["data"].([]any)
	if len(data) < 5 || len(data)%5 != 0 {
		t.Fatalf("invalid semantic token delta stream: %v", tokens)
	}
	symbols := request(4, "textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": uri}})
	if !strings.Contains(string(mustJSON(symbols["result"])), `"name":"GetGreeting"`) {
		t.Fatalf("provider function absent from symbols: %v", symbols)
	}
}
