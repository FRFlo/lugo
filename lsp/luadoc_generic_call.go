package lsp

import (
	"bytes"
	"sort"
	"strings"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/token"
)

// luaDocGenericCallContract groups the annotations used by generic inference.
type luaDocGenericCallContract struct {
	Generics []LuaDocGeneric
	Params   []LuaDocParam
	Returns  []LuaDocReturn
}

func (doc *Document) cacheEvictedGenericContract(function ast.NodeID) bool {
	var buffer [8]token.Token
	above := buffer[:0]
	hasGeneric := false
	for comment := range doc.IterateCommentsAbove(function) {
		raw := doc.sourceSlice(comment.Start, comment.End)
		if bytes.Contains(raw, tagGeneric) {
			hasGeneric = true
		}
		if bytes.Contains(raw, tagGeneric) || bytes.Contains(raw, tagParam) || bytes.Contains(raw, tagReturn) {
			above = append(above, comment)
		}
	}
	if !hasGeneric {
		return false
	}
	// Exclude prose before parsing and keep this scratch buffer local. A
	// large description must not become a retained server comment buffer.
	var comments []byte
	for i := len(above) - 1; i >= 0; i-- {
		for line := range bytes.SplitSeq(doc.sourceSlice(above[i].Start, above[i].End), []byte{'\n'}) {
			if bytes.Contains(line, tagGeneric) || bytes.Contains(line, tagParam) || bytes.Contains(line, tagReturn) {
				comments = cleanLuaCommentBytes(comments, line)
				comments = append(comments, '\n')
			}
		}
	}
	annotation := parseLuaDoc(comments, false)
	if len(annotation.Generics) == 0 || len(annotation.Returns) == 0 {
		return false
	}
	// Store a compact annotation spelling, rather than a LuaDoc or per-node
	// cache. Types are parsed on demand after eviction and the consumer's
	// TypeCache retains the inferred result. No prose or source slice survives.
	start := uint32(len(doc.evictedGenericContractData))
	data := doc.evictedGenericContractData
	for _, generic := range annotation.Generics {
		data = append(data, "@generic "...)
		data = append(data, generic.Name...)
		if generic.Parent != "" {
			data = append(data, ": "...)
			data = append(data, generic.Parent...)
		}
		data = append(data, '\n')
	}
	for _, param := range annotation.Params {
		data = append(data, "@param "...)
		data = append(data, param.Name...)
		data = append(data, ' ')
		data = append(data, param.Type...)
		data = append(data, '\n')
	}
	data = append(data, "@return "...)
	data = append(data, annotation.Returns[0].Type...)
	doc.evictedGenericContractData = data
	doc.evictedGenericContracts = append(doc.evictedGenericContracts, evictedNodeTextEntry{
		node: function, offset: start, length: uint32(len(data)) - start,
	})
	return true
}

func (doc *Document) evictedGenericCallAnnotations(function ast.NodeID) luaDocGenericCallContract {
	i := sort.Search(len(doc.evictedGenericContracts), func(i int) bool {
		return doc.evictedGenericContracts[i].node >= function
	})
	if i == len(doc.evictedGenericContracts) || doc.evictedGenericContracts[i].node != function {
		return luaDocGenericCallContract{}
	}
	entry := doc.evictedGenericContracts[i]
	annotation := parseLuaDoc(doc.evictedGenericContractData[entry.offset:entry.offset+entry.length], false)
	return luaDocGenericCallContract{Generics: annotation.Generics, Params: annotation.Params, Returns: annotation.Returns}
}

func (doc *Document) luaDocGenericCallAnnotations(definition ast.NodeID) luaDocGenericCallContract {
	if doc.Source() == nil {
		function := doc.getAssignedValue(definition)
		return doc.evictedGenericCallAnnotations(function)
	}
	if annotation := doc.GetLuaDoc(definition); annotation != nil {
		return luaDocGenericCallContract{Generics: annotation.Generics, Params: annotation.Params, Returns: annotation.Returns}
	}
	return luaDocGenericCallContract{}
}

