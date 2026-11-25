package lsp

import (
	"sort"
	"strconv"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

type hoverTableKey struct {
	uri  string
	id   ast.NodeID
	name string
}

// hoverTableDetail uses the same declaration-backed types as member inference.
func (s *Server) hoverTableDetail(doc *Document, ctx *SymbolContext, offset uint32, depth int) (string, bool) {
	if doc == nil || ctx == nil {
		return "", false
	}
	var typ TypeSet
	if ctx.IsProp {
		parent := doc.Tree.Nodes[ctx.IdentNodeID].Parent
		if parent != ast.InvalidNode {
			typ = doc.InferType(parent)
		}
		if typ.DeclNode == ast.InvalidNode && typ.CustomName == "" && ctx.TargetDoc != nil && ctx.TargetDefID != ast.InvalidNode {
			typ = ctx.TargetDoc.InferType(ctx.TargetDefID)
		}
	} else if ctx.TargetDoc != nil && ctx.TargetDefID != ast.InvalidNode {
		typ = ctx.TargetDoc.InferType(ctx.TargetDefID)
	}
	typ = doc.ContextualType(ctx.IdentNodeID, offset, typ)
	return s.formatHoverTable(doc, typ, depth, 0, make(map[hoverTableKey]bool))
}

func (s *Server) formatHoverTable(doc *Document, typ TypeSet, depth, indent int, seen map[hoverTableKey]bool) (string, bool) {
	owner := doc
	if typ.DeclURI != "" && typ.DeclURI != doc.URI {
		owner = s.Documents[typ.DeclURI]
	}
	if owner == nil || owner.Tree == nil {
		return "", false
	}
	validTable := typ.DeclNode != ast.InvalidNode && int(typ.DeclNode) < len(owner.Tree.Nodes) && owner.Tree.Nodes[typ.DeclNode].Kind == ast.KindTableExpr
	if !validTable && typ.CustomName == "" {
		return "", false
	}
	key := hoverTableKey{uri: owner.URI, id: typ.DeclNode, name: typ.CustomName}
	if seen[key] {
		return "", false
	}
	seen[key] = true
	defer delete(seen, key)

	fields := make(map[string]TypeSet)
	if validTable {
		node := owner.Tree.Nodes[typ.DeclNode]
		arrayIndex := 0
		for i := uint32(0); i < node.Count; i++ {
			fieldID := owner.Tree.ExtraList[node.Extra+i]
			field := owner.Tree.Nodes[fieldID]
			var name string
			known := false
			value := field.Right
			switch field.Kind {
			case ast.KindRecordField:
				name = string(owner.nodeSource(field.Left))
				known = name != ""
			case ast.KindIndexField:
				if field.Left != ast.InvalidNode && owner.Tree.Nodes[field.Left].Kind == ast.KindString {
					name, known = decodeLuaString(string(owner.nodeSource(field.Left)))
				}
				if !known && field.Left != ast.InvalidNode && owner.Tree.Nodes[field.Left].Kind == ast.KindNumber {
					name = "\x00" + string(owner.nodeSource(field.Left))
					known = true
				}
			default:
				arrayIndex++
				name = "\x00" + strconv.Itoa(arrayIndex)
				known = true
				value = fieldID
			}
			if known && value != ast.InvalidNode {
				fields[name] = owner.InferType(value)
			}
		}
	}
	if validTable && owner.Resolver != nil {
		def := owner.getDefForValue(typ.DeclNode)
		receiverName := string(owner.nodeSource(def))
		for _, field := range owner.Resolver.FieldDefs {
			if field.NodeID == def && len(field.ReceiverName) > 0 {
				receiverName = string(field.ReceiverName) + "." + receiverName
				def = field.ReceiverDef
				break
			}
		}
		for _, field := range owner.Resolver.FieldDefs {
			if def == ast.InvalidNode || field.ReceiverDef != def || string(field.ReceiverName) != receiverName {
				continue
			}
			parent := owner.Tree.Nodes[field.NodeID].Parent
			if parent != ast.InvalidNode && owner.Tree.Nodes[parent].Kind == ast.KindRecordField {
				continue
			}
			name := string(owner.nodeSource(field.NodeID))
			if _, exists := fields[name]; exists || name == "" {
				continue
			}
			value := owner.getAssignedValue(field.NodeID)
			if value != ast.InvalidNode {
				fields[name] = owner.InferType(value)
			}
		}
	}
	if typ.CustomName != "" && s.GlobalIndex != nil {
		lookupDoc := doc
		if owner != doc {
			lookupDoc = s.fiveMTableVisibilityDoc(owner, doc)
		}
		resource, scope := s.globalIndexContext(lookupDoc)
		className := luaDocGenericClassName(typ.CustomName)
		_, args, _ := splitLuaDocGenericType(typ.CustomName)
		hierarchy := s.GlobalIndex.LookupLuaDocClassHierarchy(resource, scope, className)
		reachable := s.luaDocClassFieldReachability(lookupDoc, hierarchy)
		bindings := luaDocGenericBindingsForHierarchy(hierarchy, args)
		classFields := s.luaDocClassFields(resource, scope, className, hierarchy)
		classDepth := func(candidate LuaDocClassFieldLookup) int {
			if candidate.ClassIndex < 0 || candidate.ClassIndex >= len(hierarchy) {
				return len(hierarchy)
			}
			return hierarchy[candidate.ClassIndex].Depth
		}
		sort.SliceStable(classFields, func(i, j int) bool { return classDepth(classFields[i]) < classDepth(classFields[j]) })
		classNames := make(map[string]bool)
		for _, candidate := range classFields {
			entry := candidate.Entry
			if entry == nil || candidate.ClassIndex < 0 || candidate.ClassIndex >= len(hierarchy) || !s.canSeeLuaDocClassFieldLookup(s.Documents[entry.URI], candidate, reachable) {
				continue
			}
			name := string(entry.Name)
			if separator := strings.LastIndexAny(name, ".:"); separator >= 0 {
				name = name[separator+1:]
			}
			if classNames[name] {
				continue
			}
			classNames[name] = true
			if entry.LuaDoc != nil && entry.LuaDoc.TypeString != "" {
				fieldType := entry.LuaDoc.TypeString
				if candidate.ClassIndex < len(bindings) {
					fieldType = substituteLuaDocGenericType(fieldType, bindings[candidate.ClassIndex])
				}
				fields[name] = ParseTypeString(fieldType)
			} else if entry.Type.Primitive != TypeUnknown {
				fields[name] = TypeSet{Basics: entry.Type.Primitive}
			}
		}
	}
	if len(fields) == 0 {
		return "", false
	}
	if depth == 0 {
		return "", true
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("{\n")
	more := false
	for _, name := range names {
		field := fields[name]
		b.WriteString(strings.Repeat("  ", indent+1))
		if strings.HasPrefix(name, "\x00") {
			b.WriteString("[" + name[1:] + "]")
		} else if !isHoverIdentifier(name) {
			b.WriteString("[" + strconv.Quote(name) + "]")
		} else {
			b.WriteString(name)
		}
		b.WriteString(": ")
		nested, nestedMore := s.formatHoverTable(owner, field, depth-1, indent+1, seen)
		if nested != "" {
			if field.CustomName != "" || field.Basics != TypeTable {
				b.WriteString(field.Format() + " ")
			}
			b.WriteString(nested)
		} else {
			b.WriteString(field.Format())
		}
		more = more || nestedMore
		b.WriteString(",\n")
	}
	b.WriteString(strings.Repeat("  ", indent))
	b.WriteString("}")
	return b.String(), more
}

func isHoverIdentifier(name string) bool {
	switch name {
	case "and", "break", "do", "else", "elseif", "end", "false", "for", "function", "goto", "if", "in", "local", "nil", "not", "or", "repeat", "return", "then", "true", "until", "while":
		return false
	}
	for i, c := range name {
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return name != ""
}
