package lsp

import (
	"fmt"
	"github.com/FRFlo/lugo/ast"
	"strings"
)

// eventDirectionQuickFix only rewrites server-side TriggerServerEvent. A
// TriggerClientEvent target is an additional argument and cannot be left in
// place when converting to TriggerEvent.
func (s *Server) eventDirectionQuickFix(doc *Document, uri string, diag Diagnostic) (CodeAction, bool) {
	if doc == nil || doc.Tree == nil || doc.Resolver == nil || diag.Code != "fivem-event-direction" {
		return CodeAction{}, false
	}
	profile := s.getDocumentFiveMProfile(doc)
	if !profile.HasResource() || profile.Env() != EnvServer {
		return CodeAction{}, false
	}
	for _, ev := range doc.FiveMEvents {
		if ev.Kind != FiveMEventTriggerServer || ev.Name == "" || ev.Name == "*" || ev.NodeID == ast.InvalidNode || int(ev.NodeID) >= len(doc.Tree.Nodes) || getNodeRange(doc.Tree, ev.NodeID) != diag.Range {
			continue
		}
		if _, builtin := EventsBuiltin[ev.Name]; builtin {
			return CodeAction{}, false
		}
		if diag.Message != "TriggerServerEvent in server script — use TriggerEvent for local events" {
			return CodeAction{}, false
		}
		id := ev.NodeID
		for id != ast.InvalidNode && int(id) < len(doc.Tree.Nodes) {
			n := doc.Tree.Nodes[id]
			if n.Kind == ast.KindCallExpr {
				if n.Left == ast.InvalidNode || int(n.Left) >= len(doc.Tree.Nodes) {
					return CodeAction{}, false
				}
				callee := doc.Tree.Nodes[n.Left]
				if callee.Kind != ast.KindIdent || string(doc.Source()[callee.Start:callee.End]) != "TriggerServerEvent" || int(n.Left) >= len(doc.Resolver.References) || doc.Resolver.References[n.Left] != ast.InvalidNode {
					return CodeAction{}, false
				}
				return CodeAction{Title: "Use 'TriggerEvent' for a local event", Kind: "quickfix", Diagnostics: []Diagnostic{diag}, Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{uri: {{Range: getNodeRange(doc.Tree, n.Left), NewText: "TriggerEvent"}}}}}, true
			}
			id = n.Parent
		}
	}
	return CodeAction{}, false
}

// unknownEventQuickFix only suggests a trigger in the same resource and
// compatible execution environment; event names are static quoted literals.
func (s *Server) unknownEventQuickFix(doc *Document, uri string, diag Diagnostic) (CodeAction, bool) {
	if doc == nil || doc.Tree == nil || diag.Code != "fivem-unknown-event" {
		return CodeAction{}, false
	}
	profile := s.getDocumentFiveMProfile(doc)
	if !profile.HasResource() || (profile.Env() != EnvClient && profile.Env() != EnvServer) {
		return CodeAction{}, false
	}
	var handler *FiveMEventInfo
	for i := range doc.FiveMEvents {
		ev := &doc.FiveMEvents[i]
		if ev.Kind == FiveMEventAddHandler && ev.NodeID != ast.InvalidNode && int(ev.NodeID) < len(doc.Tree.Nodes) && getNodeRange(doc.Tree, ev.NodeID) == diag.Range {
			handler = ev
			break
		}
	}
	if handler == nil || handler.Name == "" || handler.Name == "*" || diag.Message != fmt.Sprintf("Unknown event '%s' — no trigger or built-in definition found", handler.Name) {
		return CodeAction{}, false
	}
	if _, ok := EventsBuiltin[handler.Name]; ok {
		return CodeAction{}, false
	}
	for _, other := range s.Documents {
		if other != nil {
			if s.getDocumentFiveMProfile(other).ResourceRoot != profile.ResourceRoot {
				continue
			}
			for _, ev := range other.FiveMEvents {
				if (ev.Kind == FiveMEventTriggerLocal || ev.Kind == FiveMEventTriggerServer || ev.Kind == FiveMEventTriggerClient) && ev.Name == handler.Name {
					return CodeAction{}, false
				}
			}
		}
	}
	var match string
	for _, other := range s.Documents {
		if other == nil {
			continue
		}
		p := s.getDocumentFiveMProfile(other)
		if p.ResourceRoot != profile.ResourceRoot {
			continue
		}
		for _, ev := range other.FiveMEvents {
			compatible := ev.Kind == FiveMEventTriggerLocal && p.Env() == profile.Env() || ev.Kind == FiveMEventTriggerServer && profile.Env() == EnvServer && p.Env() == EnvClient || ev.Kind == FiveMEventTriggerClient && profile.Env() == EnvClient && p.Env() == EnvServer
			if !compatible || ev.Name == "" || levenshteinFast(strings.ToLower(handler.Name), strings.ToLower(ev.Name), 2) > 2 {
				continue
			}
			if match != "" && match != ev.Name {
				return CodeAction{}, false
			}
			match = ev.Name
		}
	}
	if match == "" {
		return CodeAction{}, false
	}
	for _, c := range match {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == ':' || c == '-' || c == '.') {
			return CodeAction{}, false
		}
	}
	n := doc.Tree.Nodes[handler.NodeID]
	raw := doc.Source()[n.Start:n.End]
	if n.Kind != ast.KindString || len(raw) < 2 || (raw[0] != '\'' && raw[0] != '"') || raw[len(raw)-1] != raw[0] {
		return CodeAction{}, false
	}
	replacement := string(raw[0]) + match + string(raw[0])
	return CodeAction{Title: fmt.Sprintf("Change event to '%s'", match), Kind: "quickfix", Diagnostics: []Diagnostic{diag}, Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{uri: {{Range: diag.Range, NewText: replacement}}}}}, true
}

