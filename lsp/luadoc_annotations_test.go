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

func TestParseLuaDoc_TagNamesRequireBoundary(t *testing.T) {
	doc := parseLuaDoc([]byte(`
@parameter payload string
@returns string
@fieldname bogus number
@classify Fake
@enumtype Fake
@modular Fake
@nodiscardSomething
@asyncSomething
@packageable
@typeof string
@aliasing Fake string
@genericity T
@overloading fun(): string
@operatorish add: string
@seeAlso Fake
@exported Fake
@deprecatedly
`), false)

	if len(doc.Params) != 0 || len(doc.Returns) != 0 || len(doc.Fields) != 0 {
		t.Fatalf("prefixed tag names parsed as annotations: params=%+v returns=%+v fields=%+v", doc.Params, doc.Returns, doc.Fields)
	}
	if doc.Class != nil || doc.Enum != nil || doc.Module != nil || doc.Type != nil || doc.Alias != nil || len(doc.Generics) != 0 || len(doc.Overloads) != 0 || len(doc.Operators) != 0 || len(doc.See) != 0 || doc.Export != "" || doc.IsDeprecated || doc.IsNoDiscard || doc.IsAsync || doc.IsPackage {
		t.Fatalf("prefixed tag names changed LuaDoc state: %+v", doc)
	}
}
