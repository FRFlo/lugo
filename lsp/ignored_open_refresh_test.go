package lsp

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestIgnoredOpenDocumentRestoresUnsavedChanges(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("ignored.lua", "disk = 1")
	s := h.server
	uri := s.pathToURI(filepath.Join(h.root, "ignored.lua"))
	notify := func(method func(Request), params any) {
		t.Helper()
		data, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		method(Request{Params: data})
	}
	open := func(version int, text string) {
		notify(s.handleDidOpen, DidOpenTextDocumentParams{TextDocument: TextDocumentItem{URI: uri, Version: version, Text: text}})
	}
	change := func(version int, text string) {
		notify(s.handleDidChange, DidChangeTextDocumentParams{TextDocument: VersionedTextDocumentIdentifier{URI: uri, Version: version}, ContentChanges: []TextDocumentContentChangeEvent{{Text: text}}})
	}
	open(1, "unsaved = 1")
	s.setIgnoreGlobs([]string{"ignored.lua"})
	s.refreshWorkspace()
	if s.Documents[uri] != nil {
		t.Fatal("ignored open document remains indexed")
	}
	change(3, "unsaved = 3")
	change(2, "stale = true")
	s.setIgnoreGlobs(nil)
	s.refreshWorkspace()
	if doc := s.Documents[uri]; doc == nil || string(doc.Source()) != "unsaved = 3" {
		t.Fatalf("restored document = %v; want latest unsaved source", doc)
	}
	if _, ok := s.ignoredOpenSources[uri]; ok {
		t.Fatal("restored source retained")
	}

	// A document opened while already ignored must also be restorable.
	notify(s.handleDidClose, DidCloseTextDocumentParams{TextDocument: TextDocumentIdentifier{URI: uri}})
	s.setIgnoreGlobs([]string{"ignored.lua"})
	s.refreshWorkspace()
	open(1, "new_unsaved = true")
	s.setIgnoreGlobs(nil)
	s.refreshWorkspace()
	if doc := s.Documents[uri]; doc == nil || string(doc.Source()) != "new_unsaved = true" {
		t.Fatalf("newly opened document = %v; want unsaved source", doc)
	}
}

func TestClosingIgnoredOpenDocumentDropsRetainedSource(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("ignored.lua", "disk = 1")
	s := h.server
	uri := s.pathToURI(filepath.Join(h.root, "ignored.lua"))
	s.OpenFiles[uri] = true
	s.updateDocument(uri, []byte("unsaved = true"))
	s.setIgnoreGlobs([]string{"ignored.lua"})
	s.refreshWorkspace()
	data, err := json.Marshal(DidCloseTextDocumentParams{TextDocument: TextDocumentIdentifier{URI: uri}})
	if err != nil {
		t.Fatal(err)
	}
	s.handleDidClose(Request{Params: data})
	if _, ok := s.ignoredOpenSources[uri]; ok {
		t.Fatal("closed document retained its source")
	}
	s.setIgnoreGlobs(nil)
	s.refreshWorkspace()
	if doc := s.Documents[uri]; doc == nil || string(doc.Source()) != "disk = 1" {
		t.Fatalf("closed document = %v; want disk source", doc)
	}
}

func TestIgnoredOpenOversizedContentIsNotRetained(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	s := h.server
	s.MaxFileSize = 4
	uri := s.pathToURI(filepath.Join(h.root, "large.lua"))
	params, err := json.Marshal(DidOpenTextDocumentParams{TextDocument: TextDocumentItem{URI: uri, Version: 1, Text: "oversized"}})
	if err != nil {
		t.Fatal(err)
	}
	s.handleDidOpen(Request{Params: params})
	s.setIgnoreGlobs([]string{"large.lua"})
	s.refreshWorkspace()
	if source, exists := s.ignoredOpenSources[uri]; !exists || source != nil {
		t.Fatalf("oversized source retained: %q, exists=%t", source, exists)
	}
	s.setIgnoreGlobs(nil)
	s.refreshWorkspace()
	if s.Documents[uri] != nil {
		t.Fatal("oversized source was indexed")
	}
	if _, exists := s.ignoredOpenSources[uri]; exists {
		t.Fatal("oversized source marker retained")
	}
}
