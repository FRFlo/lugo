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

type hoverFormatState struct {
	active  map[hoverTableKey]bool
	classes map[hoverClassKey]hoverClassFields
	aliases map[hoverClassKey]hoverAlias
}

type hoverClassKey struct {
	doc  *Document
	name string
}

type hoverClassFields struct {
	types   map[string]TypeSet
	raw     map[string]string
	names   []string
	flat    bool
	renders map[int]string
}

type hoverAlias struct {
	doc      *Document
	typeName string
}

func (s *Server) hoverIdentifierType(doc *Document, ctx *SymbolContext) TypeSet {
	typ := doc.InferType(ctx.IdentNodeID)
	if typ.DeclNode != ast.InvalidNode || typ.CustomName != "" || len(typ.Alternatives) != 0 || s.GlobalIndex == nil || !ctx.IsGlobal {
		return typ
	}
	resource := s.getDocumentFiveMProfile(doc).ResourceRoot
	if resource == "" {
		return typ
	}

	// The callback restoring a resource global can live in a different script
	// from the initializer selected as its definition.
	key := GlobalKey{PropHash: ast.HashBytes(doc.nodeSource(ctx.IdentNodeID))}
	for _, symbol := range s.visibleGlobalSymbolsFromEntries(doc, s.GlobalIndex.SymbolsByHash(key), 0) {
		source := s.Documents[symbol.URI]
		if source == doc || s.getDocumentFiveMProfile(source).ResourceRoot != resource {
			continue
		}
		value := source.getAssignedValue(symbol.NodeID)
		if value == ast.InvalidNode || source.Tree.Nodes[value].Kind != ast.KindIdent {
			continue
		}
		inferred := source.InferType(value)
		if owner, _ := s.fiveMTableGlobalOwner(inferred); owner != nil && s.getDocumentFiveMProfile(owner).ResourceRoot != resource {
			typ = mergeFiveMEventTypes(typ, inferred)
		}
	}
	return typ
}

// hoverTableDetail uses the same declaration-backed types as member inference.
func (s *Server) hoverTableDetail(doc *Document, ctx *SymbolContext, offset uint32, depth int, typ TypeSet) (string, bool) {
	if doc == nil || ctx == nil {
		return "", false
	}
	var raw string
	if ctx.IsProp {
		parent := doc.Tree.Nodes[ctx.IdentNodeID].Parent
		if parent != ast.InvalidNode {
			typ = doc.InferType(parent)
		}
		if typ.DeclNode == ast.InvalidNode && typ.CustomName == "" && ctx.TargetDoc != nil && ctx.TargetDefID != ast.InvalidNode {
			typ = ctx.TargetDoc.InferType(ctx.TargetDefID)
		}
	}
	if ctx.TargetDoc != nil && ctx.TargetDefID != ast.InvalidNode {
		if luadoc := ctx.TargetDoc.GetLuaDoc(ctx.TargetDefID); luadoc != nil {
			if luadoc.Type != nil {
				raw = luadoc.Type.Type
			} else if luadoc.Alias != nil {
				raw = luadoc.Alias.Type
			}
		}
		if typ.Basics&TypeFunction != 0 && raw == "" {
			proxy := ctx.TargetDoc.makeCallableProxyType(ctx.TargetDefID)
			raw = hoverCallableSignature(proxy)
		}
	}
	typ = doc.ContextualType(ctx.IdentNodeID, offset, typ)
	return s.formatHoverType(doc, typ, raw, depth, 0, &hoverFormatState{active: make(map[hoverTableKey]bool)})
}

