package lsp

import "testing"

func TestFiveMContractModelPreservesLocationConfidenceAndMetadata(t *testing.T) {
	link := FiveMContractLink{
		From: FiveMContractSymbol{
			Name: "open_menu", Kind: FiveMContractNUI,
			Location: FiveMContractLocation{URI: "file:///resource/client.lua", Range: Range{Start: Position{Line: 2, Character: 20}}},
			Profile:  FiveMProfileClient, Direction: FiveMContractLuaToJS,
		},
		To: FiveMContractSymbol{
			Name: "open_menu", Kind: FiveMContractNUI,
			Location: FiveMContractLocation{URI: "file:///resource/ui/app.js", Range: Range{Start: Position{Line: 4, Character: 8}}},
			Profile:  FiveMProfileClient, Direction: FiveMContractLuaToJS,
		},
		Confidence: FiveMContractConfidenceHigh,
	}
	if link.From.Name != link.To.Name || link.From.Location.URI == link.To.Location.URI {
		t.Fatalf("contract did not connect distinct source locations: %#v", link)
	}
	if link.Confidence != FiveMContractConfidenceHigh || link.From.Profile != FiveMProfileClient || link.From.Direction != FiveMContractLuaToJS {
		t.Fatalf("contract metadata was not preserved: %#v", link)
	}
}

func TestFiveMManifestContractAdapterUsesValueLocation(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nexport 'ping'\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/fxmanifest.lua")]
	got := fiveMManifestContractSymbols(doc, FiveMContractExport, "ping")
	if len(got) != 1 || got[0].Location.URI == "" || got[0].Location.Range.Start.Line != 2 {
		t.Fatalf("manifest adapter = %#v, want one value-located export", got)
	}
}
