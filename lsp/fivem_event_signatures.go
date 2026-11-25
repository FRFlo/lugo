package lsp

import (
	"sort"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// fiveMTriggerSignatures describes the handlers reachable by this trigger.
func (s *Server) fiveMTriggerSignatures(doc *Document, callID ast.NodeID, trigger FiveMEventInfo, name string) []SignatureInformation {
	if doc == nil || doc.Tree == nil || !s.getDocumentFiveMProfile(doc).HasResource() {
		return nil
	}
	var signatures []SignatureInformation
	seen := map[string]bool{}
	for _, indexed := range s.fiveMEventsNamed(trigger.Name) {
		target, registration := s.Documents[indexed.uri], indexed.event
		if target == nil || target.Tree == nil || (registration.Kind != FiveMEventAddHandler && registration.Kind != FiveMEventRegisterNet) {
			continue
		}
		kind := registration.Kind
		if kind == FiveMEventAddHandler && target.hasFiveMNetworkRegistration(registration.Name) {
			kind = FiveMEventRegisterNet
		}
		if !fiveMEventTriggerMatches(s.getDocumentFiveMProfile(doc).Env(), s.getDocumentFiveMProfile(target).Env(), trigger.Kind, kind) || !s.fiveMEventResourcesCompatible(doc, target, callID, trigger.Kind) {
			continue
		}
		owner, id := target.fiveMEventHandler(registration.HandlerID)
		if owner == nil || id == ast.InvalidNode || int(id) >= len(owner.Tree.Nodes) {
			continue
		}
		fn := owner.Tree.Nodes[id]
		labels := []string{"eventName: string"}
		if trigger.Kind == FiveMEventTriggerClient {
			labels = append(labels, "target: number|string")
		}
		if trigger.ExtraPayloadArgs != 0 {
			labels = append(labels, "bytesPerSecond: number")
		}
		annotation := fiveMFunctionAnnotation(owner, id)
		for i := uint32(0); i < fn.Count && fn.Extra+i < uint32(len(owner.Tree.ExtraList)); i++ {
			paramID := owner.Tree.ExtraList[fn.Extra+i]
			if int(paramID) >= len(owner.Tree.Nodes) {
				continue
			}
			label := string(owner.nodeSource(paramID))
			if annotation != nil {
				for _, p := range annotation.Params {
					if p.Name == label && p.Type != "" {
						label += ": " + p.Type
						break
					}
				}
			}
			labels = append(labels, label)
		}
		label := name + "(" + strings.Join(labels, ", ") + ")"
		if seen[label] {
			continue
		}
		seen[label] = true
		sig := SignatureInformation{Label: label}
		for _, p := range labels {
			sig.Parameters = append(sig.Parameters, ParameterInformation{Label: p})
		}
		signatures = append(signatures, sig)
	}
	sort.Slice(signatures, func(i, j int) bool { return signatures[i].Label < signatures[j].Label })
	return signatures
}

func fiveMTriggerForCall(doc *Document, callID ast.NodeID) (FiveMEventInfo, bool) {
	if doc == nil || doc.Tree == nil || callID == ast.InvalidNode || int(callID) >= len(doc.Tree.Nodes) {
		return FiveMEventInfo{}, false
	}
	call := doc.Tree.Nodes[callID]
	if call.Kind != ast.KindCallExpr || call.Count == 0 || call.Extra >= uint32(len(doc.Tree.ExtraList)) {
		return FiveMEventInfo{}, false
	}
	for _, ev := range doc.FiveMEvents {
		if ev.NodeID == doc.Tree.ExtraList[call.Extra] && (ev.Kind == FiveMEventTriggerLocal || ev.Kind == FiveMEventTriggerServer || ev.Kind == FiveMEventTriggerClient) {
			return ev, true
		}
	}
	return FiveMEventInfo{}, false
}
