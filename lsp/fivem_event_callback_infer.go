package lsp

import "github.com/FRFlo/lugo/ast"

// inferFiveMEventCallbackParameter follows callbacks passed to local FiveM
// events to the values supplied by matching event handlers. Network events do
// not transport Lua functions, so they must not use this path.
func (doc *Document) inferFiveMEventCallbackParameter(callID ast.NodeID, callbackArg, resultArg int) TypeSet {
	if doc.Server == nil || !doc.Server.getDocumentFiveMProfile(doc).HasResource() || callbackArg < 1 {
		return TypeSet{}
	}
	call := doc.Tree.Nodes[callID]
	if call.Count == 0 || call.Extra >= uint32(len(doc.Tree.ExtraList)) {
		return TypeSet{}
	}
	nameID := doc.Tree.ExtraList[call.Extra]
	var name string
	foundEvent := false
	for _, event := range doc.FiveMEvents {
		if event.Kind == FiveMEventTriggerLocal && event.NodeID == nameID {
			name = event.Name
			foundEvent = true
			break
		}
	}
	if !foundEvent {
		return TypeSet{}
	}

	var result TypeSet
	found := false
	for _, indexed := range doc.Server.fiveMEventsNamed(name) {
		target, event := doc.Server.Documents[indexed.uri], indexed.event
		if target == nil || target.Tree == nil {
			continue
		}
		if !fiveMCallbackEnvironmentsOverlap(doc.Server.getDocumentFiveMProfile(doc).Env(), doc.Server.getDocumentFiveMProfile(target).Env()) {
			continue
		}
		if !doc.Server.fiveMEventResourcesCompatible(doc, target, callID, FiveMEventTriggerLocal) {
			continue
		}
		{
			if event.Name != name || (event.Kind != FiveMEventAddHandler && event.Kind != FiveMEventRegisterNet) || event.HandlerID == ast.InvalidNode || int(event.HandlerID) >= len(target.Tree.Nodes) {
				continue
			}
			handlerDoc, handlerID := target.fiveMEventHandler(event.HandlerID)
			if handlerDoc == nil || handlerID == ast.InvalidNode {
				continue
			}
			handler := handlerDoc.Tree.Nodes[handlerID]
			parameter := callbackArg - 1
			if handler.Kind != ast.KindFunctionExpr || parameter >= int(handler.Count) || handler.Extra+uint32(parameter) >= uint32(len(handlerDoc.Tree.ExtraList)) {
				continue
			}
			paramID := handlerDoc.Tree.ExtraList[handler.Extra+uint32(parameter)]
			for i := 1; i < len(handlerDoc.Tree.Nodes); i++ {
				candidate := handlerDoc.Tree.Nodes[i]
				if candidate.Kind != ast.KindCallExpr || candidate.Left == ast.InvalidNode || int(candidate.Left) >= len(handlerDoc.Tree.Nodes) || handlerDoc.Tree.Nodes[candidate.Left].Kind != ast.KindIdent || handlerDoc.referenceAt(candidate.Left) != paramID || resultArg >= int(candidate.Count) || candidate.Extra+uint32(resultArg) >= uint32(len(handlerDoc.Tree.ExtraList)) {
					continue
				}
				valueID := handlerDoc.Tree.ExtraList[candidate.Extra+uint32(resultArg)]
				value := handlerDoc.InferType(valueID)
				if value.CustomName != "" && value.DeclURI == "" {
					value.DeclURI = handlerDoc.URI
				}
				if value.Basics&(TypeAny|TypeNil) != 0 && value.DeclNode == ast.InvalidNode {
					value = handlerDoc.inferFiveMResourceGlobalTable(valueID, value)
				}
				if !reusableType(value) {
					value.Basics = TypeAny
				}
				if !found {
					result, found = value, true
				} else {
					result = mergeFiveMEventTypes(result, value)
				}
				doc.Server.trackMetatableTypeDependency(target.URI, doc.URI)
				doc.Server.trackMetatableTypeDependency(handlerDoc.URI, doc.URI)
			}
		}
	}
	return result
}

