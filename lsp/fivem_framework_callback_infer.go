package lsp

import (
	"strconv"

	"github.com/FRFlo/lugo/ast"
)

type fiveMFrameworkDispatch struct {
	callbackPosition int
	known            bool
	dependencies     []string
	prefixTypes      []TypeSet
}

// fiveMFrameworkReceiver identifies an actual resource-global table, rather
// than assuming that every object with similarly named methods is ESX.
func (doc *Document) fiveMFrameworkReceiver(callID ast.NodeID) (string, uint64, bool) {
	if doc == nil || doc.Server == nil || doc.Tree == nil || callID == ast.InvalidNode || int(callID) >= len(doc.Tree.Nodes) {
		return "", 0, false
	}
	call := doc.Tree.Nodes[callID]
	if call.Kind != ast.KindCallExpr || call.Left == ast.InvalidNode || int(call.Left) >= len(doc.Tree.Nodes) {
		return "", 0, false
	}
	member := doc.Tree.Nodes[call.Left]
	if member.Kind != ast.KindMemberExpr || member.Left == ast.InvalidNode {
		return "", 0, false
	}
	typ := doc.InferType(member.Left)
	if len(typ.Alternatives) != 0 {
		return "", 0, false
	}
	owner, name := doc.Server.fiveMTableGlobalOwner(typ)
	if owner == nil || name == 0 {
		return "", 0, false
	}
	root := doc.Server.getDocumentFiveMProfile(owner).ResourceRoot
	return root, name, root != ""
}

