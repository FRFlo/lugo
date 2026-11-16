package lsp

import (
	"bytes"

	"github.com/FRFlo/lugo/ast"
)

// inferMetatableAttachment applies the last preceding setmetatable call for
// this exact local binding. Matching resolved definitions preserves lexical
// shadowing, and comparing source offsets keeps attachments in statement order.
func (doc *Document) inferMetatableAttachment(identID, defID ast.NodeID, inferred TypeSet) TypeSet {
	if doc == nil || doc.Tree == nil || doc.Resolver == nil || identID == ast.InvalidNode || defID == ast.InvalidNode || int(identID) >= len(doc.Tree.Nodes) {
		if doc == nil || doc.Tree == nil || doc.Server == nil || identID == ast.InvalidNode || int(identID) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[identID].Kind != ast.KindIdent {
			return inferred
		}
		// Global variables have no resolver-local definition. Resolve their
		// visible workspace definition, then apply its source-ordered attachment.
		name := doc.nodeSource(identID)
		nameHash := ast.HashBytes(name)
		symbols, ok := doc.Server.inferGlobalSymbols(doc, 0, nameHash)
		if !ok {
			// Tests, standalone workspaces, and partially indexed projects may
			// not have a global index. Resolver globals still provide precise
			// definitions, with the same resource visibility filter.
			for _, candidate := range doc.Server.Documents {
				if candidate == nil || candidate.Resolver == nil || !doc.Server.canSeeSymbol(doc, candidate) {
					continue
				}
				for _, globalID := range candidate.Resolver.GlobalDefs {
					if bytes.Equal(candidate.nodeSource(globalID), name) {
						symbols = append(symbols, GlobalSymbol{URI: candidate.URI, NodeID: globalID})
					}
				}
			}
		}
		if len(symbols) != 1 {
			return inferred
		}
		objectDoc := doc.Server.Documents[symbols[0].URI]
		if objectDoc == nil || objectDoc.Resolver == nil {
			return inferred
		}

		callsByDocument := make(map[*Document][]metatableAttachmentCall)
		objectIndex := objectDoc.metatableAttachmentCalls()
		if calls := objectIndex.byDefinition[symbols[0].NodeID]; len(calls) > 0 {
			callsByDocument[objectDoc] = calls
		}
		for _, setupDoc := range doc.Server.Documents {
			if setupDoc == nil || setupDoc == objectDoc || !doc.Server.canSeeSymbol(doc, setupDoc) || !doc.Server.canSeeSymbol(setupDoc, objectDoc) {
				continue
			}
			index := setupDoc.metatableAttachmentCalls()
			if calls := index.byGlobalName[nameHash]; len(calls) > 0 {
				callsByDocument[setupDoc] = calls
			}
		}
		// Keep dependencies even for ambiguous setups. A conflict can cache the
		// receiver's base type without a metatable, and removing one setup must
		// invalidate that cached result so the remaining setup can take effect.
		doc.Server.trackMetatableTypeDependency(objectDoc.URI, doc.URI)
		for setupDoc := range callsByDocument {
			if setupDoc != objectDoc {
				doc.Server.trackMetatableTypeDependency(setupDoc.URI, doc.URI)
			}
		}
		if len(callsByDocument) != 1 {
			if len(callsByDocument) > 1 {
				inferred.MetaNode = ast.InvalidNode
				inferred.MetaURI = ""
			}
			return inferred
		}
		for setupDoc, calls := range callsByDocument {
			return setupDoc.inferMetatableAttachmentCallsAt(calls, inferred, ^uint32(0))
		}
		return inferred
	}
	if doc == nil || doc.Tree == nil || doc.Resolver == nil || identID == ast.InvalidNode || defID == ast.InvalidNode || int(identID) >= len(doc.Tree.Nodes) {
		return inferred
	}
	return doc.inferMetatableAttachmentAt(defID, inferred, doc.Tree.Nodes[identID].Start)
}

func (doc *Document) inferMetatableAttachmentAt(defID ast.NodeID, inferred TypeSet, offset uint32) TypeSet {
	if doc == nil || doc.Tree == nil || doc.Resolver == nil || defID == ast.InvalidNode {
		return inferred
	}
	index := doc.metatableAttachmentCalls()
	return doc.inferMetatableAttachmentCallsAt(index.byDefinition[defID], inferred, offset)
}

func (doc *Document) inferMetatableAttachmentCallsAt(calls []metatableAttachmentCall, inferred TypeSet, offset uint32) TypeSet {
	latest := metatableAttachmentCall{node: ast.InvalidNode}
	for _, call := range calls {
		if call.start >= offset {
			break
		}
		inOtherFunction := false
		for _, function := range call.functions {
			if offset < function.start || offset > function.end {
				inOtherFunction = true
				break
			}
		}
		if inOtherFunction {
			continue
		}
		inOtherBranch := false
		for i, branch := range call.branches {
			control := call.controls[i]
			if offset >= control.start && offset <= control.end && (offset < branch.start || offset > branch.end) {
				inOtherBranch = true
				break
			}
		}
		if !inOtherBranch {
			latest = call
		}
	}
	if latest.node == ast.InvalidNode {
		return inferred
	}
	if latest.uncertain {
		for _, branch := range latest.branches {
			if offset < branch.start || offset > branch.end {
				inferred.MetaNode = ast.InvalidNode
				inferred.MetaURI = ""
				return inferred
			}
		}
	}
	metaID := latest.meta
	if int(metaID) >= len(doc.Tree.Nodes) {
		return inferred
	}
	if doc.Tree.Nodes[metaID].Kind == ast.KindNil {
		inferred.MetaNode = ast.InvalidNode
		inferred.MetaURI = ""
		return inferred
	}
	meta := doc.InferType(metaID)
	if meta.DeclNode == ast.InvalidNode || meta.DeclURI != "" && meta.DeclURI != doc.URI {
		return inferred
	}
	inferred.MetaNode = meta.DeclNode
	inferred.MetaURI = doc.URI
	return inferred
}
