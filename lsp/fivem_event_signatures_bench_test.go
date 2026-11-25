package lsp

import "testing"

func BenchmarkFiveMTriggerSignatures(b *testing.B) {
	h := newFiveMFixtureHarness(b, "resource_events")
	doc := h.docForMarker("server_hover")
	id := doc.Tree.NodeAt(uint32(h.requireMarker("server_hover").Offset))
	callID := doc.Tree.Nodes[id].Parent
	trigger, ok := fiveMTriggerForCall(doc, callID)
	if !ok {
		b.Fatal("trigger not found")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = h.server.fiveMTriggerSignatures(doc, callID, trigger, "TriggerClientEvent")
	}
}

func BenchmarkFiveMEventCompletion(b *testing.B) {
	h := newFiveMFixtureHarness(b, "resource_events")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = h.completion("server_hover")
	}
}
