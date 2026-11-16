package lsp

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSymbolsForReceiversPreservesAllSymbolsOrder(t *testing.T) {
	idx := NewGlobalIndex()
	entries := []*SymbolEntry{
		{Key: GlobalKey{ReceiverHash: 11, PropHash: 101}},
		{Key: GlobalKey{ReceiverHash: 22, PropHash: 102}},
		{Key: GlobalKey{ReceiverHash: 11, PropHash: 103}},
		{Key: GlobalKey{ReceiverHash: 33, PropHash: 104}},
	}
	idx.AddSymbol("z-resource", GlobalIndexScopeShared, "z", entries[0])
	idx.AddSymbol("a-resource", GlobalIndexScopeServer, "b", entries[1])
	idx.AddSymbol("a-resource", GlobalIndexScopeClient, "c", entries[2])
	idx.AddSymbol("a-resource", GlobalIndexScopeClient, "a", entries[3])

	receivers := map[uint64]bool{11: true, 22: true}
	want := make([]*SymbolEntry, 0, 3)
	for _, entry := range idx.AllSymbols() {
		if entry != nil && receivers[entry.Key.ReceiverHash] {
			want = append(want, entry)
		}
	}
	got := idx.SymbolsForReceivers(receivers)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered snapshot = %v, want AllSymbols order %v", got, want)
	}
}

func TestSymbolsForReceiversEmptyAndNil(t *testing.T) {
	if got := (*GlobalIndex)(nil).SymbolsForReceivers(map[uint64]bool{1: true}); got != nil {
		t.Fatalf("nil index snapshot = %v, want nil", got)
	}
	if got := NewGlobalIndex().SymbolsForReceivers(nil); got != nil {
		t.Fatalf("empty receiver snapshot = %v, want nil", got)
	}
}

func TestSymbolsForReceiversReadsLiveLegacyTablesAndEntryFields(t *testing.T) {
	idx := NewGlobalIndex()
	entry := &SymbolEntry{Name: "legacy", Key: GlobalKey{ReceiverHash: 1}}
	idx.EnsureResource("file:///legacy").Shared["legacy"] = entry

	entry.Key.ReceiverHash = 2
	if got := idx.SymbolsForReceivers(map[uint64]bool{1: true}); len(got) != 0 {
		t.Fatalf("old receiver snapshot = %#v, want no entries after mutation", got)
	}
	if got := idx.SymbolsForReceivers(map[uint64]bool{2: true}); len(got) != 1 || got[0] != entry {
		t.Fatalf("live legacy receiver snapshot = %#v, want the mutated entry", got)
	}
}

func TestExportCompletionFilteredSnapshotPreservesItemAndResolveData(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_export_completion")
	items := h.completion("export_resource_completion").Items
	var ping *CompletionItem
	for i := range items {
		if items[i].Label == "ping" {
			ping = &items[i]
			break
		}
	}
	if ping == nil {
		t.Fatalf("export completion items = %+v, missing ping", items)
	}
	if ping.Kind != FunctionCompletion || ping.Detail != "client export" || ping.SortText != "1" || ping.InsertText != "ping()" || ping.InsertTextFormat != SnippetTextFormat {
		t.Fatalf("export ping completion = %+v, want client export function with ping() snippet", *ping)
	}
	data, ok := ping.Data.(map[string]any)
	wantURI := h.server.pathToURI(filepath.Join(h.root, "export_consumer", "completion.lua"))
	if !ok || data["uri"] != wantURI {
		t.Fatalf("export ping completion data = %#v, want completion document URI", ping.Data)
	}

	encoded, err := json.Marshal(*ping)
	if err != nil {
		t.Fatalf("marshal completion item: %v", err)
	}
	h.resetRPC()
	h.server.handleCompletionResolve(Request{RPC: "2.0", ID: 1, Params: encoded})
	var response struct {
		Result CompletionItem `json:"result"`
	}
	if err := json.Unmarshal(h.lastResponse(1), &response); err != nil {
		t.Fatalf("decode resolved completion item: %v", err)
	}
	if !reflect.DeepEqual(response.Result, *ping) {
		t.Fatalf("resolved completion item = %+v, want original item %+v", response.Result, *ping)
	}
}
