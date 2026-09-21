package lsp

import (
	"regexp"
	"strings"

	"github.com/coalaura/lugo/ast"
)

// fiveMNUIContractLinks adapts the existing literal NUI scanner into the
// shared model. It is intentionally read-only; diagnostics remain unchanged.
func (s *Server) fiveMNUIContractLinks(doc *Document) []FiveMContractLink {
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
					from := FiveMContractSymbol{
						Name: handler.name, Kind: FiveMContractNUI,
						Location: fiveMContractLocation(lua.URI, lua.Source(), match[2], match[3]),
						Profile:  s.getDocumentFiveMProfile(lua).Kind, Direction: FiveMContractLuaToJS,
					}
					to := FiveMContractSymbol{
						Name: handler.name, Kind: FiveMContractNUI,
						Location: fiveMContractLocation(file.uri, file.src, handler.offset, handler.offset+len(handler.name)),
						Profile:  s.getDocumentFiveMProfile(lua).Kind, Direction: FiveMContractLuaToJS,
					}
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

// fiveMExportContractSymbols adapts parsed Lua exports while preserving the
// existing export bridge as the source of truth.
func fiveMExportContractSymbols(doc *Document) []FiveMContractSymbol {
	if doc == nil || doc.Tree == nil {
		return nil
	}
	profile := doc.FiveMProfile.Kind
	out := make([]FiveMContractSymbol, 0, len(doc.FiveMLuaExports))
	for _, export := range doc.FiveMLuaExports {
		if export.Name == "" || export.NodeID == ast.InvalidNode || int(export.NodeID) >= len(doc.Tree.Nodes) {
			continue
		}
		node := doc.Tree.Nodes[export.NodeID]
		out = append(out, FiveMContractSymbol{Name: export.Name, Kind: FiveMContractExport, Location: fiveMContractLocation(doc.URI, doc.Source(), int(node.Start), int(node.End)), Profile: profile, Direction: FiveMContractLuaToHost})
	}
	return out
}

// fiveMEventContractSymbols adapts the event records already collected during
// parsing. Dynamic event names are intentionally absent from FiveMEvents.
func fiveMEventContractSymbols(doc *Document) []FiveMContractSymbol {
	if doc == nil || doc.Tree == nil {
		return nil
	}
	profile := doc.FiveMProfile.Kind
	out := make([]FiveMContractSymbol, 0, len(doc.FiveMEvents))
	for _, event := range doc.FiveMEvents {
		if event.Name == "" || event.NodeID == ast.InvalidNode || int(event.NodeID) >= len(doc.Tree.Nodes) {
			continue
		}
		direction := FiveMContractHostToLua
		switch event.Kind {
		case FiveMEventTriggerLocal, FiveMEventTriggerServer, FiveMEventTriggerClient:
			direction = FiveMContractLuaToHost
		}
		node := doc.Tree.Nodes[event.NodeID]
		out = append(out, FiveMContractSymbol{Name: event.Name, Kind: FiveMContractEvent, Location: fiveMContractLocation(doc.URI, doc.Source(), int(node.Start), int(node.End)), Profile: profile, Direction: direction})
	}
	return out
}

var fiveMConvarContractRE = regexp.MustCompile(`(?m)\b(SetConvarReplicated|SetConvarServerInfo|SetConvar|GetConvarBool|GetConvarInt|GetConvarFloat|GetConvar)\s*\(\s*['"]([^'"]+)['"]`)

// fiveMConvarContractSymbols is a lightweight adapter for literal convar
// calls. Detailed type/default diagnostics remain in fivem_convars.go.
func fiveMConvarContractSymbols(doc *Document) []FiveMContractSymbol {
	if doc == nil {
		return nil
	}
	out := []FiveMContractSymbol{}
	for _, match := range fiveMConvarContractRE.FindAllSubmatchIndex(doc.Source(), -1) {
		name := string(doc.Source()[match[4]:match[5]])
		direction := FiveMContractLuaToHost
		if strings.HasPrefix(string(doc.Source()[match[0]:match[1]]), "Get") {
			direction = FiveMContractHostToLua
		}
		out = append(out, FiveMContractSymbol{Name: name, Kind: FiveMContractConvar, Location: fiveMContractLocation(doc.URI, doc.Source(), match[4], match[5]), Profile: doc.FiveMProfile.Kind, Direction: direction})
	}
	return out
}

// linkFiveMContractSymbols joins literal names while retaining both source
// locations. It is shared by adapters and does not diagnose unmatched names.
func linkFiveMContractSymbols(from, to []FiveMContractSymbol, confidence FiveMContractConfidence) []FiveMContractLink {
	if len(from) == 0 || len(to) == 0 {
		return nil
	}
	var links []FiveMContractLink
	for _, left := range from {
		for _, right := range to {
			if left.Name == right.Name && left.Kind == right.Kind {
				links = append(links, FiveMContractLink{From: left, To: right, Confidence: confidence})
			}
		}
	}
	return links
}

// fiveMManifestContractSymbols exposes literal manifest declarations (useful
// to consumers that want one model for Lua, web assets, and configuration).
func fiveMManifestContractSymbols(doc *Document, kind FiveMContractKind, name string) []FiveMContractSymbol {
	if doc == nil || !doc.IsFiveMManifest || doc.Server == nil {
		return nil
	}
	res := doc.Server.parseFiveMManifest(doc)
	if res == nil || res.Manifest == nil {
		return nil
	}
	var out []FiveMContractSymbol
	for _, entry := range res.Manifest.Entries {
		if name != "" && !strings.EqualFold(entry.Value, name) {
			continue
		}
		out = append(out, FiveMContractSymbol{Name: entry.Value, Kind: kind, Location: FiveMContractLocation{URI: entry.SourceURI, Range: entry.ValueRange}, Profile: FiveMProfileManifest, Direction: FiveMContractLuaToHost})
	}
	return out
}