// fiveMFrameworkDispatchContract discovers where the response callback is
// passed into the registered handler. The ESX dispatcher differs across
// versions (for example an xPlayer argument may precede source), so no fixed
// parameter offset is assumed.
func (s *Server) fiveMFrameworkDispatchContract(root string, receiver uint64) fiveMFrameworkDispatch {
	key := root + ":" + strconv.FormatUint(receiver, 16)
	s.fiveMEventsNamed("")
	index := &s.fiveMEventIndex
	index.mu.Lock()
	cached, ok := index.frameworkDispatch[key]
	if ok {
		index.mu.Unlock()
		return cached
	}
	if index.frameworkComputing[key] {
		index.mu.Unlock()
		return fiveMFrameworkDispatch{callbackPosition: -1}
	}
	index.frameworkComputing[key] = true
	version := index.version
	index.mu.Unlock()
	result := fiveMFrameworkDispatch{callbackPosition: -1}
	uncertain := false
	for _, source := range s.Documents {
		if source == nil || source.Tree == nil {
			continue
		}
		profile := s.getDocumentFiveMProfile(source)
		if profile.ResourceRoot != root || (profile.Env() != EnvServer && profile.Env() != EnvShared) {
			continue
		}
		for i := 1; i < len(source.Tree.Nodes); i++ {
			call := source.Tree.Nodes[i]
			if call.Kind != ast.KindCallExpr || call.Left == ast.InvalidNode || int(call.Left) >= len(source.Tree.Nodes) {
				continue
			}
			member := source.Tree.Nodes[call.Left]
			if member.Kind != ast.KindMemberExpr || member.Left == ast.InvalidNode || member.Right == ast.InvalidNode || int(member.Right) >= len(source.Tree.Nodes) || string(source.nodeSource(member.Right)) != "TriggerServerCallback" || ast.HashBytes(source.nodeSource(member.Left)) != receiver {
				continue
			}
			callbackParam := -1
			for j := uint32(0); j < call.Count && call.Extra+j < uint32(len(source.Tree.ExtraList)); j++ {
				arg := source.Tree.ExtraList[call.Extra+j]
				if arg != ast.InvalidNode && int(arg) < len(source.Tree.Nodes) && source.Tree.Nodes[arg].Kind == ast.KindFunctionExpr {
					if callbackParam >= 0 {
						callbackParam = -1
						break
					}
					callbackParam = int(j)
				}
			}
			if callbackParam < 0 {
				continue
			}
			ctx := s.resolveSymbolNode(source.URI, source, member.Right)
			if ctx == nil || ctx.TargetDoc == nil || ctx.TargetDefID == ast.InvalidNode {
				uncertain = true
				continue
			}
			dispatcherDoc := ctx.TargetDoc
			if s.getDocumentFiveMProfile(dispatcherDoc).ResourceRoot != root {
				uncertain = true
				continue
			}
			dispatcherID := dispatcherDoc.getAssignedValue(ctx.TargetDefID)
			if dispatcherID == ast.InvalidNode || int(dispatcherID) >= len(dispatcherDoc.Tree.Nodes) {
				uncertain = true
				continue
			}
			dispatcher := dispatcherDoc.Tree.Nodes[dispatcherID]
			if dispatcher.Kind != ast.KindFunctionExpr || callbackParam >= int(dispatcher.Count) || dispatcher.Extra+uint32(callbackParam) >= uint32(len(dispatcherDoc.Tree.ExtraList)) {
				uncertain = true
				continue
			}
			callbackDef := dispatcherDoc.Tree.ExtraList[dispatcher.Extra+uint32(callbackParam)]
			matched := false
			for n := 1; n < len(dispatcherDoc.Tree.Nodes); n++ {
				invoke := dispatcherDoc.Tree.Nodes[n]
				if invoke.Kind != ast.KindCallExpr || invoke.Left == ast.InvalidNode || int(invoke.Left) >= len(dispatcherDoc.Tree.Nodes) {
					continue
				}
				indexed := dispatcherDoc.Tree.Nodes[invoke.Left]
				if indexed.Kind != ast.KindIndexExpr || indexed.Left == ast.InvalidNode || int(indexed.Left) >= len(dispatcherDoc.Tree.Nodes) {
					continue
				}
				registry := dispatcherDoc.Tree.Nodes[indexed.Left]
				if registry.Kind != ast.KindMemberExpr || registry.Left == ast.InvalidNode || registry.Right == ast.InvalidNode || int(registry.Right) >= len(dispatcherDoc.Tree.Nodes) || string(dispatcherDoc.nodeSource(registry.Right)) != "ServerCallbacks" || ast.HashBytes(dispatcherDoc.nodeSource(registry.Left)) != receiver {
					continue
				}
				inside := false
				for parent := invoke.Parent; parent != ast.InvalidNode && int(parent) < len(dispatcherDoc.Tree.Nodes); parent = dispatcherDoc.Tree.Nodes[parent].Parent {
					if parent == dispatcherID {
						inside = true
						break
					}
				}
				if !inside {
					continue
				}
				for j := uint32(0); j < invoke.Count && invoke.Extra+j < uint32(len(dispatcherDoc.Tree.ExtraList)); j++ {
					arg := dispatcherDoc.Tree.ExtraList[invoke.Extra+j]
					if arg != ast.InvalidNode && int(arg) < len(dispatcherDoc.Tree.Nodes) && dispatcherDoc.Tree.Nodes[arg].Kind == ast.KindIdent && dispatcherDoc.referenceAt(arg) == callbackDef {
						matched = true
						if result.known && result.callbackPosition != int(j) {
							uncertain = true
						}
						result.callbackPosition, result.known = int(j), true
						prefix := make([]TypeSet, int(j))
						for p := uint32(0); p < j; p++ {
							valueID := dispatcherDoc.Tree.ExtraList[invoke.Extra+p]
							valueType := TypeSet{}
							if valueID != ast.InvalidNode && int(valueID) < len(dispatcherDoc.Tree.Nodes) && dispatcherDoc.Tree.Nodes[valueID].Kind == ast.KindIdent {
								valueDef := dispatcherDoc.referenceAt(valueID)
								paramIndex := dispatcherDoc.Tree.IndexOfExtra(dispatcherID, valueDef)
								if paramIndex >= 0 && paramIndex < int(call.Count) && call.Extra+uint32(paramIndex) < uint32(len(source.Tree.ExtraList)) {
									valueType = source.InferType(source.Tree.ExtraList[call.Extra+uint32(paramIndex)])
								}
							}
							if !reusableType(valueType) {
								valueType = dispatcherDoc.InferType(valueID)
							}
							prefix[p] = valueType
						}
						if result.prefixTypes == nil {
							result.prefixTypes = prefix
						} else if len(result.prefixTypes) == len(prefix) {
							for p := range prefix {
								if !reusableType(prefix[p]) || !reusableType(result.prefixTypes[p]) {
									result.prefixTypes[p] = TypeSet{Basics: TypeAny}
								} else {
									result.prefixTypes[p] = mergeFiveMEventTypes(result.prefixTypes[p], prefix[p])
								}
							}
						}
						result.dependencies = append(result.dependencies, source.URI, dispatcherDoc.URI)
					}
				}
			}
			if !matched {
				uncertain = true
			}
		}
	}
	if uncertain {
		result.known = false
	}
	index.mu.Lock()
	if index.version == version && index.frameworkDispatch != nil {
		delete(index.frameworkComputing, key)
		index.frameworkDispatch[key] = result
	}
	index.mu.Unlock()
	return result
}

