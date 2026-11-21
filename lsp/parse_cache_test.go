package lsp

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
	"github.com/FRFlo/lugo/semantic"
)

func TestParseCacheRoundTrip(t *testing.T) {
	c := openParseCache(t.TempDir())
	if c == nil {
		t.Fatal("could not open cache")
	}
	defer c.close()
	for _, source := range []string{"-- comment\nlocal x <const> = 1\nfor i=1,3 do print(x+i) end\n", "---@class Foo\nlocal t = { a = function(x) return x end }\nreturn t:a()\n", "local a =\nfunction broken(\n"} {
		t.Run(source, func(t *testing.T) {
			b := []byte(source)
			tree := ast.NewTree(b)
			p := parser.New(b, tree, 50)
			p.Parse()
			key := parseCacheKey(b, 50)
			c.save(key, tree, p.Errors)
			restored, errs, ok := c.load(key, b, "utf-16")
			if !ok {
				t.Fatal("valid parser output rejected")
			}
			if !slices.Equal(tree.Nodes, restored.Nodes) || !slices.Equal(tree.Comments, restored.Comments) || !slices.Equal(tree.ExtraList, restored.ExtraList) || tree.Root != restored.Root || !slices.Equal(tree.LineOffsets, restored.LineOffsets) {
				t.Fatal("restored tree differs")
			}
			if len(errs) != len(p.Errors) {
				t.Fatalf("errors: %v vs %v", errs, p.Errors)
			}
			for i := range errs {
				if errs[i] != p.Errors[i] {
					t.Fatalf("error differs: %v vs %v", errs, p.Errors)
				}
			}
		})
	}
}

