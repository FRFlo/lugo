// Command lugo-mcp exposes Lugo's parser, LSP and FiveM workspace model as
// an MCP server. It is intentionally a single-workspace process: start one
// instance per project root.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coalaura/lugo/lsp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type server struct {
	workspace *lsp.Server
	root      string

	freshnessMu sync.Mutex
	freshness   workspaceFreshness
}

type workspaceFreshness struct {
	Revision   string
	SourceHash string
	IndexedAt  string
}

type reindexArgs struct {
	Paths []string `json:"paths,omitempty"`
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
	Limit    *int   `json:"limit,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
	Detail   *bool  `json:"detail,omitempty"`
}

type symbolContextArgs struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
}

type workspaceEditArgs struct {
	Edits        []workspaceFileEdits      `json:"edits"`
	Changes      map[string][]lsp.TextEdit `json:"changes,omitempty"`
	SourceHashes map[string]string         `json:"sourceHashes,omitempty"`
}

type workspaceFileEdits struct {
	Path         string         `json:"path"`
	SourceHash   string         `json:"sourceHash,omitempty"`
	ExpectedHash string         `json:"expectedHash,omitempty"`
	Hash         string         `json:"hash,omitempty"`
	Edits        []lsp.TextEdit `json:"edits"`
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
	workspaceOutputSchema := schema(`{"type":"object","required":["root","documents","resources"],"properties":{"root":{"type":"string"},"documents":{"type":"array","items":{"type":"string"}},"resources":{"type":"array","items":{"type":"object"}}}}`)
	objectOutputSchema := schema(`{"type":"object"}`)
	editSchema := schema(`{"type":"object","properties":{"edits":{"type":"array","items":{"type":"object","required":["path","edits"],"properties":{"path":{"type":"string"},"sourceHash":{"type":"string"},"expectedHash":{"type":"string"},"edits":{"type":"array"}}}},"changes":{"type":"object"},"sourceHashes":{"type":"object"}}}`)
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
		Name:         "lugo_diagnostics",
		Description:  "Compute parser, Lua, type, and FiveM diagnostics for a workspace Lua file.",
		InputSchema:  schema(`{"type":"object","required":["path"],"properties":{"path":{"type":"string"}}}`),
		OutputSchema: objectOutputSchema,
	}, s.diagnostics)
	m.AddTool(&mcp.Tool{
		Name:        "lugo_lsp_request_advanced",
		Description: "Advanced escape hatch for an arbitrary supported LSP method. Prefer the dedicated lugo_* capability tools whenever available.",
		InputSchema: schema(`{"type":"object","required":["method"],"properties":{"method":{"type":"string"},"params":{"type":"object"}}}`),
	}, s.lspRequest)
	m.AddTool(&mcp.Tool{
		Name:         "lugo_workspace",
		Description:  "Summarize indexed Lua documents and FiveM resources in the active workspace.",
		InputSchema:  schema(`{"type":"object"}`),
		OutputSchema: workspaceOutputSchema,
	}, s.workspaceSummary)
	m.AddTool(&mcp.Tool{
		Name:         "lugo_symbol_context",
		Description:  "Get the hover, definition, and references for a symbol at a position.",
		InputSchema:  schema(`{"type":"object","required":["path","line","character"],"properties":{"path":{"type":"string"},"line":{"type":"integer","minimum":0},"character":{"type":"integer","minimum":0}}}`),
		OutputSchema: schema(`{"type":"object"}`),
	}, s.symbolContext)
	m.AddTool(&mcp.Tool{
		Name:         "lugo_workspace_status",
		Description:  "Return a deterministic status summary of the indexed workspace.",
		InputSchema:  schema(`{"type":"object"}`),
		OutputSchema: schema(`{"type":"object"}`),
	}, s.workspaceStatus)
	m.AddTool(&mcp.Tool{
		Name:         "lugo_fivem_resources",
		Description:  "List FiveM resources, manifests, dependencies, profiles, and exports.",
		InputSchema:  fivemListSchema,
		OutputSchema: fivemOutputSchema,
	}, s.fivemResources)
	m.AddTool(&mcp.Tool{
		Name:         "lugo_fivem_events",
		Description:  "List registered and triggered FiveM events across resources.",
		InputSchema:  fivemListSchema,
		OutputSchema: fivemOutputSchema,
	}, s.fivemEvents)
	m.AddTool(&mcp.Tool{
		Name:         "lugo_fivem_exports",
		Description:  "List client and server FiveM exports across resources.",
		InputSchema:  fivemListSchema,
		OutputSchema: fivemOutputSchema,
	}, s.fivemExports)
	m.AddTool(&mcp.Tool{
		Name:         "lugo_validate_workspace_edit",
		Description:  "Validate a proposed workspace edit without changing files. Checks workspace-relative paths, source hashes, ranges, and overlaps.",
		InputSchema:  editSchema,
		OutputSchema: schema(`{"type":"object","required":["valid","files","errors"]}`),
	}, s.validateEdits)
	m.AddTool(&mcp.Tool{
		Name:         "lugo_preview_workspace_edit",
		Description:  "Preview a proposed workspace edit without changing files. Returns the resulting source only when validation succeeds.",
		InputSchema:  editSchema,
		OutputSchema: schema(`{"type":"object","required":["valid","files","errors"]}`),
	}, s.validateEdits)
	m.AddTool(&mcp.Tool{
		Name:         "lugo_reindex",
		Description:  "Reindex the active workspace, optionally requesting specific workspace-relative paths.",
		InputSchema:  schema(`{"type":"object","properties":{"paths":{"type":"array","items":{"type":"string"}}}}`),
		OutputSchema: schema(`{"type":"object","required":["revision","sourceHash","indexedAt","paths"]}`),
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
	m.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "lugo://workspace/document/{+path}",
		Name:        "Workspace document",
		Description: "Read a workspace-relative Lua document as text.",
		MIMEType:    "text/plain",
	}, s.documentResource)
	m.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "lugo://workspace/resource/{name}",
		Name:        "FiveM resource metadata",
		Description: "Read metadata for a named FiveM resource as JSON.",
		MIMEType:    "application/json",
	}, s.fivemResource)
}

func (s *server) documentResource(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	const prefix = "lugo://workspace/document/"
	if !strings.HasPrefix(req.Params.URI, prefix) {
		return nil, fmt.Errorf("invalid document resource URI")
	}
	path, err := url.PathUnescape(strings.TrimPrefix(req.Params.URI, prefix))
	if err != nil {
		return nil, fmt.Errorf("invalid document resource URI: %w", err)
	}
	file, err := s.safePath(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "text/plain", Text: string(data)}}}, nil
}

func (s *server) fivemResource(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	const prefix = "lugo://workspace/resource/"
	if !strings.HasPrefix(req.Params.URI, prefix) {
		return nil, fmt.Errorf("invalid resource URI")
	}
	name, err := url.PathUnescape(strings.TrimPrefix(req.Params.URI, prefix))
	if err != nil || name == "" || strings.Contains(name, "/") {
		return nil, fmt.Errorf("invalid resource URI")
	}
	result, err := s.summary()
	if err != nil {
		return nil, err
	}
	for _, resource := range result["resources"].([]map[string]any) {
		if resource["name"] != name {
			continue
		}
		data, err := json.MarshalIndent(resource, "", "  ")
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)}}}, nil
	}
	return nil, fmt.Errorf("FiveM resource %q not found", name)
}

func (s *server) registerPrompts(m *mcp.Server) {
	m.AddPrompt(&mcp.Prompt{
		Name:        "lugo_fivem_review",
		Description: "Review a FiveM Lua change using Lugo diagnostics and resource metadata.",
		Arguments:   []*mcp.PromptArgument{{Name: "path", Description: "Workspace-relative Lua file to review", Required: true}},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		path := req.Params.Arguments["path"]
		return &mcp.GetPromptResult{Description: "Lugo FiveM review", Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: fmt.Sprintf("Review %s using lugo_diagnostics, lugo_hover, lugo_definition, lugo_references, and lugo_workspace. Report actionable issues and preserve existing behavior.", path)}}}}, nil
	})
}

func schema(value string) json.RawMessage { return json.RawMessage(value) }

var fivemListSchema = schema(`{"type":"object","properties":{"resource":{"type":"string"},"limit":{"type":"integer","minimum":1},"cursor":{"type":"string"},"detail":{"type":"boolean"}}}`)
var fivemOutputSchema = schema(`{"oneOf":[{"type":"array","items":{"type":"object"}},{"type":"object","required":["items","nextCursor","truncated","total"],"properties":{"items":{"type":"array","items":{"type":"object"}},"nextCursor":{"type":"string"},"truncated":{"type":"boolean"},"hasMore":{"type":"boolean"},"total":{"type":"integer"}}}]}`)

var advancedReadOnlyMethods = map[string]bool{
	"textDocument/hover": true, "textDocument/completion": true, "textDocument/signatureHelp": true,
	"textDocument/definition": true, "textDocument/typeDefinition": true, "textDocument/implementation": true,
	"textDocument/references": true, "textDocument/documentSymbol": true, "workspace/symbol": true,
	"textDocument/inlayHint": true, "textDocument/semanticTokens/full": true, "textDocument/foldingRange": true,
	"textDocument/selectionRange": true, "textDocument/codeLens": true, "textDocument/documentLink": true,
	"textDocument/prepareCallHierarchy": true,
}

func (s *server) lspRequest(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args toolArgs
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, err
	}
	if args.Method == "" {
		return nil, fmt.Errorf("method is required")
	}
	if !advancedReadOnlyMethods[args.Method] {
		return nil, fmt.Errorf("advanced request method %q is not supported; only read-only methods are allowed", args.Method)
	}
	if err := s.validateAdvancedParams(args.Params); err != nil {
		return nil, err
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
		case "textDocument/formatting":
			params["options"] = args.Options
		case "textDocument/rangeFormatting":
			params["range"] = map[string]any{"start": map[string]int{"line": args.Line, "character": args.Character}, "end": map[string]int{"line": args.EndLine, "character": args.EndCharacter}}
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
	return structuredResult(string(result)), nil
}

func (s *server) reindex(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args reindexArgs
	arguments := json.RawMessage(`{}`)
	if req != nil && req.Params != nil && len(req.Params.Arguments) != 0 {
		arguments = req.Params.Arguments
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(args.Paths))
	for _, name := range args.Paths {
		if _, err := s.safePath(name); err != nil {
			return nil, err
		}
		paths = append(paths, filepath.ToSlash(filepath.Clean(name)))
	}
	// The LSP workspace currently exposes an atomic workspace refresh. Keep the
	// requested paths in the response so clients can use the same contract for
	// selective refreshes without making this operation mutate source files.
	if _, err := s.workspace.MCPRequest("lugo/reindex", json.RawMessage(`{}`)); err != nil {
		return nil, err
	}
	freshness, err := s.captureFreshness()
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	result := map[string]any{
		"revision": freshness.Revision, "sourceHash": freshness.SourceHash,
		"indexedSourceHash": freshness.Revision, "currentSourceHash": freshness.SourceHash,
		"indexedAt": freshness.IndexedAt, "lastIndexedAt": freshness.IndexedAt, "paths": paths,
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return structuredResult(string(data)), nil
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
	return structuredResult(string(data)), nil
}

func (s *server) symbolContext(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args symbolContextArgs
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, err
	}
	path, err := s.safePath(args.Path)
	if err != nil {
		return nil, err
	}
	uri := s.workspace.MCPDocumentURI(path)
	position := map[string]int{"line": args.Line, "character": args.Character}
	request := func(method string) (json.RawMessage, error) {
		params := map[string]any{"textDocument": map[string]string{"uri": uri}, "position": position}
		if method == "textDocument/references" {
			params["context"] = map[string]bool{"includeDeclaration": true}
		}
		data, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		return s.workspace.MCPRequest(method, data)
	}
	hover, err := request("textDocument/hover")
	if err != nil {
		return nil, err
	}
	definition, err := request("textDocument/definition")
	if err != nil {
		return nil, err
	}
	references, err := request("textDocument/references")
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"path":       args.Path,
		"position":   position,
		"hover":      json.RawMessage(hover),
		"definition": json.RawMessage(definition),
		"references": json.RawMessage(references),
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return structuredResult(string(data)), nil
}

func (s *server) workspaceStatus(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	summary, err := s.summary()
	if err != nil {
		return nil, err
	}
	documents := make([]string, 0, len(summary["documents"].([]string)))
	for _, uri := range summary["documents"].([]string) {
		parsed, err := url.Parse(uri)
		if err != nil {
			return nil, fmt.Errorf("invalid indexed document URI: %s", uri)
		}
		if parsed.Scheme == "" {
			// Embedded standard-library documents are represented by virtual,
			// workspace-relative names rather than file URIs.
			rel := filepath.Clean(uri)
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				return nil, fmt.Errorf("indexed document escapes workspace root: %s", uri)
			}
			documents = append(documents, filepath.ToSlash(rel))
			continue
		}
		if parsed.Scheme != "file" {
			return nil, fmt.Errorf("invalid indexed document URI: %s", uri)
		}
		path := filepath.FromSlash(parsed.Path)
		if len(path) >= 3 && path[0] == filepath.Separator && path[2] == ':' {
			path = path[1:]
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("indexed document escapes workspace root: %s", uri)
		}
		documents = append(documents, filepath.ToSlash(rel))
	}
	sort.Strings(documents)
	resources := summary["resources"].([]map[string]any)
	freshness, err := s.currentFreshness()
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"root":              s.root,
		"documentCount":     len(documents),
		"documents":         documents,
		"resourceCount":     len(resources),
		"revision":          freshness.Revision,
		"indexedRevision":   freshness.Revision,
		"sourceHash":        freshness.SourceHash,
		"indexedSourceHash": freshness.Revision,
		"currentSourceHash": freshness.SourceHash,
		"indexedAt":         freshness.IndexedAt,
		"lastIndexedAt":     freshness.IndexedAt,
		"fresh":             freshness.Revision == freshness.SourceHash,
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return structuredResult(string(data)), nil
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
	items := result["resources"].([]map[string]any)
	data, err := marshalFivemList(items, args, func(item map[string]any) map[string]any {
		return map[string]any{"name": item["name"]}
	})
	if err != nil {
		return nil, err
	}
	return structuredResult(string(data)), nil
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
	data, err := marshalFivemList(result, args, func(item map[string]any) map[string]any {
		return map[string]any{"resource": item["resource"], "name": item["name"], "kind": item["kind"]}
	})
	if err != nil {
		return nil, err
	}
	return structuredResult(string(data)), nil
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
	data, err := marshalFivemList(result, args, func(item map[string]any) map[string]any {
		return map[string]any{"resource": item["resource"]}
	})
	if err != nil {
		return nil, err
	}
	return structuredResult(string(data)), nil
}

func marshalFivemList(items []map[string]any, args fivemArgs, compact func(map[string]any) map[string]any) ([]byte, error) {
	// Preserve the original array contract when pagination/detail was not requested.
	if args.Limit == nil && args.Cursor == "" && args.Detail == nil {
		return json.MarshalIndent(items, "", "  ")
	}
	start := 0
	if args.Cursor != "" {
		if _, err := fmt.Sscanf(args.Cursor, "%d", &start); err != nil || start < 0 {
			return nil, fmt.Errorf("cursor must be a non-negative integer")
		}
	}
	limit := len(items)
	if args.Limit != nil {
		if *args.Limit < 1 {
			return nil, fmt.Errorf("limit must be at least 1")
		}
		limit = *args.Limit
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := make([]map[string]any, end-start)
	copy(page, items[start:end])
	if args.Detail != nil && !*args.Detail {
		for i, item := range page {
			page[i] = compact(item)
		}
	}
	next := ""
	if end < len(items) {
		next = fmt.Sprintf("%d", end)
	}
	return json.MarshalIndent(map[string]any{
		"items": page, "nextCursor": next, "truncated": end < len(items),
		"hasMore": end < len(items), "total": len(items),
	}, "", "  ")
}

func (s *server) currentFreshness() (workspaceFreshness, error) {
	s.freshnessMu.Lock()
	defer s.freshnessMu.Unlock()
	current, err := workspaceSourceHash(s.root)
	if err != nil {
		return workspaceFreshness{}, err
	}
	if s.freshness.IndexedAt == "" {
		s.freshness = workspaceFreshness{Revision: current, SourceHash: current, IndexedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	}
	return workspaceFreshness{Revision: s.freshness.Revision, SourceHash: current, IndexedAt: s.freshness.IndexedAt}, nil
}

func (s *server) captureFreshness() (workspaceFreshness, error) {
	hash, err := workspaceSourceHash(s.root)
	if err != nil {
		return workspaceFreshness{}, err
	}
	s.freshnessMu.Lock()
	s.freshness = workspaceFreshness{Revision: hash, SourceHash: hash, IndexedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	result := s.freshness
	s.freshnessMu.Unlock()
	return result, nil
}

func workspaceSourceHash(root string) (string, error) {
	var entries []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".lua") {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			entries = append(entries, filepath.ToSlash(rel)+"\x00"+sourceHash(data))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(entries)
	return sourceHash([]byte(strings.Join(entries, "\n"))), nil
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

func (s *server) validateAdvancedParams(params map[string]any) error {
	var validate func(any) error
	validate = func(value any) error {
		switch value := value.(type) {
		case map[string]any:
			for key, nested := range value {
				if text, ok := nested.(string); ok {
					switch key {
					case "path":
						if _, err := s.safePath(text); err != nil {
							return fmt.Errorf("unsafe path parameter: %w", err)
						}
					case "uri":
						if err := s.validateAdvancedURI(text); err != nil {
							return err
						}
					}
				}
				if err := validate(nested); err != nil {
					return err
				}
			}
		case []any:
			for _, nested := range value {
				if err := validate(nested); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return validate(params)
}

func (s *server) validateAdvancedURI(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "file" || parsed.Host != "" {
		return fmt.Errorf("path-bearing URI must be a local file in the workspace")
	}
	path := filepath.FromSlash(parsed.Path)
	if len(path) >= 3 && path[0] == filepath.Separator && path[2] == ':' {
		path = path[1:]
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("unsafe path parameter: %w", err)
	}
	rel, err := filepath.Rel(s.root, filepath.Clean(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes workspace root")
	}
	return nil
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

// validateEdits is deliberately read-only. It validates and applies proposed
// edits to an in-memory copy solely to produce a preview; callers must apply
// the returned edit themselves after any required confirmation.
func (s *server) validateEdits(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args workspaceEditArgs
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, err
	}
	files := append([]workspaceFileEdits(nil), args.Edits...)
	for path, edits := range args.Changes {
		files = append(files, workspaceFileEdits{Path: path, Edits: edits, SourceHash: args.SourceHashes[path]})
	}
	result := map[string]any{"valid": true, "files": []any{}, "errors": []string{}}
	fileResults := result["files"].([]any)
	errorsFound := result["errors"].([]string)
	if len(files) == 0 {
		errorsFound = append(errorsFound, "at least one edit is required")
	}
	seen := map[string]bool{}
	for _, file := range files {
		entry := map[string]any{"path": file.Path, "valid": false}
		fileResults = append(fileResults, entry)
		path, err := s.safePath(file.Path)
		if err != nil {
			errorsFound = append(errorsFound, fmt.Sprintf("%s: %v", file.Path, err))
			continue
		}
		if seen[path] {
			errorsFound = append(errorsFound, fmt.Sprintf("%s: duplicate edit target", file.Path))
			continue
		}
		seen[path] = true
		source, err := os.ReadFile(path)
		if err != nil {
			errorsFound = append(errorsFound, fmt.Sprintf("%s: %v", file.Path, err))
			continue
		}
		hash := sourceHash(source)
		entry["sourceHash"] = hash
		expected := file.SourceHash
		if expected == "" {
			expected = file.ExpectedHash
		}
		if expected == "" {
			expected = file.Hash
		}
		if expected != "" && expected != hash {
			errorsFound = append(errorsFound, fmt.Sprintf("%s: source is stale (expected hash %s, current hash %s)", file.Path, expected, hash))
			continue
		}
		type locatedEdit struct {
			edit       lsp.TextEdit
			start, end int
		}
		located := make([]locatedEdit, 0, len(file.Edits))
		for _, edit := range file.Edits {
			start, startErr := sourceOffset(source, edit.Range.Start)
			end, endErr := sourceOffset(source, edit.Range.End)
			if startErr != nil || endErr != nil {
				errText := startErr
				if errText == nil {
					errText = endErr
				}
				errorsFound = append(errorsFound, fmt.Sprintf("%s: invalid edit range: %v", file.Path, errText))
				located = nil
				break
			}
			if end < start {
				errorsFound = append(errorsFound, fmt.Sprintf("%s: edit range ends before it starts", file.Path))
				located = nil
				break
			}
			located = append(located, locatedEdit{edit: edit, start: start, end: end})
		}
		if located == nil && len(file.Edits) != 0 {
			continue
		}
		for i := 0; i < len(located); i++ {
			for j := i + 1; j < len(located); j++ {
				if located[j].start < located[i].start {
					located[i], located[j] = located[j], located[i]
				}
			}
		}
		for i := 1; i < len(located); i++ {
			if located[i].start < located[i-1].end {
				errorsFound = append(errorsFound, fmt.Sprintf("%s: overlapping edits", file.Path))
				located = nil
				break
			}
		}
		if located == nil {
			continue
		}
		preview := append([]byte(nil), source...)
		for i := len(located) - 1; i >= 0; i-- {
			e := located[i]
			preview = append(preview[:e.start], append([]byte(e.edit.NewText), preview[e.end:]...)...)
		}
		entry["valid"] = true
		entry["preview"] = string(preview)
		entry["editCount"] = len(located)
	}
	result["files"] = fileResults
	result["errors"] = errorsFound
	result["valid"] = len(errorsFound) == 0
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return structuredResult(string(data)), nil
}

func sourceHash(source []byte) string {
	hash := sha256.Sum256(source)
	return hex.EncodeToString(hash[:])
}

func sourceOffset(source []byte, position lsp.Position) (int, error) {
	line := uint32(0)
	start := 0
	for start < len(source) && line < position.Line {
		if source[start] == '\n' {
			line++
		}
		start++
	}
	if line != position.Line {
		return 0, fmt.Errorf("line %d is outside source", position.Line)
	}
	end := start
	for end < len(source) && source[end] != '\n' {
		end++
	}
	units := uint32(0)
	for offset := start; offset < end; {
		runeValue, size := utf8.DecodeRune(source[offset:end])
		if runeValue == utf8.RuneError && size == 1 {
			return 0, fmt.Errorf("invalid UTF-8 source")
		}
		if units == position.Character {
			return offset, nil
		}
		units += 1
		if runeValue > 0xffff {
			units++
		}
		offset += size
	}
	if units == position.Character {
		return end, nil
	}
	return 0, fmt.Errorf("character %d is outside line %d", position.Character, position.Line)
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// structuredResult keeps the existing text block for clients that only support
// TextContent while also exposing the same JSON value through MCP's structured
// output field. All callers pass JSON produced by the workspace model.
func structuredResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: text}},
		StructuredContent: json.RawMessage(text),
	}
}
