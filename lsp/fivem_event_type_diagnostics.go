package lsp

import (
	"fmt"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// fiveMCallbackArgumentDiagnostics checks only calls on a cross-document
// callback result, and only when all known callable alternatives have a
// concrete, non-overloaded primitive parameter contract.
func (s *Server) fiveMCallbackArgumentDiagnostics(doc *Document, call ast.Node) []Diagnostic {
	if doc == nil || doc.Tree == nil || (call.Kind != ast.KindCallExpr && call.Kind != ast.KindMethodCall) || call.Left == ast.InvalidNode || int(call.Left) >= len(doc.Tree.Nodes) || !s.getDocumentFiveMProfile(doc).HasResource() {
		return nil
	}
	if !s.fiveMResourceHasLocalEventArguments(doc) {
		return nil
	}
	var receiverID ast.NodeID
	var callable TypeSet
	var member ast.Node
	if call.Kind == ast.KindMethodCall {
		if call.Right == ast.InvalidNode || int(call.Right) >= len(doc.Tree.Nodes) {
			return nil
		}
		receiverID = call.Left
		member = ast.Node{Kind: ast.KindMemberExpr, Left: call.Left, Right: call.Right, Start: call.Start, End: call.End}
	} else {
		callee := doc.Tree.Nodes[call.Left]
		if callee.Kind != ast.KindMemberExpr || callee.Left == ast.InvalidNode {
			return nil
		}
		receiverID = callee.Left
	}
	receiver := doc.InferType(receiverID)
	if receiver.Basics&TypeAny != 0 {
		return nil
	}
	crossDocument := receiver.DeclURI != "" && receiver.DeclURI != doc.URI
	for _, variant := range receiver.Alternatives {
		if variant.DeclURI != "" && variant.DeclURI != doc.URI {
			crossDocument = true
		}
	}
	if !crossDocument {
		return nil
	}
	if call.Kind == ast.KindMethodCall {
		callable = doc.inferMemberExprWithContext(member, nil, nil, false)
	} else {
		callable = doc.InferType(call.Left)
	}
	candidates := callable.Alternatives
	if len(candidates) == 0 {
		candidates = []TypeSet{callable}
	}
	var diagnostics []Diagnostic
	for i := uint32(0); i < call.Count && call.Extra+i < uint32(len(doc.Tree.ExtraList)); i++ {
		argID := doc.Tree.ExtraList[call.Extra+i]
		if argID == ast.InvalidNode || int(argID) >= len(doc.Tree.Nodes) {
			continue
		}
		actual := doc.InferType(argID)
		if actual.Basics == TypeUnknown || actual.Basics&TypeAny != 0 || actual.CustomName != "" || len(actual.Alternatives) != 0 {
			continue
		}
		var expectedLabels []string
		allReject := true
		for _, candidate := range candidates {
			params, ok := s.fiveMConcreteCallableParams(candidate, call.Kind == ast.KindMethodCall)
			if !ok || int(i) >= len(params) || params[i].Name == "..." {
				allReject = false
				break
			}
			expected := ParseTypeString(params[i].Type)
			if expected.Basics == TypeUnknown || expected.Basics&TypeAny != 0 || expected.CustomName != "" {
				allReject = false
				break
			}
			if expected.Basics&actual.Basics != 0 {
				allReject = false
				break
			}
			expectedLabels = append(expectedLabels, params[i].Type)
		}
		if allReject && len(expectedLabels) != 0 {
			diagnostics = append(diagnostics, Diagnostic{Range: getNodeRange(doc.Tree, argID), Severity: SeverityWarning, Code: "argument-type-mismatch", Message: fmt.Sprintf("Argument has type %s; expected %s.", actual.Format(), strings.Join(expectedLabels, " or "))})
		}
	}
	return diagnostics
}

func (s *Server) fiveMConcreteCallableParams(t TypeSet, methodCall bool) ([]LuaDocParam, bool) {
	if t.Basics&TypeAny != 0 || t.Basics&^(TypeFunction|TypeTable) != 0 {
		return nil, false
	}
	if len(t.CallParams) != 0 {
		if methodCall {
			if t.CallParams[0].Name != "self" {
				return nil, false
			}
			return t.CallParams[1:], true
		}
		return t.CallParams, true
	}
	if t.DeclURI == "" || t.DeclNode == ast.InvalidNode {
		return nil, false
	}
	owner := s.Documents[t.DeclURI]
	if owner == nil || owner.Tree == nil || int(t.DeclNode) >= len(owner.Tree.Nodes) || owner.Tree.Nodes[t.DeclNode].Kind != ast.KindFunctionExpr {
		return nil, false
	}
	def := owner.getDefForValue(t.DeclNode)
	if def == ast.InvalidNode {
		return nil, false
	}
	annotation := owner.GetLuaDoc(def)
	if annotation == nil || len(annotation.Params) == 0 || len(annotation.Overloads) != 0 || len(annotation.Generics) != 0 {
		return nil, false
	}
	if methodCall {
		if annotation.Params[0].Name == "self" {
			return annotation.Params[1:], true
		}
		parent := owner.Tree.Nodes[t.DeclNode].Parent
		if parent == ast.InvalidNode || int(parent) >= len(owner.Tree.Nodes) {
			return nil, false
		}
		statement := owner.Tree.Nodes[parent]
		if statement.Kind != ast.KindFunctionStmt || statement.Left == ast.InvalidNode || int(statement.Left) >= len(owner.Tree.Nodes) || owner.Tree.Nodes[statement.Left].Kind != ast.KindMethodName {
			return nil, false
		}
	}
	return annotation.Params, true
}

// fiveMEventPayloadTypeDiagnostics validates a literal event payload against
// explicitly annotated compatible handlers. A missing or ambiguous contract
// prevents a warning; handlers are alternatives, not simultaneous demands.
func (s *Server) fiveMEventPayloadTypeDiagnostics(doc *Document, callID ast.NodeID) []Diagnostic {
	if doc == nil || doc.Tree == nil || len(doc.FiveMEvents) == 0 || callID == ast.InvalidNode || int(callID) >= len(doc.Tree.Nodes) || !s.getDocumentFiveMProfile(doc).HasResource() {
		return nil
	}
	call := doc.Tree.Nodes[callID]
	if call.Kind != ast.KindCallExpr || call.Count == 0 || call.Extra >= uint32(len(doc.Tree.ExtraList)) {
		return nil
	}
	if call.Left == ast.InvalidNode || int(call.Left) >= len(doc.Tree.Nodes) {
		return nil
	}
	callee := doc.Tree.Nodes[call.Left]
	framework := false
	if callee.Kind == ast.KindMemberExpr && callee.Right != ast.InvalidNode && int(callee.Right) < len(doc.Tree.Nodes) && string(doc.nodeSource(callee.Right)) == "TriggerServerCallback" {
		framework = true
	} else if callee.Kind == ast.KindIdent {
		switch string(doc.nodeSource(call.Left)) {
		case "TriggerEvent", "TriggerServerEvent", "TriggerClientEvent", "TriggerLatentServerEvent", "TriggerLatentClientEvent":
		default:
			return nil
		}
	} else {
		return nil
	}
	nameID := doc.Tree.ExtraList[call.Extra]
	var trigger FiveMEventInfo
	foundTrigger := false
	for _, event := range doc.FiveMEvents {
		if event.NodeID == nameID && (event.Kind == FiveMEventTriggerLocal || event.Kind == FiveMEventTriggerClient || event.Kind == FiveMEventTriggerServer || event.Kind == FiveMFrameworkTriggerServerCallback) {
			trigger, foundTrigger = event, true
			break
		}
	}
	if !foundTrigger {
		return nil
	}
	offset := uint32(1) + uint32(trigger.ExtraPayloadArgs)
	if trigger.Kind == FiveMEventTriggerClient {
		offset++
	}
	var frameworkRoot string
	var frameworkReceiver uint64
	var frameworkDispatch fiveMFrameworkDispatch
	if framework {
		if env := s.getDocumentFiveMProfile(doc).Env(); env != EnvClient && env != EnvShared {
			return nil
		}
		var ok bool
		frameworkRoot, frameworkReceiver, ok = doc.fiveMFrameworkReceiver(callID)
		if !ok {
			return nil
		}
		frameworkDispatch = s.fiveMFrameworkDispatchContract(frameworkRoot, frameworkReceiver)
		if !frameworkDispatch.known {
			return nil
		}
		offset = 2
	}
	type contract struct {
		params          []LuaDocParam
		known           bool
		parameterOffset int
	}
	var contracts []contract
	for _, indexed := range s.fiveMEventsNamed(trigger.Name) {
		registrationDoc := s.Documents[indexed.uri]
		registration := indexed.event
		if registrationDoc == nil || registrationDoc.Tree == nil || registration.HandlerID == ast.InvalidNode {
			continue
		}
		parameterOffset := 0
		if framework {
			if registration.Kind != FiveMFrameworkRegisterServerCallback || registration.NodeID == ast.InvalidNode || int(registration.NodeID) >= len(registrationDoc.Tree.Nodes) {
				continue
			}
			env := s.getDocumentFiveMProfile(registrationDoc).Env()
			if env != EnvServer && env != EnvShared {
				continue
			}
			root, receiver, ok := registrationDoc.fiveMFrameworkReceiver(registrationDoc.Tree.Nodes[registration.NodeID].Parent)
			if !ok || root != frameworkRoot || receiver != frameworkReceiver || !s.fiveMEventResourcesCompatible(doc, registrationDoc, callID, FiveMEventTriggerServer) {
				continue
			}
			parameterOffset = frameworkDispatch.callbackPosition + 1
		} else {
			if registration.Kind != FiveMEventAddHandler && registration.Kind != FiveMEventRegisterNet {
				continue
			}
			kind := registration.Kind
			if kind == FiveMEventAddHandler && registrationDoc.hasFiveMNetworkRegistration(registration.Name) {
				kind = FiveMEventRegisterNet
			}
			if !fiveMEventTriggerMatches(s.getDocumentFiveMProfile(doc).Env(), s.getDocumentFiveMProfile(registrationDoc).Env(), trigger.Kind, kind) || !s.fiveMEventResourcesCompatible(doc, registrationDoc, callID, trigger.Kind) {
				continue
			}
		}
		owner, handlerID := registrationDoc.fiveMEventHandler(registration.HandlerID)
		if owner == nil || handlerID == ast.InvalidNode {
			contracts = append(contracts, contract{})
			continue
		}
		annotation := fiveMFunctionAnnotation(owner, handlerID)
		if annotation == nil || len(annotation.Params) == 0 || len(annotation.Overloads) != 0 || len(annotation.Generics) != 0 {
			contracts = append(contracts, contract{})
			continue
		}
		contracts = append(contracts, contract{params: annotation.Params, known: true, parameterOffset: parameterOffset})
	}
	if len(contracts) == 0 {
		return nil
	}
	var diagnostics []Diagnostic
	for i := offset; i < call.Count && call.Extra+i < uint32(len(doc.Tree.ExtraList)); i++ {
		argID := doc.Tree.ExtraList[call.Extra+i]
		if argID == ast.InvalidNode || int(argID) >= len(doc.Tree.Nodes) {
			continue
		}
		actual := doc.InferType(argID)
		if actual.Basics == TypeUnknown || actual.Basics&TypeAny != 0 || actual.CustomName != "" || len(actual.Alternatives) != 0 {
			continue
		}
		if trigger.Kind != FiveMEventTriggerLocal && actual.Basics&TypeFunction != 0 {
			continue
		}
		payloadIndex := int(i - offset)
		allReject := true
		var labels []string
		for _, contract := range contracts {
			paramIndex := payloadIndex + contract.parameterOffset
			if !contract.known || paramIndex >= len(contract.params) || contract.params[paramIndex].Name == "..." {
				allReject = false
				break
			}
			label := contract.params[paramIndex].Type
			expected := ParseTypeString(label)
			if expected.Basics == TypeUnknown || expected.Basics&TypeAny != 0 || expected.CustomName != "" || expected.Basics&actual.Basics != 0 {
				allReject = false
				break
			}
			labels = append(labels, label)
		}
		if allReject && len(labels) != 0 {
			diagnostics = append(diagnostics, Diagnostic{Range: getNodeRange(doc.Tree, argID), Severity: SeverityWarning, Code: "fivem-event-argument-type", Message: fmt.Sprintf("Event '%s' argument has type %s; expected %s.", trigger.Name, actual.Format(), strings.Join(labels, " or "))})
		}
	}
	return diagnostics
}

func fiveMFunctionAnnotation(doc *Document, functionID ast.NodeID) *LuaDoc {
	if doc == nil || doc.Tree == nil || functionID == ast.InvalidNode || int(functionID) >= len(doc.Tree.Nodes) {
		return nil
	}
	def := doc.getDefForValue(functionID)
	if def == ast.InvalidNode {
		parent := doc.Tree.Nodes[functionID].Parent
		if parent != ast.InvalidNode && int(parent) < len(doc.Tree.Nodes) {
			node := doc.Tree.Nodes[parent]
			if (node.Kind == ast.KindFunctionStmt || node.Kind == ast.KindLocalFunction) && node.Left != ast.InvalidNode {
				def = node.Left
			}
		}
	}
	if def != ast.InvalidNode {
		annotation := doc.GetLuaDoc(def)
		if annotation != nil && (len(annotation.Params) != 0 || len(annotation.Overloads) != 0) {
			return annotation
		}
	}
	return doc.GetLuaDoc(functionID)
}
