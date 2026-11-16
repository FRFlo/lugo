package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPFormattingOptions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.lua"), []byte("local value=1\nreturn value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	impl := mcp.NewServer(&mcp.Implementation{Name: "lugo-mcp-formatting", Version: "1"}, nil)
	s := &server{workspace: workspace, root: root}
	s.registerTools(impl)

	ctx := context.Background()
	transport1, transport2 := mcp.NewInMemoryTransports()
	if _, err := impl.Connect(ctx, transport1, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "formatting-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, transport2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tests := []struct {
		name      string
		options   map[string]any
		wantError bool
	}{
		{name: "omitted options"},
		{name: "explicit valid options", options: map[string]any{"tabSize": 2, "insertSpaces": true}},
		{name: "explicit invalid options", options: map[string]any{"tabSize": 0, "insertSpaces": true}, wantError: true},
	}
	for _, tool := range []string{"lugo_format", "lugo_range_format"} {
		for _, tt := range tests {
			t.Run(tool+"/"+tt.name, func(t *testing.T) {
				args := map[string]any{"path": "main.lua"}
				if tool == "lugo_range_format" {
					args["line"] = 0
					args["character"] = 0
					args["endLine"] = 1
					args["endCharacter"] = 20
				}
				if tt.options != nil {
					args["options"] = tt.options
				}
				result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
				if tt.wantError {
					if callErr == nil && (result == nil || !result.IsError) {
						t.Fatalf("invalid formatting options unexpectedly succeeded: %+v", result)
					}
					if result != nil && !result.IsError && !strings.Contains(callErr.Error(), "tabSize must be between 1 and 16") {
						t.Fatalf("invalid options error = %v, want tabSize validation error", callErr)
					}
					return
				}
				if callErr != nil {
					t.Fatal(callErr)
				}
				if result == nil || len(result.Content) == 0 || result.IsError {
					t.Fatalf("formatting returned invalid result: %+v", result)
				}
			})
		}
	}
}
