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

func TestMCPInMemoryTransportCoversRegisteredSurface(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fxmanifest.lua"), []byte("fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.lua"), []byte("local value = 1\nreturn value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	impl := mcp.NewServer(&mcp.Implementation{Name: "lugo-mcp-integration", Version: "1"}, nil)
	s := &server{workspace: workspace, root: root}
	s.registerTools(impl)
	s.registerResources(impl)
	s.registerPrompts(impl)

	ctx := context.Background()
	transport1, transport2 := mcp.NewInMemoryTransports()
	if _, err := impl.Connect(ctx, transport1, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "integration-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, transport2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	wantTools := map[string]bool{
		"lugo_hover": true, "lugo_completion": true, "lugo_signature_help": true,
		"lugo_definition": true, "lugo_type_definition": true, "lugo_implementation": true,
		"lugo_references": true, "lugo_document_symbols": true, "lugo_workspace_symbols": true,
		"lugo_format": true, "lugo_range_format": true, "lugo_rename": true,
		"lugo_prepare_rename": true, "lugo_code_actions": true, "lugo_inlay_hints": true,
		"lugo_semantic_tokens": true, "lugo_folding_ranges": true, "lugo_selection_ranges": true,
		"lugo_code_lens": true, "lugo_document_links": true, "lugo_prepare_call_hierarchy": true,
		"lugo_diagnostics": true, "lugo_lsp_request_advanced": true, "lugo_workspace": true,
		"lugo_symbol_context": true, "lugo_workspace_status": true, "lugo_fivem_resources": true,
		"lugo_fivem_events": true, "lugo_fivem_exports": true, "lugo_fivem_contracts": true, "lugo_validate_workspace_edit": true,
		"lugo_preview_workspace_edit": true, "lugo_reindex": true,
	}
	gotTools := map[string]bool{}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		gotTools[tool.Name] = true
	}
	if len(gotTools) != len(wantTools) {
		t.Fatalf("registered tools = %d, want %d (%v)", len(gotTools), len(wantTools), gotTools)
	}
	for name := range wantTools {
		if !gotTools[name] {
			t.Errorf("missing registered tool %q", name)
		}
	}

	position := map[string]any{"path": "main.lua", "line": 1, "character": 7}
	calls := map[string]map[string]any{}
	for name := range wantTools {
		calls[name] = map[string]any{}
	}
	for _, name := range []string{"lugo_hover", "lugo_completion", "lugo_signature_help", "lugo_definition", "lugo_type_definition", "lugo_implementation", "lugo_references", "lugo_prepare_rename", "lugo_prepare_call_hierarchy"} {
		calls[name] = position
	}
	calls["lugo_document_symbols"] = map[string]any{"path": "main.lua"}
	calls["lugo_workspace_symbols"] = map[string]any{"query": "value"}
	calls["lugo_format"] = map[string]any{"path": "main.lua", "options": map[string]any{"tabSize": 4, "insertSpaces": true}}
	calls["lugo_range_format"] = map[string]any{"path": "main.lua", "line": 0, "character": 0, "endLine": 1, "endCharacter": 12, "options": map[string]any{"tabSize": 4, "insertSpaces": true}}
	calls["lugo_rename"] = map[string]any{"path": "main.lua", "line": 1, "character": 7, "newName": "renamed"}
	calls["lugo_code_actions"] = map[string]any{"path": "main.lua", "line": 0, "character": 0, "endLine": 1, "endCharacter": 12, "context": map[string]any{}}
	calls["lugo_inlay_hints"] = map[string]any{"path": "main.lua", "line": 0, "character": 0, "endLine": 1, "endCharacter": 12}
	calls["lugo_semantic_tokens"] = map[string]any{"path": "main.lua"}
	calls["lugo_folding_ranges"] = map[string]any{"path": "main.lua"}
	calls["lugo_selection_ranges"] = position
	calls["lugo_code_lens"] = map[string]any{"path": "main.lua"}
	calls["lugo_document_links"] = map[string]any{"path": "main.lua"}
	calls["lugo_diagnostics"] = map[string]any{"path": "main.lua"}
	calls["lugo_symbol_context"] = position
	calls["lugo_lsp_request_advanced"] = map[string]any{"method": "textDocument/hover", "params": map[string]any{"textDocument": map[string]any{"uri": workspace.MCPDocumentURI(filepath.Join(root, "main.lua"))}, "position": map[string]any{"line": 1, "character": 7}}}
	calls["lugo_validate_workspace_edit"] = map[string]any{"edits": []any{map[string]any{"path": "main.lua", "edits": []any{}}}}
	calls["lugo_preview_workspace_edit"] = calls["lugo_validate_workspace_edit"]
	calls["lugo_reindex"] = map[string]any{"paths": []string{"main.lua"}}

	for name, arguments := range calls {
		name, arguments := name, arguments
		t.Run("tool/"+name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || len(result.Content) == 0 || result.IsError {
				t.Fatalf("tool returned invalid result: %+v", result)
			}
		})
	}

	resources, err := session.ListResources(ctx, nil)
	if err != nil || len(resources.Resources) != 1 || resources.Resources[0].URI != "lugo://workspace/summary" {
		t.Fatalf("resources = %+v, err = %v", resources, err)
	}
	templates, err := session.ListResourceTemplates(ctx, nil)
	if err != nil || len(templates.ResourceTemplates) != 2 {
		t.Fatalf("resource templates = %+v, err = %v", templates, err)
	}
	if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "lugo://workspace/summary"}); err != nil {
		t.Fatal(err)
	}
	if result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "lugo://workspace/document/main.lua"}); err != nil || len(result.Contents) != 1 || result.Contents[0].Text == "" {
		t.Fatalf("document resource = %+v, err = %v", result, err)
	}
	summary, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "lugo_workspace", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var summaryValue struct {
		Resources []struct {
			Name string `json:"name"`
		} `json:"resources"`
	}
	structured, err := json.Marshal(summary.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(structured, &summaryValue); err != nil || len(summaryValue.Resources) == 0 {
		t.Fatalf("workspace resources = %+v, err = %v", summaryValue, err)
	}
	resourceURI := "lugo://workspace/resource/" + summaryValue.Resources[0].Name
	if result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: resourceURI}); err != nil || len(result.Contents) != 1 {
		t.Fatalf("FiveM resource = %+v, err = %v", result, err)
	}

	prompts, err := session.ListPrompts(ctx, nil)
	if err != nil || len(prompts.Prompts) != 1 || prompts.Prompts[0].Name != "lugo_fivem_review" {
		t.Fatalf("prompts = %+v, err = %v", prompts, err)
	}
	prompt, err := session.GetPrompt(ctx, &mcp.GetPromptParams{Name: "lugo_fivem_review", Arguments: map[string]string{"path": "main.lua"}})
	if err != nil || len(prompt.Messages) != 1 || !strings.Contains(prompt.Messages[0].Content.(*mcp.TextContent).Text, "lugo_diagnostics") {
		t.Fatalf("prompt = %+v, err = %v", prompt, err)
	}
}

