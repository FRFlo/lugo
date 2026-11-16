package lsp

import (
	"github.com/FRFlo/lugo/ast"
)

type newIndexCandidateKey struct {
	doc  *Document
	node ast.NodeID
}

type newIndexBindingKey struct {
	doc      *Document
	def      ast.NodeID
	declDoc  *Document
	declNode ast.NodeID
}

// metatableFieldMayBeNilAt reports whether a table field can be absent at the
// lookup point. The base value includes unconditional writes; completed branch
// writes are additional possible states until a later unconditional write
// supersedes them.
func metatableFieldMayBeNilAt(doc *Document, tableID ast.NodeID, name string, offset uint32, base ast.NodeID) bool {
	if doc == nil || doc.Tree == nil || base == ast.InvalidNode || int(base) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[base].Kind == ast.KindNil {
		return true
	}
	if doc.Resolver == nil || offset == ^uint32(0) {
		return false
	}
	definition := doc.getDefForValue(tableID)
	if definition == ast.InvalidNode {
		return false
	}
	writes := doc.metatableFieldWrites().fieldWrites[definition][name]
	for i := len(writes) - 1; i >= 0; i-- {
		write := writes[i]
		if write.start >= offset || len(write.branches) == 0 || write.value == ast.InvalidNode || int(write.value) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[write.value].Kind != ast.KindNil {
			continue
		}
		branch, control := write.branches[0], write.controls[0]
		if control.end >= offset {
			continue
		}
		visible := true
		for _, function := range write.functions {
			if offset < function.start || offset > function.end {
				visible = false
				break
			}
		}
		if !visible {
			continue
		}
		newerInBranch, superseded := false, false
		for j := i + 1; j < len(writes); j++ {
			other := writes[j]
			if other.start >= offset {
				break
			}
			sameFunctions := len(write.functions) == len(other.functions)
			if sameFunctions {
				for k := range write.functions {
					if write.functions[k] != other.functions[k] {
						sameFunctions = false
						break
					}
				}
			}
			if !sameFunctions {
				continue
			}
			otherVisible := true
			for _, function := range other.functions {
				if offset < function.start || offset > function.end {
					otherVisible = false
					break
				}
			}
			if !otherVisible {
				continue
			}
			if len(other.branches) == 0 && other.start > control.end {
				superseded = true
			}
			if len(other.branches) > 0 && other.controls[0] == control && other.branches[0] == branch {
				newerInBranch = true
			}
		}
		if !newerInBranch && !superseded {
			return true
		}
	}
	return false
}

// routedNewIndexCandidates computes same-document routing in source order.
// isNewIndexRoutedWrite reconstructs the same presence state for each offset;
// doing that independently for every candidate makes a run of writes quadratic.
// This state is deliberately scoped to one inference and needs no invalidation.
func routedNewIndexCandidates(candidates []newIndexWriteCandidate, name string) map[newIndexCandidateKey]bool {
	routed := make(map[newIndexCandidateKey]bool, len(candidates))
	presentByBinding := make(map[newIndexBindingKey]bool)
	initialized := make(map[newIndexBindingKey]bool)
	for i := len(candidates) - 1; i >= 0; i-- {
		candidate := candidates[i]
		doc := candidate.doc
		if doc == nil || doc.Tree == nil || doc.Resolver == nil || candidate.tree != doc.Tree || candidate.member == ast.InvalidNode || int(candidate.member) >= len(doc.Tree.Nodes) || candidate.node == ast.InvalidNode || int(candidate.node) >= len(doc.Tree.Nodes) {
			continue
		}
		member := doc.Tree.Nodes[candidate.member]
		targetType := doc.InferType(member.Left)
		declDoc := doc
		if targetType.DeclURI != "" && targetType.DeclURI != doc.URI && doc.Server != nil {
			declDoc = doc.Server.Documents[targetType.DeclURI]
		}
		if declDoc == nil || declDoc.Tree == nil {
			continue
		}
		definition := ast.InvalidNode
		if targetType.DeclNode != ast.InvalidNode && int(targetType.DeclNode) < len(declDoc.Tree.Nodes) {
			// DeclNode is owned by DeclURI. Inferred receiver types can point
			// into a stub or another workspace document, so resolve it there.
			definition = declDoc.getDefForValue(targetType.DeclNode)
		}
		if definition != ast.InvalidNode && declDoc == doc && doc.referenceAt(member.Left) != definition {
			continue
		}
		key := newIndexBindingKey{doc: doc, def: definition, declDoc: declDoc, declNode: targetType.DeclNode}
		if !initialized[key] {
			if targetType.DeclNode != ast.InvalidNode {
				if ownValue := declDoc.findFieldInTable(targetType.DeclNode, name); ownValue != ast.InvalidNode && int(ownValue) < len(declDoc.Tree.Nodes) {
					presentByBinding[key] = declDoc.Tree.Nodes[ownValue].Kind != ast.KindNil
				}
			}
			initialized[key] = true
		}
		present := presentByBinding[key]
		isRouted := false
		if !present && targetType.MetaNode != ast.InvalidNode {
			priorMeta := doc
			priorOffset := candidate.assignStart
			if targetType.MetaURI != "" && targetType.MetaURI != doc.URI {
				if doc.Server == nil {
					continue
				}
				priorMeta = doc.Server.Documents[targetType.MetaURI]
				priorOffset = ^uint32(0)
			}
			if priorMeta != nil {
				method := priorMeta.metatableFieldValueAt(targetType.MetaNode, "__newindex", priorOffset)
				isRouted = method != ast.InvalidNode && int(method) < len(priorMeta.Tree.Nodes) && priorMeta.Tree.Nodes[method].Kind != ast.KindNil
			}
		}
		routed[newIndexCandidateKey{doc: doc, node: candidate.node}] = isRouted
		if !isRouted && definition != ast.InvalidNode {
			if value := doc.getAssignedValue(candidate.node); value != ast.InvalidNode && int(value) < len(doc.Tree.Nodes) {
				presentByBinding[key] = doc.Tree.Nodes[value].Kind != ast.KindNil
			}
		}
	}
	return routed
}