// unregisteredEventQuickFix chooses a receiving document only when it contains
// the sole matching handler in the same resource and runtime.
func (s *Server) unregisteredEventQuickFix(doc *Document, uri string, diag Diagnostic) (CodeAction, bool) {
	if doc == nil || doc.Tree == nil || diag.Code != "fivem-unregistered-net-event" {
		return CodeAction{}, false
	}
	profile := s.getDocumentFiveMProfile(doc)
	if !profile.HasResource() {
		return CodeAction{}, false
	}
	var trigger *FiveMEventInfo
	for i := range doc.FiveMEvents {
		ev := &doc.FiveMEvents[i]
		if (ev.Kind == FiveMEventTriggerServer || ev.Kind == FiveMEventTriggerClient) && ev.NodeID != ast.InvalidNode && int(ev.NodeID) < len(doc.Tree.Nodes) && getNodeRange(doc.Tree, ev.NodeID) == diag.Range {
			trigger = ev
			break
		}
	}
	if trigger == nil || trigger.Name == "" || diag.Message != fmt.Sprintf("Network event '%s' triggered but no RegisterNetEvent found in the indexed workspace", trigger.Name) {
		return CodeAction{}, false
	}
	if b, ok := lookupFiveMBuiltinEvent(trigger.Name); ok && b.NetworkBuiltin {
		return CodeAction{}, false
	}
	for _, c := range trigger.Name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == ':' || c == '-' || c == '.') {
			return CodeAction{}, false
		}
	}
	targetEnv := EnvServer
	if trigger.Kind == FiveMEventTriggerClient {
		targetEnv = EnvClient
	}
	var target *Document
	handlers := 0
	for _, other := range s.Documents {
		if other == nil {
			continue
		}
		p := s.getDocumentFiveMProfile(other)
		for _, ev := range other.FiveMEvents {
			if ev.Name != trigger.Name {
				continue
			}
			if ev.Kind == FiveMEventRegisterNet && p.ResourceRoot == profile.ResourceRoot && (p.Env() == targetEnv || p.Env() == EnvShared) {
				return CodeAction{}, false
			}
			if ev.Kind == FiveMEventAddHandler && p.ResourceRoot == profile.ResourceRoot && p.Env() == targetEnv {
				handlers++
				target = other
			}
		}
	}
	if handlers != 1 || target == nil || target.Tree == nil || strings.HasPrefix(string(target.Source()), "#!") || strings.HasPrefix(string(target.Source()), "\xef\xbb\xbf") {
		return CodeAction{}, false
	}
	return CodeAction{Title: fmt.Sprintf("Register network event '%s'", trigger.Name), Kind: "quickfix", Diagnostics: []Diagnostic{diag}, Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{target.URI: {{Range: Range{}, NewText: fmt.Sprintf("RegisterNetEvent('%s')\n", trigger.Name)}}}}}, true
}
