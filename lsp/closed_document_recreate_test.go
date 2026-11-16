package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestClosedLuaDocumentDeleteRecreate(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider.lua", "function RecreatedProvider() return 1 end\n")
	path := filepath.Join(h.root, "provider.lua")
	h.reindex()

	uri := h.server.pathToURI(path)
	doc := h.server.Documents[uri]
	if doc == nil || len(doc.Source()) == 0 {
		t.Fatal("provider should be indexed with source")
	}
	tree, resolver := doc.Tree, doc.Resolver

	closeParams, err := json.Marshal(DidCloseTextDocumentParams{TextDocument: TextDocumentIdentifier{URI: uri}})
	if err != nil {
		t.Fatal(err)
	}
	h.server.handleDidClose(Request{Params: closeParams})
	if doc.Tree != tree || doc.Resolver != resolver {
		t.Fatal("closing the document must retain its AST and resolver")
	}
	if len(doc.Source()) == 0 {
		t.Fatal("closing the document must preserve source while its AST and resolver are retained")
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	sendWatchedFileChange(t, h.server, uri, 3)
	if _, ok := h.server.Documents[uri]; ok {
		t.Fatal("deleted provider should be removed from the document cache")
	}

	const recreatedSource = "function RecreatedProvider() return 2 end\n"
	if err := os.WriteFile(path, []byte(recreatedSource), 0o600); err != nil {
		t.Fatal(err)
	}
	sendWatchedFileChange(t, h.server, uri, 1)
	recreated := h.server.Documents[uri]
	if recreated == nil {
		t.Fatal("recreated provider should be indexed")
	}
	if got := string(recreated.Source()); got != recreatedSource {
		t.Fatalf("recreated source = %q, want %q", got, recreatedSource)
	}
}

func sendWatchedFileChange(t *testing.T, s *Server, uri string, changeType int) {
	t.Helper()
	params, err := json.Marshal(DidChangeWatchedFilesParams{Changes: []FileEvent{{URI: uri, Type: changeType}}})
	if err != nil {
		t.Fatal(err)
	}
	s.handleDidChangeWatchedFiles(Request{Params: params})
}
