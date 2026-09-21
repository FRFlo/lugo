package lsp

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	"github.com/coalaura/plain"
)

func TestManifestAuthoringNavigation(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	s.Writer = new(bytes.Buffer)
	s.Log = plain.New(plain.WithTarget(io.Discard))
	addFiveMTestDocument(t, s, filepath.Join(root, "resource", "client.lua"), "return true")
	manifest := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "fxmanifest.lua"), "client_script 'client.lua'\n")

	params, _ := json.Marshal(CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: manifest.URI},
		Position:     Position{Line: 0, Character: 15},
	})
	resetManifestRPC(s)
	s.handleCompletion(Request{RPC: "2.0", ID: 1, Params: params})
	var completion CompletionList
	decodeManifestResponse(t, s, &completion)
	if !hasCompletion(completion.Items, "client.lua") {
		t.Fatalf("manifest path completion missing client.lua: %#v", completion.Items)
	}

	params, _ = json.Marshal(DocumentLinkParams{TextDocument: TextDocumentIdentifier{URI: manifest.URI}})
	resetManifestRPC(s)
	s.handleDocumentLink(Request{RPC: "2.0", ID: 2, Params: params})
	var links []DocumentLink
	decodeManifestResponse(t, s, &links)
	if len(links) != 1 || links[0].Target != s.pathToURI(filepath.Join(root, "resource", "client.lua")) {
		t.Fatalf("manifest document links = %#v", links)
	}

	params, _ = json.Marshal(TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: manifest.URI},
		Position:     Position{Line: 0, Character: 18},
	})
	resetManifestRPC(s)
	s.handleDefinition(Request{RPC: "2.0", ID: 3, Params: params})
	var definitions []Location
	decodeManifestResponse(t, s, &definitions)
	if len(definitions) != 1 || definitions[0].URI != s.pathToURI(filepath.Join(root, "resource", "client.lua")) {
		t.Fatalf("manifest definitions = %#v", definitions)
	}
}

func TestManifestPathUpdatesOnLuaRename(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	s.Writer = new(bytes.Buffer)
	s.Log = plain.New(plain.WithTarget(io.Discard))
	manifest := addFiveMTestDocument(t, s, filepath.Join(root, "resource", "fxmanifest.lua"), "client_script 'client.lua'\n")
	_ = manifest

	params, _ := json.Marshal(WillRenameFilesParams{Files: []FileRename{{
		OldURI: s.pathToURI(filepath.Join(root, "resource", "client.lua")),
		NewURI: s.pathToURI(filepath.Join(root, "resource", "renamed.lua")),
	}}})
	resetManifestRPC(s)
	s.handleWillRenameFiles(Request{RPC: "2.0", ID: 4, Params: params})
	var edit WorkspaceEdit
	decodeManifestResponse(t, s, &edit)
	changes := edit.Changes[manifest.URI]
	if len(changes) != 1 || changes[0].NewText != "'renamed.lua'" {
		t.Fatalf("manifest rename edits = %#v", changes)
	}
}

func hasCompletion(items []CompletionItem, label string) bool {
	for _, item := range items {
		if item.Label == label {
			return true
		}
	}
	return false
}

func resetManifestRPC(s *Server) {
	if buffer, ok := s.Writer.(*bytes.Buffer); ok {
		buffer.Reset()
	}
}

func decodeManifestResponse(t *testing.T, s *Server, out any) {
	t.Helper()
	buffer, ok := s.Writer.(*bytes.Buffer)
	if !ok {
		t.Fatal("server writer is not a bytes buffer")
	}
	body := buffer.Bytes()
	if split := bytes.Index(body, []byte("\r\n\r\n")); split >= 0 {
		body = body[split+4:]
	}
	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(response.Result)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, out); err != nil {
		t.Fatal(err)
	}
}
