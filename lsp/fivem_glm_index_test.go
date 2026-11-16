package lsp

import (
	"fmt"
	"testing"
)

func TestFiveMGLMNumericIndexTypes(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
client_script 'main.lua'
`)
	h.writeWorkspaceFile("resource/main.lua", `local m = mat3x2()
local column = m[2]
local component = m[2][1]
local polygon = glm.polygon.new({vec3(), vec3(), vec3()})
local point = polygon[2]
local polygon2 = glm.polygon.new({vec3()})
local point2 = polygon2[1]
local count = #polygon
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	for _, tt := range []struct {
		name, want string
	}{
		{"column", "Vector2"},
		{"component", "number"},
		{"point", "Vector3"},
		{"point2", "Vector3"},
		{"count", "number"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := doc.InferType(findIdentByOccurrence(t, doc.Tree, tt.name, 0))
			if name := fiveMGLMTestTypeName(got); name != tt.want {
				t.Fatalf("%s inferred as %q (%#v), want %q", tt.name, name, got, tt.want)
			}
		})
	}
}

func TestFiveMGLMMatrixColumnDimensions(t *testing.T) {
	for columns := 2; columns <= 4; columns++ {
		for rows := 2; rows <= 4; rows++ {
			typ := TypeSet{CustomName: fmt.Sprintf("Matrix%dx%d", columns, rows)}
			got, ok := fiveMGLMIndexType(typ, float64(columns))
			if !ok || got.CustomName != fmt.Sprintf("Vector%d", rows) {
				t.Errorf("%s[%d] = %#v, %v; want Vector%d", typ.CustomName, columns, got, ok, rows)
			}
			if _, ok := fiveMGLMIndexType(typ, float64(columns+1)); ok {
				t.Errorf("%s[%d] unexpectedly inferred a column", typ.CustomName, columns+1)
			}
		}
	}
}

func TestFiveMGLMNumericIndexBounds(t *testing.T) {
	for _, tt := range []struct {
		name, expr string
		want       string
	}{
		{"matrix valid column", "mat2x4()[2]", "Vector4"},
		{"matrix invalid column", "mat2x4()[3]", "unknown"},
		{"vector invalid component", "vec2()[3]", "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("resource/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
client_script 'main.lua'
`)
			h.writeWorkspaceFile("resource/main.lua", "local value = "+tt.expr+"\n")
			h.reindex()
			doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
			got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "value", 0))
			if name := fiveMGLMTestTypeName(got); name != tt.want {
				t.Fatalf("%s inferred as %q (%#v), want %q", tt.expr, name, got, tt.want)
			}
		})
	}
}

func TestFiveMGLMIndexInferenceIsResourceScoped(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("plain.lua", `local m = mat3x2()
local column = m[2]
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/plain.lua")]
	got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "column", 0))
	if got.CustomName != "" || got.Basics != TypeUnknown {
		t.Fatalf("plain Lua matrix indexing inferred as %#v, want unknown", got)
	}
}

func TestFiveMGLMIndexDoesNotAssumeShadowedConstructor(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `local function mat3() return {} end
local value = mat3()[1]
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "value", 0))
	if got.CustomName == "Vector3" {
		t.Fatalf("shadowed constructor inferred GLM matrix column: %#v", got)
	}
}

func TestFiveMGLMIndexDoesNotAssumeUnrelatedMethod(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `local factory = { mat3 = function() return {} end }
local value = factory.mat3()[1]
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "value", 0))
	if got.CustomName == "Vector3" {
		t.Fatalf("unrelated method inferred GLM matrix column: %#v", got)
	}
}

func TestFiveMGLMIndexDoesNotTreatQualifiedMethodsAsBuiltins(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `local foo = {
  vec = function(...) return "text" end,
  vec3 = function(...) return "text" end,
  polygon = function(...) return "text" end,
  nestedPolygon = { new = function(...) return "text" end },
}
local generic = foo.vec(1, 2)[1]
local suffixed = foo.vec3(1, 2, 3)[1]
local polygon = foo.polygon({})[1]
local polygonNew = foo.nestedPolygon.new({})[1]
local globalPolygon = polygon({})[1]
local nestedGLM = glm.custom.vec3(1, 2, 3)[1]
local polygonCall = glm.polygon({})[1]
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	for _, name := range []string{"generic", "suffixed", "polygon", "polygonNew", "globalPolygon", "nestedGLM", "polygonCall"} {
		t.Run(name, func(t *testing.T) {
			got := doc.InferType(findIdentByOccurrence(t, doc.Tree, name, 0))
			if got.Basics&TypeNumber != 0 || got.CustomName != "" {
				t.Fatalf("qualified user call %s inferred as %#v, want no GLM type", name, got)
			}
		})
	}
}

func TestFiveMGLMIndexDoesNotAssumeShadowedGLMNamespace(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `local glm = { polygon = function(...) return "text" end }
local result = glm.polygon({})[1]
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
	if got.Basics&TypeNumber != 0 || got.CustomName != "" {
		t.Fatalf("shadowed glm.polygon inferred as %#v, want no GLM type", got)
	}
}