// A resource global can be initialized in a shared script and reassigned to
// nil in a later script before a callback restores it. Preserve the known
// table identity alongside the uncertain/nil alternatives in that case.
func (doc *Document) inferFiveMResourceGlobalTable(id ast.NodeID, result TypeSet) TypeSet {
	if doc.Server == nil || doc.Server.GlobalIndex == nil || id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[id].Kind != ast.KindIdent || doc.referenceAt(id) != ast.InvalidNode {
		return result
	}
	resource := doc.Server.getDocumentFiveMProfile(doc).ResourceRoot
	if resource == "" {
		return result
	}
	hash := ast.HashBytes(doc.nodeSource(id))
	key := GlobalKey{PropHash: hash}
	for _, symbol := range doc.Server.visibleGlobalSymbolsFromEntries(doc, doc.Server.GlobalIndex.SymbolsByHash(key), 0) {
		owner := doc.Server.Documents[symbol.URI]
		if owner == nil || owner.Tree == nil || doc.Server.getDocumentFiveMProfile(owner).ResourceRoot != resource {
			continue
		}
		value := owner.getAssignedValue(symbol.NodeID)
		if value == ast.InvalidNode || int(value) >= len(owner.Tree.Nodes) || owner.Tree.Nodes[value].Kind != ast.KindTableExpr {
			continue
		}
		if result.DeclNode != ast.InvalidNode && (result.DeclURI != owner.URI || result.DeclNode != value) {
			result.DeclURI, result.DeclNode = "", ast.InvalidNode
			return result
		}
		result.Basics |= TypeTable
		result.DeclURI, result.DeclNode = owner.URI, value
		doc.Server.trackMetatableTypeDependency(owner.URI, doc.URI)
	}
	return result
}

func fiveMCallbackEnvironmentsOverlap(a, b FileEnv) bool {
	return (a == EnvClient || a == EnvServer || a == EnvShared) &&
		(b == EnvClient || b == EnvServer || b == EnvShared) &&
		(a == EnvShared || b == EnvShared || a == b)
}

// inferFiveMEventHandlerParameter infers payload parameters from literal
// triggers. For network events, only explicitly registered network handlers
// are considered, and the client target is not part of the payload.
func (doc *Document) inferFiveMEventHandlerParameter(defID, funcID ast.NodeID) TypeSet {
	if doc.Server == nil || !doc.Server.getDocumentFiveMProfile(doc).HasResource() {
		return TypeSet{}
	}
	index := doc.Tree.IndexOfExtra(funcID, defID)
	if index < 0 {
		return TypeSet{}
	}
	var result TypeSet
	found := false
	registrations := make([]fiveMIndexedEvent, 0, len(doc.FiveMEvents))
	for _, registration := range doc.FiveMEvents {
		registrations = append(registrations, fiveMIndexedEvent{uri: doc.URI, event: registration})
	}
	if name := doc.fiveMFunctionName(funcID); name != "" {
		for _, indexed := range doc.Server.fiveMEventsForHandler(name) {
			if indexed.uri != doc.URI {
				registrations = append(registrations, indexed)
			}
		}
	}
	for _, indexed := range registrations {
		registrationDoc, registration := doc.Server.Documents[indexed.uri], indexed.event
		if registrationDoc == nil || registrationDoc.Tree == nil {
			continue
		}
		if registration.Kind != FiveMEventAddHandler && registration.Kind != FiveMEventRegisterNet {
			continue
		}
		handlerDoc, handlerID := registrationDoc.fiveMEventHandler(registration.HandlerID)
		if handlerDoc != doc || handlerID != funcID {
			continue
		}
		kind := registration.Kind
		if kind == FiveMEventAddHandler && registrationDoc.hasFiveMNetworkRegistration(registration.Name) {
			kind = FiveMEventRegisterNet
		}
		for _, indexed := range doc.Server.fiveMEventsNamed(registration.Name) {
			source, trigger := doc.Server.Documents[indexed.uri], indexed.event
			if source == nil || source.Tree == nil {
				continue
			}
			{
				if trigger.Name != registration.Name || !fiveMEventTriggerMatches(doc.Server.getDocumentFiveMProfile(source).Env(), doc.Server.getDocumentFiveMProfile(registrationDoc).Env(), trigger.Kind, kind) {
					continue
				}
				if trigger.NodeID == ast.InvalidNode || int(trigger.NodeID) >= len(source.Tree.Nodes) {
					continue
				}
				callID := source.Tree.Nodes[trigger.NodeID].Parent
				if callID == ast.InvalidNode || int(callID) >= len(source.Tree.Nodes) {
					continue
				}
				if !doc.Server.fiveMEventResourcesCompatible(source, registrationDoc, callID, trigger.Kind) {
					continue
				}
				call := source.Tree.Nodes[callID]
				offset := index + 1 + int(trigger.ExtraPayloadArgs)
				if trigger.Kind == FiveMEventTriggerClient {
					offset++
				}
				if call.Kind != ast.KindCallExpr {
					continue
				}
				var value TypeSet
				if offset >= int(call.Count) {
					value.Basics = TypeNil
					if call.Count > 1 && call.Extra+call.Count-1 < uint32(len(source.Tree.ExtraList)) {
						last := source.Tree.Nodes[source.Tree.ExtraList[call.Extra+call.Count-1]]
						if last.Kind == ast.KindVararg || last.Kind == ast.KindCallExpr || last.Kind == ast.KindMethodCall {
							value.Basics = TypeAny
						}
					}
				} else {
					if call.Extra+uint32(offset) >= uint32(len(source.Tree.ExtraList)) {
						continue
					}
					valueID := source.Tree.ExtraList[call.Extra+uint32(offset)]
					// Lua functions cannot be sent as network event payloads.
					if trigger.Kind != FiveMEventTriggerLocal && source.Tree.Nodes[valueID].Kind == ast.KindFunctionExpr {
						continue
					}
					value = source.InferType(valueID)
					if value.CustomName != "" && value.DeclURI == "" {
						value.DeclURI = source.URI
					}
					if trigger.Kind != FiveMEventTriggerLocal && value.Basics&TypeFunction != 0 {
						continue
					}
				}
				if !reusableType(value) {
					value.Basics = TypeAny
				}
				if !found {
					result, found = value, true
				} else {
					result = mergeFiveMEventTypes(result, value)
				}
				doc.Server.trackMetatableTypeDependency(source.URI, doc.URI)
				doc.Server.trackMetatableTypeDependency(registrationDoc.URI, doc.URI)
			}
		}
	}
	return result
}

