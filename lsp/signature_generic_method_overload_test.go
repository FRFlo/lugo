package lsp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLuaDocGenericMethodOverloadSignatureHelpSelectsConcreteAndGeneric(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Box
local Box = {}
---@generic T
---@overload fun(self: Box, value: T): T
---@overload fun(self: Box, value: string): string
---@param value any
function Box:map(value) end
`)
	h.writeWorkspaceFile("use.lua", `---@type Box
local box = {}
box:map(123 --[[@generic_number]])
box:map("text" --[[@generic_string]])
`)
	h.reindex()

	for _, test := range []struct {
		marker string
		want   string
	}{
		{marker: "generic_number", want: "value: T"},
		{marker: "generic_string", want: "value: string"},
	} {
		t.Run(test.marker, func(t *testing.T) {
			help := h.signatureHelp(test.marker)
			if help == nil || len(help.Signatures) != 2 {
				t.Fatalf("signature help = %#v, want both method overloads", help)
			}
			if help.ActiveSignature < 0 || help.ActiveSignature >= len(help.Signatures) || !strings.Contains(help.Signatures[help.ActiveSignature].Label, test.want) {
				t.Fatalf("active signature = %#v, want %q", help, test.want)
			}
			if strings.Contains(help.Signatures[0].Label, "self:") || strings.Contains(help.Signatures[1].Label, "self:") {
				t.Fatalf("colon overload signatures expose implicit self: %#v", help.Signatures)
			}
		})
	}
}

func TestLuaDocBoundGenericMethodOverloadSignatureHelpRejectsOutOfBoundArgument(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Base
local Base = {}
---@class Child: Base
local Child = {}
---@class Box
local Box = {}
---@generic T: Base
---@overload fun(self: Box, value: T): T
---@overload fun(self: Box, value: string): string
---@param value any
function Box:map(value) end
`)
	h.writeWorkspaceFile("use.lua", `---@type Box
local box = {}
---@type string
local text = "text"
box:map(text --[[@bound_generic_string]])
`)
	h.reindex()

	help := h.signatureHelp("bound_generic_string")
	if help == nil || help.ActiveSignature != 1 || !strings.Contains(help.Signatures[help.ActiveSignature].Label, "value: string") {
		t.Fatalf("active signature = %#v, want unbounded string overload", help)
	}
}

func BenchmarkLuaDocGenericMethodOverloadSignatureHelp(b *testing.B) {
	h := newFiveMFixtureHarnessWithoutIndex(b)
	h.writeWorkspaceFile("types.lua", `---@class Box
local Box = {}
---@generic T
---@overload fun(self: Box, value: T): T
---@overload fun(self: Box, value: string): string
---@param value any
function Box:map(value) end
`)
	h.writeWorkspaceFile("use.lua", `---@type Box
local box = {}
box:map("text" --[[@generic_method_overload_benchmark]])
`)
	h.reindex()
	marker := h.requireMarker("generic_method_overload_benchmark")
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
