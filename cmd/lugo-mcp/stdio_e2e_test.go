package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPStdioObservabilityArtifact exercises the real MCP process boundary.
// It is opt-in because it builds and spawns the command, and writes a report
// that can be retained by CI or a local verification run.
func TestMCPStdioObservabilityArtifact(t *testing.T) {
	if os.Getenv("LUGO_E2E") != "1" {
		t.Skip("set LUGO_E2E=1 to run the spawned-process observability scenario")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.lua"), []byte("local answer = 42\nreturn answer\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fxmanifest.lua"), []byte("fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n"), 0600); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(t.TempDir(), "lugo-mcp")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", bin, ".")
	build.Dir = "."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build MCP: %v\n%s", err, output)
	}

	journal := filepath.Join(t.TempDir(), "trace.ndjson")
	cmd := exec.CommandContext(ctx, bin, root)
	cmd.Env = append(os.Environ(),
		"LUGO_TELEMETRY=true",
		"LUGO_TELEMETRY_LOCAL_ONLY=true",
		"LUGO_MCP_TELEMETRY=1",
		"LUGO_TRACE_JOURNAL="+journal,
		"LUGO_TRACE_MAX_BYTES=65536",
	)
	client := mcp.NewClient(&mcp.Implementation{Name: "lugo-e2e-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	workspace, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "lugo_workspace", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "lugo_diagnostics", Arguments: map[string]any{"path": "main.lua"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(workspace.Content) == 0 || len(diagnostics.Content) == 0 {
		t.Fatal("MCP returned empty workspace or diagnostics result")
	}

	traceData, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(traceData), "lugo.mcp.boundary") || !strings.Contains(string(traceData), "lugo.lsp.request") {
		t.Fatalf("trace journal missed MCP/LSP boundaries: %s", traceData)
	}

	report := map[string]any{
		"scenario":                  "MCP stdio workspace and diagnostics",
		"workspace_root":            "[REDACTED]",
		"workspace_content_items":   len(workspace.Content),
		"diagnostics_content_items": len(diagnostics.Content),
		"workspace_payload_bytes":   len(fmt.Sprint(workspace.Content)),
		"diagnostics_payload_bytes": len(fmt.Sprint(diagnostics.Content)),
		"trace_bytes":               len(traceData),
		"trace_events":              strings.Count(string(traceData), "\"event\""),
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	reportPath := os.Getenv("LUGO_E2E_ARTIFACT")
	if reportPath == "" {
		reportPath = filepath.Join(t.TempDir(), "observability-report.json")
	}
	if err := os.WriteFile(reportPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("observability artifact: %s", reportPath)
}