func (doc *Document) fiveMFunctionName(funcID ast.NodeID) string {
	if doc == nil || doc.Tree == nil || funcID == ast.InvalidNode || int(funcID) >= len(doc.Tree.Nodes) {
		return ""
	}
	parent := doc.Tree.Nodes[funcID].Parent
	if parent != ast.InvalidNode && int(parent) < len(doc.Tree.Nodes) {
		statement := doc.Tree.Nodes[parent]
		if statement.Kind == ast.KindLocalFunction || statement.Kind == ast.KindFunctionStmt {
			if leaf := doc.fiveMReferenceLeaf(statement.Left); leaf != ast.InvalidNode {
				return doc.fiveMReferenceName(leaf)
			}
		}
	}
	if def := doc.getDefForValue(funcID); def != ast.InvalidNode && int(def) < len(doc.Tree.Nodes) && doc.Tree.Nodes[def].Kind == ast.KindIdent {
		return string(doc.nodeSource(def))
	}
	return ""
}

func (doc *Document) fiveMEventHandler(id ast.NodeID) (*Document, ast.NodeID) {
	if doc == nil || doc.Tree == nil || id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return nil, ast.InvalidNode
	}
	if doc.Tree.Nodes[id].Kind == ast.KindFunctionExpr {
		return doc, id
	}
	if doc.Server == nil {
		return nil, ast.InvalidNode
	}
	if doc.Tree.Nodes[id].Kind == ast.KindIndexExpr {
		inferred := doc.InferType(id)
		owner := doc.Server.Documents[inferred.DeclURI]
		if owner != nil && owner.Tree != nil && inferred.DeclNode != ast.InvalidNode && int(inferred.DeclNode) < len(owner.Tree.Nodes) && owner.Tree.Nodes[inferred.DeclNode].Kind == ast.KindFunctionExpr {
			return owner, inferred.DeclNode
		}
		return nil, ast.InvalidNode
	}
	leaf := doc.fiveMReferenceLeaf(id)
	if leaf == ast.InvalidNode {
		return nil, ast.InvalidNode
	}
	ctx := doc.Server.resolveSymbolNode(doc.URI, doc, leaf)
	if ctx == nil || ctx.TargetDoc == nil || ctx.TargetDefID == ast.InvalidNode {
		return nil, ast.InvalidNode
	}
	value := ctx.TargetDoc.getAssignedValue(ctx.TargetDefID)
	if value == ast.InvalidNode || int(value) >= len(ctx.TargetDoc.Tree.Nodes) || ctx.TargetDoc.Tree.Nodes[value].Kind != ast.KindFunctionExpr {
		return nil, ast.InvalidNode
	}
	return ctx.TargetDoc, value
}

