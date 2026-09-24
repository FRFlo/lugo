package lsp

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// A stale URI is a normal LSP request during workspace refreshes; each request
// must acknowledge it rather than silently dropping the JSON-RPC response.
func TestFiveMRequestsForUnindexedDocumentReturnNull(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_client_server_shared")
	uri := h.server.pathToURI(filepath.Join(h.root, "surface_resource", "removed.lua"))
	position := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 0, Character: 0},
	}

	cases := []struct {
		name   string
		call   func(Request)
		params any
	}{
		{"hover", h.server.handleHover, position},
		{"definition", h.server.handleDefinition, position},
		{"references", h.server.handleReferences, ReferenceParams{TextDocument: position.TextDocument, Position: position.Position}},
		{"prepare rename", h.server.handlePrepareRename, position},
		{"rename", h.server.handleRename, RenameParams{TextDocument: position.TextDocument, Position: position.Position, NewName: "renamed"}},
		{"document symbols", h.server.handleDocumentSymbol, DocumentSymbolParams{TextDocument: position.TextDocument}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params, err := json.Marshal(tc.params)
			if err != nil {
				t.Fatal(err)
			}
			h.resetRPC()
			tc.call(Request{RPC: "2.0", ID: 37, Params: params})
			var response struct {
				RPC    string          `json:"jsonrpc"`
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if err := json.Unmarshal(h.lastResponse(37), &response); err != nil {
				t.Fatal(err)
			}
			if response.RPC != "2.0" || response.ID != 37 || string(response.Result) != "null" || len(response.Error) != 0 {
				t.Fatalf("stale document response = %+v, want JSON-RPC null result", response)
			}
		})
	}
}

// A rename must never edit a same-spelled global in another resource, a
// different execution profile, or a plain Lua file outside the resource.
func TestFiveMRenameRespectsResourceAndExecutionScope(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_client_server_shared", "plain_lua")
	h.writeWorkspaceFile("surface_resource/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
client_scripts { 'client.lua', 'client_consumer.lua' }
server_scripts { 'server.lua', 'server_consumer.lua' }
shared_script 'shared.lua'
`)
	h.writeWorkspaceFile("surface_resource/client.lua", `--[[@rename_client_def]]SCOPED_RENAME = 1`)
	h.writeWorkspaceFile("surface_resource/client_consumer.lua", `return --[[@rename_client_ref]]SCOPED_RENAME`)
	h.writeWorkspaceFile("surface_resource/server.lua", `--[[@rename_server_def]]SCOPED_RENAME = 2`)
	h.writeWorkspaceFile("surface_resource/server_consumer.lua", `return --[[@rename_server_ref]]SCOPED_RENAME`)
	h.writeWorkspaceFile("plain_lua/plain.lua", `return --[[@rename_plain_ref]]SCOPED_RENAME`)
	h.writeWorkspaceFile("other_resource/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
client_script 'client.lua'
`)
	h.writeWorkspaceFile("other_resource/client.lua", `--[[@rename_other_def]]SCOPED_RENAME = 3`)
	h.reindex()

	for _, tc := range []struct {
		name     string
		marker   string
		included []string
		excluded []string
	}{
		{"client definition", "rename_client_def", []string{"rename_client_def", "rename_client_ref"}, []string{"rename_server_def", "rename_server_ref", "rename_other_def", "rename_plain_ref"}},
		{"server definition", "rename_server_def", []string{"rename_server_def", "rename_server_ref"}, []string{"rename_client_def", "rename_client_ref", "rename_other_def", "rename_plain_ref"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			marker := h.requireMarker(tc.marker)
			params, err := json.Marshal(RenameParams{TextDocument: TextDocumentIdentifier{URI: marker.URI}, Position: marker.Position, NewName: "RENAMED"})
			if err != nil {
				t.Fatal(err)
			}
			h.resetRPC()
			h.server.handleRename(Request{RPC: "2.0", ID: 1, Params: params})
			var envelope struct {
				Result WorkspaceEdit `json:"result"`
			}
			if err := json.Unmarshal(h.lastResponse(1), &envelope); err != nil {
				t.Fatal(err)
			}
			for _, name := range tc.included {
				m := h.requireMarker(name)
				found := false
				for _, edit := range envelope.Result.Changes[m.URI] {
					if edit.Range.Start == m.Position && edit.NewText == "RENAMED" {
						found = true
					}
				}
				if !found {
					t.Errorf("rename omitted %s: %+v", name, envelope.Result.Changes)
				}
			}
			for _, name := range tc.excluded {
				m := h.requireMarker(name)
				for _, edit := range envelope.Result.Changes[m.URI] {
					if edit.Range.Start == m.Position {
						t.Errorf("rename crossed scope into %s: %+v", name, edit)
					}
				}
			}
		})
	}
}
