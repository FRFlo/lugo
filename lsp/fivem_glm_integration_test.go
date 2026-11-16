package lsp

import (
	"sort"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestFiveMGLMConstructorAssignmentAndSpacedPolygonPath(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `local before = glm.vec3(1, 2, 3)
glm.vec3 = function(...) return "text" end
local reassigned = glm.vec3(1, 2, 3)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	var assignmentStart uint32
	for _, field := range doc.Resolver.FieldDefs {
		if field.PropHash == ast.HashBytes([]byte("vec3")) {
			assignmentStart = doc.Tree.Nodes[field.NodeID].Start
		}
	}
	if assignmentStart == 0 {
		t.Fatal("did not find glm.vec3 assignment")
	}
	var beforeFound, afterFound bool
	for _, node := range doc.Tree.Nodes {
		if node.Kind != ast.KindCallExpr || string(doc.nodeSource(node.Left)) != "glm.vec3" {
			continue
		}
		name, ok := doc.fiveMGLMCallName(node)
		if node.Start < assignmentStart {
			beforeFound = true
			if !ok || name != "vec3" {
				t.Fatalf("glm.vec3 before assignment classified as %q, %v; want vec3, true", name, ok)
			}
		} else {
			afterFound = true
			if ok {
				t.Fatalf("reassigned glm.vec3 call classified as built-in %q", name)
			}
		}
	}
	if !beforeFound || !afterFound {
		t.Fatalf("found calls before/after assignment = %v/%v", beforeFound, afterFound)
	}

	h2 := newFiveMFixtureHarnessWithoutIndex(t)
	h2.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h2.writeWorkspaceFile("resource/main.lua", `local spaced = glm . polygon . new({vec3(1, 2, 3)})[1]
`)
	h2.reindex()
	doc2 := h2.server.Documents[h2.server.pathToURI(h2.root+"/resource/main.lua")]
	if got := doc2.InferType(findIdentByOccurrence(t, doc2.Tree, "spaced", 0)); got.CustomName != "Vector3" {
		t.Fatalf("spaced glm.polygon.new result inferred as %#v, want Vector3", got)
	}
}

func TestFiveMGLMConstructorIndexAssignmentOrderAndEscapedKey(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `local other = {}
other["vec3"] = function(...) return "other" end
local before = glm.vec3(1, 2, 3)
glm["vec3"] = function(...) return "custom" end
local after = glm.vec3(1, 2, 3)
glm["v\x65c3"] = function(...) return "escaped" end
local escaped = glm.vec3(1, 2, 3)
local unaffected = glm.vec3(1, 2, 3)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	wantBuiltin := map[string]bool{"before": true, "after": false, "escaped": false, "unaffected": false}
	// Calls are associated with their preceding local variable declaration by source order.
	var calls []ast.Node
	for _, node := range doc.Tree.Nodes {
		if node.Kind == ast.KindCallExpr && string(doc.nodeSource(node.Left)) == "glm.vec3" {
			calls = append(calls, node)
		}
	}
	sort.Slice(calls, func(i, j int) bool { return calls[i].Start < calls[j].Start })
	orderedNames := []string{"before", "after", "escaped", "unaffected"}
	if len(calls) != len(orderedNames) {
		t.Fatalf("found %d constructor calls, want %d", len(calls), len(orderedNames))
	}
	for i, node := range calls {
		name, ok := doc.fiveMGLMCallName(node)
		want := wantBuiltin[orderedNames[i]]
		if ok != want || (ok && name != "vec3") {
			t.Errorf("%s call classified as %q, %v; want builtin=%v", orderedNames[i], name, ok, want)
		}
	}
}

func TestFiveMGLMConstructorIdentityTracksGlobalNamespaceReassignment(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `local before = glm.vec3(1, 2, 3)
local replacement = { vec3 = function(...) return "custom" end }
glm = replacement
local after = glm.vec3(1, 2, 3)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	var calls []ast.Node
	for _, node := range doc.Tree.Nodes {
		if node.Kind == ast.KindCallExpr && string(doc.nodeSource(node.Left)) == "glm.vec3" {
			calls = append(calls, node)
		}
	}
	sort.Slice(calls, func(i, j int) bool { return calls[i].Start < calls[j].Start })
	if len(calls) != 2 {
		t.Fatalf("found %d glm.vec3 calls, want 2", len(calls))
	}
	if name, ok := doc.fiveMGLMCallName(calls[0]); !ok || name != "vec3" {
		t.Fatalf("glm.vec3 before global reassignment classified as %q, %v", name, ok)
	}
	if name, ok := doc.fiveMGLMCallName(calls[1]); ok {
		t.Fatalf("glm.vec3 after global reassignment classified as builtin %q", name)
	}
}

func TestFiveMGLMConstructorIgnoresLocalNamespaceFieldWrites(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `do
  local glm = {}
  glm.vec3 = function(...) return "local custom" end
  local shadowed = glm.vec3(1, 2, 3)
end
local globalValue = glm.vec3(1, 2, 3)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	var calls []ast.Node
	for _, node := range doc.Tree.Nodes {
		if node.Kind == ast.KindCallExpr && string(doc.nodeSource(node.Left)) == "glm.vec3" {
			calls = append(calls, node)
		}
	}
	sort.Slice(calls, func(i, j int) bool { return calls[i].Start < calls[j].Start })
	if len(calls) != 2 {
		t.Fatalf("found %d glm.vec3 calls, want 2", len(calls))
	}
	if name, ok := doc.fiveMGLMCallName(calls[0]); ok {
		t.Fatalf("local glm.vec3 call classified as builtin %q", name)
	}
	if name, ok := doc.fiveMGLMCallName(calls[1]); !ok || name != "vec3" {
		t.Fatalf("global glm.vec3 after local block classified as %q, %v; want builtin", name, ok)
	}
}

func TestFiveMGLMMemberInference(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
client_script 'main.lua'
`)
	h.writeWorkspaceFile("resource/main.lua", `---@type Vector3
local v = {}
local x = v.x
local xy = v.xy
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	if got := inferNamedMember(t, doc, "x"); got.Basics != TypeNumber {
		t.Fatalf("vector component = %#v, want number", got)
	}
	if got := inferNamedMember(t, doc, "xy"); got.CustomName != "Vector2" {
		t.Fatalf("vector swizzle = %#v, want Vector2", got)
	}
}

func TestPlainLuaVector3ClassDoesNotAssumeGLM(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Vector3
---@field x string
local Vector3 = {}
`)
	h.writeWorkspaceFile("main.lua", `---@type Vector3
local v = {}
local value = v.x
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/main.lua")]
	if got := inferNamedMember(t, doc, "x"); got.Basics != TypeString {
		t.Fatalf("user Vector3.x = %#v, want string", got)
	}
}

func TestFiveMUserVectorClassFieldTakesPrecedence(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
client_scripts { 'types.lua', 'main.lua' }
`)
	h.writeWorkspaceFile("resource/types.lua", `---@class Vector3
---@field x string
local Vector3 = {}
`)
	h.writeWorkspaceFile("resource/main.lua", `---@type Vector3
local v = {}
local value = v.x
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	if got := inferNamedMember(t, doc, "x"); got.Basics != TypeString {
		t.Fatalf("user Vector3.x = %#v, want string", got)
	}
}

func TestFiveMGLMNumericIndexInference(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
client_script 'main.lua'
`)
	h.writeWorkspaceFile("resource/main.lua", `---@type Vector3
local vector = {}
local result = vector[2]
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	id := findIdentByOccurrence(t, doc.Tree, "result", 0)
	if got := doc.InferType(id); got.Basics != TypeNumber {
		t.Fatalf("vector numeric index = %#v, want number", got)
	}
}

func TestFiveMGLMGenericVectorConstructorDimensions(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `local scalar = vec(1)
local vector2 = vec(1, 2)
local vector3 = ivec(1, 2, 3)
local vector4 = bvec(1, 2, 3, 4)
local packed = vec({1, 2}, 3)
local packedVector = vec(vec2(1), 3)
local fixedScalar = vec1(1)
local integerScalar = ivec1(1)
local booleanFixedScalar = bvec1(true)
local booleanScalar = bvec(true)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	for _, tt := range []struct{ name, want string }{
		{"scalar", "number"}, {"vector2", "Vector2"}, {"vector3", "Vector3"},
		{"vector4", "Vector4"}, {"packed", "Vector3"}, {"fixedScalar", "number"},
		{"packedVector", "Vector3"}, {"integerScalar", "number"},
		{"booleanFixedScalar", "boolean"}, {"booleanScalar", "boolean"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := doc.InferType(findIdentByOccurrence(t, doc.Tree, tt.name, 0))
			if name := fiveMGLMTestTypeName(got); name != tt.want {
				t.Fatalf("%s inferred as %q (%#v), want %q", tt.name, name, got, tt.want)
			}
		})
	}
}

func TestFiveMGLMRuntimeConstructorAliasesAndNamespace(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `local globalAlias = vector(1, 2, 3)
local namespaceGeneric = glm.vec(1, 2, 3)
local namespaceFixed = glm.vec3(1, 2, 3)
local namespaceQuat = glm.quat()
local namespaceMat = glm.mat4()
local namespaceGenericMat = glm.mat()
local genericMat = mat()
local genericMatColumn = genericMat[1]
local inferredMat = mat(vec3(1, 2, 3), vec3(4, 5, 6))
local inferredMatColumn = inferredMat[1]
local copiedMat = mat(mat3x2())
local copiedMatColumn = copiedMat[1]
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	for _, tt := range []struct{ name, want string }{
		{"globalAlias", "Vector3"}, {"namespaceGeneric", "Vector3"},
		{"namespaceFixed", "Vector3"}, {"namespaceQuat", "Quat"},
		{"namespaceMat", "Matrix4x4"}, {"namespaceGenericMat", "Matrix4x4"},
		{"genericMat", "Matrix4x4"},
		{"genericMatColumn", "Vector4"}, {"inferredMat", "Matrix2x3"},
		{"inferredMatColumn", "Vector3"}, {"copiedMat", "Matrix3x2"},
		{"copiedMatColumn", "Vector2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := doc.InferType(findIdentByOccurrence(t, doc.Tree, tt.name, 0))
			if name := fiveMGLMTestTypeName(got); name != tt.want {
				t.Fatalf("%s inferred as %q (%#v), want %q", tt.name, name, got, tt.want)
			}
		})
	}
}

func TestFiveMGLMGenericVectorConstructorsRespectShadowing(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `local vec = function(a) return "shadow" end
local result = vec(1)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
	if got.CustomName == "Vector2" || got.CustomName == "Vector3" || got.CustomName == "Vector4" {
		t.Fatalf("shadowed vec inferred as built-in vector %#v", got)
	}
}
