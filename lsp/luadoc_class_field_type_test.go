package lsp

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestLuaDocClassFieldTypeIsPreservedInWorkspaceIndex(t *testing.T) {
	root := t.TempDir()
	s := NewServer("test")
	s.RootURI = s.pathToURI(root)
	s.WorkspaceFolders = []string{root}
	uri := s.pathToURI(filepath.Join(root, "types.lua"))
	source := []byte("---@class Widget\n---@field owner Person\n---@field onChange fun(name: string, count: number): boolean\n---@field amount number\nlocal Widget = {}\n")
	s.updateDocument(uri, source)

	owner := s.GlobalIndex.LookupLuaDocClassField(ResourceURI(uri), GlobalIndexScopeShared, "Widget", "owner")
	if owner == nil || owner.LuaDoc == nil || owner.LuaDoc.ParsedType().CustomName != "Person" {
		t.Fatalf("indexed custom class type = %+v, want Person", owner)
	}

	callable := s.GlobalIndex.LookupLuaDocClassField(ResourceURI(uri), GlobalIndexScopeShared, "Widget", "onChange")
	if callable == nil || callable.LuaDoc == nil || callable.LuaDoc.TypeString != "fun(name: string, count: number): boolean" {
		t.Fatal("callable field missing from workspace index")
	}
	callType := callable.LuaDoc.ParsedType()
	if callType.Basics&TypeFunction == 0 || callType.CallSig == "" || len(callType.CallParams) != 2 || len(callType.CallRet) != 1 {
		t.Fatalf("indexed callable type = %+v, want full signature, parameters, and return", callType)
	}
	if callType.CallParams[0].Type != "string" || callType.CallParams[1].Type != "number" || callType.CallRet[0].Type != "boolean" {
		t.Fatalf("indexed callable details = %+v, want string, number => boolean", callType)
	}

	primitive := s.GlobalIndex.LookupLuaDocClassField(ResourceURI(uri), GlobalIndexScopeShared, "Widget", "amount")
	if primitive == nil || primitive.LuaDoc == nil || primitive.LuaDoc.ParsedType().Basics != TypeNumber {
		t.Fatalf("indexed primitive type = %+v, want number", primitive)
	}
}

func TestLuaDocClassFieldTypeSurvivesReindex(t *testing.T) {
	root := t.TempDir()
	s := NewServer("test")
	s.RootURI = s.pathToURI(root)
	s.WorkspaceFolders = []string{root}
	uri := s.pathToURI(filepath.Join(root, "types.lua"))
	update := func(fieldType string) *SymbolEntry {
		source := []byte("---@class Widget\n---@field owner " + fieldType + "\nlocal Widget = {}\n")
		s.updateDocument(uri, source)
		return s.GlobalIndex.LookupLuaDocClassField(ResourceURI(uri), GlobalIndexScopeShared, "Widget", "owner")
	}

	if got := update("Person"); got == nil || got.LuaDoc == nil || got.LuaDoc.TypeString != "Person" || got.LuaDoc.ParsedType().CustomName != "Person" {
		t.Fatalf("initial indexed type = %+v, want Person", got)
	}
	if got := update("number"); got == nil || got.LuaDoc == nil || got.LuaDoc.TypeString != "number" || got.LuaDoc.ParsedType().Basics != TypeNumber {
		t.Fatalf("reindexed type = %+v, want number", got)
	}
}

func BenchmarkLuaDocClassFieldIndexMemory(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprintf("fields_%d", count), func(b *testing.B) {
			var source strings.Builder
			source.WriteString("---@class Widget\n")
			for i := 0; i < count; i++ {
				fmt.Fprintf(&source, "---@field field%d string\n", i)
			}
			source.WriteString("local Widget = {}\n")
			text := []byte(source.String())
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				s := NewServer("bench")
				s.updateDocument("file:///bench.lua", text)
			}
		})
	}
}

func TestLuaDocDataMemoryFootprint(t *testing.T) {
	if size := unsafe.Sizeof(LuaDocData{}); size > 160 {
		t.Fatalf("LuaDocData occupies %d bytes; want at most 160 for per-field metadata", size)
	}
}
