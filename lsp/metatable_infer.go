package lsp

import (
	"github.com/FRFlo/lugo/ast"
)

// inferMetatableField follows Lua's table-valued __index chain. The depth and
// visited checks mirror Lua's bounded lookup and keep malformed cycles safe.
func (doc *Document) inferMetatableField(metaID ast.NodeID, name []byte, visited map[metatableVisit]struct{}, depth int) (TypeSet, bool) {
	return doc.inferMetatableFieldAt(metaID, name, visited, depth, ^uint32(0))
}

func (doc *Document) inferMetatableFieldAt(metaID ast.NodeID, name []byte, visited map[metatableVisit]struct{}, depth int, offset uint32) (TypeSet, bool) {
	if doc == nil || doc.Tree == nil || metaID == ast.InvalidNode || int(metaID) >= len(doc.Tree.Nodes) || depth >= 200 {
		return TypeSet{}, false
	}
	key := metatableVisit{uri: doc.URI, node: metaID}
	if _, exists := visited[key]; exists {
		return TypeSet{}, false
	}
	visited[key] = struct{}{}

	indexValues := doc.metatableFieldAlternativesAt(metaID, "__index", offset)
	if len(indexValues) > 1 {
		var union TypeSet
		for _, valueID := range indexValues {
			if int(valueID) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[valueID].Kind == ast.KindNil {
				continue
			}
			valueDoc, targetID := doc.resolveMetatableIndexValue(valueID)
			if valueDoc == nil || targetID == ast.InvalidNode || int(targetID) >= len(valueDoc.Tree.Nodes) {
				continue
			}
			if valueDoc.Tree.Nodes[targetID].Kind == ast.KindTableExpr {
				field := valueDoc.metatableFieldValueAt(targetID, string(name), metatableOffsetForDocument(doc, valueDoc, offset))
				if field != ast.InvalidNode && valueDoc.Tree.Nodes[field].Kind != ast.KindNil {
					part := valueDoc.InferType(field)
					union.Basics |= part.Basics
					if union.DeclNode == ast.InvalidNode {
						union.DeclNode, union.DeclURI = part.DeclNode, part.DeclURI
					}
				}
			} else if valueDoc.Tree.Nodes[targetID].Kind == ast.KindFunctionExpr {
				part := valueDoc.inferFunctionReturnType(targetID)
				union.Basics |= part.Basics
				if union.DeclNode == ast.InvalidNode {
					union.DeclNode, union.DeclURI = part.DeclNode, part.DeclURI
				}
			}
		}
		if union.Basics != 0 {
			return union, true
		}
	}
	if valueID := doc.metatableFieldValueAt(metaID, "__index", offset); valueID != ast.InvalidNode && doc.Tree.Nodes[valueID].Kind != ast.KindNil {
		valueDoc, targetID := doc.resolveMetatableIndexValue(valueID)
		if valueDoc != nil && targetID != ast.InvalidNode {
			node := valueDoc.Tree.Nodes[targetID]
			switch node.Kind {
			case ast.KindTableExpr:
				if fieldID := valueDoc.metatableFieldValueAt(targetID, string(name), metatableOffsetForDocument(doc, valueDoc, offset)); fieldID != ast.InvalidNode && valueDoc.Tree.Nodes[fieldID].Kind != ast.KindNil {
					return valueDoc.InferType(fieldID), true
				}
				// Preserve a metatable attached to the table through a separate
				// setmetatable call on the original reference.
				indexType := doc.InferType(valueID)
				if doc.Tree.Nodes[valueID].Kind == ast.KindIdent {
					indexType = doc.inferMetatableAttachmentAt(doc.referenceAt(valueID), indexType, offset)
				}
				if indexType.MetaNode != ast.InvalidNode {
					metaDoc := doc
					if indexType.MetaURI != "" && indexType.MetaURI != doc.URI && doc.Server != nil {
						metaDoc = doc.Server.Documents[indexType.MetaURI]
					}
					if metaDoc != nil {
						if inherited, ok := metaDoc.inferMetatableFieldAt(indexType.MetaNode, name, visited, depth+1, metatableOffsetForDocument(doc, metaDoc, offset)); ok {
							return inherited, true
						}
					}
				}
			case ast.KindFunctionExpr:
				return valueDoc.inferFunctionReturnType(targetID), true
			}
			if node.Kind != ast.KindTableExpr {
				targetType := valueDoc.InferType(targetID)
				if targetType.DeclNode != ast.InvalidNode && targetType.DeclURI == valueDoc.URI {
					if fieldID := valueDoc.metatableFieldValueAt(targetType.DeclNode, string(name), metatableOffsetForDocument(doc, valueDoc, offset)); fieldID != ast.InvalidNode && valueDoc.Tree.Nodes[fieldID].Kind != ast.KindNil {
						return valueDoc.InferType(fieldID), true
					}
				}
				if targetType.MetaNode != ast.InvalidNode {
					metaDoc := valueDoc
					if targetType.MetaURI != "" && targetType.MetaURI != valueDoc.URI && valueDoc.Server != nil {
						metaDoc = valueDoc.Server.Documents[targetType.MetaURI]
					}
					if metaDoc != nil {
						if inherited, ok := metaDoc.inferMetatableFieldAt(targetType.MetaNode, name, visited, depth+1, metatableOffsetForDocument(doc, metaDoc, offset)); ok {
							return inherited, true
						}
					}
				}
			}
		}
	}

	return TypeSet{}, false
}

func (doc *Document) resolveMetatableIndexValue(id ast.NodeID) (*Document, ast.NodeID) {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return nil, ast.InvalidNode
	}
	node := doc.Tree.Nodes[id]
	if node.Kind == ast.KindIdent {
		if def := doc.referenceAt(id); def != ast.InvalidNode {
			value := doc.getAssignedValue(def)
			if value != ast.InvalidNode {
				return doc, value
			}
		} else if doc.Server != nil {
			hash := ast.HashBytes(doc.nodeSource(id))
			if symbols, ok := doc.Server.inferGlobalSymbols(doc, 0, hash); ok && len(symbols) != 0 {
				if target := doc.Server.Documents[symbols[0].URI]; target != nil {
					if value := target.getAssignedValue(symbols[0].NodeID); value != ast.InvalidNode {
						doc.Server.trackMetatableTypeDependency(target.URI, doc.URI)
						return target, value
					}
				}
			}
		}
	}
	return doc, id
}

type metatableVisit struct {
	uri  string
	node ast.NodeID
}

func metatableOffsetForDocument(source, target *Document, offset uint32) uint32 {
	if source != target {
		return ^uint32(0)
	}
	return offset
}