// inferLuaDocGenericCall substitutes call argument types into generic LuaDoc
// parameters when a return annotation names one of those generic parameters.
func (doc *Document) inferLuaDocGenericCall(call ast.Node) (TypeSet, bool) {
	if doc == nil || doc.Tree == nil || (call.Kind != ast.KindCallExpr && call.Kind != ast.KindMethodCall) || call.Left == ast.InvalidNode {
		return TypeSet{}, false
	}
	calleeID := call.Left
	paramOffset := uint32(0)
	if call.Kind == ast.KindMethodCall {
		calleeID = call.Right
	} else if doc.Tree.Nodes[calleeID].Kind == ast.KindMemberExpr {
		calleeID = doc.Tree.Nodes[calleeID].Right
	}
	if calleeID == ast.InvalidNode {
		return TypeSet{}, false
	}
	calleeDoc, definition := doc, doc.referenceAt(calleeID)
	if definition == ast.InvalidNode && doc.Server != nil {
		if ctx := doc.Server.resolveSymbolNode(doc.URI, doc, calleeID); ctx != nil && ctx.TargetDoc != nil && ctx.TargetDefID != ast.InvalidNode {
			calleeDoc, definition = ctx.TargetDoc, ctx.TargetDefID
		}
	}
	// FiveM's `exports('name', identity)` bridge indexes the identifier used
	// as the second argument. Follow that intermediate reference to the
	// annotated declaration before reading its generic contract.
	type visitedDefinition struct {
		uri string
		id  ast.NodeID
	}
	var visited map[visitedDefinition]struct{}
	var dependencyDocs []string
	markVisited := func(key visitedDefinition) bool {
		if visited == nil {
			visited = make(map[visitedDefinition]struct{})
		}
		if _, exists := visited[key]; exists {
			return false
		}
		visited[key] = struct{}{}
		return true
	}
	for definition != ast.InvalidNode && calleeDoc.Tree != nil {
		if int(definition) >= len(calleeDoc.Tree.Nodes) || calleeDoc.Tree.Nodes[definition].Kind != ast.KindIdent {
			break
		}
		key := visitedDefinition{uri: calleeDoc.URI, id: definition}
		if target := calleeDoc.referenceAt(definition); target != ast.InvalidNode && target != definition {
			if !markVisited(key) {
				return TypeSet{}, false
			}
			definition = target
			continue
		}
		// Export bridges can point at a local alias declaration. Resolve the
		// alias's assigned expression before trying cross-document symbols.
		if assigned := calleeDoc.getAssignedValue(definition); assigned != ast.InvalidNode && assigned != definition {
			if calleeDoc.Tree.Nodes[assigned].Kind != ast.KindIdent {
				definition = assigned
				continue
			}
			if target := calleeDoc.referenceAt(assigned); target != ast.InvalidNode && target != definition {
				if !markVisited(key) {
					return TypeSet{}, false
				}
				definition = target
				continue
			}
			if calleeDoc.Server != nil {
				if ctx := calleeDoc.Server.resolveSymbolNode(calleeDoc.URI, calleeDoc, assigned); ctx != nil && ctx.TargetDoc != nil && ctx.TargetDefID != ast.InvalidNode && !(ctx.TargetDoc == calleeDoc && ctx.TargetDefID == definition) {
					if !markVisited(key) {
						return TypeSet{}, false
					}
					if ctx.TargetDoc != calleeDoc {
						dependencyDocs = append(dependencyDocs, calleeDoc.URI)
					}
					calleeDoc, definition = ctx.TargetDoc, ctx.TargetDefID
					continue
				}
			}
		}
		parent := calleeDoc.Tree.Nodes[definition].Parent
		if calleeDoc.Server == nil || parent == ast.InvalidNode || int(parent) >= len(calleeDoc.Tree.Nodes) || calleeDoc.Tree.Nodes[parent].Kind != ast.KindCallExpr {
			break
		}
		ctx := calleeDoc.Server.resolveSymbolNode(calleeDoc.URI, calleeDoc, definition)
		if ctx == nil || ctx.TargetDoc == nil || ctx.TargetDefID == ast.InvalidNode || ctx.TargetDoc == calleeDoc && ctx.TargetDefID == definition {
			break
		}
		if !markVisited(key) {
			return TypeSet{}, false
		}
		if ctx.TargetDoc != calleeDoc {
			dependencyDocs = append(dependencyDocs, calleeDoc.URI)
		}
		calleeDoc, definition = ctx.TargetDoc, ctx.TargetDefID
	}
	if definition == ast.InvalidNode || calleeDoc.Tree == nil || calleeDoc.Server == nil || int(definition) >= len(calleeDoc.Tree.Nodes) {
		return TypeSet{}, false
	}
	annotation := calleeDoc.luaDocGenericCallAnnotations(definition)
	if len(annotation.Generics) == 0 || len(annotation.Returns) == 0 {
		return TypeSet{}, false
	}
	for _, dependencyURI := range dependencyDocs {
		if dependencyURI != doc.URI {
			doc.Server.trackMetatableTypeDependency(dependencyURI, doc.URI)
		}
	}
	if calleeDoc != doc {
		doc.Server.trackMetatableTypeDependency(calleeDoc.URI, doc.URI)
	}
	if call.Kind == ast.KindMethodCall && len(annotation.Params) > 0 && annotation.Params[0].Name == "self" {
		paramOffset = 1
	}

	genericBounds := make(map[string]string, len(annotation.Generics))
	for _, generic := range annotation.Generics {
		genericBounds[generic.Name] = generic.Parent
	}
	bindings := make(map[string]TypeSet, len(annotation.Generics))
	methodHasExplicitSelf := call.Kind == ast.KindMethodCall && luaDocFunctionHasExplicitSelf(calleeDoc, definition)
	for i, param := range annotation.Params {
		if call.Kind == ast.KindMethodCall && param.Name == "self" {
			continue
		}
		var argIndex uint32
		if parameterIndex, ok := luaDocFunctionParameterIndex(calleeDoc, definition, param.Name); ok {
			argIndex = uint32(parameterIndex)
			if methodHasExplicitSelf {
				if argIndex == 0 {
					continue
				}
				argIndex--
			}
		} else {
			if uint32(i) < paramOffset {
				continue
			}
			argIndex = uint32(i) - paramOffset
		}
		if argIndex >= call.Count || call.Extra+argIndex >= uint32(len(doc.Tree.ExtraList)) {
			continue
		}
		argumentType := doc.InferType(doc.Tree.ExtraList[call.Extra+argIndex])
		if argumentType.Basics == TypeUnknown && argumentType.CustomName == "" {
			continue
		}
		inferGenericTypeBindings(param.Type, argumentType, genericBounds, bindings)
	}

	result, ok := substituteGenericType(annotation.Returns[0].Type, genericBounds, bindings)
	return result, ok
}