// metatableNewIndexTarget resolves only table-valued __newindex handlers.
// Function handlers execute arbitrary code, so their effects are not inferred.
func (doc *Document) metatableNewIndexTarget(metaID ast.NodeID) (*Document, ast.NodeID, bool) {
	if doc == nil || metaID == ast.InvalidNode || int(metaID) >= len(doc.Tree.Nodes) {
		return nil, ast.InvalidNode, false
	}
	valueID := doc.metatableFieldValueAt(metaID, "__newindex", ^uint32(0))
	if valueID == ast.InvalidNode {
		return nil, ast.InvalidNode, false
	}
	targetDoc, targetID := doc.resolveMetatableIndexValue(valueID)
	if targetDoc == nil || targetID == ast.InvalidNode || int(targetID) >= len(targetDoc.Tree.Nodes) {
		return nil, ast.InvalidNode, false
	}
	if targetDoc.Tree.Nodes[targetID].Kind == ast.KindTableExpr {
		return targetDoc, targetID, true
	}
	targetType := targetDoc.InferType(targetID)
	if targetType.DeclNode != ast.InvalidNode {
		declDoc := targetDoc
		if targetType.DeclURI != "" && targetType.DeclURI != targetDoc.URI && targetDoc.Server != nil {
			declDoc = targetDoc.Server.Documents[targetType.DeclURI]
		}
		if declDoc != nil && int(targetType.DeclNode) < len(declDoc.Tree.Nodes) && declDoc.Tree.Nodes[targetType.DeclNode].Kind == ast.KindTableExpr {
			return declDoc, targetType.DeclNode, true
		}
	}
	return nil, ast.InvalidNode, false
}

