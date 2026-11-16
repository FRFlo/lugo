package lsp

import (
	"encoding/json"
	"testing"
)

func BenchmarkLuaDocOverloadSignatureHelp(b *testing.B) {
	h := newFiveMFixtureHarnessWithoutIndex(b)
	h.writeWorkspaceFile("types.lua", `---@class Converter
---@overload fun(a: number, b: number): boolean
---@overload fun(a: number, b: string): string
local Converter = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Converter
local convert = {}
local result = convert(42, "x")--[[@signature_help_benchmark]]
`)
	h.reindex()

	marker := h.requireMarker("signature_help_benchmark")
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