func luaDocFunctionParameterIndex(doc *Document, definition ast.NodeID, name string) (int, bool) {
	if doc == nil || doc.Tree == nil || definition == ast.InvalidNode || int(definition) >= len(doc.Tree.Nodes) || name == "" {
		return 0, false
	}
	functionID := doc.getAssignedValue(definition)
	if functionID == ast.InvalidNode && doc.Tree.Nodes[definition].Kind == ast.KindFunctionExpr {
		functionID = definition
	}
	if functionID == ast.InvalidNode || int(functionID) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[functionID].Kind != ast.KindFunctionExpr {
		return 0, false
	}
	function := doc.Tree.Nodes[functionID]
	for i := uint32(0); i < function.Count && function.Extra+i < uint32(len(doc.Tree.ExtraList)); i++ {
		paramID := doc.Tree.ExtraList[function.Extra+i]
		if string(doc.nodeSource(paramID)) == name {
			return int(i), true
		}
	}
	return 0, false
}

func luaDocFunctionHasExplicitSelf(doc *Document, definition ast.NodeID) bool {
	_, found := luaDocFunctionParameterIndex(doc, definition, "self")
	return found
}

func genericParameterName(typeName string, generics map[string]string) (string, bool) {
	typeName = strings.TrimSpace(typeName)
	optional := strings.HasSuffix(typeName, "?")
	if optional {
		typeName = strings.TrimSuffix(typeName, "?")
	}
	if _, ok := generics[typeName]; ok {
		return typeName, optional
	}
	return "", false
}

func genericParameterNames(typeName string, generics map[string]string) map[string]bool {
	result := make(map[string]bool)
	for part := range splitUnionType(typeName) {
		if generic, optional := genericParameterName(part, generics); generic != "" {
			result[generic] = result[generic] || optional
		}
	}
	return result
}

func inferGenericTypeBindings(typePattern string, actual TypeSet, generics map[string]string, bindings map[string]TypeSet) {
	for part := range splitUnionType(typePattern) {
		part = strings.TrimSpace(part)
		if generic, _ := genericParameterName(part, generics); generic != "" {
			inferred := actual
			binding := bindings[generic]
			mergeInferredType(&binding, inferred)
			bindings[generic] = binding
			continue
		}
		if patternElement, patternArray := splitLuaDocArrayType(part); patternArray {
			if actualElement, actualArray := splitLuaDocArrayType(actual.CustomName); actualArray {
				inferGenericTypeBindings(patternElement, ParseTypeString(actualElement), generics, bindings)
			}
			continue
		}

		patternName, patternArgs, isPatternGeneric := splitLuaDocGenericType(part)
		actualName, actualArgs, isActualGeneric := splitLuaDocGenericType(actual.CustomName)
		if !isPatternGeneric || !isActualGeneric || patternName != actualName || len(patternArgs) != len(actualArgs) {
			continue
		}
		for i, patternArg := range patternArgs {
			inferGenericTypeBindings(patternArg, ParseTypeString(actualArgs[i]), generics, bindings)
		}
	}
}