// Index references by their final literal name, then verify the resolved
// definition before joining events. This also supports table-owned handlers.
func (doc *Document) fiveMReferenceLeaf(id ast.NodeID) ast.NodeID {
	if doc == nil || doc.Tree == nil || id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return ast.InvalidNode
	}
	node := doc.Tree.Nodes[id]
	if node.Kind == ast.KindString {
		return ast.InvalidNode
	}
	if node.Kind == ast.KindMemberExpr || node.Kind == ast.KindMethodName || node.Kind == ast.KindIndexExpr {
		id = node.Right
	}
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return ast.InvalidNode
	}
	if kind := doc.Tree.Nodes[id].Kind; kind == ast.KindIdent || kind == ast.KindString {
		return id
	}
	return ast.InvalidNode
}

func (doc *Document) fiveMReferenceName(id ast.NodeID) string {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return ""
	}
	name := string(doc.nodeSource(id))
	if doc.Tree.Nodes[id].Kind == ast.KindString {
		return unquoteLuaString(name)
	}
	return name
}

func (doc *Document) hasFiveMNetworkRegistration(name string) bool {
	profile := doc.Server.getDocumentFiveMProfile(doc)
	for _, indexed := range doc.Server.fiveMEventsNamed(name) {
		other, event := doc.Server.Documents[indexed.uri], indexed.event
		if other == nil || other.Tree == nil {
			continue
		}
		otherProfile := doc.Server.getDocumentFiveMProfile(other)
		if otherProfile.ResourceRoot != profile.ResourceRoot || !fiveMCallbackEnvironmentsOverlap(profile.Env(), otherProfile.Env()) {
			continue
		}
		if event.Kind == FiveMEventRegisterNet {
			return true
		}
	}
	return false
}

func fiveMEventTriggerMatches(source, target FileEnv, trigger, registration FiveMEventKind) bool {
	switch trigger {
	case FiveMEventTriggerLocal:
		return fiveMCallbackEnvironmentsOverlap(source, target)
	case FiveMEventTriggerServer:
		return registration == FiveMEventRegisterNet && (source == EnvClient || source == EnvShared) && (target == EnvServer || target == EnvShared)
	case FiveMEventTriggerClient:
		return registration == FiveMEventRegisterNet && (source == EnvServer || source == EnvShared) && (target == EnvClient || target == EnvShared)
	}
	return false
}

// fiveMTableGlobalOwner resolves only a table with a proven global definition
// in its declaring document. Local tables must not inherit unrelated globals.
func (s *Server) fiveMTableGlobalOwner(t TypeSet) (*Document, uint64) {
	if s == nil || s.GlobalIndex == nil || t.DeclURI == "" || t.DeclNode == ast.InvalidNode {
		return nil, 0
	}
	owner := s.Documents[t.DeclURI]
	if owner == nil || owner.Tree == nil || owner.Resolver == nil {
		return nil, 0
	}
	for _, id := range owner.Resolver.GlobalDefs {
		if owner.getAssignedValue(id) == t.DeclNode {
			return owner, ast.HashBytes(owner.nodeSource(id))
		}
	}
	return nil, 0
}

// inferFiveMNamedEventCallbackParameter follows callbacks passed as local
// function identifiers instead of inline function expressions.
func (doc *Document) inferFiveMNamedEventCallbackParameter(defID, funcID ast.NodeID) TypeSet {
	if doc.Server == nil || doc.Tree == nil || !doc.Server.getDocumentFiveMProfile(doc).HasResource() {
		return TypeSet{}
	}
	param := doc.Tree.IndexOfExtra(funcID, defID)
	if param < 0 {
		return TypeSet{}
	}
	name := doc.fiveMFunctionName(funcID)
	if name == "" {
		return TypeSet{}
	}
	var result TypeSet
	found := false
	for _, indexed := range doc.Server.fiveMEventsForCallback(name) {
		source, event := doc.Server.Documents[indexed.uri], indexed.event
		if source == nil || source.Tree == nil || event.NodeID == ast.InvalidNode || int(event.NodeID) >= len(source.Tree.Nodes) {
			continue
		}
		callID := source.Tree.Nodes[event.NodeID].Parent
		if callID == ast.InvalidNode || int(callID) >= len(source.Tree.Nodes) {
			continue
		}
		call := source.Tree.Nodes[callID]
		if call.Kind != ast.KindCallExpr {
			continue
		}
		for i := uint32(1); i < call.Count && call.Extra+i < uint32(len(source.Tree.ExtraList)); i++ {
			arg := source.Tree.ExtraList[call.Extra+i]
			if source.fiveMReferenceLeaf(arg) == ast.InvalidNode {
				continue
			}
			owner, value := source.fiveMEventHandler(arg)
			if owner != doc || value != funcID {
				continue
			}
			var inferred TypeSet
			if event.Kind == FiveMFrameworkTriggerServerCallback {
				inferred = source.inferFiveMFrameworkCallbackResult(callID, int(i), param)
			} else {
				inferred = source.inferFiveMEventCallbackParameter(callID, int(i), param)
			}
			if !reusableType(inferred) {
				continue
			}
			doc.Server.trackMetatableTypeDependency(source.URI, doc.URI)
			if !found {
				result, found = inferred, true
				continue
			}
			result = mergeFiveMEventTypes(result, inferred)
		}
	}
	return result
}

