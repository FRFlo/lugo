package lsp

import (
	"reflect"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestFiveMConvarPrefilterPreservesRegexMatches(t *testing.T) {
	for _, source := range []string{
		`SetConvar('name', 'value')`,
		`GetConvarInt ( "name", 0 )`,
		`-- SetConvarReplicated('commented', 'value')`,
		`local text = 'Convar but no call'`,
		`SetConVar('different case', 'value')`,
	} {
		doc := &Document{Tree: ast.NewTree([]byte(source))}
		want := []FiveMContractSymbol{}
		for _, match := range fiveMConvarContractRE.FindAllSubmatchIndex(doc.Source(), -1) {
			name := string(doc.Source()[match[4]:match[5]])
			direction := FiveMContractLuaToHost
			if len(doc.Source()[match[0]:match[1]]) >= 3 && string(doc.Source()[match[0]:match[0]+3]) == "Get" {
				direction = FiveMContractHostToLua
			}
			want = append(want, FiveMContractSymbol{Name: name, Kind: FiveMContractConvar, Location: fiveMContractLocation(doc.URI, doc.Source(), match[4], match[5]), Profile: doc.FiveMProfile.Kind, Direction: direction})
		}
		if got := fiveMConvarContractSymbols(doc); !reflect.DeepEqual(got, want) {
			t.Errorf("convar adapter for %q = %#v, want regex result %#v", source, got, want)
		}
	}
}

func TestFiveMNUIContractRequestCachePreservesLinks(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nui_page 'web/index.html'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("resource/client.lua", "RegisterNUICallback('save', function() end)\nSendNUIMessage({ action = 'save' })\n")
	h.writeWorkspaceFile("resource/web/index.html", `<script>fetch('https://${GetParentResourceName()}/save')</script>`)
	h.reindex()
	var docs []*Document
	for _, doc := range h.server.Documents {
		if doc != nil && h.server.getDocResourceRoot(doc) != "" {
			docs = append(docs, doc)
		}
	}
	cache := newFiveMNUIContractScanCache()
	for _, doc := range docs {
		want := fiveMNUIContractLinksReference(h.server, doc)
		got := h.server.fiveMNUIContractLinksWithCache(doc, cache)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("cached NUI links for %s = %#v, want %#v", doc.URI, got, want)
		}
	}
}

func fiveMNUIContractLinksReference(s *Server, doc *Document) []FiveMContractLink {
	if s == nil || doc == nil || s.getDocResourceRoot(doc) == "" {
		return nil
	}
	var links []FiveMContractLink
	for _, file := range s.nuiResourceFiles(doc) {
		for _, handler := range file.names {
			for _, lua := range s.Documents {
				if lua == nil || s.getDocResourceRoot(lua) != s.getDocResourceRoot(doc) {
					continue
				}
				for _, match := range nuiCallbackLuaRE.FindAllSubmatchIndex(lua.Source(), -1) {
					if string(lua.Source()[match[2]:match[3]]) != handler.name {
						continue
					}
					from := FiveMContractSymbol{Name: handler.name, Kind: FiveMContractNUI, Location: fiveMContractLocation(lua.URI, lua.Source(), match[2], match[3]), Profile: s.getDocumentFiveMProfile(lua).Kind, Direction: FiveMContractLuaToJS}
					to := FiveMContractSymbol{Name: handler.name, Kind: FiveMContractNUI, Location: fiveMContractLocation(file.uri, file.src, handler.offset, handler.offset+len(handler.name)), Profile: s.getDocumentFiveMProfile(lua).Kind, Direction: FiveMContractLuaToJS}
					links = append(links, FiveMContractLink{From: from, To: to, Confidence: FiveMContractConfidenceHigh})
				}
				for _, match := range nuiMessageRE.FindAllSubmatchIndex(lua.Source(), -1) {
					if string(lua.Source()[match[4]:match[5]]) != handler.name {
						continue
					}
					from := FiveMContractSymbol{Name: handler.name, Kind: FiveMContractNUI, Location: fiveMContractLocation(lua.URI, lua.Source(), match[4], match[5]), Profile: s.getDocumentFiveMProfile(lua).Kind, Direction: FiveMContractLuaToJS}
					to := FiveMContractSymbol{Name: handler.name, Kind: FiveMContractNUI, Location: fiveMContractLocation(file.uri, file.src, handler.offset, handler.offset+len(handler.name)), Profile: s.getDocumentFiveMProfile(lua).Kind, Direction: FiveMContractLuaToJS}
					links = append(links, FiveMContractLink{From: from, To: to, Confidence: FiveMContractConfidenceHigh})
				}
			}
		}
	}
	return links
}
