package lsp

import (
	"path/filepath"
	"testing"
)

func TestLuaDocWorkspaceIndexesClassFields(t *testing.T) {
	root := t.TempDir()
	s := NewServer("test")
	s.RootURI = s.pathToURI(root)
	s.WorkspaceFolders = []string{root}
	uri := s.pathToURI(filepath.Join(root, "types.lua"))
	source := []byte("---@class Person\n---@field name string\n---@field age number\nlocal Person = {}\n")
	s.updateDocument(uri, source)

	for _, tc := range []struct {
		name string
		want string
	}{{"name", "string"}, {"age", "number"}} {
		entry := s.GlobalIndex.LookupLuaDocClassField(ResourceURI(uri), GlobalIndexScopeShared, "Person", tc.name)
		if entry == nil {
			t.Errorf("class field %q was not indexed", tc.name)
			continue
		}
		if entry.URI != uri || entry.Name != SymbolName("Person."+tc.name) {
			t.Errorf("entry = %+v, want URI %q and name Person.%s", entry, uri, tc.name)
		}
		wantType := ParseTypeString(tc.want)
		if entry.Type.Primitive != wantType.Basics {
			t.Errorf("field type = %+v, want %q", entry.Type, tc.want)
		}
	}
}

func TestLuaDocWorkspaceIndexesClassFieldTypesAndUpdates(t *testing.T) {
	root := t.TempDir()
	s := NewServer("test")
	s.RootURI = s.pathToURI(root)
	s.WorkspaceFolders = []string{root}
	uri := s.pathToURI(filepath.Join(root, "types.lua"))
	first := []byte("---@class Person\n---@field age number\nlocal Person = {}\n")
	s.updateDocument(uri, first)
	if got := s.GlobalIndex.LookupLuaDocClassField(ResourceURI(uri), GlobalIndexScopeShared, "Person", "age"); got == nil || got.URI != uri || got.Type.Primitive != TypeNumber {
		t.Fatalf("indexed field = %+v, want field from %s", got, uri)
	}

	second := []byte("---@class Person\n---@field age string\nlocal Person = {}\n")
	s.updateDocument(uri, second)
	if got := s.GlobalIndex.LookupLuaDocClassField(ResourceURI(uri), GlobalIndexScopeShared, "Person", "age"); got == nil || got.URI != uri || got.Type.Primitive != TypeString {
		t.Fatalf("updated field = %+v, want field from %s", got, uri)
	}
}
