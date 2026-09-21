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

func TestFiveMReviewPromptUsesCurrentMCPTools(t *testing.T) {
	serverImpl := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	(&server{}).registerPrompts(serverImpl)
	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := serverImpl.Connect(ctx, t1, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.GetPrompt(ctx, &mcp.GetPromptParams{Name: "lugo_fivem_review", Arguments: map[string]string{"path": "client/main.lua"}})
	if err != nil {
		t.Fatal(err)
	}
	text := result.Messages[0].Content.(*mcp.TextContent).Text
	if strings.Contains(text, "lugo_lsp_request") || strings.Contains(text, "textDocument/diagnostic") {
		t.Fatalf("prompt contains stale tool or method: %q", text)
	}
	for _, want := range []string{"lugo_diagnostics", "lugo_hover", "lugo_definition", "lugo_references", "lugo_workspace"} {
		if !strings.Contains(text, want) {
			t.Errorf("prompt missing current tool %q: %q", want, text)
		}
	}
}

func TestRangeFormattingRejectsUnsafePath(t *testing.T) {
	s := &server{root: t.TempDir()}
	request := func(path string) *mcp.CallToolRequest {
		return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"path":"` + path + `","line":0,"character":0,"endLine":0,"endCharacter":1,"options":{"tabSize":2,"insertSpaces":true}}`)}}
	}
	for _, path := range []string{"", "../outside.lua"} {
		if _, err := s.capability("textDocument/rangeFormatting")(context.Background(), request(path)); err == nil {
			t.Errorf("range formatting path %q unexpectedly succeeded", path)
		}
	}
}

func TestRangeFormattingForwardsRange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.lua")
	if err := os.WriteFile(path, []byte("local first=1\nlocal second=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	workspace.FeatureFormatting = true
	s := &server{workspace: workspace, root: root}
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"path":"main.lua","line":1,"character":0,"endLine":1,"endCharacter":15,"options":{"tabSize":2,"insertSpaces":true}}`)}}
	result, err := s.capability("textDocument/rangeFormatting")(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, `"range":{"start":{"line":1`) {
		t.Fatalf("range formatting did not return an edit for the requested range: %s", text)
	}
}
