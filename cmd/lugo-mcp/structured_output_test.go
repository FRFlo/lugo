package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStructuredOutputContractForWorkspaceDiagnosticsAndFiveM(t *testing.T) {
	root := t.TempDir()
	for name, contents := range map[string]string{
		"fxmanifest.lua": "fx_version 'cerulean'\ngame 'gta5'\n",
		"main.lua":       "local value = 1\nreturn value\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{workspace: workspace, root: root}
	impl := mcp.NewServer(&mcp.Implementation{Name: "structured-output-test", Version: "1"}, nil)
	s.registerTools(impl)
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := impl.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools := map[string]*mcp.Tool{}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		tools[tool.Name] = tool
	}
	for _, name := range []string{"lugo_workspace", "lugo_diagnostics"} {
		if tools[name] == nil || tools[name].OutputSchema == nil {
			t.Fatalf("%s must declare an output schema", name)
		}
	}
	for _, name := range []string{"lugo_fivem_resources", "lugo_fivem_events", "lugo_fivem_exports"} {
		if tools[name] == nil {
			t.Fatalf("%s must be advertised", name)
		}
		if tools[name].OutputSchema != nil {
			t.Fatalf("%s must not advertise a union output schema unsupported by pi", name)
		}
	}

	calls := []struct {
		name string
		args map[string]any
	}{
		{"lugo_workspace", map[string]any{}},
		{"lugo_diagnostics", map[string]any{"path": "main.lua"}},
		{"lugo_fivem_resources", map[string]any{}},
		{"lugo_fivem_events", map[string]any{}},
		{"lugo_fivem_exports", map[string]any{}},
	}
	for _, call := range calls {
		t.Run(call.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: call.name, Arguments: call.args})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Content) != 1 {
				t.Fatalf("%s returned %d content blocks, want text compatibility block", call.name, len(result.Content))
			}
			if _, ok := result.Content[0].(*mcp.TextContent); !ok {
				t.Fatalf("%s content type = %T, want *mcp.TextContent", call.name, result.Content[0])
			}
			if call.name == "lugo_workspace" || call.name == "lugo_diagnostics" {
				if result.StructuredContent == nil {
					t.Fatalf("%s returned no structured content", call.name)
				}
				if _, err := json.Marshal(result.StructuredContent); err != nil {
					t.Fatalf("%s structured content is not JSON: %v", call.name, err)
				}
			} else if result.StructuredContent != nil {
				t.Fatalf("%s returned structured content for an array-root result", call.name)
			}
		})
	}
}
