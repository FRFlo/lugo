package lsp

import "github.com/FRFlo/lugo/ast"

// completionIndexType returns the table used by a metatable's __index.
func (doc *Document) completionIndexType(metaID ast.NodeID) TypeSet {
	return doc.completionIndexTypeAt(metaID, ^uint32(0))
}

func (doc *Document) completionIndexTypeAt(metaID ast.NodeID, offset uint32) TypeSet {
	if metaID == ast.InvalidNode || int(metaID) >= len(doc.Tree.Nodes) {
		return TypeSet{}
	}
	if indexID := doc.metatableFieldValueAt(metaID, "__index", offset); indexID != ast.InvalidNode && doc.Tree.Nodes[indexID].Kind != ast.KindNil {
		inferred := doc.InferType(indexID)
		if doc.Tree.Nodes[indexID].Kind == ast.KindIdent {
			inferred = doc.inferMetatableAttachmentAt(doc.referenceAt(indexID), inferred, offset)
		}
		return inferred
	}
	return TypeSet{}
}
