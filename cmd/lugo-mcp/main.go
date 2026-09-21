// Command lugo-mcp exposes Lugo's parser, LSP and FiveM workspace model as
// an MCP server. It is intentionally a single-workspace process: start one
// instance per project root.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/coalaura/lugo/lsp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type server struct {
	workspace *lsp.Server
	root      string
}

type toolArgs struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

type fileArgs struct {
	Path string `json:"path"`
}

type capabilityArgs struct {
	Path               string         `json:"path,omitempty"`
	Line               int            `json:"line,omitempty"`
	Character          int            `json:"character,omitempty"`
	EndLine            int            `json:"endLine,omitempty"`
	EndCharacter       int            `json:"endCharacter,omitempty"`
	Query              string         `json:"query,omitempty"`
	NewName            string         `json:"newName,omitempty"`
	IncludeDeclaration bool           `json:"includeDeclaration,omitempty"`
	Options            map[string]any `json:"options,omitempty"`
	Context            map[string]any `json:"context,omitempty"`
}

type fivemArgs struct {
	Resource string `json:"resource,omitempty"`
	Profile  string `json:"profile,omitempty"`
}

func main() {
	log.SetOutput(os.Stderr)
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		log.Fatal(err)
	}
	workspace, err := lsp.NewMCPWorkspace(abs)
	if err != nil {
		log.Fatal(err)
	}

	s := &server{workspace: workspace, root: abs}
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "lugo-mcp", Version: "0.1.0"}, nil)
	s.registerTools(mcpServer)
	s.registerResources(mcpServer)
	s.registerPrompts(mcpServer)
	if err := mcpServer.Run(context.Background(), &mcp.StdioTransport{}); err != nil && !errors.Is(err, io.EOF) {
		log.Fatal(err)
	}
}

func (s *server) registerTools(m *mcp.Server) {
	fileSchema := schema(`{"type":"object","required":["path"],"properties":{"path":{"type":"string"},"options":{"type":"object"}}}`)
	positionSchema := schema(`{"type":"object","required":["path","line","character"],"properties":{"path":{"type":"string"},"line":{"type":"integer","minimum":0},"character":{"type":"integer","minimum":0},"includeDeclaration":{"type":"boolean"}}}`)
	rangeSchema := schema(`{"type":"object","required":["path","line","character","endLine","endCharacter"],"properties":{"path":{"type":"string"},"line":{"type":"integer","minimum":0},"character":{"type":"integer","minimum":0},"endLine":{"type":"integer","minimum":0},"endCharacter":{"type":"integer","minimum":0},"options":{"type":"object"},"context":{"type":"object"}}}`)
	workspaceSchema := schema(`{"type":"object","required":["query"],"properties":{"query":{"type":"string"}}}`)
	renameSchema := schema(`{"type":"object","required":["path","line","character","newName"],"properties":{"path":{"type":"string"},"line":{"type":"integer","minimum":0},"character":{"type":"integer","minimum":0},"newName":{"type":"string"}}}`)
	for _, capability := range []struct {
		name, method, description string
		input                     json.RawMessage
	}{
		{"lugo_hover", "textDocument/hover", "Get hover documentation and inferred type at a Lua position.", positionSchema},
		{"lugo_completion", "textDocument/completion", "Get context-aware Lua and FiveM completions at a position.", positionSchema},
		{"lugo_signature_help", "textDocument/signatureHelp", "Get function signature help at a call position.", positionSchema},
		{"lugo_definition", "textDocument/definition", "Find the definition at a Lua position.", positionSchema},
		{"lugo_type_definition", "textDocument/typeDefinition", "Find the type definition at a Lua position.", positionSchema},
		{"lugo_implementation", "textDocument/implementation", "Find implementations at a Lua position.", positionSchema},
		{"lugo_references", "textDocument/references", "Find references to the symbol at a Lua position.", positionSchema},
		{"lugo_document_symbols", "textDocument/documentSymbol", "List symbols declared in a Lua document.", fileSchema},
		{"lugo_workspace_symbols", "workspace/symbol", "Search symbols across the indexed workspace.", workspaceSchema},
		{"lugo_format", "textDocument/formatting", "Format a Lua document.", fileSchema},
		{"lugo_range_format", "textDocument/rangeFormatting", "Format a selected Lua range.", rangeSchema},
		{"lugo_rename", "textDocument/rename", "Prepare and apply a symbol rename as a WorkspaceEdit.", renameSchema},
		{"lugo_prepare_rename", "textDocument/prepareRename", "Check whether a symbol can be renamed and get its range.", positionSchema},
		{"lugo_code_actions", "textDocument/codeAction", "Get safe fixes and refactoring code actions.", rangeSchema},
		{"lugo_inlay_hints", "textDocument/inlayHint", "Get parameter names and inferred type inlay hints.", rangeSchema},
		{"lugo_semantic_tokens", "textDocument/semanticTokens/full", "Get semantic tokens for a Lua document.", fileSchema},
		{"lugo_folding_ranges", "textDocument/foldingRange", "Get foldable Lua and comment ranges.", fileSchema},
		{"lugo_selection_ranges", "textDocument/selectionRange", "Get syntax-aware selection expansion ranges.", positionSchema},
		{"lugo_code_lens", "textDocument/codeLens", "Get actionable code lenses.", fileSchema},
		{"lugo_document_links", "textDocument/documentLink", "Get links discovered in a Lua document.", fileSchema},
		{"lugo_prepare_call_hierarchy", "textDocument/prepareCallHierarchy", "Prepare call hierarchy items at a Lua position.", positionSchema},
	} {
		m.AddTool(&mcp.Tool{Name: capability.name, Description: capability.description, InputSchema: capability.input}, s.capability(capability.method))
	}
	m.AddTool(&mcp.Tool{
		Name:        "lugo_diagnostics",
		Description: "Compute parser, Lua, type, and FiveM diagnostics for a workspace Lua file.",
		InputSchema: schema(`{"type":"object","required":["path"],"properties":{"path":{"type":"string"}}}`),
	}, s.diagnostics)
	m.AddTool(&mcp.Tool{
		Name:        "lugo_lsp_request_advanced",
		Description: "Advanced escape hatch for an arbitrary supported LSP method. Prefer the dedicated lugo_* capability tools whenever available.",
		InputSchema: schema(`{"type":"object","required":["method"],"properties":{"method":{"type":"string"},"params":{"type":"object"}}}`),
	}, s.lspRequest)
	m.AddTool(&mcp.Tool{
		Name:        "lugo_workspace",
		Description: "Summarize indexed Lua documents and FiveM resources in the active workspace.",
		InputSchema: schema(`{"type":"object"}`),
	}, s.workspaceSummary)
	m.AddTool(&mcp.Tool{
		Name:        "lugo_fivem_resources",
		Description: "List FiveM resources, manifests, dependencies, profiles, and exports.",
		InputSchema: schema(`{"type":"object","properties":{"resource":{"type":"string"}}}`),
	}, s.fivemResources)
	m.AddTool(&mcp.Tool{
		Name:        "lugo_fivem_events",
		Description: "List registered and triggered FiveM events across resources.",
		InputSchema: schema(`{"type":"object","properties":{"resource":{"type":"string"}}}`),
	}, s.fivemEvents)
	m.AddTool(&mcp.Tool{
		Name:        "lugo_fivem_exports",
		Description: "List client and server FiveM exports across resources.",
		InputSchema: schema(`{"type":"object","properties":{"resource":{"type":"string"}}}`),
	}, s.fivemExports)
	m.AddTool(&mcp.Tool{
		Name:        "lugo_reindex",
		Description: "Reindex the active workspace and refresh diagnostics and FiveM resource metadata.",
		InputSchema: schema(`{"type":"object"}`),
	}, s.reindex)
}

