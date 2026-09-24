package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/coalaura/plain"
)

// NewMCPWorkspace creates a synchronously indexed LSP workspace for the MCP
// adapter. It deliberately keeps the MCP transport separate from the LSP
// stdio transport while reusing the same parser, resolver, diagnostics and
// FiveM resource graph.
func NewMCPWorkspace(root string) (*Server, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat workspace root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace root is not a directory: %s", abs)
	}

	s := NewServer("mcp")
	s.Reader = bufio.NewReader(strings.NewReader(""))
	s.Writer = io.Discard
	s.Log = plain.New(plain.WithTarget(io.Discard))
	s.RootURI = s.pathToURI(abs)
	s.lowerRootPath = strings.ToLower(abs)
	s.WorkspaceFolders = []string{s.RootURI}
	s.lowerWorkspaceFolders = []string{s.lowerRootPath}

	s.applyInitializationOptions(defaultInitializationOptions())
	s.refreshWorkspace()

	return s, nil
}

// MCPDocumentURI converts a workspace path to the URI used by LSP methods.
func (s *Server) MCPDocumentURI(path string) string {
	return s.pathToURI(path)
}

// MCPFiveMContracts returns the literal FiveM runtime surfaces discovered by
// the existing parser and asset scanners. It is read-only and deliberately
// avoids invoking diagnostics or changing resolver state.
func (s *Server) MCPFiveMContracts() FiveMContractSnapshot {
	if s == nil {
		return FiveMContractSnapshot{}
	}
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()

	uris := make([]string, 0, len(s.Documents))
	for uri := range s.Documents {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	snapshot := FiveMContractSnapshot{}
	eventSources, eventTargets := []FiveMContractSymbol{}, []FiveMContractSymbol{}
	convarSources, convarTargets := []FiveMContractSymbol{}, []FiveMContractSymbol{}
	nuiRoots := make(map[string]bool)
	for _, uri := range uris {
		doc := s.Documents[uri]
		if doc == nil {
			continue
		}
		events := fiveMEventContractSymbols(doc)
		snapshot.Symbols = append(snapshot.Symbols, events...)
		for _, symbol := range events {
			if symbol.Direction == FiveMContractLuaToHost {
				eventSources = append(eventSources, symbol)
			} else {
				eventTargets = append(eventTargets, symbol)
			}
		}
		exports := fiveMExportContractSymbols(doc)
		snapshot.Symbols = append(snapshot.Symbols, exports...)
		convars := fiveMConvarContractSymbols(doc)
		snapshot.Symbols = append(snapshot.Symbols, convars...)
		for _, symbol := range convars {
			if symbol.Direction == FiveMContractLuaToHost {
				convarSources = append(convarSources, symbol)
			} else {
				convarTargets = append(convarTargets, symbol)
			}
		}
		if doc.IsFiveMManifest {
			if resource := s.parseFiveMManifest(doc); resource != nil && resource.Manifest != nil {
				for _, entry := range resource.Manifest.Entries {
					snapshot.Manifests = append(snapshot.Manifests, FiveMContractManifest{Name: entry.NormalizedName, Value: entry.Value, Location: FiveMContractLocation{URI: entry.SourceURI, Range: entry.ValueRange}})
				}
			}
		}
		if root := s.getDocResourceRoot(doc); root != "" && !nuiRoots[root] {
			nuiRoots[root] = true
			snapshot.Links = append(snapshot.Links, s.fiveMNUIContractLinks(doc)...)
		}
	}
	snapshot.Links = append(snapshot.Links, linkFiveMContractSymbols(eventSources, eventTargets, FiveMContractConfidenceHigh)...)
	snapshot.Links = append(snapshot.Links, linkFiveMContractSymbols(convarSources, convarTargets, FiveMContractConfidenceHigh)...)
	sort.Slice(snapshot.Symbols, func(i, j int) bool { return fiveMContractSymbolLess(snapshot.Symbols[i], snapshot.Symbols[j]) })
	sort.Slice(snapshot.Links, func(i, j int) bool {
		left, right := snapshot.Links[i], snapshot.Links[j]
		if fiveMContractSymbolLess(left.From, right.From) {
			return true
		}
		if fiveMContractSymbolLess(right.From, left.From) {
			return false
		}
		if fiveMContractSymbolLess(left.To, right.To) {
			return true
		}
		if fiveMContractSymbolLess(right.To, left.To) {
			return false
		}
		return left.Confidence < right.Confidence
	})
	sort.Slice(snapshot.Manifests, func(i, j int) bool {
		left, right := snapshot.Manifests[i], snapshot.Manifests[j]
		if left.Location.URI != right.Location.URI {
			return left.Location.URI < right.Location.URI
		}
		if left.Location.Range.Start.Line != right.Location.Range.Start.Line {
			return left.Location.Range.Start.Line < right.Location.Range.Start.Line
		}
		if left.Location.Range.Start.Character != right.Location.Range.Start.Character {
			return left.Location.Range.Start.Character < right.Location.Range.Start.Character
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.Value < right.Value
	})
	return snapshot
}

func fiveMContractSymbolLess(left, right FiveMContractSymbol) bool {
	if left.Location.URI != right.Location.URI {
		return left.Location.URI < right.Location.URI
	}
	if left.Location.Range.Start.Line != right.Location.Range.Start.Line {
		return left.Location.Range.Start.Line < right.Location.Range.Start.Line
	}
	if left.Location.Range.Start.Character != right.Location.Range.Start.Character {
		return left.Location.Range.Start.Character < right.Location.Range.Start.Character
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	return left.Direction < right.Direction
}

// MCPDiagnostics computes diagnostics for one indexed document and returns
// the same publishDiagnostics payload used by LSP clients.
func (s *Server) MCPDiagnostics(uri string) (json.RawMessage, error) {
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()

	if _, ok := s.Documents[uri]; !ok {
		return nil, fmt.Errorf("document is not indexed: %s", uri)
	}
	var output bytes.Buffer
	oldWriter := s.Writer
	s.Writer = &output
	defer func() { s.Writer = oldWriter }()
	s.OpenFiles[uri] = true
	s.publishDiagnostics(uri)

	reader := bufio.NewReader(bytes.NewReader(output.Bytes()))
	for {
		message, err := ReadMessage(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read diagnostics: %w", err)
		}
		var envelope struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(message, &envelope); err != nil {
			return nil, err
		}
		if envelope.Method == "textDocument/publishDiagnostics" {
			return envelope.Params, nil
		}
	}
	return json.RawMessage(`{"uri":"` + uri + `","diagnostics":[]}`), nil
}

// MCPRequest dispatches one LSP request against the indexed workspace and
// returns its JSON result. It is intentionally serialized because the LSP
// server reuses parser and response buffers for allocation-free operation.
func (s *Server) MCPRequest(method string, params json.RawMessage) (json.RawMessage, error) {
	return s.MCPRequestContext(context.Background(), method, params)
}

// MCPRequestContext dispatches an MCP-backed LSP request while preserving the
// caller's trace context across the LSP boundary.
func (s *Server) MCPRequestContext(ctx context.Context, method string, params json.RawMessage) (result json.RawMessage, resultErr error) {
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()

	if s == nil {
		return nil, fmt.Errorf("nil LSP workspace")
	}
	if params == nil {
		params = json.RawMessage(`{}`)
	}

	var output bytes.Buffer
	oldWriter := s.Writer
	s.Writer = &output
	defer func() { s.Writer = oldWriter }()

	traceCtx, _ := StartSpan(ctx)
	RecordTelemetry(traceCtx, "lugo.mcp.lsp_request.start", map[string]any{"method": method})
	started := time.Now()
	defer func() {
		status := "ok"
		if resultErr != nil {
			status = "error"
		}
		RecordTelemetry(traceCtx, "lugo.mcp.lsp_request.finish", map[string]any{"method": method, "status": status, "duration_ms": time.Since(started).Milliseconds()})
	}()
	s.handleMessageContext(withRecoverableMCPRequest(traceCtx), Request{
		RPC:    "2.0",
		Method: method,
		Params: params,
		ID:     json.RawMessage("1"),
	})

	reader := bufio.NewReader(bytes.NewReader(output.Bytes()))
	for {
		message, err := ReadMessage(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read LSP response: %w", err)
		}

		var envelope struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *ResponseError  `json:"error"`
		}
		if err := json.Unmarshal(message, &envelope); err != nil {
			return nil, fmt.Errorf("decode LSP response: %w", err)
		}
		if string(envelope.ID) != "1" {
			continue
		}
		if envelope.Error != nil {
			return nil, fmt.Errorf("LSP %d: %s", envelope.Error.Code, envelope.Error.Message)
		}
		if envelope.Result == nil {
			return json.RawMessage("null"), nil
		}
		return envelope.Result, nil
	}

	resultErr = fmt.Errorf("LSP response missing")
	RecordTelemetry(traceCtx, "lugo.mcp.lsp_request.missing_response", map[string]any{
		"boundary":     "embedded_mcp",
		"method_class": mcpMethodClass(method),
		"status":       "missing_response",
	})
	return nil, resultErr
}
