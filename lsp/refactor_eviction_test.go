package lsp

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestRenameGlobalAfterClosedSourceEviction(t *testing.T) {
	h := newFiveMFixtureHarness(t, "plain_lua")
	const name = "GLOBAL_RENAME_AFTER_EVICTION"
	h.writeWorkspaceFile("plain_lua/rename_target.lua", `--[[@rename_eviction_target]]`+name+` = 1`)
	h.writeWorkspaceFile("plain_lua/rename_closed_definition.lua", name+` = 2`)
	h.writeWorkspaceFile("plain_lua/rename_closed_reference.lua", `return `+name)
	h.reindex()

	target := h.requireMarker("rename_eviction_target")
	h.server.OpenFiles[target.URI] = true
	evictClosedDocumentCaches(h.server)

	definitionDoc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "plain_lua/rename_closed_definition.lua"))]
	referenceDoc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "plain_lua/rename_closed_reference.lua"))]
	if definitionDoc == nil || referenceDoc == nil {
		t.Fatal("rename fixture documents were not indexed")
	}
	if definitionDoc.Source() != nil || referenceDoc.Source() != nil {
		t.Fatal("setup did not evict closed document sources")
	}
	if got := string(referenceDoc.nodeSource(referenceDoc.Resolver.GlobalRefs[0])); got != name {
		t.Fatalf("evicted global reference text = %q, want %q", got, name)
	}

	params, err := json.Marshal(RenameParams{
		TextDocument: TextDocumentIdentifier{URI: target.URI},
		Position:     target.Position,
		NewName:      "RENAMED_GLOBAL",
	})
	if err != nil {
		t.Fatal(err)
	}
	h.resetRPC()
	h.server.handleRename(Request{RPC: "2.0", ID: 1, Params: params})

	var response struct {
		Result WorkspaceEdit `json:"result"`
	}
	if err := json.Unmarshal(h.lastResponse(1), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Result.Changes[target.URI]) == 0 {
		t.Fatal("rename omitted the open target definition")
	}
	if len(response.Result.Changes[referenceDoc.URI]) == 0 {
		t.Fatal("rename omitted the evicted reference with retained token text")
	}
	if edits := response.Result.Changes[definitionDoc.URI]; len(edits) != 0 {
		t.Fatalf("rename emitted edits for an evicted definition with unavailable text: %+v", edits)
	}
}
