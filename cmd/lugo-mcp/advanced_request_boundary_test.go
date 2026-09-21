package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func advancedRequest(path string, method string, params map[string]any) *mcp.CallToolRequest {
	arguments := map[string]any{"method": method, "params": params}
	if path != "" {
		arguments["params"] = map[string]any{"textDocument": map[string]any{"uri": path}}
	}
	data, _ := json.Marshal(arguments)
	return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: data}}
}

func TestLSPRequestAdvancedRejectsStatefulWriteAndLifecycleMethods(t *testing.T) {
	s := &server{}
	for _, method := range []string{
		"textDocument/didOpen",
		"textDocument/didChange",
		"textDocument/formatting",
		"textDocument/rename",
		"shutdown",
		"exit",
	} {
		t.Run(method, func(t *testing.T) {
			_, err := s.lspRequest(context.Background(), advancedRequest("", method, nil))
			if err == nil {
				t.Fatalf("advanced request %q unexpectedly succeeded", method)
			}
		})
	}
}

func TestLSPRequestAdvancedRejectsUnsafePathParams(t *testing.T) {
	s := &server{root: t.TempDir()}
	for _, uri := range []string{
		"file:///outside.lua",
		"file:///tmp/../outside.lua",
		"file://" + filepath.ToSlash(filepath.Join(s.root, "..", "outside.lua")),
	} {
		t.Run(uri, func(t *testing.T) {
			_, err := s.lspRequest(context.Background(), advancedRequest(uri, "textDocument/hover", nil))
			if err == nil || !strings.Contains(err.Error(), "workspace") {
				t.Fatalf("unsafe URI %q error = %v, want workspace boundary error", uri, err)
			}
		})
	}
}

func TestLSPRequestAdvancedRejectsSymlinkedURI(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.lua"), []byte("return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	s := &server{root: root}
	uri := "file://" + filepath.ToSlash(filepath.Join(root, "escape", "outside.lua"))
	_, err := s.lspRequest(context.Background(), advancedRequest(uri, "textDocument/hover", nil))
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("symlinked URI error = %v, want workspace boundary error", err)
	}
}

func TestLSPRequestAdvancedAllowsSupportedReadOnlyMethod(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.lua"), []byte("local value = 1\nreturn value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{workspace: workspace, root: root}
	uri := workspace.MCPDocumentURI(filepath.Join(root, "main.lua"))
	result, err := s.lspRequest(context.Background(), advancedRequest(uri, "textDocument/hover", nil))
	if err != nil {
		t.Fatalf("supported read-only request failed: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("got %d result blocks, want 1", len(result.Content))
	}
}