func (s *server) registerResources(m *mcp.Server) {
	m.AddResource(&mcp.Resource{
		URI:         "lugo://workspace/summary",
		Name:        "Workspace summary",
		Description: "Current indexed Lua documents and FiveM resources.",
		MIMEType:    "application/json",
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		result, err := s.summary()
		if err != nil {
			return nil, err
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: "lugo://workspace/summary", MIMEType: "application/json", Text: string(data)}}}, nil
	})
}

func (s *server) registerPrompts(m *mcp.Server) {
	m.AddPrompt(&mcp.Prompt{
		Name:        "lugo_fivem_review",
		Description: "Review a FiveM Lua change using Lugo diagnostics and resource metadata.",
		Arguments:   []*mcp.PromptArgument{{Name: "path", Description: "Workspace-relative Lua file to review", Required: true}},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		path := req.Params.Arguments["path"]
		return &mcp.GetPromptResult{Description: "Lugo FiveM review", Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: fmt.Sprintf("Review %s with lugo_lsp_request using textDocument/diagnostic, textDocument/hover, textDocument/definition, textDocument/references, and the FiveM workspace summary. Report actionable issues and preserve existing behavior.", path)}}}}, nil
	})
}

func schema(value string) json.RawMessage { return json.RawMessage(value) }

func (s *server) lspRequest(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args toolArgs
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, err
	}
	if args.Method == "" {
		return nil, fmt.Errorf("method is required")
	}
	params, err := json.Marshal(args.Params)
	if err != nil {
		return nil, err
	}
	result, err := s.workspace.MCPRequest(args.Method, params)
	if err != nil {
		return nil, err
	}
	return textResult(string(result)), nil
}