func TestParseCacheInvalidationAndUpgrade(t *testing.T) {
	root := t.TempDir()
	c := openParseCache(root)
	if c == nil {
		t.Fatal("could not open cache")
	}
	source := []byte("local value = 1\n")
	tree := ast.NewTree(source)
	p := parser.New(source, tree, 50)
	p.Parse()
	key := parseCacheKey(source, 50)
	c.save(key, tree, p.Errors)
	if _, _, ok := c.load(key, source, "utf-8"); !ok {
		t.Fatal("cache miss")
	}
	if bytes.Equal(key, parseCacheKey([]byte("local value = 2\n"), 50)) || bytes.Equal(key, parseCacheKey(source, 10)) {
		t.Fatal("cache key did not change")
	}
	if _, err := c.db.Exec("UPDATE parse_entries SET checksum=x'00'"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := c.load(key, source, "utf-16"); ok {
		t.Fatal("corrupt cache accepted")
	}
	c.save(key, tree, p.Errors)
	r := semantic.New(tree)
	r.Resolve(tree.Root)
	r.Cleanup()
	c.saveSemantic(key, r)
	if _, err := c.db.Exec("UPDATE cache_metadata SET build=x'00'"); err != nil {
		t.Fatal(err)
	}
	c.save(key, tree, p.Errors)
	c.close()
	c = openParseCache(root)
	if c == nil {
		t.Fatal("could not reopen cache")
	}
	defer c.close()
	if _, _, ok := c.load(key, source, "utf-16"); ok {
		t.Fatal("old cache survived upgrade")
	}
	var count int
	if err := c.db.QueryRow("SELECT count(*) FROM parse_entries").Scan(&count); err != nil || count != 0 {
		t.Fatalf("entries after upgrade: %d, %v", count, err)
	}
	if err := c.db.QueryRow("SELECT count(*) FROM semantic_entries").Scan(&count); err != nil || count != 0 {
		t.Fatalf("semantic entries after upgrade: %d, %v", count, err)
	}
}

func TestParseCacheWorkspaceRestart(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'main.lua'\n")
	h.writeWorkspaceFile("main.lua", "SharedCacheSymbol = 123\n")
	h.reindex()
	uri := h.server.pathToURI(filepath.Join(h.root, "main.lua"))
	first := h.server.Documents[uri]
	if first == nil {
		t.Fatal("missing first document")
	}
	second := newFiveMFixtureHarnessWithoutIndex(t).server
	second.RootURI = h.server.RootURI
	second.WorkspaceFolders = h.server.WorkspaceFolders
	second.lowerRootPath = h.server.lowerRootPath
	second.lowerWorkspaceFolders = h.server.lowerWorkspaceFolders
	second.refreshWorkspace()
	restored := second.Documents[uri]
	if restored == nil || !slices.Equal(first.Tree.Nodes, restored.Tree.Nodes) || !bytes.Equal(first.Source(), restored.Source()) {
		t.Fatal("workspace restart changed document")
	}
	if len(second.GlobalIndex.WorkspaceSymbols("SharedCacheSymbol", 10)) == 0 {
		t.Fatal("global symbol not rebuilt")
	}
	path := filepath.Join(h.root, "main.lua")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("ChangedCacheSymbo = 456\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	third := newFiveMFixtureHarnessWithoutIndex(t).server
	third.RootURI = second.RootURI
	third.WorkspaceFolders = second.WorkspaceFolders
	third.lowerRootPath = second.lowerRootPath
	third.lowerWorkspaceFolders = second.lowerWorkspaceFolders
	third.refreshWorkspace()
	if len(third.GlobalIndex.WorkspaceSymbols("ChangedCacheSymbo", 10)) == 0 || len(third.GlobalIndex.WorkspaceSymbols("SharedCacheSymbol", 10)) != 0 {
		t.Fatal("changed file was not reindexed")
	}
}

func TestSemanticCacheRoundTrip(t *testing.T) {
	root := t.TempDir()
	c := openParseCache(root)
	if c == nil {
		t.Fatal("could not open cache")
	}
	defer c.close()
	source := []byte("local obj = {}\nobj.field = 3\nlocal a = obj.field\nfunction f(x) local x = x; return obj.field + x end\n")
	tree := ast.NewTree(source)
	p := parser.New(source, tree, 50)
	p.Parse()
	r := semantic.New(tree)
	r.Resolve(tree.Root)
	r.Cleanup()
	key := parseCacheKey(source, 50)
	c.saveSemantic(key, r)
	restored := c.loadSemantic(key, tree)
	if restored == nil {
		t.Fatal("missing semantic cache")
	}
	if !slices.Equal(r.References, restored.References) || !slices.Equal(r.GlobalDefs, restored.GlobalDefs) || !slices.Equal(r.GlobalRefs, restored.GlobalRefs) || !slices.Equal(r.LocalDefs, restored.LocalDefs) || !slices.Equal(r.DuplicateLocals, restored.DuplicateLocals) || !slices.Equal(r.ShadowedOuter, restored.ShadowedOuter) || !slices.Equal(r.Reassignments, restored.Reassignments) || !reflect.DeepEqual(r.FieldDefs, restored.FieldDefs) || !reflect.DeepEqual(r.PendingFields, restored.PendingFields) {
		t.Fatal("restored resolution differs")
	}
	// A restored resolver must remain reusable for editor changes.
	restored.Reset()
	restored.Resolve(tree.Root)
	restored.Cleanup()
	if !slices.Equal(r.References, restored.References) {
		t.Fatal("resolver reset changed references")
	}
	if _, err := c.db.Exec("UPDATE semantic_entries SET checksum=x'00'"); err != nil {
		t.Fatal(err)
	}
	if c.loadSemantic(key, tree) != nil {
		t.Fatal("corrupt semantic cache accepted")
	}
}

func TestWorkspaceCachesUseEachRoot(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("first.lua", "FirstWorkspaceCacheSymbol = 1\n")
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "second.lua"), []byte("SecondWorkspaceCacheSymbol = 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	h.server.WorkspaceFolders = append(h.server.WorkspaceFolders, h.server.pathToURI(other))
	h.server.lowerWorkspaceFolders = append(h.server.lowerWorkspaceFolders, strings.ToLower(filepath.Clean(other)))
	h.reindex()
	for _, tc := range []struct{ root, source string }{{h.root, "FirstWorkspaceCacheSymbol = 1\n"}, {other, "SecondWorkspaceCacheSymbol = 2\n"}} {
		path := filepath.Join(tc.root, ".lugo", "index.sqlite")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing cache %s: %v", path, err)
		}
		if _, err := os.Stat(filepath.Join(tc.root, ".lugo", ".gitignore")); err != nil {
			t.Fatal(err)
		}
		c := openParseCache(tc.root)
		if c == nil {
			t.Fatal("could not open cache")
		}
		key := parseCacheKey([]byte(tc.source), h.server.MaxParseErrors)
		tree, _, ok := c.load(key, []byte(tc.source), "utf-16")
		if !ok || c.loadSemantic(key, tree) == nil {
			c.close()
			t.Fatalf("missing entries in %s", path)
		}
		c.close()
	}
}

func TestWorkspaceCachePreservesExistingIgnoreFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".lugo")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".gitignore")
	const contents = "existing-pattern\n"
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	c := openParseCache(root)
	if c == nil {
		t.Fatal("could not open cache")
	}
	c.close()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != contents {
		t.Fatalf("existing ignore file changed: %q, %v", got, err)
	}
}