func TestMCPFiveMContractsReturnsDeterministicStructuredConsumerResult(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"fxmanifest.lua": "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\nui_page 'web/index.html'\nfiles { 'web/app.js' }\nexport 'manifest_ping'\nserver_export 'manifest_server_ping'\n",
		"client.lua":     "RegisterNetEvent('weather:update')\nTriggerServerEvent('weather:request')\nexports('client_ping', function() end)\nRegisterNUICallback('save', function() end)\nSendNUIMessage({ action = 'open_menu' })\nSetConvar('weather_mode', 'rain')\nGetConvar('weather_mode', 'clear')\n",
		"server.lua":     "RegisterNetEvent('weather:request')\nTriggerClientEvent('weather:update', -1)\nexports('server_ping', function() end)\n",
		"web/index.html": "<script src=\"app.js\"></script>\n",
		"web/app.js":     "fetch('https://resource/save'); window.addEventListener('message', e => e.data.action === 'open_menu');\n",
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	impl := mcp.NewServer(&mcp.Implementation{Name: "lugo-mcp-contracts", Version: "1"}, nil)
	s := &server{workspace: workspace, root: root}
	s.registerTools(impl)
	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := impl.Connect(ctx, t1, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "consumer", Version: "1"}, nil).Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "lugo_fivem_contracts", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	structured, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Symbols   []struct{ Name, Kind string } `json:"symbols"`
		Links     []struct{ Confidence string } `json:"links"`
		Manifests []struct {
			Name, Value string
		} `json:"manifests"`
	}
	if err := json.Unmarshal(structured, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Symbols) == 0 || len(got.Links) == 0 || len(got.Manifests) == 0 {
		t.Fatalf("contracts = %s, want symbols, links, and manifests", structured)
	}
	if !strings.Contains(string(structured), `"weather:update"`) || !strings.Contains(string(structured), `"client_ping"`) || !strings.Contains(string(structured), `"open_menu"`) || !strings.Contains(string(structured), `"weather_mode"`) || !strings.Contains(string(structured), `"ui_page"`) {
		t.Fatalf("contracts omitted a FiveM surface: %s", structured)
	}
	if got.Links[0].Confidence != "high" {
		t.Fatalf("link confidence = %q, want high", got.Links[0].Confidence)
	}
	second, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "lugo_fivem_contracts", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	secondStructured, err := json.Marshal(second.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if string(structured) != string(secondStructured) {
		t.Fatalf("contract output is not deterministic:\nfirst: %s\nsecond: %s", structured, secondStructured)
	}
}

func TestMCPInMemoryTransportRejectsUnsafePathsAndMethods(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.lua"), []byte("return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	impl := mcp.NewServer(&mcp.Implementation{Name: "lugo-mcp-boundary", Version: "1"}, nil)
	s := &server{workspace: workspace, root: root}
	s.registerTools(impl)
	s.registerResources(impl)
	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := impl.Connect(ctx, t1, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1"}, nil).Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for name, args := range map[string]map[string]any{
		"lugo_diagnostics":          {"path": "../outside.lua"},
		"lugo_reindex":              {"paths": []string{"../outside.lua"}},
		"lugo_lsp_request_advanced": {"method": "textDocument/didOpen", "params": map[string]any{}},
	} {
		if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args}); err == nil {
			t.Errorf("unsafe request %q was accepted", name)
		}
	}
	for _, uri := range []string{"lugo://workspace/document/../outside.lua", "lugo://workspace/resource/a/b", "lugo://workspace/resource/missing"} {
		if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri}); err == nil {
			t.Errorf("unsafe resource URI %q was accepted", uri)
		}
	}
}
