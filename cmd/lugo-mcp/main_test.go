package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/FRFlo/lugo/lsp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPToolsOverInMemoryTransport(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fxmanifest.lua"), []byte("fx_version 'cerulean'\ngame 'gta5'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.lua"), []byte("local value = 1\nreturn value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{workspace: workspace, root: root}
	serverImpl := mcp.NewServer(&mcp.Implementation{Name: "lugo-mcp-test", Version: "0.1.0"}, nil)
	s.registerTools(serverImpl)
	s.registerResources(serverImpl)
	s.registerPrompts(serverImpl)

	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := serverImpl.Connect(ctx, t1, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	toolNames := map[string]bool{}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		toolNames[tool.Name] = true
	}
	for _, name := range []string{"lugo_hover", "lugo_diagnostics", "lugo_fivem_resources", "lugo_lsp_request_advanced"} {
		if !toolNames[name] {
			t.Fatalf("missing dedicated tool %q", name)
		}
	}
	for _, name := range []string{"lugo_read_file", "lugo_preview_file_write", "lugo_apply_file_write"} {
		if toolNames[name] {
			t.Fatalf("generic filesystem tool %q should not be exposed", name)
		}
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "lugo_workspace", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("got %d result blocks, want 1", len(result.Content))
	}
	diagnostics, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "lugo_diagnostics", Arguments: map[string]any{"path": "main.lua"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics.Content) != 1 {
		t.Fatalf("got %d diagnostics result blocks, want 1", len(diagnostics.Content))
	}
}

func TestSafePathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.lua"), []byte("return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := (&server{root: root}).safePath("escape/outside.lua"); err == nil {
		t.Fatal("symlinked path unexpectedly accepted")
	}
}

func newTestWorkspace(root string) (*lsp.Server, error) {
	return lsp.NewMCPWorkspace(root)
}
