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

	"github.com/FRFlo/lugo/lsp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type server struct {
	workspace *lsp.Server
	root      string

	// workspaceMu protects direct reads of LSP workspace maps while reindexing
	// replaces the index. The LSP serializes requests internally, but summary
	// reads those maps directly.
	workspaceMu sync.RWMutex
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

type fiveMContractOutput struct {
	Symbols   []fiveMContractSymbolOutput   `json:"symbols"`
	Links     []fiveMContractLinkOutput     `json:"links"`
	Manifests []fiveMContractManifestOutput `json:"manifests"`
}

type fiveMContractSymbolOutput struct {
	Name      string       `json:"name"`
	Kind      string       `json:"kind"`
	Location  lsp.Location `json:"location"`
	Profile   string       `json:"profile"`
	Direction string       `json:"direction"`
}

type fiveMContractLinkOutput struct {
	From       fiveMContractSymbolOutput `json:"from"`
	To         fiveMContractSymbolOutput `json:"to"`
	Confidence string                    `json:"confidence"`
}

type fiveMContractManifestOutput struct {
	Name     string       `json:"name"`
	Value    string       `json:"value"`
	Location lsp.Location `json:"location"`
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

type locatedEdit struct {
	newText    string
	start, end int
	index      int
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

	s, err := newServer(workspace, abs)
	if err != nil {
		log.Fatal(err)
	}
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
		Name:         "lugo_fivem_contracts",
		Description:  "Return deterministic, read-only FiveM event, export, NUI, convar, and manifest contracts with source locations.",
		InputSchema:  schema(`{"type":"object"}`),
		OutputSchema: schema(`{"type":"object","required":["symbols","links","manifests"],"properties":{"symbols":{"type":"array","items":{"type":"object"}},"links":{"type":"array","items":{"type":"object"}},"manifests":{"type":"array","items":{"type":"object"}}}}`),
	}, s.fivemContracts)
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
	s.workspaceMu.RLock()
	result, err := s.workspace.MCPDiagnostics(s.workspace.MCPDocumentURI(path))
	s.workspaceMu.RUnlock()
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
	s.workspaceMu.Lock()
	_, err := s.workspace.MCPRequest("lugo/reindex", json.RawMessage(`{}`))
	s.workspaceMu.Unlock()
	if err != nil {
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
		for _, key := range []string{"resource", "name", "kind", "uri"} {
			left, right := fmt.Sprint(result[i][key]), fmt.Sprint(result[j][key])
			if left != right {
				return left < right
			}
		}
		return false
	})
	data, err := marshalFivemList(result, args, func(item map[string]any) map[string]any {
		return map[string]any{"resource": item["resource"], "name": item["name"], "kind": item["kind"]}
	})
	if err != nil {
		return nil, err
	}
	return structuredResult(string(data)), nil
}

