package lsp

import (
	"slices"
	"testing"
)

func TestGlobalIndexFieldKeysForReceivers(t *testing.T) {
	idx := NewGlobalIndex()
	want := []GlobalKey{{ReceiverHash: 1, PropHash: 2}, {ReceiverHash: 1, PropHash: 3}, {ReceiverHash: 7, PropHash: 5}}
	for i, key := range append(append([]GlobalKey{}, want...), GlobalKey{ReceiverHash: 9, PropHash: 6}, GlobalKey{ReceiverHash: 1}) {
		idx.AddSymbol("file:///a", GlobalIndexScopeShared, SymbolName(string(rune('a'+i))), &SymbolEntry{Key: key})
	}
	idx.AddSymbol("file:///b", GlobalIndexScopeShared, "duplicate", &SymbolEntry{Key: want[0]})
	got := idx.FieldKeysForReceivers(map[uint64]bool{1: true, 7: true})
	if len(got) != len(want) {
		t.Fatalf("keys = %#v, want %#v", got, want)
	}
	for _, key := range want {
		if !slices.Contains(got, key) {
			t.Errorf("missing key %#v in %#v", key, got)
		}
	}
}
