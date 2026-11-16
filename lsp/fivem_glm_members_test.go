package lsp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func BenchmarkFiveMGLMGenericVectorCallType(b *testing.B) {
	h := newFiveMFixtureHarnessWithoutIndex(b)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", "local value = vec(1, 2, 3)\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	var call ast.Node
	for _, n := range doc.Tree.Nodes {
		if n.Kind == ast.KindCallExpr && string(doc.nodeSource(n.Left)) == "vec" {
			call = n
			break
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		glmMemoryTypeSink = doc.fiveMGLMGenericVectorCallType(call, "vec")
	}
}

func BenchmarkFiveMGLMOverrideLookupScaling(b *testing.B) {
	for _, lines := range []int{64, 4096} {
		b.Run(fmt.Sprintf("lines_%d", lines), func(b *testing.B) {
			h := newFiveMFixtureHarnessWithoutIndex(b)
			h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
			var source strings.Builder
			for i := 0; i < lines; i++ {
				fmt.Fprintf(&source, "local noise%d = %d\n", i, i)
			}
			source.WriteString("do local glm = {}; glm.vec3 = function() return {} end end\nglm[\"v\\x65c3\"] = function() return {} end\nlocal value = glm.vec3()\n")
			h.writeWorkspaceFile("resource/main.lua", source.String())
			h.reindex()
			doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
			var call ast.Node
			for _, node := range doc.Tree.Nodes {
				if node.Kind == ast.KindCallExpr && string(doc.nodeSource(node.Left)) == "glm.vec3" {
					call = node
					break
				}
			}
			_ = h.server.newIndexWriteCandidates("vec3")
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_, _ = doc.fiveMGLMCallName(call)
			}
		})
	}
}

func BenchmarkFiveMGLMCallIdentity(b *testing.B) {
	h := newFiveMFixtureHarnessWithoutIndex(b)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `local a = vec3(1, 2, 3)
local alias = vector(1, 2, 3)
local gm = glm.vec3(1, 2, 3)
local gmat = glm.mat4()
local ggeneric = glm.mat()
local gquat = glm.quat()
local b = glm . polygon . new({})
glm.vec3 = function() return {} end
glm["v\x65c3"] = function() return {} end
local reassigned = glm.vec3()
local foo = { vec3 = function() return {} end }
local c = foo.vec3()
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	var calls []ast.Node
	for _, n := range doc.Tree.Nodes {
		if n.Kind == ast.KindCallExpr {
			calls = append(calls, n)
		}
	}
	_ = h.server.newIndexWriteCandidates("vec3")
	b.ReportAllocs()
	for b.Loop() {
		for _, call := range calls {
			_, _ = doc.fiveMGLMCallName(call)
		}
	}
}

func TestFiveMGLMKnownVectorMembers(t *testing.T) {
	tests := []struct {
		name string
		typ  string
		prop string
		want string
	}{
		{"vector component", "Vector3", "z", "number"},
		{"color alias", "Vector4", "a", "number"},
		{"dimension n", "Vector2", "n", "number"},
		{"dimension dim", "Quat", "dim", "number"},
		{"quat angle", "Quat", "angle", "number"},
		{"quat axis", "Quat", "axis", "Vector3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := fiveMGLMMemberType(TypeSet{CustomName: tt.typ}, tt.prop)
			if !ok || fiveMGLMTestTypeName(got) != tt.want {
				t.Fatalf("fiveMGLMMemberType(%s, %q) = %s, %v; want %s, true", tt.typ, tt.prop, fiveMGLMTestTypeName(got), ok, tt.want)
			}
		})
	}
}

func TestFiveMGLMSwizzleType(t *testing.T) {
	tests := []struct {
		name, receiver, swizzle, want string
		ok                            bool
	}{
		{"scalar", "Vector3", "r", "number", true},
		{"two components", "Vector3", "xy", "Vector2", true},
		{"four components", "Vector3", "xzyx", "Vector4", true},
		{"color swizzle is not builtin", "Vector4", "rgba", "", false},
		{"quat result depends on normalization", "Quat", "wxyz", "any", true},
		{"component outside dimension", "Vector2", "z", "", false},
		{"mixed aliases are not builtin", "Vector4", "xr", "", false},
		{"too long", "Vector4", "xyzwx", "", false},
		{"unknown receiver", "Other", "xy", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := fiveMGLMSwizzleType(TypeSet{CustomName: tt.receiver}, tt.swizzle)
			if ok != tt.ok || (ok && fiveMGLMTestTypeName(got) != tt.want) {
				t.Fatalf("fiveMGLMSwizzleType(%s, %q) = %s, %v; want %s, %v", tt.receiver, tt.swizzle, fiveMGLMTestTypeName(got), ok, tt.want, tt.ok)
			}
		})
	}
}

func fiveMGLMTestTypeName(t TypeSet) string {
	if t.CustomName != "" {
		return t.CustomName
	}
	if t.Basics == TypeNumber {
		return "number"
	}
	if t.Basics == TypeAny {
		return "any"
	}
	if t.Basics == TypeBoolean {
		return "boolean"
	}
	return "unknown"
}

func TestFiveMGLMDoesNotInventMatrixMembers(t *testing.T) {
	if _, ok := fiveMGLMMemberType(TypeSet{CustomName: "Matrix4x4"}, "x"); ok {
		t.Fatal("matrix has no documented named scalar members")
	}
	if _, ok := fiveMGLMSwizzleType(TypeSet{CustomName: "Matrix4x4"}, "xy"); ok {
		t.Fatal("matrix values must not be treated as vectors for swizzling")
	}
}

var glmMemoryTypeSink TypeSet
var glmMemoryOKSink bool

func TestFiveMGLMCommonAccessDoesNotAllocate(t *testing.T) {
	cases := []struct {
		name   string
		lookup func()
	}{
		{"member", func() { glmMemoryTypeSink, glmMemoryOKSink = fiveMGLMMemberType(TypeSet{CustomName: "Vector3"}, "x") }},
		{"swizzle", func() {
			glmMemoryTypeSink, glmMemoryOKSink = fiveMGLMSwizzleType(TypeSet{CustomName: "Vector3"}, "xyz")
		}},
		{"matrix", func() { glmMemoryTypeSink, glmMemoryOKSink = fiveMGLMIndexType(TypeSet{CustomName: "Matrix3x2"}, 1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := testing.AllocsPerRun(100, tc.lookup); got != 0 {
				t.Fatalf("access allocated %.1f times, want zero", got)
			}
			if !glmMemoryOKSink {
				t.Fatal("lookup failed")
			}
		})
	}
}

func BenchmarkFiveMGLMCommonAccess(b *testing.B) {
	for _, tc := range []struct {
		name   string
		lookup func()
	}{
		{"member", func() { glmMemoryTypeSink, glmMemoryOKSink = fiveMGLMMemberType(TypeSet{CustomName: "Vector3"}, "x") }},
		{"swizzle", func() {
			glmMemoryTypeSink, glmMemoryOKSink = fiveMGLMSwizzleType(TypeSet{CustomName: "Vector3"}, "xyz")
		}},
		{"matrix", func() { glmMemoryTypeSink, glmMemoryOKSink = fiveMGLMIndexType(TypeSet{CustomName: "Matrix3x2"}, 1) }},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				tc.lookup()
			}
		})
	}
}
