package lsp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLuaDocMethodOverloadSignatureHelpHandlesColonAndDotCalls(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Vector
local Vector = {}

---@overload fun(self: Vector, min: number, max: number): Vector
---@overload fun(self: Vector, min: Vector, max: Vector): Vector
---@param min number|Vector
---@param max number|Vector
---@return Vector
function Vector:clamp(min, max) end

---@overload fun(t: number): Vector
---@overload fun(target: Vector, t: number): Vector
---@return Vector
function Vector:lerp(target, t) end
`)
	h.writeWorkspaceFile("use.lua", `---@type Vector
local v = {}
v:clamp(v, --[[@colon_method_overload]]v)
Vector.clamp(v, 1, --[[@dot_method_overload]]2)
v:lerp(v, --[[@colon_implicit_self_overload]]1)
Vector.lerp(v, v, --[[@dot_synthetic_self_overload]]1)
`)
	h.reindex()

	colon := h.signatureHelp("colon_method_overload")
	if colon == nil || len(colon.Signatures) != 2 {
		t.Fatalf("colon method overload signatures = %#v, want both overloads", colon)
	}
	if colon.ActiveSignature != 1 {
		t.Fatalf("colon active signature = %d, want vector bounds overload", colon.ActiveSignature)
	}
	if colon.ActiveParameter != 1 {
		t.Fatalf("colon active parameter = %d, want second explicit argument", colon.ActiveParameter)
	}
	if strings.Contains(colon.Signatures[0].Label, "self:") || strings.Contains(colon.Signatures[1].Label, "self:") {
		t.Fatalf("colon signatures should omit implicit self: %#v", colon.Signatures)
	}

	dot := h.signatureHelp("dot_method_overload")
	if dot == nil || len(dot.Signatures) != 2 {
		t.Fatalf("dot method overload signatures = %#v, want both overloads", dot)
	}
	if dot.ActiveSignature != 0 {
		t.Fatalf("dot active signature = %d, want numeric bounds overload", dot.ActiveSignature)
	}
	if dot.ActiveParameter != 2 {
		t.Fatalf("dot active parameter = %d, want third explicit argument", dot.ActiveParameter)
	}
	if !strings.Contains(dot.Signatures[0].Label, "self: Vector") {
		t.Fatalf("dot signature should retain explicit self: %#v", dot.Signatures[0])
	}

	colonImplicit := h.signatureHelp("colon_implicit_self_overload")
	if colonImplicit == nil || colonImplicit.ActiveSignature != 1 || strings.Contains(colonImplicit.Signatures[1].Label, "self:") {
		t.Fatalf("colon overload without annotated self = %#v, want selected second overload without self", colonImplicit)
	}
	dotSynthetic := h.signatureHelp("dot_synthetic_self_overload")
	if dotSynthetic == nil || dotSynthetic.ActiveSignature != 1 || !strings.Contains(dotSynthetic.Signatures[1].Label, "self: Vector") {
		t.Fatalf("dot overload without annotated self = %#v, want selected second overload with explicit receiver", dotSynthetic)
	}
}

func BenchmarkLuaDocMethodOverloadSignatureHelp(b *testing.B) {
	h := newFiveMFixtureHarnessWithoutIndex(b)
	h.writeWorkspaceFile("types.lua", `---@class Vector
local Vector = {}
---@overload fun(self: Vector, min: number, max: number): Vector
---@overload fun(self: Vector, min: Vector, max: Vector): Vector
---@return Vector
function Vector:clamp(min, max) end
`)
	h.writeWorkspaceFile("use.lua", `---@type Vector
local v = {}
v:clamp(v, --[[@method_overload_benchmark]]v)
`)
	h.reindex()
	marker := h.requireMarker("method_overload_benchmark")
	params, err := json.Marshal(SignatureHelpParams{
		TextDocument: TextDocumentIdentifier{URI: marker.URI},
		Position:     marker.Position,
	})
	if err != nil {
		b.Fatal(err)
	}
	request := Request{RPC: "2.0", ID: 1, Params: params}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h.resetRPC()
		h.server.handleSignatureHelp(request)
	}
}