// A shared table may be enriched by scripts in either runtime. Interpret the
// provider through the consumer's runtime when looking up those fields.
func (s *Server) fiveMTableVisibilityDoc(owner, consumer *Document) *Document {
	if owner == nil || consumer == nil {
		return owner
	}
	providerProfile := s.getDocumentFiveMProfile(owner)
	consumerProfile := s.getDocumentFiveMProfile(consumer)
	if providerProfile.Env() != EnvShared {
		return owner
	}
	if consumerProfile.Env() == EnvClient {
		providerProfile.Kind = FiveMProfileClient
	} else if consumerProfile.Env() == EnvServer {
		providerProfile.Kind = FiveMProfileServer
	} else {
		return owner
	}
	return &Document{URI: owner.URI, FiveMProfile: providerProfile, FiveMProfileCached: true}
}

// mergeFiveMEventTypes keeps distinct object identities as alternatives while
// combining primitive bits. It never mutates slices held by the type cache.
func mergeFiveMEventTypes(left, right TypeSet) TypeSet {
	hasShape := func(t TypeSet) bool {
		return t.CustomName != "" || t.DeclNode != ast.InvalidNode || t.MetaNode != ast.InvalidNode
	}
	sameShape := func(a, b TypeSet) bool {
		return a.CustomName == b.CustomName && a.DeclURI == b.DeclURI && a.DeclNode == b.DeclNode && a.MetaURI == b.MetaURI && a.MetaNode == b.MetaNode
	}
	keepVariants := len(left.Alternatives) != 0 || len(right.Alternatives) != 0 || (hasShape(left) && hasShape(right) && !sameShape(left, right))
	var variants []TypeSet
	if keepVariants {
		add := func(t TypeSet) {
			if !hasShape(t) {
				return
			}
			t.Alternatives = nil
			for i := range variants {
				if sameShape(variants[i], t) {
					variants[i].Basics |= t.Basics
					return
				}
			}
			variants = append(variants, t)
		}
		if len(left.Alternatives) != 0 {
			for _, variant := range left.Alternatives {
				add(variant)
			}
		} else {
			add(left)
		}
		if len(right.Alternatives) != 0 {
			for _, variant := range right.Alternatives {
				add(variant)
			}
		} else {
			add(right)
		}
	}
	result := left
	result.Basics |= right.Basics
	if result.CustomName == "" {
		result.CustomName = right.CustomName
	} else if right.CustomName != "" && result.CustomName != right.CustomName {
		result.CustomName = ""
	}
	if result.DeclNode == ast.InvalidNode {
		result.DeclNode, result.DeclURI = right.DeclNode, right.DeclURI
	} else if right.DeclNode != ast.InvalidNode && (result.DeclNode != right.DeclNode || result.DeclURI != right.DeclURI) {
		result.DeclNode, result.DeclURI = ast.InvalidNode, ""
	}
	if result.MetaNode == ast.InvalidNode {
		result.MetaNode, result.MetaURI = right.MetaNode, right.MetaURI
	} else if right.MetaNode != ast.InvalidNode && (result.MetaNode != right.MetaNode || result.MetaURI != right.MetaURI) {
		result.MetaNode, result.MetaURI = ast.InvalidNode, ""
	}
	if len(variants) > 1 {
		result.Alternatives = variants
		result.CustomName, result.DeclURI, result.MetaURI = "", "", ""
		result.DeclNode, result.MetaNode = ast.InvalidNode, ast.InvalidNode
	}
	return result
}