func (doc *Document) inferFiveMFrameworkCallbackResult(callID ast.NodeID, argIndex, paramIndex int) TypeSet {
	if argIndex != 1 || doc.Server == nil || doc.Tree == nil {
		return TypeSet{}
	}
	var trigger FiveMEventInfo
	found := false
	for _, event := range doc.FiveMEvents {
		if event.Kind == FiveMFrameworkTriggerServerCallback && event.NodeID != ast.InvalidNode && int(event.NodeID) < len(doc.Tree.Nodes) && doc.Tree.Nodes[event.NodeID].Parent == callID {
			trigger, found = event, true
			break
		}
	}
	if !found {
		return TypeSet{}
	}
	env := doc.Server.getDocumentFiveMProfile(doc).Env()
	if env != EnvClient && env != EnvShared {
		return TypeSet{}
	}
	root, receiver, ok := doc.fiveMFrameworkReceiver(callID)
	if !ok {
		return TypeSet{}
	}
	doc.Server.trackMetatableTypeDependency(root, doc.URI)
	dispatch := doc.Server.fiveMFrameworkDispatchContract(root, receiver)
	if !dispatch.known {
		return TypeSet{}
	}
	var result TypeSet
	have := false
	for _, indexed := range doc.Server.fiveMEventsNamed(trigger.Name) {
		registrationDoc, registration := doc.Server.Documents[indexed.uri], indexed.event
		if registrationDoc == nil || registrationDoc.Tree == nil || registration.Kind != FiveMFrameworkRegisterServerCallback {
			continue
		}
		profile := doc.Server.getDocumentFiveMProfile(registrationDoc)
		if profile.Env() != EnvServer && profile.Env() != EnvShared {
			continue
		}
		if registration.NodeID == ast.InvalidNode || int(registration.NodeID) >= len(registrationDoc.Tree.Nodes) {
			continue
		}
		registrationCall := registrationDoc.Tree.Nodes[registration.NodeID].Parent
		if !doc.Server.fiveMEventResourcesCompatible(doc, registrationDoc, callID, FiveMEventTriggerServer) {
			continue
		}
		otherRoot, otherReceiver, valid := registrationDoc.fiveMFrameworkReceiver(registrationCall)
		if !valid || otherRoot != root || otherReceiver != receiver {
			continue
		}
		doc.Server.trackMetatableTypeDependency(registrationDoc.URI, doc.URI)
		handlerDoc, handlerID := registrationDoc.fiveMEventHandler(registration.HandlerID)
		if handlerDoc == nil || handlerID == ast.InvalidNode {
			continue
		}
		handler := handlerDoc.Tree.Nodes[handlerID]
		if dispatch.callbackPosition >= int(handler.Count) || handler.Extra+uint32(dispatch.callbackPosition) >= uint32(len(handlerDoc.Tree.ExtraList)) {
			continue
		}
		callbackDef := handlerDoc.Tree.ExtraList[handler.Extra+uint32(dispatch.callbackPosition)]
		for i := 1; i < len(handlerDoc.Tree.Nodes); i++ {
			call := handlerDoc.Tree.Nodes[i]
			if call.Kind != ast.KindCallExpr || call.Left == ast.InvalidNode || int(call.Left) >= len(handlerDoc.Tree.Nodes) || handlerDoc.Tree.Nodes[call.Left].Kind != ast.KindIdent || handlerDoc.referenceAt(call.Left) != callbackDef || paramIndex >= int(call.Count) || call.Extra+uint32(paramIndex) >= uint32(len(handlerDoc.Tree.ExtraList)) {
				continue
			}
			valueID := handlerDoc.Tree.ExtraList[call.Extra+uint32(paramIndex)]
			value := handlerDoc.InferType(valueID)
			if value.CustomName != "" && value.DeclURI == "" {
				value.DeclURI = handlerDoc.URI
			}
			if !reusableType(value) {
				value.Basics = TypeAny
			}
			if !have {
				result, have = value, true
			} else {
				result = mergeFiveMEventTypes(result, value)
			}
			doc.Server.trackMetatableTypeDependency(registrationDoc.URI, doc.URI)
			doc.Server.trackMetatableTypeDependency(handlerDoc.URI, doc.URI)
			for _, uri := range dispatch.dependencies {
				doc.Server.trackMetatableTypeDependency(uri, doc.URI)
			}
		}
	}
	return result
}