func (s *server) capability(method string) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args capabilityArgs
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
		params := map[string]any{}
		if method == "workspace/symbol" {
			params["query"] = args.Query
		} else {
			path, err := s.safePath(args.Path)
			if err != nil {
				return nil, err
			}
			uri := s.workspace.MCPDocumentURI(path)
			params["textDocument"] = map[string]string{"uri": uri}
			params["position"] = map[string]int{"line": args.Line, "character": args.Character}
		}
		switch method {
		case "textDocument/references":
			params["context"] = map[string]bool{"includeDeclaration": args.IncludeDeclaration}
		case "textDocument/rename":
			if args.NewName == "" {
				return nil, fmt.Errorf("newName is required for rename")
			}
			params["newName"] = args.NewName
		case "textDocument/inlayHint", "textDocument/semanticTokens/range":
			params["range"] = map[string]any{"start": map[string]int{"line": args.Line, "character": args.Character}, "end": map[string]int{"line": args.EndLine, "character": args.EndCharacter}}
		case "textDocument/formatting", "textDocument/rangeFormatting":
			params["options"] = args.Options
		case "textDocument/codeAction":
			params["range"] = map[string]any{"start": map[string]int{"line": args.Line, "character": args.Character}, "end": map[string]int{"line": args.EndLine, "character": args.EndCharacter}}
			params["context"] = args.Context
		case "textDocument/selectionRange":
			params["positions"] = []map[string]int{{"line": args.Line, "character": args.Character}}
		}
		data, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		result, err := s.workspace.MCPRequest(method, data)
		if err != nil {
			return nil, err
		}
		return textResult(string(result)), nil
	}
}

func (s *server) diagnostics(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args fileArgs
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, err
	}
	path, err := s.safePath(args.Path)
	if err != nil {
		return nil, err
	}
	result, err := s.workspace.MCPDiagnostics(s.workspace.MCPDocumentURI(path))
	if err != nil {
		return nil, err
	}
	return textResult(string(result)), nil
}

func (s *server) reindex(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := s.workspace.MCPRequest("lugo/reindex", json.RawMessage(`{}`))
	if err != nil {
		return nil, err
	}
	return textResult(string(result)), nil
}

func (s *server) workspaceSummary(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := s.summary()
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return textResult(string(data)), nil
}

func (s *server) fivemResources(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args fivemArgs
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, err
	}
	result, err := s.summary()
	if err != nil {
		return nil, err
	}
	if args.Resource != "" {
		filtered := make([]map[string]any, 0, 1)
		for _, resource := range result["resources"].([]map[string]any) {
			if resource["name"] == args.Resource {
				filtered = append(filtered, resource)
			}
		}
		result["resources"] = filtered
	}
	data, err := json.MarshalIndent(result["resources"], "", "  ")
	if err != nil {
		return nil, err
	}
	return textResult(string(data)), nil
}

func (s *server) fivemEvents(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args fivemArgs
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0)
	if graph := s.workspace.FiveMResourceGraph; graph != nil {
		for name, node := range graph.ByName {
			if args.Resource != "" && args.Resource != name || node == nil {
				continue
			}
			events := append(append(append([]lsp.FiveMEventRef{}, node.ServerHandlers...), node.ClientHandlers...), node.SharedHandlers...)
			events = append(events, node.ServerTriggers...)
			events = append(events, node.ClientTriggers...)
			events = append(events, node.SharedTriggers...)
			for _, event := range events {
				result = append(result, map[string]any{"resource": name, "name": event.Name, "uri": event.URI, "kind": event.Kind})
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i]["resource"].(string)+result[i]["name"].(string) < result[j]["resource"].(string)+result[j]["name"].(string)
	})
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return textResult(string(data)), nil
}

func (s *server) fivemExports(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args fivemArgs
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0)
	if graph := s.workspace.FiveMResourceGraph; graph != nil {
		for name, node := range graph.ByName {
			if args.Resource != "" && args.Resource != name || node == nil || node.Resource == nil {
				continue
			}
			result = append(result, map[string]any{"resource": name, "client": node.Resource.ClientExports, "server": node.Resource.ServerExports})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i]["resource"].(string) < result[j]["resource"].(string) })
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return textResult(string(data)), nil
}

func (s *server) summary() (map[string]any, error) {
	documents := make([]string, 0, len(s.workspace.Documents))
	for uri := range s.workspace.Documents {
		documents = append(documents, uri)
	}
	sort.Strings(documents)
	resources := make([]map[string]any, 0)
	if graph := s.workspace.FiveMResourceGraph; graph != nil {
		for name, node := range graph.ByName {
			if node == nil || node.Resource == nil {
				continue
			}
			resources = append(resources, map[string]any{"name": name, "root": node.RootURI, "dependencies": node.Dependencies, "provides": node.Provides, "clientExports": node.Resource.ClientExports, "serverExports": node.Resource.ServerExports})
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i]["name"].(string) < resources[j]["name"].(string) })
	return map[string]any{"root": s.root, "documents": documents, "resources": resources}, nil
}

func (s *server) safePath(name string) (string, error) {
	if filepath.IsAbs(name) || strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("path must be a non-empty workspace-relative path")
	}
	path := filepath.Join(s.root, filepath.Clean(name))
	rel, err := filepath.Rel(s.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace root")
	}
	return path, nil
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}
