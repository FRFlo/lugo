package lsp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestGetGlobalSymbolsTraversesDeclaredParentsInOrder(t *testing.T) {
	server := NewServer("test")
	const uri = "file:///inheritance-priority.lua"
	doc := newIndexTestDocument(t, "local marker = {}\n")
	doc.URI = uri
	doc.Server = server
	server.Documents[uri] = doc
	resource := ResourceURI(uri)
	addClass := func(name string, parents []string) {
		server.GlobalIndex.AddSymbol(resource, GlobalIndexScopeShared, SymbolName(name), &SymbolEntry{
			URI: uri, NodeID: 1, Parent: firstParent(parents), Parents: parents,
			Key: GlobalKey{PropHash: ast.HashBytes([]byte(name))},
		})
	}
	addClass("Child", []string{"FirstParent", "SecondParent"})
	addClass("FirstParent", nil)
	addClass("SecondParent", nil)
	propHash := ast.HashBytes([]byte("value"))
	server.GlobalIndex.AddSymbol(resource, GlobalIndexScopeShared, "FirstParent.value", &SymbolEntry{
		URI: uri, NodeID: 1, Key: GlobalKey{ReceiverHash: ast.HashBytes([]byte("FirstParent")), PropHash: propHash},
	})
	server.GlobalIndex.AddSymbol(resource, GlobalIndexScopeShared, "SecondParent.value", &SymbolEntry{
		URI: uri, NodeID: 2, Key: GlobalKey{ReceiverHash: ast.HashBytes([]byte("SecondParent")), PropHash: propHash},
	})
	secondOnlyHash := ast.HashBytes([]byte("secondOnly"))
	server.GlobalIndex.AddSymbol(resource, GlobalIndexScopeShared, "SecondParent.secondOnly", &SymbolEntry{
		URI: uri, NodeID: 2, Key: GlobalKey{ReceiverHash: ast.HashBytes([]byte("SecondParent")), PropHash: secondOnlyHash},
	})

	got, ok := server.getGlobalSymbols(doc, ast.HashBytes([]byte("Child")), propHash)
	if !ok || len(got) != 1 || got[0].Name != "FirstParent.value" {
		t.Fatalf("inherited collision = %+v, %t; want first declared parent", got, ok)
	}
	got, ok = server.getGlobalSymbols(doc, ast.HashBytes([]byte("Child")), secondOnlyHash)
	if !ok || len(got) != 1 || got[0].Name != "SecondParent.secondOnly" {
		t.Fatalf("second-parent-only symbol = %+v, %t; want SecondParent.secondOnly", got, ok)
	}
}

func TestGetGlobalSymbolsTraversesMoreThan64DeclaredParents(t *testing.T) {
	server := NewServer("test")
	const uri = "file:///many-parents.lua"
	doc := newIndexTestDocument(t, "local marker = {}\n")
	doc.URI, doc.Server = uri, server
	server.Documents[uri] = doc
	resource := ResourceURI(uri)
	parents := make([]string, 65)
	for i := range parents {
		parents[i] = fmt.Sprintf("Parent%d", i)
		server.GlobalIndex.AddSymbol(resource, GlobalIndexScopeShared, SymbolName(parents[i]), &SymbolEntry{
			URI: uri, NodeID: 1, Key: GlobalKey{PropHash: ast.HashBytes([]byte(parents[i]))},
		})
	}
	server.GlobalIndex.AddSymbol(resource, GlobalIndexScopeShared, "Child", &SymbolEntry{
		URI: uri, NodeID: 1, Parent: parents[0], Parents: parents,
		Key: GlobalKey{PropHash: ast.HashBytes([]byte("Child"))},
	})
	propHash := ast.HashBytes([]byte("deep"))
	server.GlobalIndex.AddSymbol(resource, GlobalIndexScopeShared, "Parent64.deep", &SymbolEntry{
		URI: uri, NodeID: 2, Key: GlobalKey{ReceiverHash: ast.HashBytes([]byte(parents[64])), PropHash: propHash},
	})

	got, ok := server.getGlobalSymbols(doc, ast.HashBytes([]byte("Child")), propHash)
	if !ok || len(got) != 1 || got[0].Name != "Parent64.deep" {
		t.Fatalf("65th declared parent = %+v, %t; want Parent64.deep", got, ok)
	}
}

func firstParent(parents []string) string {
	if len(parents) == 0 {
		return ""
	}
	return parents[0]
}

func TestClassFieldCompletionUsesDeclaredOrderAtSameDepth(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class SecondParent
local SecondParent = {}
function SecondParent:shared(secondArg) end

---@class FirstParent
local FirstParent = {}
function FirstParent:shared(firstArg) end

---@class Child: SecondParent, FirstParent
local Child = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Child
local value = {}
value:--[[@parent_order_completion]]
`)
	h.reindex()
	item := completionItemByLabel(h.completion("parent_order_completion"), "shared")
	if item == nil || !strings.Contains(item.InsertText, "secondArg") || strings.Contains(item.InsertText, "firstArg") {
		t.Fatalf("same-depth parent completion = %#v, want declared-first SecondParent signature", item)
	}
}

func BenchmarkGetGlobalSymbolsMultipleParents(b *testing.B) {
	for _, parentCount := range []int{2, 16, 64} {
		b.Run(fmt.Sprintf("parents_%d", parentCount), func(b *testing.B) {
			server := NewServer("inheritance-benchmark")
			const uri = "file:///inheritance-benchmark.lua"
			doc := benchmarkMetatableDocument(b, []byte("local marker = {}\n"))
			doc.URI, doc.Server = uri, server
			server.Documents[uri] = doc
			resource := ResourceURI(uri)
			parents := make([]string, parentCount)
			for i := range parents {
				parents[i] = fmt.Sprintf("Parent%d", i)
				server.GlobalIndex.AddSymbol(resource, GlobalIndexScopeShared, SymbolName(parents[i]), &SymbolEntry{
					URI: uri, NodeID: 1, Key: GlobalKey{PropHash: ast.HashBytes([]byte(parents[i]))},
				})
			}
			server.GlobalIndex.AddSymbol(resource, GlobalIndexScopeShared, "Child", &SymbolEntry{
				URI: uri, NodeID: 1, Parent: parents[0], Parents: parents,
				Key: GlobalKey{PropHash: ast.HashBytes([]byte("Child"))},
			})
			propHash := ast.HashBytes([]byte("deep"))
			lastParent := parents[parentCount-1]
			server.GlobalIndex.AddSymbol(resource, GlobalIndexScopeShared, SymbolName(lastParent+".deep"), &SymbolEntry{
				URI: uri, NodeID: 2, Key: GlobalKey{ReceiverHash: ast.HashBytes([]byte(lastParent)), PropHash: propHash},
			})
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, ok := server.getGlobalSymbols(doc, ast.HashBytes([]byte("Child")), propHash); !ok {
					b.Fatal("missing final parent symbol")
				}
			}
		})
	}
}
