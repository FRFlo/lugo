package lsp

import "testing"

func TestParseLuaDoc_LuaCATSCompatibilityAnnotations(t *testing.T) {
	doc := parseLuaDoc([]byte(`
@enum Color The available colors
@module app.colors The colors module
@nodiscard
@async
@package
@field Red number
`), false)

	if doc.Enum == nil || doc.Enum.Name != "Color" || doc.Enum.Desc != "The available colors" {
		t.Fatalf("enum parsed incorrectly: %+v", doc.Enum)
	}
	if doc.Module == nil || doc.Module.Name != "app.colors" || doc.Module.Desc != "The colors module" {
		t.Fatalf("module parsed incorrectly: %+v", doc.Module)
	}
	if !doc.IsNoDiscard {
		t.Error("@nodiscard should set IsNoDiscard")
	}
	if !doc.IsAsync {
		t.Error("@async should set IsAsync")
	}
	if !doc.IsPackage {
		t.Error("@package should set IsPackage")
	}
	if len(doc.Fields) != 1 || doc.Fields[0].Name != "Red" {
		t.Fatalf("enum field parsed incorrectly: %+v", doc.Fields)
	}
}

func TestParseLuaDoc_AnnotationFlagsDoNotBecomeDescription(t *testing.T) {
	doc := parseLuaDoc([]byte(`
Description
@nodiscard
@async
@package
More description
`), false)

	if doc.Description != "Description\nMore description" {
		t.Fatalf("annotation flags should not affect description: %q", doc.Description)
	}
}
