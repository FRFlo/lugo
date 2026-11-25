package lsp

import (
	"slices"
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
		params := make([]ParameterInformation, 1, int(fn.Count)+3)
		params[0].Label = "eventName: string"
		if trigger.Kind == FiveMEventTriggerClient {
			params = append(params, ParameterInformation{Label: "target: number|string"})
		}
		if trigger.ExtraPayloadArgs != 0 {
			params = append(params, ParameterInformation{Label: "bytesPerSecond: number"})
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
			params = append(params, ParameterInformation{Label: label})
		}
		var builder strings.Builder
		size := len(name) + 2
		for _, part := range params {
			size += len(part.Label)
		}
		if len(params) > 1 {
			size += 2 * (len(params) - 1)
		}
		builder.Grow(size)
		builder.WriteString(name)
		builder.WriteByte('(')
		for i, part := range params {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(part.Label)
		}
		builder.WriteByte(')')
		label := builder.String()
		if seen[label] {
			continue
		}
		seen[label] = true
		signatures = append(signatures, SignatureInformation{Label: label, Parameters: params})
	}
	slices.SortFunc(signatures, func(a, b SignatureInformation) int { return strings.Compare(a.Label, b.Label) })
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