func (doc *Document) isNewIndexRoutedWrite(receiver TypeSet, name string, offset uint32) bool {
	if doc == nil || receiver.MetaNode == ast.InvalidNode {
		return false
	}
	declDoc := doc
	if receiver.DeclURI != "" && receiver.DeclURI != doc.URI && doc.Server != nil {
		declDoc = doc.Server.Documents[receiver.DeclURI]
	}
	present := false
	if declDoc != nil && receiver.DeclNode != ast.InvalidNode {
		if ownValue := declDoc.findFieldInTable(receiver.DeclNode, name); ownValue != ast.InvalidNode && int(ownValue) < len(declDoc.Tree.Nodes) {
			present = declDoc.Tree.Nodes[ownValue].Kind != ast.KindNil
		}
	}
	if declDoc == doc && receiver.DeclNode != ast.InvalidNode {
		definition := doc.getDefForValue(receiver.DeclNode)
		var candidates []newIndexWriteCandidate
		if doc.Server != nil {
			candidates = doc.Server.newIndexWriteCandidates(name)
		} else {
			candidates = buildNewIndexWriteCandidates([]*Document{doc}, name)
		}
		for i := len(candidates) - 1; i >= 0; i-- {
			candidate := candidates[i]
			if candidate.doc != doc || candidate.tree != doc.Tree || candidate.assignStart >= offset || int(candidate.member) >= len(doc.Tree.Nodes) {
				continue
			}
			member := doc.Tree.Nodes[candidate.member]
			if definition == ast.InvalidNode || doc.referenceAt(member.Left) != definition {
				continue
			}
			priorType := doc.InferType(member.Left)
			if !present && priorType.MetaNode != ast.InvalidNode {
				priorMeta := doc
				priorOffset := candidate.assignStart
				if priorType.MetaURI != "" && priorType.MetaURI != doc.URI {
					if doc.Server == nil {
						continue
					}
					priorMeta = doc.Server.Documents[priorType.MetaURI]
					priorOffset = ^uint32(0)
				}
				if priorMeta != nil {
					method := priorMeta.metatableFieldValueAt(priorType.MetaNode, "__newindex", priorOffset)
					if method != ast.InvalidNode && priorMeta.Tree.Nodes[method].Kind != ast.KindNil {
						continue
					}
				}
			}
			if value := doc.getAssignedValue(candidate.node); value != ast.InvalidNode {
				present = doc.Tree.Nodes[value].Kind != ast.KindNil
			}
		}
	}
	if present {
		return false
	}

	metaDoc := doc
	if receiver.MetaURI != "" && receiver.MetaURI != doc.URI && doc.Server != nil {
		metaDoc = doc.Server.Documents[receiver.MetaURI]
	}
	if metaDoc == nil || receiver.MetaNode == ast.InvalidNode || int(receiver.MetaNode) >= len(metaDoc.Tree.Nodes) {
		return false
	}
	lookupOffset := offset
	if metaDoc != doc {
		lookupOffset = ^uint32(0)
	}
	value := metaDoc.metatableFieldValueAt(receiver.MetaNode, "__newindex", lookupOffset)
	return value != ast.InvalidNode && metaDoc.Tree.Nodes[value].Kind != ast.KindNil
}

