package lsp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func BenchmarkHoverDetail(b *testing.B) {
	h := newFiveMFixtureHarnessWithoutIndex(b)
	var src strings.Builder
	src.WriteString("---@class Nested\n---@field label string\n---@field enabled boolean\nlocal Nested = {}\n")
	src.WriteString("---@class Record\n")
	for i := range 40 {
		fmt.Fprintf(&src, "---@field field%d Nested\n", i)
	}
	src.WriteString("local Record = {}\n---@type Record\nlocal value = {}\nlocal x = val--[[@record]]ue\n")
	src.WriteString("---@param input Record[]\n---@return table<string, Record>\nlocal function process(input) return {} end\nlocal p = pro--[[@function]]cess\n")
	src.WriteString("local plain = 1\nlocal n = pla--[[@primitive]]in\nlocal object = { first = { second = { value = true } }, count = 2 }\nlocal o = obj--[[@table]]ect\n")
	h.writeWorkspaceFile("bench.lua", src.String())
	h.reindex()
	for _, tc := range []struct {
		name, marker string
		depth        int
	}{
		{"primitive", "primitive", 0}, {"table-summary", "table", 0}, {"table-full", "table", -1},
		{"class-summary", "record", 0}, {"class-full", "record", -1}, {"function-full", "function", -1},
	} {
		b.Run(tc.name, func(b *testing.B) {
			marker := h.requireMarker(tc.marker)
			params, err := json.Marshal(struct {
				TextDocumentPositionParams
				Depth int `json:"lugoTableDepth"`
			}{TextDocumentPositionParams: TextDocumentPositionParams{TextDocument: TextDocumentIdentifier{URI: marker.URI}, Position: marker.Position}, Depth: tc.depth})
			if err != nil {
				b.Fatal(err)
			}
			req := Request{RPC: "2.0", ID: 1, Params: params}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				h.resetRPC()
				h.server.handleHover(req)
			}
		})
	}
}
