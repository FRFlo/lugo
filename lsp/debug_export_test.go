package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/coalaura/lugo/ast"
	"github.com/coalaura/lugo/semantic"
)

func TestNormalizeDebugExportCategories(t *testing.T) {
	t.Run("DefaultsWhenEmpty", func(t *testing.T) {
		got := normalizeDebugExportCategories(nil)
		if !slices.Equal(got, debugExportDefaultCategories) {
			t.Fatalf("normalizeDebugExportCategories(nil) = %#v, want %#v", got, debugExportDefaultCategories)
		}
	})

	t.Run("FiltersUnknownAndDuplicates", func(t *testing.T) {
		got := normalizeDebugExportCategories([]string{
			debugExportCategoryIdentifiers,
			"unknown",
			debugExportCategoryTokens,
			debugExportCategoryIdentifiers,
		})

		want := []string{debugExportCategoryIdentifiers, debugExportCategoryTokens}
		if !slices.Equal(got, want) {
			t.Fatalf("normalizeDebugExportCategories() = %#v, want %#v", got, want)
		}
	})
}

func TestBuildDebugExport(t *testing.T) {
	source := []byte("local playerName = GetPlayerName()\nprint(playerName)\n")
	tree := parseResolverLua(t, source)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)

	server := NewServer("test-version")
	server.IsIndexing = false
	server.RootURI = "file:///workspace"
	server.WorkspaceFolders = []string{"file:///workspace"}
	server.Documents["file:///workspace/main.lua"] = &Document{
		Server:      server,
		URI:         "file:///workspace/main.lua",
		Path:        "/workspace/main.lua",
		ModuleName:  "main",
		Tree:        tree,
		Resolver:    resolver,
		IsWorkspace: true,
	}
	server.OpenFiles["file:///workspace/main.lua"] = true
	server.GlobalIndex.AddSymbol("file:///workspace", GlobalIndexScopeShared, "GetPlayerName", &SymbolEntry{
		Key:    GlobalKey{PropHash: ast.HashBytes([]byte("GetPlayerName"))},
		URI:    "file:///workspace/main.lua",
		NodeID: findIdentByOccurrence(t, tree, "GetPlayerName", 0),
	})

	content, err := server.buildDebugExport(DebugExportParams{Categories: []string{
		debugExportCategoryIdentifiers,
		debugExportCategorySemantic,
		debugExportCategoryGlobalIndex,
	}})
	if err != nil {
		t.Fatalf("buildDebugExport() error = %v", err)
	}
	contentAgain, err := server.buildDebugExport(DebugExportParams{Categories: []string{
		debugExportCategoryIdentifiers,
		debugExportCategorySemantic,
		debugExportCategoryGlobalIndex,
	}})
	if err != nil {
		t.Fatalf("second buildDebugExport() error = %v", err)
	}
	if content != contentAgain {
		t.Fatal("buildDebugExport() produced different JSON for identical inputs")
	}

	var payload debugExportPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("exported JSON did not unmarshal: %v\n%s", err, content)
	}

	if payload.Metadata.ServerVersion != "test-version" || payload.Metadata.RootURI != "file:///workspace" {
		t.Fatalf("metadata = %+v, want version/root", payload.Metadata)
	}
	if !slices.Equal(payload.Metadata.Categories, []string{debugExportCategoryIdentifiers, debugExportCategorySemantic, debugExportCategoryGlobalIndex}) {
		t.Fatalf("categories = %#v", payload.Metadata.Categories)
	}
	if len(payload.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(payload.Documents))
	}

	doc := payload.Documents[0]
	if doc.URI != "file:///workspace/main.lua" || !doc.Open || !doc.IsWorkspace {
		t.Fatalf("document metadata = %+v", doc)
	}
	if len(doc.Tokens) != 0 || len(doc.AST) != 0 || len(doc.Comments) != 0 {
		t.Fatalf("unselected sections exported: tokens=%d ast=%d comments=%d", len(doc.Tokens), len(doc.AST), len(doc.Comments))
	}
	if !debugExportHasIdentifier(doc.Identifiers, "playerName") || !debugExportHasIdentifier(doc.Identifiers, "GetPlayerName") {
		t.Fatalf("identifiers = %#v, want playerName and GetPlayerName", doc.Identifiers)
	}
	if doc.Semantic == nil || len(doc.Semantic.References) == 0 || len(doc.Semantic.LocalDefs) == 0 {
		t.Fatalf("semantic summary = %+v, want references and local defs", doc.Semantic)
	}
	if len(payload.GlobalIndex) != 1 || payload.GlobalIndex[0].Name != "GetPlayerName" {
		t.Fatalf("global index = %#v, want GetPlayerName", payload.GlobalIndex)
	}
}

func TestBuildDebugExportUsesDiskSourceForEvictedDocument(t *testing.T) {
	source := []byte("-- debug source fallback\nlocal evictedName = 42\nprint(evictedName)\n")
	path := filepath.Join(t.TempDir(), "evicted.lua")
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tree := parseResolverLua(t, source)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	tree.Source = nil

	server := NewServer("test-version")
	server.IsIndexing = false
	server.Documents["file:///workspace/evicted.lua"] = &Document{
		Server:      server,
		URI:         "file:///workspace/evicted.lua",
		Path:        path,
		Tree:        tree,
		Resolver:    resolver,
		IsWorkspace: true,
	}

	content, err := server.buildDebugExport(DebugExportParams{})
	if err != nil {
		t.Fatalf("buildDebugExport() error = %v", err)
	}

	var payload debugExportPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("exported JSON did not unmarshal: %v\n%s", err, content)
	}
	if len(payload.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(payload.Documents))
	}

	doc := payload.Documents[0]
	if doc.SourceBytes != len(source) || doc.Hash != ast.HashBytes(source) {
		t.Fatalf("source metadata = bytes %d hash %d, want bytes %d hash %d", doc.SourceBytes, doc.Hash, len(source), ast.HashBytes(source))
	}
	if !debugExportHasToken(doc.Tokens, "local") || !debugExportHasIdentifier(doc.Identifiers, "evictedName") {
		t.Fatalf("source-derived tokens missing: tokens=%#v identifiers=%#v", doc.Tokens, doc.Identifiers)
	}
	if len(doc.Comments) != 1 || doc.Comments[0].Text != "-- debug source fallback" {
		t.Fatalf("comments = %#v, want fallback comment text", doc.Comments)
	}
	if doc.Semantic == nil || !debugExportHasNodeRef(doc.Semantic.LocalDefs, "evictedName") {
		t.Fatalf("semantic local defs = %+v, want evictedName from fallback source", doc.Semantic)
	}
}

func debugExportHasIdentifier(identifiers []debugExportIdentifier, name string) bool {
	for _, ident := range identifiers {
		if ident.Text == name {
			return true
		}
	}
	return false
}

func debugExportHasToken(tokens []debugExportToken, text string) bool {
	for _, tok := range tokens {
		if tok.Text == text {
			return true
		}
	}
	return false
}

func debugExportHasNodeRef(refs []debugExportNodeRef, name string) bool {
	for _, ref := range refs {
		if ref.Name == name {
			return true
		}
	}
	return false
}