func (s *server) fivemContracts(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	snapshot := s.workspace.MCPFiveMContracts()
	result := fiveMContractOutput{
		Symbols:   make([]fiveMContractSymbolOutput, len(snapshot.Symbols)),
		Links:     make([]fiveMContractLinkOutput, len(snapshot.Links)),
		Manifests: make([]fiveMContractManifestOutput, len(snapshot.Manifests)),
	}
	for i, symbol := range snapshot.Symbols {
		result.Symbols[i] = contractSymbolOutput(symbol)
	}
	for i, link := range snapshot.Links {
		result.Links[i] = fiveMContractLinkOutput{From: contractSymbolOutput(link.From), To: contractSymbolOutput(link.To), Confidence: contractConfidence(link.Confidence)}
	}
	for i, manifest := range snapshot.Manifests {
		result.Manifests[i] = fiveMContractManifestOutput{Name: manifest.Name, Value: manifest.Value, Location: lsp.Location{URI: manifest.Location.URI, Range: manifest.Location.Range}}
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return structuredResult(string(data)), nil
}

func contractSymbolOutput(symbol lsp.FiveMContractSymbol) fiveMContractSymbolOutput {
	return fiveMContractSymbolOutput{Name: symbol.Name, Kind: string(symbol.Kind), Location: lsp.Location{URI: symbol.Location.URI, Range: symbol.Location.Range}, Profile: symbol.Profile.String(), Direction: string(symbol.Direction)}
}

func contractConfidence(confidence lsp.FiveMContractConfidence) string {
	switch confidence {
	case lsp.FiveMContractConfidenceLow:
		return "low"
	case lsp.FiveMContractConfidenceMedium:
		return "medium"
	case lsp.FiveMContractConfidenceHigh:
		return "high"
	default:
		return "unknown"
	}
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

// newServer records the source revision that was indexed by NewMCPWorkspace.
// Status queries must never establish the indexed revision: by then files may
// already have changed on disk.
func newServer(workspace *lsp.Server, root string) (*server, error) {
	s := &server{workspace: workspace, root: root}
	if _, err := s.captureFreshness(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *server) currentFreshness() (workspaceFreshness, error) {
	// Hashing walks and reads workspace files, so do it before taking the
	// freshness mutex. This keeps status snapshots responsive during slow I/O.
	current, err := workspaceSourceHash(s.root)
	if err != nil {
		return workspaceFreshness{}, err
	}
	s.freshnessMu.Lock()
	if s.freshness.IndexedAt == "" {
		// Keep direct server construction backward compatible for embedders. The
		// command path always uses newServer, which captures at creation time.
		s.freshness = workspaceFreshness{Revision: current, IndexedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	}
	indexed := s.freshness
	s.freshnessMu.Unlock()
	return workspaceFreshness{Revision: indexed.Revision, SourceHash: current, IndexedAt: indexed.IndexedAt}, nil
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
	s.workspaceMu.RLock()
	defer s.workspaceMu.RUnlock()
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
	if _, err := s.confinedPath(path); err != nil {
		return err
	}
	return nil
}

func (s *server) safePath(name string) (string, error) {
	if filepath.IsAbs(name) || strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("path must be a non-empty workspace-relative path")
	}
	return s.confinedPath(filepath.Join(s.root, filepath.Clean(name)))
}

// confinedPath resolves both the workspace root and target before comparing
// them. Lexical checks alone permit a workspace symlink to point outside it.
func (s *server) confinedPath(path string) (string, error) {
	root, err := filepath.EvalSymlinks(s.root)
	if err != nil {
		return "", fmt.Errorf("cannot resolve workspace root: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("cannot resolve workspace root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path in workspace: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path in workspace: %w", err)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace root")
	}
	return resolved, nil
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
		if expected == "" {
			errorsFound = append(errorsFound, fmt.Sprintf("%s: source hash is required for preview", file.Path))
			continue
		}
		if expected != hash {
			errorsFound = append(errorsFound, fmt.Sprintf("%s: source is stale (expected hash %s, current hash %s)", file.Path, expected, hash))
			continue
		}
		positions := make([]lsp.Position, 0, len(file.Edits)*2)
		for _, edit := range file.Edits {
			positions = append(positions, edit.Range.Start, edit.Range.End)
		}
		offsets, err := sourceOffsets(source, positions)
		if err != nil {
			errorsFound = append(errorsFound, fmt.Sprintf("%s: invalid edit range: %v", file.Path, err))
			continue
		}
		located := make([]locatedEdit, 0, len(file.Edits))
		for i, edit := range file.Edits {
			start, end := offsets[i*2], offsets[i*2+1]
			if end < start {
				errorsFound = append(errorsFound, fmt.Sprintf("%s: edit range ends before it starts", file.Path))
				located = nil
				break
			}
			located = append(located, locatedEdit{newText: edit.NewText, start: start, end: end, index: i})
		}
		if located == nil {
			continue
		}
		sort.Slice(located, func(i, j int) bool {
			if located[i].start != located[j].start {
				return located[i].start < located[j].start
			}
			return located[i].index < located[j].index
		})
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
		preview := renderEdits(source, located)
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

func sourceOffsets(source []byte, positions []lsp.Position) ([]int, error) {
	lineStarts := []int{0}
	for offset, b := range source {
		if b == '\n' {
			lineStarts = append(lineStarts, offset+1)
		}
	}
	type requestedOffset struct {
		position lsp.Position
		index    int
	}
	requested := make([]requestedOffset, len(positions))
	for i, position := range positions {
		if int(position.Line) >= len(lineStarts) {
			return nil, fmt.Errorf("line %d is outside source", position.Line)
		}
		requested[i] = requestedOffset{position: position, index: i}
	}
	sort.Slice(requested, func(i, j int) bool {
		if requested[i].position.Line != requested[j].position.Line {
			return requested[i].position.Line < requested[j].position.Line
		}
		return requested[i].position.Character < requested[j].position.Character
	})
	offsets := make([]int, len(positions))
	line := uint32(^uint32(0))
	offset := 0
	units := uint32(0)
	for _, request := range requested {
		if request.position.Line != line {
			line = request.position.Line
			offset = lineStarts[line]
			units = 0
		}
		for offset < len(source) && source[offset] != '\n' && units < request.position.Character {
			runeValue, size := utf8.DecodeRune(source[offset:])
			if runeValue == utf8.RuneError && size == 1 {
				return nil, fmt.Errorf("invalid UTF-8 source")
			}
			units++
			if runeValue > 0xffff {
				units++
			}
			offset += size
		}
		if units != request.position.Character {
			return nil, fmt.Errorf("character %d is outside line %d", request.position.Character, request.position.Line)
		}
		offsets[request.index] = offset
	}
	return offsets, nil
}

func renderEdits(source []byte, edits []locatedEdit) []byte {
	size := len(source)
	for _, edit := range edits {
		size += len(edit.newText) - (edit.end - edit.start)
	}
	preview := make([]byte, 0, size)
	last := 0
	for _, edit := range edits {
		preview = append(preview, source[last:edit.start]...)
		preview = append(preview, edit.newText...)
		last = edit.end
	}
	return append(preview, source[last:]...)
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
