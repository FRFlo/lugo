package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParserErrorLimitChangesWithoutWorkspaceReindex(t *testing.T) {
	for _, evicted := range []bool{false, true} {
		t.Run(map[bool]string{false: "open unsaved", true: "closed evicted"}[evicted], func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			s := h.server
			s.IsIndexing = false
			s.workDoneProgressSupport = true
			uri := s.pathToURI(filepath.Join(h.root, "errors.lua"))
			source := []byte("local =\nlocal =\nlocal =\n")
			if err := os.WriteFile(s.uriToPath(uri), source, 0644); err != nil {
				t.Fatal(err)
			}
			s.MaxParseErrors = 1
			s.updateDocument(uri, source)
			doc := s.Documents[uri]
			if len(doc.Errors) != 1 {
				t.Fatalf("initial errors = %d", len(doc.Errors))
			}
			if evicted {
				doc.Tree.Source = nil
			} else {
				s.OpenFiles[uri] = true
				if err := os.WriteFile(s.uriToPath(uri), []byte("return 1"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			originalTree := doc.Tree
			send := func(limit int) {
				t.Helper()
				opts := defaultInitializationOptions()
				opts.ParserMaxErrors = limit
				data, err := json.Marshal(DidChangeConfigurationParams{Settings: opts})
				if err != nil {
					t.Fatal(err)
				}
				h.rpcOut.Reset()
				s.handleDidChangeConfiguration(Request{Params: data})
				if strings.Contains(h.rpcOut.String(), "window/workDoneProgress/create") {
					t.Fatal("syntax-only change triggered workspace indexing")
				}
				if doc.Tree != originalTree {
					t.Fatal("syntax-only change replaced indexed tree")
				}
			}
			// Initialize other settings so the first notification differs only in the limit.
			opts := defaultInitializationOptions()
			opts.ParserMaxErrors = 1
			s.applyInitializationOptions(opts)
			send(0)
			if len(doc.Errors) <= 1 {
				t.Fatalf("unlimited errors = %d, expected more than one", len(doc.Errors))
			}
			send(1)
			if len(doc.Errors) != 1 {
				t.Fatalf("limited errors = %d", len(doc.Errors))
			}
			send(1)
			if h.rpcOut.Len() != 0 {
				t.Fatal("unchanged limit republished diagnostics")
			}
		})
	}
}

func TestParserErrorLimitPreservesOversizedDiagnostic(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	s := h.server
	s.IsIndexing = false
	uri := s.pathToURI(filepath.Join(h.root, "large.lua"))
	s.updateDocument(uri, []byte("local ="))
	s.OpenFiles[uri] = true
	s.oversizedOpenFiles = map[string]bool{uri: true}
	opts := defaultInitializationOptions()
	opts.ParserMaxErrors = 1
	s.applyInitializationOptions(opts)
	opts.ParserMaxErrors = 2
	data, err := json.Marshal(DidChangeConfigurationParams{Settings: opts})
	if err != nil {
		t.Fatal(err)
	}
	h.rpcOut.Reset()
	s.handleDidChangeConfiguration(Request{Params: data})
	if strings.Contains(h.rpcOut.String(), uri) {
		t.Fatal("oversized document's size diagnostic was overwritten")
	}
}

func TestParserErrorLimitKeepsErrorsOnReadFailure(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	s := h.server
	s.MaxParseErrors = 1
	uri := s.pathToURI(filepath.Join(h.root, "missing.lua"))
	s.updateDocument(uri, []byte("local =\nlocal =\n"))
	doc := s.Documents[uri]
	if len(doc.Errors) != 1 {
		t.Fatalf("initial errors = %d", len(doc.Errors))
	}
	before := doc.Errors[0]
	doc.Tree.Source = nil
	s.MaxParseErrors = 0
	s.updateParseErrorLimit(1)
	if len(doc.Errors) != 1 || doc.Errors[0] != before {
		t.Fatal("read failure must preserve the prior errors")
	}
}

func TestParserErrorLimitReloadsEvictedChangedSourceConsistently(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	s := h.server
	s.MaxParseErrors = 1
	filename := filepath.Join(h.root, "changed.lua")
	uri := s.pathToURI(filename)
	s.updateDocument(uri, []byte("local =\nlocal =\n"))
	doc := s.Documents[uri]
	doc.Tree.Source = nil
	fresh := "local fresh = 1\n"
	if err := os.WriteFile(filename, []byte(fresh), 0644); err != nil {
		t.Fatal(err)
	}
	s.MaxParseErrors = 0
	s.updateParseErrorLimit(1)
	if string(doc.Tree.Source) != fresh || len(doc.Errors) != 0 {
		t.Fatalf("source = %q, errors = %v", doc.Tree.Source, doc.Errors)
	}
	if len(doc.Resolver.LocalDefs) == 0 {
		t.Fatal("changed source was not re-resolved")
	}
}