func splitLuaDocArrayType(typeName string) (string, bool) {
	typeName = strings.TrimSpace(typeName)
	if strings.HasSuffix(typeName, "[]") {
		return strings.TrimSpace(typeName[:len(typeName)-2]), true
	}
	return "", false
}

func substituteGenericType(typeName string, generics map[string]string, bindings map[string]TypeSet) (TypeSet, bool) {
	var result TypeSet
	found := false
	for part := range splitUnionType(typeName) {
		if generic, optional := genericParameterName(part, generics); generic != "" {
			if binding, ok := bindings[generic]; ok {
				if optional {
					binding.Basics |= TypeNil
				}
				mergeInferredType(&result, binding)
				found = true
				continue
			}
			if bound := generics[generic]; bound != "" {
				binding := ParseTypeString(bound)
				if optional {
					binding.Basics |= TypeNil
				}
				mergeInferredType(&result, binding)
				found = true
				continue
			}
			return TypeSet{}, false
		}
		stringBindings := make(map[string]string, len(bindings)+len(generics))
		for name, binding := range bindings {
			stringBindings[name] = luaDocTypeSetString(binding)
		}
		for name, bound := range generics {
			if _, exists := stringBindings[name]; !exists && bound != "" {
				stringBindings[name] = bound
			}
		}
		substituted := substituteGenericLuaDocType(part, stringBindings)
		value := ParseTypeString(substituted)
		mergeInferredType(&result, value)
		found = true
	}
	return result, found
}

func luaDocTypeSetString(typeSet TypeSet) string {
	if typeSet.CallSig != "" {
		return typeSet.CallSig
	}
	return typeSet.Format()
}

func substituteGenericLuaDocType(typeName string, bindings map[string]string) string {
	var result []string
	for part := range splitUnionType(typeName) {
		part = strings.TrimSpace(part)
		optional := strings.HasSuffix(part, "?")
		if optional {
			part = strings.TrimSpace(strings.TrimSuffix(part, "?"))
		}
		if replacement, ok := bindings[part]; ok {
			if optional && !strings.Contains(replacement, "nil") {
				replacement += " | nil"
			}
			result = append(result, replacement)
			continue
		}
		if element, ok := splitLuaDocArrayType(part); ok {
			part = substituteGenericLuaDocType(element, bindings) + "[]"
		} else if callable, ok := parseCallableType(part); ok {
			for i := range callable.Params {
				if callable.Params[i].Type != "" {
					callable.Params[i].Type = substituteGenericLuaDocType(callable.Params[i].Type, bindings)
				}
			}
			for i := range callable.Returns {
				callable.Returns[i].Type = substituteGenericLuaDocType(callable.Returns[i].Type, bindings)
			}
			part = buildCallableSignature(callable.Params, callable.Returns)
			if strings.HasPrefix(strings.TrimSpace(typeName), "funcref(") {
				part = "funcref" + strings.TrimPrefix(part, "fun")
			}
		} else if name, args, ok := splitLuaDocGenericType(part); ok {
			for i := range args {
				args[i] = substituteGenericLuaDocType(args[i], bindings)
			}
			part = name + "<" + strings.Join(args, ", ") + ">"
		}
		if optional {
			part += "?"
		}
		result = append(result, part)
	}
	return strings.Join(result, " | ")
}

func mergeInferredType(target *TypeSet, source TypeSet) {
	target.Basics |= source.Basics
	if target.CustomName == "" {
		target.CustomName = source.CustomName
	}
	if target.DeclNode == ast.InvalidNode {
		target.DeclNode, target.DeclURI = source.DeclNode, source.DeclURI
	}
	if target.MetaNode == ast.InvalidNode {
		target.MetaNode, target.MetaURI = source.MetaNode, source.MetaURI
	}
	if target.CallSig == "" && source.CallSig != "" {
		target.CallURI, target.CallNode = source.CallURI, source.CallNode
		target.CallSig = source.CallSig
		target.CallParams = source.CallParams
		target.CallRet = source.CallRet
	}
}
