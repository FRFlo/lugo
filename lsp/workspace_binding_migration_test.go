package lsp

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/coalaura/plain"
)

func TestFiveMManifestBindingMigrationColdWarmAndRemoval(t *testing.T) {
	server := NewServer("binding-migration")
	server.Log = plain.New(plain.WithTarget(io.Discard))
	server.DiagShadowing = true
	server.DiagShadowingLoopVar = true
	root := t.TempDir()
	server.Writer = &bytes.Buffer{}
	server.IsIndexing = false
	manifestURI := server.pathToURI(filepath.Join(root, "r", "fxmanifest.lua"))
	legacyURI := server.pathToURI(filepath.Join(root, "r", "__resource.lua"))
	nestedManifestURI := server.pathToURI(filepath.Join(root, "r", "nested", "fxmanifest.lua"))
	nestedScriptURI := server.pathToURI(filepath.Join(root, "r", "nested", "child.lua"))
	defsURI := server.pathToURI(filepath.Join(root, "r", "defs.lua"))
	useURI := server.pathToURI(filepath.Join(root, "r", "use.lua"))
	manifest := []byte("fx_version 'cerulean'\ngame 'gta5'\ndependency 'foo'\nclient_scripts {'defs.lua','use.lua'}")
	defs := []byte("function split() i=1 end\nfunction scopedName() local scopedName=false return scopedName end")
	use := []byte("for i=1,3 do print(i) end")
	for _, uri := range []string{manifestURI, defsURI, useURI, nestedManifestURI, nestedScriptURI} {
		if err := os.MkdirAll(filepath.Dir(server.uriToPath(uri)), 0755); err != nil {
			t.Fatal(err)
		}
	}
	nestedManifest := []byte("fx_version 'cerulean'\ngame 'gta5'\nclient_script 'child.lua'")
	nestedScript := []byte("function child() end")
	for uri, source := range map[string][]byte{manifestURI: manifest, defsURI: defs, useURI: use, nestedManifestURI: nestedManifest, nestedScriptURI: nestedScript} {
		if err := os.WriteFile(server.uriToPath(uri), source, 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Cold workspace refresh exercises the parse-result barrier with multiple
	// workers and should place scripts under the manifest before publication.
	server.WorkspaceFolders = []string{root}
	server.refreshWorkspace()
	assertScope := func(uri string, want GlobalIndexScope) {
		t.Helper()
		doc := server.Documents[uri]
		_, got := server.globalIndexContext(doc)
		if got != want {
			t.Fatalf("%s scope = %s, want %s", uri, got, want)
		}
	}
	assertScope(defsURI, GlobalIndexScopeClient)
	assertScope(useURI, GlobalIndexScopeClient)
	assertScope(nestedScriptURI, GlobalIndexScopeClient)
	globalI := server.GlobalIndex.LookupByScope(ResourceURI(server.pathToURI(filepath.Join(root, "r"))), GlobalIndexScopeClient, "i")
	if globalI == nil || globalI.URI != defsURI {
		t.Fatalf("cold migrated client binding for i = %#v, want source %s", globalI, defsURI)
	}

	// Warm re-publication is idempotent, and a client->server manifest edit
	// moves unchanged script bindings without reparsing their source.
	defsTree, useTree := server.Documents[defsURI].Tree, server.Documents[useURI].Tree
	server.updateDocument(manifestURI, []byte("fx_version 'cerulean'\ngame 'gta5'\nserver_scripts {'defs.lua','use.lua'}"))
	assertScope(defsURI, GlobalIndexScopeServer)
	assertScope(useURI, GlobalIndexScopeServer)
	assertScope(nestedScriptURI, GlobalIndexScopeClient)
	if server.Documents[defsURI].Tree != defsTree || server.Documents[useURI].Tree != useTree {
		t.Fatal("manifest update replaced an unchanged script AST")
	}
	server.updateDocument(manifestURI, []byte("fx_version 'cerulean'\ngame 'gta5'\nserver_scripts {'defs.lua','use.lua'}"))
	assertScope(defsURI, GlobalIndexScopeServer)

	// The legacy manifest is retained as a fallback, but fxmanifest has priority.
	legacySource := []byte("client_script 'defs.lua'\nclient_script 'use.lua'")
	if err := os.WriteFile(server.uriToPath(legacyURI), legacySource, 0644); err != nil {
		t.Fatal(err)
	}
	server.updateDocument(legacyURI, legacySource)
	assertScope(defsURI, GlobalIndexScopeServer)

	// Removing fxmanifest restores the legacy topology and migrates the scripts.
	if err := os.Remove(server.uriToPath(manifestURI)); err != nil {
		t.Fatal(err)
	}
	server.refreshWorkspace()
	assertScope(defsURI, GlobalIndexScopeClient)
	assertScope(useURI, GlobalIndexScopeClient)
	assertScope(nestedScriptURI, GlobalIndexScopeClient)

	// Removing the final manifest returns scripts to plain-Lua placement.
	if err := os.Remove(server.uriToPath(legacyURI)); err != nil {
		t.Fatal(err)
	}
	server.refreshWorkspace()
	assertScope(defsURI, GlobalIndexScopeShared)
	assertScope(useURI, GlobalIndexScopeShared)
	resourceRoot := ResourceURI(server.pathToURI(filepath.Join(root, "r")))
	resourceScope := server.GlobalIndex.Resources[resourceRoot]
	if resourceScope == nil || resourceScope.Identity != resourceRoot || len(resourceScope.ScriptScopes) != 0 {
		t.Fatalf("removed resource index scope was not detached: %#v", resourceScope)
	}
	if deps := server.GlobalIndex.DepGraph.DependencyList("r"); len(deps) != 0 {
		t.Fatalf("removed resource retained dependency edges: %v", deps)
	}
}

func countShadowGlobal(t *testing.T, server *Server, root, rel string) int {
	t.Helper()
	uri := server.pathToURI(filepath.Join(root, filepath.FromSlash(rel)))
	server.publishDiagnostics(uri)
	n := 0
	for _, d := range server.diagBuf {
		if d.Code == "shadow-global" {
			n++
		}
	}
	return n
}
func TestManifestBindingMigrationDiagnosticOrder(t *testing.T) {
	for _, early := range []bool{false, true} {
		name := "manifest-first"
		if early {
			name = "script-first"
		}
		t.Run(name, func(t *testing.T) {
			s := NewServer("diagnostic-order")
			s.DiagShadowing = true
			s.DiagShadowingLoopVar = true
			root := t.TempDir()
			out := &bytes.Buffer{}
			s.Writer = out
			h := &fiveMFixtureHarness{t: t, server: s, root: root, rpcOut: out}
			files := map[string]string{"r/fxmanifest.lua": "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts {'defs.lua','use.lua'}", "r/defs.lua": "function split() i=1 end\nfunction scopedName() local scopedName=false return scopedName end", "r/use.lua": "for i=1,3 do print(i) end"}
			for p, v := range files {
				path := filepath.Join(root, p)
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, []byte(v), 0644)
			}
			order := []string{"r/fxmanifest.lua", "r/defs.lua", "r/use.lua"}
			if early {
				order = []string{"r/defs.lua", "r/fxmanifest.lua", "r/use.lua"}
			}
			s.IsIndexing = false
			for _, p := range order {
				s.updateDocument(s.pathToURI(filepath.Join(root, p)), []byte(files[p]))
			}
			s.IsIndexing = false
			check := func(stage string) {
				for _, p := range []string{"r/defs.lua", "r/use.lua"} {
					doc := s.Documents[s.pathToURI(filepath.Join(root, p))]
					res, scope := s.globalIndexContext(doc)
					n := 0
					for _, diag := range h.diagnostics(p) {
						if diag.Code == "shadow-global" {
							n++
							t.Logf("%s %s %s", stage, p, diag.Message)
						}
					}
					want := 1
					if n != want {
						t.Fatalf("%s %s shadow=%d want %d", stage, p, n, want)
					}
					t.Logf("%s %s resource=%s scope=%s", stage, p, res, scope)
				}
			}
			check("initial")
			key := GlobalKey{PropHash: ast.HashBytes([]byte("i"))}
			for _, e := range s.GlobalIndex.SymbolsByHash(key) {
				t.Logf("hash still retains i URI=%s", e.URI)
			}
			for _, doc := range s.Documents {
				doc.TypeCache.Drop()
				doc.Inferring = nil
				doc.LuaDocCache = nil
			}
			facts := s.beginWorkspaceDiagnosticFacts()
			check("fresh-facts")
			s.endWorkspaceDiagnosticFacts(facts)
		})
	}
}

func TestManifestBindingMigrationSyntheticDiagnostics(t *testing.T) {
	for _, early := range []bool{false, true} {
		name := "manifest-first"
		if early {
			name = "script-first"
		}
		t.Run(name, func(t *testing.T) {
			s := NewServer("synthetic-diagnostics")
			s.DiagShadowing = true
			s.DiagShadowingLoopVar = true
			root := t.TempDir()
			out := &bytes.Buffer{}
			s.Writer = out
			h := &fiveMFixtureHarness{t: t, server: s, root: root, rpcOut: out}
			files := map[string]string{
				"resource/fxmanifest.lua": "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts {'defs.lua','uses.lua'}",
				"resource/defs.lua":       syntheticGlobalDefinitions(46),
				"resource/uses.lua":       syntheticLocalShadows(46),
			}
			for p, v := range files {
				path := filepath.Join(root, p)
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, []byte(v), 0644)
			}
			scripts := []string{"resource/defs.lua", "resource/uses.lua"}
			manifests := []string{"resource/fxmanifest.lua"}
			order := append(append([]string{}, manifests...), scripts...)
			if early {
				order = append(append([]string{}, scripts...), manifests...)
			}
			s.IsIndexing = false
			for _, p := range order {
				s.updateDocument(s.pathToURI(filepath.Join(root, p)), []byte(files[p]))
			}
			s.IsIndexing = false
			count := 0
			for _, p := range scripts {
				for _, d := range h.diagnostics(p) {
					if d.Code == "shadow-global" {
						count++
					}
				}
			}
			want := 46
			if count != want {
				t.Fatalf("shadow=%d want=%d", count, want)
			}
		})
	}
}

func syntheticGlobalDefinitions(count int) string {
	var source strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&source, "shadowName%d = true\n", i)
	}
	return source.String()
}

func syntheticLocalShadows(count int) string {
	var source strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&source, "local shadowName%d = true\n", i)
	}
	return source.String()
}
