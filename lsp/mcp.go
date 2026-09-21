package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

	s.handleMessage(Request{
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

	return json.RawMessage("null"), nil
}