func (s *Server) formatHoverTable(doc *Document, typ TypeSet, depth, indent int, seen *hoverFormatState) (string, bool) {
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
	if typ.CustomName != "" {
		key.id = ast.InvalidNode
		key.name = luaDocGenericClassName(typ.CustomName)
	}
	if seen.active[key] {
		return "", false
	}
	seen.active[key] = true
	defer delete(seen.active, key)

	fields := make(map[string]TypeSet)
	rawFields := make(map[string]string)
	var sortedNames []string
	var flatClassKey hoverClassKey
	var flatClass bool
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
				if depth == 0 {
					return "", true
				}
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
				if depth == 0 {
					return "", true
				}
				fields[name] = owner.InferType(value)
			}
		}
	}
	if validTable && owner != doc {
		if _, receiver := s.fiveMTableGlobalOwner(typ); receiver != 0 {
			view := s.fiveMTableVisibilityDoc(owner, doc)
			keys := s.GlobalIndex.FieldKeysForReceivers(map[uint64]bool{receiver: true})
			for _, key := range keys {
				for _, symbol := range s.visibleGlobalSymbolsFromEntries(view, s.GlobalIndex.SymbolsByHash(key), 0) {
					fieldDoc := s.Documents[symbol.URI]
					name := string(fieldDoc.nodeSource(symbol.NodeID))
					if _, exists := fields[name]; exists || name == "" {
						continue
					}
					if value := fieldDoc.getAssignedValue(symbol.NodeID); value != ast.InvalidNode {
						if depth == 0 {
							return "", true
						}
						fields[name] = fieldDoc.InferType(value)
					}
				}
			}
		}
	}
	if typ.CustomName != "" && s.GlobalIndex != nil {
		lookupDoc := doc
		if owner != doc {
			lookupDoc = s.fiveMTableVisibilityDoc(owner, doc)
		}
		classKey := hoverClassKey{doc: lookupDoc, name: typ.CustomName}
		cached, exists := seen.classes[classKey]
		if !exists {
			cached = hoverClassFields{types: make(map[string]TypeSet), raw: make(map[string]string)}
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
					if depth == 0 {
						return "", true
					}
					fieldType := entry.LuaDoc.TypeString
					if candidate.ClassIndex < len(bindings) {
						fieldType = substituteLuaDocGenericType(fieldType, bindings[candidate.ClassIndex])
					}
					cached.types[name] = ParseTypeString(fieldType)
					cached.raw[name] = fieldType
				} else if entry.Type.Primitive != TypeUnknown {
					if depth == 0 {
						return "", true
					}
					cached.types[name] = TypeSet{Basics: entry.Type.Primitive}
				}
			}
			if seen.classes == nil {
				seen.classes = make(map[hoverClassKey]hoverClassFields)
			}
			cached.names = make([]string, 0, len(cached.types))
			cached.flat = len(cached.types) > 0
			for name, field := range cached.types {
				cached.names = append(cached.names, name)
				if field.CustomName != "" || field.DeclNode != ast.InvalidNode || field.CallSig != "" || len(field.Alternatives) != 0 || field.Basics == 0 || field.Basics&(field.Basics-1) != 0 || field.Basics&(TypeFunction|TypeTable) != 0 || strings.ContainsAny(cached.raw[name], "|?[]{}()<>") {
					cached.flat = false
				}
			}
			sort.Strings(cached.names)
			seen.classes[classKey] = cached
		}
		if len(fields) == 0 {
			fields, rawFields = cached.types, cached.raw
			sortedNames = cached.names
			if cached.flat {
				flatClass, flatClassKey = true, classKey
				if depth != 0 {
					if rendered, ok := cached.renders[indent]; ok {
						return rendered, false
					}
				}
			}
		} else {
			for name, fieldType := range cached.types {
				fields[name] = fieldType
				rawFields[name] = cached.raw[name]
			}
		}
	}
	if len(fields) == 0 {
		return "", false
	}
	if depth == 0 {
		return "", true
	}
	names := sortedNames
	if names == nil {
		names = make([]string, 0, len(fields))
		for name := range fields {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	var b strings.Builder
	b.Grow(2 + len(fields)*(indent*2+24))
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
		nested, nestedMore := s.formatHoverType(owner, field, rawFields[name], depth-1, indent+1, seen)
		if nested != "" {
			b.WriteString(nested)
		} else if rawFields[name] != "" {
			b.WriteString(rawFields[name])
		} else {
			b.WriteString(field.Format())
		}
		more = more || nestedMore
		b.WriteString(",\n")
	}
	b.WriteString(strings.Repeat("  ", indent))
	b.WriteString("}")
	result := b.String()
	if flatClass {
		cached := seen.classes[flatClassKey]
		if cached.renders == nil {
			cached.renders = make(map[int]string)
		}
		cached.renders[indent] = result
		seen.classes[flatClassKey] = cached
	}
	return result, more
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