// findNewIndexWrite finds an assignment redirected into receiverType's table.
// It deliberately requires the same resolved local binding, preserving shadowing.
func (doc *Document) newIndexWrite(receiverType TypeSet, name string, offset uint32) (*Document, ast.NodeID, ast.NodeID) {
	if doc == nil || doc.Resolver == nil || receiverType.DeclNode == ast.InvalidNode {
		return nil, ast.InvalidNode, ast.InvalidNode
	}
	var candidates []newIndexWriteCandidate
	if doc.Server != nil {
		candidates = doc.Server.newIndexWriteCandidates(name)
	} else {
		candidates = buildNewIndexWriteCandidates([]*Document{doc}, name)
	}
	routedCandidates := routedNewIndexCandidates(candidates, name)
	var bestDoc *Document
	var bestNode, bestValue ast.NodeID = ast.InvalidNode, ast.InvalidNode
	var bestStart uint32
	for _, candidate := range candidates {
		candidateDoc, nodeID := candidate.doc, candidate.node
		// Offsets have meaning only within one syntax tree. Cross-document writes
		// have no statically known execution order, so they cannot be ranked here.
		if candidateDoc == nil || candidateDoc != doc || candidateDoc.Resolver == nil || candidateDoc.Tree == nil || candidate.tree != candidateDoc.Tree || candidate.member == ast.InvalidNode || int(candidate.member) >= len(candidateDoc.Tree.Nodes) || nodeID == ast.InvalidNode || int(nodeID) >= len(candidateDoc.Tree.Nodes) || candidate.assignStart >= offset {
			continue
		}
		if bestDoc != nil && candidate.assignStart < bestStart {
			break
		}
		member := candidateDoc.Tree.Nodes[candidate.member]
		targetType := candidateDoc.InferType(member.Left)
		if targetType.MetaNode == ast.InvalidNode {
			continue
		}
		routed, known := routedCandidates[newIndexCandidateKey{doc: candidateDoc, node: candidate.node}]
		if !known {
			routed = candidateDoc.isNewIndexRoutedWrite(targetType, name, candidate.assignStart)
		}
		if !routed {
			continue
		}
		metaDoc := candidateDoc
		if targetType.MetaURI != "" && targetType.MetaURI != candidateDoc.URI {
			if candidateDoc.Server == nil {
				continue
			}
			metaDoc = candidateDoc.Server.Documents[targetType.MetaURI]
		}
		if metaDoc == nil {
			continue
		}
		matched := false
		seen := make(map[metatableVisit]struct{})
		for metaDoc != nil && targetType.MetaNode != ast.InvalidNode {
			key := metatableVisit{uri: metaDoc.URI, node: targetType.MetaNode}
			if _, exists := seen[key]; exists {
				break
			}
			seen[key] = struct{}{}
			indexOffset := candidate.assignStart
			if metaDoc != candidateDoc {
				// This offset belongs to candidateDoc's tree. For another document,
				// use its latest known metatable value without comparing offsets.
				indexOffset = ^uint32(0)
			}
			indexID := metaDoc.metatableFieldValueAt(targetType.MetaNode, "__newindex", indexOffset)
			if indexID == ast.InvalidNode {
				break
			}
			fallbackType := metaDoc.InferType(indexID)
			fallbackDoc := metaDoc
			if fallbackType.DeclURI != "" && fallbackType.DeclURI != metaDoc.URI {
				if metaDoc.Server == nil {
					break
				}
				fallbackDoc = metaDoc.Server.Documents[fallbackType.DeclURI]
			}
			if fallbackDoc == nil || fallbackType.DeclNode == ast.InvalidNode || int(fallbackType.DeclNode) >= len(fallbackDoc.Tree.Nodes) || fallbackDoc.Tree.Nodes[fallbackType.DeclNode].Kind != ast.KindTableExpr {
				break
			}
			if fallbackType.DeclNode == receiverType.DeclNode && (receiverType.DeclURI == "" || receiverType.DeclURI == fallbackDoc.URI) {
				matched = true
				break
			}
			fallbackOffset := candidate.assignStart
			if fallbackDoc != candidateDoc {
				fallbackOffset = ^uint32(0)
			}
			field := fallbackDoc.metatableFieldValueAt(fallbackType.DeclNode, name, fallbackOffset)
			if !metatableFieldMayBeNilAt(fallbackDoc, fallbackType.DeclNode, name, fallbackOffset, field) {
				break
			}
			metaDoc = fallbackDoc
			if fallbackType.MetaURI != "" && fallbackType.MetaURI != fallbackDoc.URI {
				if fallbackDoc.Server == nil {
					break
				}
				metaDoc = fallbackDoc.Server.Documents[fallbackType.MetaURI]
			}
			targetType = fallbackType
		}
		if !matched {
			continue
		}
		if bestDoc == nil || candidate.assignStart > bestStart {
			bestDoc, bestNode, bestValue = candidateDoc, nodeID, candidateDoc.getAssignedValue(nodeID)
			bestStart = candidate.assignStart
		}
	}
	return bestDoc, bestNode, bestValue
}

