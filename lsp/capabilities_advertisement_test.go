package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/coalaura/plain"
)

func initializeCapabilitiesForTest(t *testing.T) ServerCapabilities {
	t.Helper()
	s := NewServer("test")
	var output bytes.Buffer
	s.Writer = &output
	s.Log = plain.New(plain.WithTarget(io.Discard))
	s.handleInitialize(Request{
		RPC:    "2.0",
		ID:     float64(1),
		Params: json.RawMessage(`{"capabilities":{}}`),
	})

	message, err := ReadMessage(bufioReader(&output))
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	var response Response
	if err := json.Unmarshal(message, &response); err != nil {
		t.Fatalf("unmarshal initialize response: %v", err)
	}
	var result InitializeResult
	encoded, err := json.Marshal(response.Result)
	if err != nil {
		t.Fatalf("marshal capabilities: %v", err)
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("unmarshal initialize result: %v", err)
	}
	return result.Capabilities
}

func bufioReader(r io.Reader) *bufio.Reader { return bufio.NewReader(r) }

func TestAdvertisesNavigationSurface(t *testing.T) {
	capabilities := initializeCapabilitiesForTest(t)
	checks := map[string]bool{
		"definitionProvider":         capabilities.DefinitionProvider,
		"referencesProvider":         capabilities.ReferencesProvider,
		"typeDefinitionProvider":     capabilities.TypeDefinitionProvider,
		"implementationProvider":     capabilities.ImplementationProvider,
		"documentSymbolProvider":     capabilities.DocumentSymbolProvider,
		"workspaceSymbolProvider":    capabilities.WorkspaceSymbolProvider,
		"callHierarchyProvider":      capabilities.CallHierarchyProvider,
		"documentHighlightProvider":  capabilities.DocumentHighlightProvider,
		"selectionRangeProvider":     capabilities.SelectionRangeProvider,
		"foldingRangeProvider":       capabilities.FoldingRangeProvider,
		"linkedEditingRangeProvider": capabilities.LinkedEditingRangeProvider,
	}
	for name, enabled := range checks {
		if !enabled {
			t.Errorf("%s = false, want true", name)
		}
	}
	if capabilities.DocumentLinkProvider == nil {
		t.Fatal("documentLinkProvider = nil, want provider options")
	}
}

func TestAdvertisesLinkedEditingAndSourceFixAll(t *testing.T) {
	capabilities := initializeCapabilitiesForTest(t)
	if !capabilities.LinkedEditingRangeProvider {
		t.Fatal("linkedEditingRangeProvider = false, want true")
	}

	codeAction, ok := capabilities.CodeActionProvider.(map[string]any)
	if !ok {
		t.Fatalf("codeActionProvider = %T, want object", capabilities.CodeActionProvider)
	}
	kinds, ok := codeAction["codeActionKinds"].([]any)
	if !ok {
		t.Fatalf("codeActionKinds = %T, want array", codeAction["codeActionKinds"])
	}
	for _, kind := range kinds {
		if kind == "source.fixAll" {
			return
		}
	}
	t.Fatalf("codeActionKinds = %#v, want source.fixAll", kinds)
}
