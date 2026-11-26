package lsp

import (
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// formatHoverType preserves composite LuaDoc syntax while expanding its members.
func (s *Server) formatHoverType(doc *Document, typ TypeSet, raw string, depth, indent int, seen *hoverFormatState) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" && len(typ.Alternatives) > 0 {
		if depth == 0 {
			return "", true
		}
		parts := make([]string, 0, len(typ.Alternatives)+1)
		var covered BasicType
		more := false
		for _, alternative := range typ.Alternatives {
			covered |= alternative.Basics
			expanded, next := s.formatHoverType(doc, alternative, "", depth-1, indent, seen)
			if expanded == "" {
				expanded = alternative.Format()
			}
			parts = append(parts, expanded)
			more = more || next
		}
		remaining := typ
		remaining.Alternatives = nil
		remaining.CustomName = ""
		remaining.Basics &^= covered
		if remaining.Basics != TypeUnknown {
			parts = append(parts, remaining.Format())
		}
		return strings.Join(parts, " | "), more
	}
	functionDoc := doc
	if typ.DeclURI != "" && typ.DeclURI != doc.URI {
		functionDoc = s.Documents[typ.DeclURI]
	}
	functionNode := typ.DeclNode
	isFunction := functionDoc != nil && functionDoc.Tree != nil && functionNode != ast.InvalidNode && int(functionNode) < len(functionDoc.Tree.Nodes) && functionDoc.Tree.Nodes[functionNode].Kind == ast.KindFunctionExpr
	if raw == "" {
		if typ.CallSig != "" {
			raw = hoverCallableSignature(typ)
		} else if isFunction {
			raw = hoverCallableSignature(functionDoc.makeCallableProxyType(functionNode))
		} else {
			raw = typ.CustomName
		}
	}
	child := func(text string) (string, bool) {
		expanded, more := s.formatHoverType(doc, ParseTypeString(text), text, depth-1, indent+1, seen)
		if expanded == "" {
			expanded = text
		}
		return expanded, more
	}

	if callable, ok := parseCallableType(raw); ok {
		if isFunction {
			key := hoverTableKey{uri: functionDoc.URI, id: functionNode, name: "function"}
			if seen.active[key] {
				return "", false
			}
			seen.active[key] = true
			defer delete(seen.active, key)
		}
		if depth == 0 {
			return "", true
		}
		params := make([]string, 0, len(callable.Params))
		more := false
		for _, param := range callable.Params {
			label := param.Name
			if param.Type != "" {
				t, next := child(param.Type)
				more = more || next
				if label != "" {
					label += ": "
				}
				label += t
			}
			params = append(params, label)
		}
		prefix := "fun"
		if strings.HasPrefix(raw, "funcref(") {
			prefix = "funcref"
		}
		result := prefix + "(" + strings.Join(params, ", ") + ")"
		if len(callable.Returns) > 0 {
			returns := splitTopLevelList(callable.Returns[0].Type, ',')
			for i, item := range returns {
				var next bool
				returns[i], next = child(item)
				more = more || next
			}
			result += ": " + strings.Join(returns, ", ")
		} else if isFunction {
			inferred := functionDoc.inferFunctionReturnType(functionNode)
			if inferred.Basics != TypeUnknown || inferred.CustomName != "" {
				expanded, next := s.formatHoverType(functionDoc, inferred, "", depth-1, indent+1, seen)
				if expanded == "" {
					expanded = inferred.Format()
				}
				result += ": " + expanded
				more = more || next
			}
		}
		return result, more
	}

	var alternatives []string
	var first string
	haveFirst := false
	for part := range splitUnionType(raw) {
		if !haveFirst {
			first, haveFirst = part, true
			continue
		}
		if alternatives == nil {
			alternatives = append(alternatives, first)
		}
		alternatives = append(alternatives, part)
	}
	if alternatives != nil {
		if depth == 0 {
			return "", true
		}
		more := false
		for i, part := range alternatives {
			var next bool
			alternatives[i], next = child(part)
			more = more || next
		}
		return strings.Join(alternatives, " | "), more
	}
	if strings.HasSuffix(raw, "?") {
		if depth == 0 {
			return "", true
		}
		expanded, more := child(strings.TrimSuffix(raw, "?"))
		return expanded + "?", more
	}
	if element, ok := splitLuaDocArrayType(raw); ok {
		if depth == 0 {
			return "", true
		}
		expanded, more := child(element)
		return expanded + "[]", more
	}
	if hoverWrappedType(raw) {
		items := splitTopLevelList(raw[1:len(raw)-1], ',')
		if depth == 0 {
			return "", true
		}
		more := false
		for i, item := range items {
			var next bool
			items[i], next = child(item)
			more = more || next
		}
		return "(" + strings.Join(items, ", ") + ")", more
	}
	if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
		items := splitTopLevelList(strings.TrimSpace(raw[1:len(raw)-1]), ',')
		if len(items) == 0 || len(items) == 1 && items[0] == "" {
			return "", false
		}
		if depth == 0 {
			return "", true
		}
		more := false
		for i, item := range items {
			colon := indexTopLevelByte(item, ':')
			if colon < 0 {
				continue
			}
			value, next := child(strings.TrimSpace(item[colon+1:]))
			items[i] = strings.TrimSpace(item[:colon]) + ": " + value
			more = more || next
		}
		return "{ " + strings.Join(items, ", ") + " }", more
	}
	if raw != "" && raw == typ.CustomName && !strings.ContainsAny(raw, "<>{}[]()") && s.GlobalIndex != nil {
		aliasKey := hoverClassKey{doc: doc, name: raw}
		alias, found := seen.aliases[aliasKey]
		if !found {
			var receiver uint64
			name := raw
			if dot := strings.LastIndexByte(raw, '.'); dot >= 0 {
				receiver = ast.HashBytes([]byte(raw[:dot]))
				name = raw[dot+1:]
			}
			symbols, _ := s.inferGlobalSymbols(doc, receiver, ast.HashBytes([]byte(name)))
			for _, entry := range symbols {
				owner := s.Documents[entry.URI]
				if owner == nil || owner.Tree == nil || !s.canSeeSymbol(doc, owner) {
					continue
				}
				luadoc := owner.GetLuaDoc(entry.NodeID)
				if luadoc != nil && luadoc.Alias != nil && luadoc.Alias.Name == raw && luadoc.Alias.Type != "" {
					alias = hoverAlias{doc: owner, typeName: luadoc.Alias.Type}
					break
				}
			}
			if depth != 0 || len(seen.active) > 0 {
				if seen.aliases == nil {
					seen.aliases = make(map[hoverClassKey]hoverAlias)
				}
				seen.aliases[aliasKey] = alias
			}
		}
		if alias.doc != nil {
			key := hoverTableKey{uri: alias.doc.URI, name: "alias:" + raw}
			if seen.active[key] {
				return "", false
			}
			if depth == 0 {
				return "", true
			}
			seen.active[key] = true
			expanded, more := s.formatHoverType(alias.doc, ParseTypeString(alias.typeName), alias.typeName, depth-1, indent, seen)
			delete(seen.active, key)
			if expanded == "" {
				expanded = alias.typeName
			}
			return raw + " = " + expanded, more
		}
	}

	if typ.DeclNode != 0 || typ.CustomName != "" {
		shape, more := s.formatHoverTable(doc, typ, depth, indent, seen)
		if shape != "" {
			if typ.CustomName != "" || typ.Basics != TypeTable {
				return typ.Format() + " " + shape, more
			}
			return shape, more
		}
		if more {
			return "", true
		}
	}
	if name, args, ok := splitLuaDocGenericType(raw); ok {
		if depth == 0 {
			return "", true
		}
		more := false
		for i, arg := range args {
			var next bool
			args[i], next = child(arg)
			more = more || next
		}
		return name + "<" + strings.Join(args, ", ") + ">", more
	}
	return "", false
}

func hoverCallableSignature(typ TypeSet) string {
	signature := typ.CallSig
	if len(typ.CallRet) > 1 {
		returns := make([]string, 0, len(typ.CallRet))
		for _, ret := range typ.CallRet {
			if ret.Type == "" {
				returns = append(returns, "any")
			} else {
				returns = append(returns, ret.Type)
			}
		}
		signature += ": " + strings.Join(returns, ", ")
	}
	return signature
}

func hoverWrappedType(raw string) bool {
	if len(raw) < 2 || raw[0] != '(' || raw[len(raw)-1] != ')' {
		return false
	}
	depth := 0
	for i, ch := range raw {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i != len(raw)-1 {
				return false
			}
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}