// newIndexWriteValues returns possible write values for inference. Same-document
// writes use the lookup offset; other documents contribute conservatively because
// their execution order relative to the lookup is unknown. Conditional branches
// are merged as an approximation rather than treated as execution order.
func (doc *Document) newIndexWriteValues(receiverType TypeSet, name string, offset uint32) []TypeSet {
	if doc == nil {
		return nil
	}
	var candidates []newIndexWriteCandidate
	if doc.Server != nil {
		candidates = doc.Server.newIndexWriteCandidates(name)
	} else {
		candidates = buildNewIndexWriteCandidates([]*Document{doc}, name)
	}
	routedCandidates := routedNewIndexCandidates(candidates, name)
	var values []TypeSet
	for _, candidate := range candidates {
		candidateDoc := candidate.doc
		if candidateDoc == nil || (candidateDoc == doc && candidate.assignStart >= offset) {
			continue
		}
		// The lookup offset only orders writes in the current syntax tree.
		// Writes in other documents are possible contributions, but their
		// offsets cannot be compared with it and their execution order is unknown.
		// The candidate's own offset is still local to candidateDoc's tree.
		if candidateDoc != doc && (candidateDoc.Resolver == nil || candidateDoc.Tree == nil) || candidateDoc == doc && doc.Resolver == nil {
			continue
		}
		node, value := candidate.node, candidateDoc.getAssignedValue(candidate.node)
		if node == ast.InvalidNode || value == ast.InvalidNode || candidate.tree != candidateDoc.Tree || candidate.member == ast.InvalidNode || int(candidate.member) >= len(candidateDoc.Tree.Nodes) || int(node) >= len(candidateDoc.Tree.Nodes) {
			continue
		}
		member := candidateDoc.Tree.Nodes[candidate.member]
		targetType := candidateDoc.InferType(member.Left)
		if targetType.MetaNode == ast.InvalidNode {
			continue
		}
		routed, known := routedCandidates[newIndexCandidateKey{doc: candidateDoc, node: node}]
		if !known {
			routed = candidateDoc.isNewIndexRoutedWrite(targetType, name, candidate.assignStart)
		}
		if !routed {
			continue
		}
		metaDoc := candidateDoc
		if targetType.MetaURI != "" && targetType.MetaURI != candidateDoc.URI {
			if candidateDoc.Server == nil {
				continue
			}
			metaDoc = candidateDoc.Server.Documents[targetType.MetaURI]
		}
		matched := false
		var seen [8]metatableVisit
		seenCount := 0
		var overflowSeen map[metatableVisit]struct{}
		for metaDoc != nil && targetType.MetaNode != ast.InvalidNode {
			key := metatableVisit{uri: metaDoc.URI, node: targetType.MetaNode}
			exists := false
			for i := 0; i < seenCount; i++ {
				if seen[i] == key {
					exists = true
					break
				}
			}
			if !exists && overflowSeen != nil {
				_, exists = overflowSeen[key]
			}
			if exists {
				break
			}
			if seenCount < len(seen) {
				seen[seenCount] = key
				seenCount++
			} else {
				if overflowSeen == nil {
					overflowSeen = make(map[metatableVisit]struct{})
				}
				overflowSeen[key] = struct{}{}
			}
			indexOffset := candidate.assignStart
			if metaDoc != candidateDoc {
				indexOffset = ^uint32(0)
			}
			indexID := metaDoc.metatableFieldValueAt(targetType.MetaNode, "__newindex", indexOffset)
			if indexID == ast.InvalidNode {
				break
			}
			fallbackType := metaDoc.InferType(indexID)
			fallbackDoc := metaDoc
			if fallbackType.DeclURI != "" && fallbackType.DeclURI != metaDoc.URI {
				if metaDoc.Server == nil {
					break
				}
				fallbackDoc = metaDoc.Server.Documents[fallbackType.DeclURI]
			}
			if fallbackDoc == nil || fallbackType.DeclNode == ast.InvalidNode || int(fallbackType.DeclNode) >= len(fallbackDoc.Tree.Nodes) || fallbackDoc.Tree.Nodes[fallbackType.DeclNode].Kind != ast.KindTableExpr {
				break
			}
			if fallbackType.DeclNode == receiverType.DeclNode && (receiverType.DeclURI == "" || receiverType.DeclURI == fallbackDoc.URI) {
				matched = true
				break
			}
			fallbackOffset := candidate.assignStart
			if fallbackDoc != candidateDoc {
				fallbackOffset = ^uint32(0)
			}
			field := fallbackDoc.metatableFieldValueAt(fallbackType.DeclNode, name, fallbackOffset)
			if !metatableFieldMayBeNilAt(fallbackDoc, fallbackType.DeclNode, name, fallbackOffset, field) {
				break
			}
			metaDoc = fallbackDoc
			if fallbackType.MetaURI != "" && fallbackType.MetaURI != fallbackDoc.URI {
				if fallbackDoc.Server == nil {
					break
				}
				metaDoc = fallbackDoc.Server.Documents[fallbackType.MetaURI]
			}
			targetType = fallbackType
		}
		if !matched {
			continue
		}
		values = append(values, candidateDoc.InferType(value))
		if candidateDoc == doc && !doc.newIndexWriteIsConditionalAt(node, offset) {
			break
		}
	}
	return values
}

// A write in a control-flow body is only guaranteed for a lookup inside the
// same body; outside it, earlier writes may also reach the lookup.
func (doc *Document) newIndexWriteIsConditionalAt(node ast.NodeID, offset uint32) bool {
	if node == ast.InvalidNode || int(node) >= len(doc.Tree.Nodes) {
		return true
	}
	child := node
	for parent := doc.Tree.Nodes[node].Parent; parent != ast.InvalidNode && int(parent) < len(doc.Tree.Nodes); parent = doc.Tree.Nodes[parent].Parent {
		ancestor := doc.Tree.Nodes[parent]
		switch ancestor.Kind {
		case ast.KindIf, ast.KindElseIf, ast.KindElse, ast.KindWhile, ast.KindForNum, ast.KindForIn, ast.KindRepeat, ast.KindFunctionExpr:
			branch := doc.Tree.Nodes[child]
			if offset < branch.Start || offset > branch.End {
				return true
			}
		}
		child = parent
	}
	return false
}