func TestWorkspaceCacheUsesGitRootForSubdirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: elsewhere\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "src")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	s := NewServer("test")
	s.WorkspaceFolders = []string{s.pathToURI(sub)}
	roots := s.openWorkspaceCaches()
	if len(roots) != 1 || roots[0].cache == nil {
		t.Fatal("cache not opened")
	}
	roots[0].cache.close()
	if _, err := os.Stat(filepath.Join(root, ".lugo", "index.sqlite")); err != nil {
		t.Fatalf("cache absent from Git root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sub, ".lugo")); !os.IsNotExist(err) {
		t.Fatalf("unexpected cache inside subdirectory: %v", err)
	}
}

func TestSemanticCacheRebindsModulesAfterRestart(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", "local m = require('mod')\nprint(m.field)\n")
	h.reindex()
	target := filepath.Join(h.root, "mod.lua")
	if err := os.WriteFile(target, []byte("return {field = 1}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	start := func() *Server {
		s := newFiveMFixtureHarnessWithoutIndex(t).server
		s.RootURI = h.server.RootURI
		s.WorkspaceFolders = h.server.WorkspaceFolders
		s.lowerRootPath = h.server.lowerRootPath
		s.lowerWorkspaceFolders = h.server.lowerWorkspaceFolders
		s.refreshWorkspace()
		return s
	}
	fieldHash := func(s *Server) uint64 {
		doc := s.Documents[s.pathToURI(filepath.Join(h.root, "main.lua"))]
		if doc == nil || doc.Resolver == nil {
			t.Fatal("main document missing")
		}
		for _, field := range doc.Resolver.PendingFields {
			if field.PropHash == ast.HashBytes([]byte("field")) {
				return field.ReceiverHash
			}
		}
		t.Fatal("field reference missing")
		return 0
	}
	s := start()
	want := ast.HashBytesConcat([]byte("module:"), nil, []byte(s.pathToURI(target)))
	if got := fieldHash(s); got != want {
		t.Fatalf("created module hash = %d, want %d", got, want)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	s = start()
	if got := fieldHash(s); got == want {
		t.Fatal("deleted module retained cached binding")
	}
}

func TestWorkspaceCachesUseNestedGitRepositories(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	for _, repo := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(h.root, repo, ".git"), 0700); err != nil {
			t.Fatal(err)
		}
		h.writeWorkspaceFile(repo+"/main.lua", "Nested_"+repo+" = true\n")
	}
	h.reindex()
	for _, repo := range []string{"alpha", "beta"} {
		root := filepath.Join(h.root, repo)
		c := openParseCache(root)
		if c == nil {
			t.Fatalf("missing cache for %s", repo)
		}
		source := []byte("Nested_" + repo + " = true\n")
		key := parseCacheKey(source, h.server.MaxParseErrors)
		tree, _, ok := c.load(key, source, "utf-16")
		if !ok || c.loadSemantic(key, tree) == nil {
			c.close()
			t.Fatalf("missing entries for %s", repo)
		}
		other := "alpha"
		if repo == other {
			other = "beta"
		}
		otherSource := []byte("Nested_" + other + " = true\n")
		if _, _, ok := c.load(parseCacheKey(otherSource, h.server.MaxParseErrors), otherSource, "utf-16"); ok {
			c.close()
			t.Fatalf("%s contains %s entry", repo, other)
		}
		c.close()
	}
}

func TestSemanticCacheCompactsReceiverNames(t *testing.T) {
	v := cachedSemantic{FieldDefs: []semantic.FieldDef{{ReceiverName: []byte("shared")}, {ReceiverName: []byte("other")}}, PendingFields: []semantic.FieldRef{{ReceiverName: []byte("shared")}}}
	compactCachedReceiverNames(&v)
	a, b := v.FieldDefs[0].ReceiverName, v.PendingFields[0].ReceiverName
	if string(a) != "shared" || string(v.FieldDefs[1].ReceiverName) != "other" || string(b) != "shared" || &a[0] != &b[0] {
		t.Fatal("receiver names were not compacted and shared")
	}
}