// inferFiveMFrameworkHandlerParameter follows arguments through the observed
// request dispatcher. The response callback slot comes from its actual body;
// request payload arguments follow that slot in the registered handler.
func (doc *Document) inferFiveMFrameworkHandlerParameter(defID, functionID ast.NodeID) TypeSet {
	if doc == nil || doc.Server == nil || doc.Tree == nil {
		return TypeSet{}
	}
	env := doc.Server.getDocumentFiveMProfile(doc).Env()
	if env != EnvServer && env != EnvShared {
		return TypeSet{}
	}
	param := doc.Tree.IndexOfExtra(functionID, defID)
	if param < 0 {
		return TypeSet{}
	}
	registrations := make([]fiveMIndexedEvent, 0, len(doc.FiveMEvents))
	for _, event := range doc.FiveMEvents {
		registrations = append(registrations, fiveMIndexedEvent{uri: doc.URI, event: event})
	}
	if name := doc.fiveMFunctionName(functionID); name != "" {
		for _, indexed := range doc.Server.fiveMEventsForHandler(name) {
			if indexed.uri != doc.URI {
				registrations = append(registrations, indexed)
			}
		}
	}
	var result TypeSet
	found := false
	for _, indexed := range registrations {
		registrationDoc, registration := doc.Server.Documents[indexed.uri], indexed.event
		if registrationDoc == nil || registrationDoc.Tree == nil || registration.Kind != FiveMFrameworkRegisterServerCallback || registration.NodeID == ast.InvalidNode || int(registration.NodeID) >= len(registrationDoc.Tree.Nodes) {
			continue
		}
		owner, handlerID := registrationDoc.fiveMEventHandler(registration.HandlerID)
		if owner != doc || handlerID != functionID {
			continue
		}
		callID := registrationDoc.Tree.Nodes[registration.NodeID].Parent
		root, receiver, ok := registrationDoc.fiveMFrameworkReceiver(callID)
		if !ok {
			continue
		}
		doc.Server.trackMetatableTypeDependency(root, doc.URI)
		dispatch := doc.Server.fiveMFrameworkDispatchContract(root, receiver)
		if !dispatch.known {
			continue
		}
		for _, uri := range dispatch.dependencies {
			doc.Server.trackMetatableTypeDependency(uri, doc.URI)
		}
		doc.Server.trackMetatableTypeDependency(registrationDoc.URI, doc.URI)
		if param == dispatch.callbackPosition {
			return TypeSet{Basics: TypeFunction}
		}
		if param < dispatch.callbackPosition {
			if param < len(dispatch.prefixTypes) && reusableType(dispatch.prefixTypes[param]) {
				return dispatch.prefixTypes[param]
			}
			continue
		}
		payloadIndex := param - dispatch.callbackPosition - 1
		for _, trigger := range doc.Server.fiveMEventsNamed(registration.Name) {
			source := doc.Server.Documents[trigger.uri]
			event := trigger.event
			if source == nil || source.Tree == nil || event.Kind != FiveMFrameworkTriggerServerCallback || event.NodeID == ast.InvalidNode || int(event.NodeID) >= len(source.Tree.Nodes) {
				continue
			}
			sourceEnv := doc.Server.getDocumentFiveMProfile(source).Env()
			if sourceEnv != EnvClient && sourceEnv != EnvShared {
				continue
			}
			sourceCallID := source.Tree.Nodes[event.NodeID].Parent
			if sourceCallID == ast.InvalidNode || int(sourceCallID) >= len(source.Tree.Nodes) || !doc.Server.fiveMEventResourcesCompatible(source, registrationDoc, sourceCallID, FiveMEventTriggerServer) {
				continue
			}
			sourceRoot, sourceReceiver, valid := source.fiveMFrameworkReceiver(sourceCallID)
			if !valid || sourceRoot != root || sourceReceiver != receiver {
				continue
			}
			call := source.Tree.Nodes[sourceCallID]
			argIndex := uint32(payloadIndex + 2)
			var typ TypeSet
			if argIndex >= call.Count {
				typ.Basics = TypeNil
				if call.Count > 2 && call.Extra+call.Count-1 < uint32(len(source.Tree.ExtraList)) {
					last := source.Tree.Nodes[source.Tree.ExtraList[call.Extra+call.Count-1]]
					if last.Kind == ast.KindVararg || last.Kind == ast.KindCallExpr || last.Kind == ast.KindMethodCall {
						typ.Basics = TypeAny
					}
				}
			} else if call.Extra+argIndex < uint32(len(source.Tree.ExtraList)) {
				arg := source.Tree.ExtraList[call.Extra+argIndex]
				typ = source.InferType(arg)
				if typ.Basics&TypeFunction != 0 {
					continue
				}
				if typ.CustomName != "" && typ.DeclURI == "" {
					typ.DeclURI = source.URI
				}
				if !reusableType(typ) {
					typ.Basics = TypeAny
				}
			} else {
				continue
			}
			if !found {
				result, found = typ, true
			} else {
				result = mergeFiveMEventTypes(result, typ)
			}
			doc.Server.trackMetatableTypeDependency(source.URI, doc.URI)
		}
	}
	return result
}
