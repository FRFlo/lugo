package lsp

import "testing"

func TestFrameworkAdapterSelectionIsVersionedAndOptIn(t *testing.T) {
	adapters := selectFrameworkAdapters([]FrameworkAdapterConfig{
		{Name: "ESX", Version: "1.2.0"},
		{Name: "qbcore", Version: "missing"},
		{Name: "unknown", Version: "1"},
		{Name: "ox", Version: "1", Enabled: new(false)},
	})
	if len(adapters) != 1 || adapters[0].Name != "esx" {
		t.Fatalf("selected adapters = %#v, want only ESX v1", adapters)
	}
}

func TestFrameworkAdapterPacksDeclareNeutralMetadata(t *testing.T) {
	for _, pack := range frameworkAdapterPacks {
		if pack.Name == "" || pack.Version == "" || len(pack.Symbols) == 0 {
			t.Fatalf("invalid adapter pack: %#v", pack)
		}
		for _, symbol := range pack.Symbols {
			if symbol.Name == "" || symbol.Detail == "" {
				t.Fatalf("invalid %s symbol: %#v", pack.Name, symbol)
			}
		}
	}
}

func TestUnknownFrameworkDoesNotProduceMetadata(t *testing.T) {
	if got := adapterCompletions(selectFrameworkAdapters([]FrameworkAdapterConfig{{Name: "not-a-framework"}})); len(got) != 0 {
		t.Fatalf("unknown framework completions = %#v", got)
	}
}
