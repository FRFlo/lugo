package lsp

import (
	"bytes"
	"iter"
	"strings"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/token"
)

type BasicType uint16

const (
	TypeUnknown  BasicType = 0
	TypeNil      BasicType = 1 << 0
	TypeBoolean  BasicType = 1 << 1
	TypeNumber   BasicType = 1 << 2
	TypeString   BasicType = 1 << 3
	TypeFunction BasicType = 1 << 4
	TypeTable    BasicType = 1 << 5
	TypeUserdata BasicType = 1 << 6
	TypeThread   BasicType = 1 << 7
	TypeAny      BasicType = 1 << 8
)

// TypeSet efficiently represents union types as bitmasks and custom names.
type TypeSet struct {
	CustomName string
	DeclURI    string
	DeclNode   ast.NodeID
	MetaURI    string
	MetaNode   ast.NodeID
	CallURI    string
	CallNode   ast.NodeID
	CallSig    string
	CallParams []LuaDocParam
	CallRet    []LuaDocReturn
	Basics     BasicType
}

func (s *Server) inferGlobalSymbols(srcDoc *Document, recHash, propHash uint64) ([]GlobalSymbol, bool) {
	if s == nil || s.GlobalIndex == nil {
		return nil, false
	}

	syms := s.visibleGlobalSymbolsFromEntries(srcDoc, s.GlobalIndex.SymbolsByHash(GlobalKey{ReceiverHash: recHash, PropHash: propHash}), 10)
	if len(syms) == 0 {
		return nil, false
	}

	return syms, true
}

func (s *Server) inferFirstGlobalSymbolEntry(srcDoc *Document, recHash, propHash uint64) *SymbolEntry {
	if s == nil || s.GlobalIndex == nil {
		return nil
	}

	syms := s.visibleGlobalSymbolsFromEntries(srcDoc, s.GlobalIndex.SymbolsByHash(GlobalKey{ReceiverHash: recHash, PropHash: propHash}), 1)
	if len(syms) == 0 {
		return nil
	}

	for _, entry := range s.GlobalIndex.SymbolsByHash(GlobalKey{ReceiverHash: recHash, PropHash: propHash}) {
		if entry != nil && entry.URI == syms[0].URI && entry.NodeID == syms[0].NodeID {
			return entry
		}
	}

	return nil
}

func splitUnionType(tStr string) iter.Seq[string] {
	return func(yield func(string) bool) {
		var (
			depth int
			start int
		)

		for i := 0; i < len(tStr); i++ {
			char := tStr[i]

			if char == '(' || char == '<' || char == '{' || char == '[' {
				depth++
			} else if char == ')' || char == '>' || char == '}' || char == ']' {
				depth--
			} else if char == '|' && depth <= 0 {
				if !yield(strings.TrimSpace(tStr[start:i])) {
					return
				}

				start = i + 1
			}
		}

		if start < len(tStr) {
			yield(strings.TrimSpace(tStr[start:]))
		}
	}
}

func ParseTypeString(tStr string) TypeSet {
	var typeSet TypeSet

	for part := range splitUnionType(tStr) {
		if strings.HasSuffix(part, "?") {
			part = part[:len(part)-1]

			typeSet.Basics |= TypeNil
		}

		switch part {
		case "number", "integer", "float":
			typeSet.Basics |= TypeNumber
		case "string":
			typeSet.Basics |= TypeString
		case "boolean", "bool":
			typeSet.Basics |= TypeBoolean
		case "table":
			typeSet.Basics |= TypeTable
		case "function", "fun":
			typeSet.Basics |= TypeFunction
		case "nil":
			typeSet.Basics |= TypeNil
		case "any":
			typeSet.Basics |= TypeAny
		case "userdata":
			typeSet.Basics |= TypeUserdata
		case "thread":
			typeSet.Basics |= TypeThread
		default:
			if strings.HasPrefix(part, "fun(") {
				typeSet.Basics |= TypeFunction

				if typeSet.CallSig == "" {
					if callable, ok := parseCallableType(part); ok {
						typeSet.CallSig = callable.Signature
						typeSet.CallParams = callable.Params
						typeSet.CallRet = callable.Returns
					}
				}
			} else if strings.HasPrefix(part, "funcref(") {
				typeSet.Basics |= TypeTable

				if typeSet.CallSig == "" {
					if callable, ok := parseCallableType(part); ok {
						typeSet.CallSig = callable.Signature
						typeSet.CallParams = callable.Params
						typeSet.CallRet = callable.Returns
					}
				}
			} else if strings.HasPrefix(part, "{") {
				typeSet.Basics |= TypeTable
			} else if part != "" {
				typeSet.CustomName = part
			}
		}
	}

	return typeSet
}

func (typeSet TypeSet) Format() string {
	var parts []string

	if typeSet.Basics&TypeNumber != 0 {
		parts = append(parts, "number")
	}

	if typeSet.Basics&TypeString != 0 {
		parts = append(parts, "string")
	}

	if typeSet.Basics&TypeBoolean != 0 {
		parts = append(parts, "boolean")
	}

	if typeSet.Basics&TypeTable != 0 {
		parts = append(parts, "table")
	}

	if typeSet.Basics&TypeFunction != 0 {
		parts = append(parts, "function")
	}

	if typeSet.Basics&TypeUserdata != 0 {
		parts = append(parts, "userdata")
	}

	if typeSet.Basics&TypeThread != 0 {
		parts = append(parts, "thread")
	}

	if typeSet.CustomName != "" {
		parts = append(parts, typeSet.CustomName)
	}

	if typeSet.Basics&TypeNil != 0 {
		parts = append(parts, "nil")
	}

	if typeSet.Basics&TypeAny != 0 {
		if len(parts) == 0 {
			return "any"
		}

		parts = append(parts, "any")
	}

	if len(parts) == 0 {
		return "any"
	}

	return strings.Join(parts, " | ")
}

// InferType infers the type of a given AST node lazily and caches it.
func (doc *Document) InferType(id ast.NodeID) TypeSet {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return TypeSet{}
	}

	if doc.TypeCache.nodes != len(doc.Tree.Nodes) {
		doc.TypeCache.Reset(len(doc.Tree.Nodes))
	}
	t := doc.TypeCache.Get(id)
	if reusableType(t) {
		return t
	}

	if len(doc.Inferring) != len(doc.Tree.Nodes) {
		if cap(doc.Inferring) >= len(doc.Tree.Nodes) {
			doc.Inferring = doc.Inferring[:len(doc.Tree.Nodes)]
			clear(doc.Inferring)
		} else {
			doc.Inferring = make([]bool, len(doc.Tree.Nodes))
		}
	}

	if doc.Inferring[id] {
		return TypeSet{} // Cycle detected
	}

	doc.Inferring[id] = true

	defer func() {
		doc.Inferring[id] = false
	}()

	var typeSet TypeSet

	node := doc.Tree.Nodes[id]

	switch node.Kind {
	case ast.KindNumber:
		typeSet.Basics = TypeNumber
	case ast.KindString:
		typeSet.Basics = TypeString
	case ast.KindHashedString:
		typeSet.Basics = TypeNumber
	case ast.KindTrue, ast.KindFalse:
		typeSet.Basics = TypeBoolean
	case ast.KindNil:
		typeSet.Basics = TypeNil
	case ast.KindFunctionExpr, ast.KindLocalFunction, ast.KindFunctionStmt:
		typeSet.Basics = TypeFunction
		typeSet.DeclNode = id
		typeSet.DeclURI = doc.URI
	case ast.KindTableExpr:
		typeSet.Basics = TypeTable
		typeSet.DeclNode = id
		typeSet.DeclURI = doc.URI
	case ast.KindBinaryExpr:
		op := node.Extra
		resolveOperator := func(receiver TypeSet, name string) (TypeSet, bool) {
			return doc.inferOperatorMetamethodAt(receiver, name, node.Start)
		}

		switch token.Kind(op) {
		case token.Plus, token.Minus, token.Asterisk, token.Slash, token.FloorSlash, token.Modulo, token.Caret, token.BitAnd, token.BitOr, token.BitXor, token.ShiftLeft, token.ShiftRight:
			leftType := doc.InferType(node.Left)
			rightType := doc.InferType(node.Right)

			if result, ok := InferMetatableBinaryOperator(token.Kind(op), leftType, rightType, resolveOperator); ok {
				typeSet = result
				break
			}
			if name, ok := LuaMetatableOperatorName(token.Kind(op)); ok {
				if result, found := doc.inferLuaDocOperator(leftType, rightType, name); found {
					typeSet = result
					break
				}
				if result, found := doc.inferLuaDocOperator(rightType, leftType, name); found {
					typeSet = result
					break
				}
			}
			if leftType.CustomName != "" {
				typeSet.CustomName = leftType.CustomName
				typeSet.Basics = leftType.Basics
			} else if rightType.CustomName != "" {
				typeSet.CustomName = rightType.CustomName
				typeSet.Basics = rightType.Basics
			} else {
				typeSet.Basics = TypeNumber
			}
		case token.Concat:
			left, right := doc.InferType(node.Left), doc.InferType(node.Right)
			if result, ok := InferMetatableBinaryOperator(token.Concat, left, right, resolveOperator); ok {
				typeSet = result
			} else if result, ok := doc.inferLuaDocOperator(left, right, "__concat"); ok {
				typeSet = result
			} else if result, ok := doc.inferLuaDocOperator(right, left, "__concat"); ok {
				typeSet = result
			} else {
				typeSet.Basics = TypeString
			}
		case token.Eq, token.NotEq, token.Less, token.LessEq, token.Greater, token.GreaterEq:
			typeSet.Basics = TypeBoolean
		case token.And, token.Or:
			leftType := doc.InferType(node.Left)
			rightType := doc.InferType(node.Right)

			typeSet.Basics = leftType.Basics | rightType.Basics

			if leftType.Basics == TypeUnknown && leftType.CustomName == "" {
				typeSet.Basics |= TypeAny
			}

			if rightType.Basics == TypeUnknown && rightType.CustomName == "" {
				typeSet.Basics |= TypeAny
			}

			if typeSet.CustomName == "" {
				typeSet.CustomName = leftType.CustomName
			}

			if typeSet.CustomName == "" {
				typeSet.CustomName = rightType.CustomName
			}

			if rightType.DeclNode != ast.InvalidNode {
				typeSet.DeclNode = rightType.DeclNode
				typeSet.DeclURI = rightType.DeclURI
			} else if leftType.DeclNode != ast.InvalidNode {
				typeSet.DeclNode = leftType.DeclNode
				typeSet.DeclURI = leftType.DeclURI
			}

			if rightType.MetaNode != ast.InvalidNode {
				typeSet.MetaNode = rightType.MetaNode
				typeSet.MetaURI = rightType.MetaURI
			} else if leftType.MetaNode != ast.InvalidNode {
				typeSet.MetaNode = leftType.MetaNode
				typeSet.MetaURI = leftType.MetaURI
			}
		}
	case ast.KindUnaryExpr:
		src := doc.sourceSlice(node.Start, node.End)

		if bytes.HasPrefix(src, []byte("not")) {
			typeSet.Basics = TypeBoolean
		} else {
			op := token.Minus
			if len(src) > 0 && src[0] == '#' {
				op = token.Hash
			} else if len(src) > 0 && src[0] == '~' {
				op = token.BitXor
			}
			resolveOperator := func(receiver TypeSet, name string) (TypeSet, bool) {
				return doc.inferOperatorMetamethodAt(receiver, name, node.Start)
			}
			operand := doc.InferType(node.Right)
			if result, ok := InferMetatableUnaryOperator(op, operand, resolveOperator); ok {
				typeSet = result
			} else if name, valid := LuaMetatableUnaryOperatorName(op); valid {
				if result, found := doc.inferLuaDocOperator(operand, TypeSet{}, name); found {
					typeSet = result
				} else {
					typeSet.Basics = TypeNumber
				}
			} else {
				typeSet.Basics = TypeNumber
			}
		}
	case ast.KindParenExpr:
		typeSet = doc.InferType(node.Left)
	case ast.KindIdent:
		typeSet = doc.inferIdent(id)
	case ast.KindMemberExpr:
		typeSet = doc.inferMemberExpr(node)
	case ast.KindIndexExpr:
		typeSet = doc.inferMemberExpr(node)
		if typeSet.Basics == TypeUnknown && typeSet.CustomName == "" {
			receiver := doc.fiveMGLMExprType(node.Left, 0)
			if doc.Server != nil && doc.Server.getDocumentFiveMProfile(doc).IsResourceProfile() {
				index := 0.0
				known := false
				if value, evaluated := doc.evalNode(node.Right, 0); evaluated && value.kind == ast.KindNumber {
					index, known = value.num, true
				} else if doc.InferType(node.Right).Basics&TypeNumber == 0 {
					known = true // Non-numeric keys do not use the GLM numeric accessor.
				}
				if !known || index != 0 {
					if result, ok := fiveMGLMIndexType(receiver, index); ok {
						typeSet = result
					}
				}
			}
			if typeSet.Basics == TypeUnknown {
				if result, ok := doc.inferLuaDocOperator(receiver, doc.InferType(node.Right), "__index"); ok {
					typeSet = result
				}
			}
		}
	case ast.KindMethodName:
		typeSet = doc.inferMethodSelfType(id)
	case ast.KindCallExpr, ast.KindMethodCall:
		typeSet = doc.inferCallExpr(node)
	}

	doc.TypeCache.Put(id, typeSet)

	return typeSet
}

func (doc *Document) inferIdent(id ast.NodeID) TypeSet {
	var (
		targetDoc = doc
		targetDef = doc.referenceAt(id)
	)

	localDefID := targetDef
	identName := doc.nodeSource(id)
	identHash := ast.HashBytes(identName)

	if bytes.Equal(identName, []byte("self")) && targetDef != ast.InvalidNode && int(targetDef) < len(doc.Tree.Nodes) && doc.Tree.Nodes[targetDef].Kind == ast.KindMethodName {
		return doc.inferMethodSelfType(targetDef)
	}

	if doc.Server != nil {
		ctx := doc.Server.resolveSymbolNode(doc.URI, doc, id)
		if ctx != nil && ctx.TargetDoc != nil && ctx.TargetDefID != ast.InvalidNode {
			targetDoc = ctx.TargetDoc
			targetDef = ctx.TargetDefID
		}
	}

	if bytes.Equal(identName, []byte("self")) && targetDef != ast.InvalidNode && int(targetDef) < len(targetDoc.Tree.Nodes) && targetDoc.Tree.Nodes[targetDef].Kind == ast.KindMethodName {
		return targetDoc.inferMethodSelfType(targetDef)
	}

	if targetDef == ast.InvalidNode {
		if doc.Server != nil {
			switch ast.String(identName) {
			case "source":
				if doc.Server.isFiveMServerEventSourceReference(doc, id) {
					return TypeSet{Basics: TypeNumber}
				}
			case "exports":
				if doc.Server.isFiveMGlobalAvailable(doc, "exports") {
					return TypeSet{Basics: TypeTable}
				}
			}
		}

		return doc.inferMetatableAttachment(id, ast.InvalidNode, TypeSet{})
	}

	luadoc := targetDoc.GetLuaDoc(targetDef)

	var t TypeSet

	if luadoc != nil && luadoc.Type != nil {
		t = ParseTypeString(luadoc.Type.Type)
	} else if luadoc != nil && luadoc.Class != nil {
		t = TypeSet{CustomName: luadoc.Class.Name}
	} else {
		valID := targetDoc.getAssignedValue(targetDef)
		if valID != ast.InvalidNode {
			t = targetDoc.InferType(valID)
		} else if targetDoc.Tree.Nodes[targetDef].Kind == ast.KindIdent {
			parentID := targetDoc.Tree.Nodes[targetDef].Parent
			if parentID != ast.InvalidNode {
				parentNode := targetDoc.Tree.Nodes[parentID]

				switch parentNode.Kind {
				case ast.KindFunctionExpr:
					t = targetDoc.inferFunctionParameter(targetDef, parentID)
				case ast.KindNameList:
					t = targetDoc.inferLoopVariable(targetDef, parentID)
				}
			}
		}
	}

	checkReassignments := func(d *Document) {
		for _, reassignment := range d.Resolver.Reassignments {
			var match bool

			if reassignment.DefID != ast.InvalidNode {
				if d == targetDoc && reassignment.DefID == targetDef {
					match = true
				} else if d == doc && reassignment.DefID == localDefID {
					match = true
				}
			} else {
				if reassignment.NameHash == identHash {
					match = true
				}
			}

			if match {
				rt := d.InferType(reassignment.ValID)

				if rt.Basics == TypeUnknown && rt.CustomName == "" {
					t.Basics |= TypeAny
				} else {
					t.Basics |= rt.Basics

					if t.CustomName == "" {
						t.CustomName = rt.CustomName
					}

					if t.DeclNode == ast.InvalidNode && rt.DeclNode != ast.InvalidNode {
						t.DeclNode = rt.DeclNode
						t.DeclURI = rt.DeclURI
					}

					if t.MetaNode == ast.InvalidNode && rt.MetaNode != ast.InvalidNode {
						t.MetaNode = rt.MetaNode
						t.MetaURI = rt.MetaURI
					}
				}
			}
		}
	}

	checkReassignments(doc)

	if targetDoc != doc {
		checkReassignments(targetDoc)
	}

	if targetDoc != doc && targetDef != ast.InvalidNode {
		doc.Server.trackMetatableTypeDependency(targetDoc.URI, doc.URI)
		result := targetDoc.inferMetatableAttachmentAt(targetDef, t, ^uint32(0))
		if localDefID == ast.InvalidNode {
			return doc.inferMetatableAttachment(id, ast.InvalidNode, result)
		}
		return result
	}
	return doc.inferMetatableAttachment(id, targetDef, t)
}

func (doc *Document) inferMethodSelfType(methodNameID ast.NodeID) TypeSet {
	if doc == nil || doc.Tree == nil || methodNameID == ast.InvalidNode || int(methodNameID) >= len(doc.Tree.Nodes) {
		return TypeSet{}
	}

	methodName := doc.Tree.Nodes[methodNameID]
	if methodName.Kind != ast.KindMethodName {
		return TypeSet{}
	}

	if doc.Server != nil {
		if luadoc := doc.GetLuaDoc(methodNameID); luadoc != nil {
			for _, param := range luadoc.Params {
				if param.Name == "self" && param.Type != "" {
					return ParseTypeString(param.Type)
				}
			}
		}
	}

	if methodName.Left == ast.InvalidNode {
		return TypeSet{}
	}

	return doc.InferType(methodName.Left)
}

func (doc *Document) inferFunctionParameter(defID, funcExprID ast.NodeID) TypeSet {
	grandParentID := doc.Tree.Nodes[funcExprID].Parent
	if grandParentID == ast.InvalidNode {
		return doc.inferBridgeCallbackParameter(defID, funcExprID)
	}

	grandParentNode := doc.Tree.Nodes[grandParentID]

	var funcDefID = ast.InvalidNode

	switch grandParentNode.Kind {
	case ast.KindLocalFunction, ast.KindFunctionStmt:
		funcDefID = grandParentNode.Left
	case ast.KindAssign, ast.KindLocalAssign, ast.KindRecordField:
		funcDefID = grandParentID
	}

	if funcDefID == ast.InvalidNode {
		return doc.inferBridgeCallbackParameter(defID, funcExprID)
	}

	funcDoc := doc.GetLuaDoc(funcDefID)
	if funcDoc == nil {
		return doc.inferBridgeCallbackParameter(defID, funcExprID)
	}

	paramName := ast.String(doc.nodeSource(defID))

	for _, p := range funcDoc.Params {
		if p.Name == paramName {
			return ParseTypeString(p.Type)
		}
	}

	return doc.inferBridgeCallbackParameter(defID, funcExprID)
}

func (doc *Document) inferBridgeCallbackParameter(defID, funcExprID ast.NodeID) TypeSet {
	if doc.Server == nil || funcExprID == ast.InvalidNode || defID == ast.InvalidNode {
		return TypeSet{}
	}

	paramIndex := doc.Tree.IndexOfExtra(funcExprID, defID)
	if paramIndex == -1 {
		return TypeSet{}
	}

	parentID := doc.Tree.Nodes[funcExprID].Parent
	if parentID == ast.InvalidNode || int(parentID) >= len(doc.Tree.Nodes) {
		return TypeSet{}
	}

	callID := parentID
	callNode := doc.Tree.Nodes[callID]
	argIndex := -1

	if callNode.Kind == ast.KindExprList {
		callID = callNode.Parent
		if callID == ast.InvalidNode || int(callID) >= len(doc.Tree.Nodes) {
			return TypeSet{}
		}

		callNode = doc.Tree.Nodes[callID]
		argIndex = doc.Tree.IndexOfExtra(parentID, funcExprID)
	} else {
		argIndex = doc.Tree.IndexOfExtra(callID, funcExprID)
	}

	if callNode.Kind != ast.KindCallExpr && callNode.Kind != ast.KindMethodCall {
		return TypeSet{}
	}

	if argIndex == -1 {
		return TypeSet{}
	}

	funcIdentID := callNode.Left
	if callNode.Kind == ast.KindMethodCall {
		funcIdentID = callNode.Right
	} else if funcIdentID != ast.InvalidNode && int(funcIdentID) < len(doc.Tree.Nodes) && doc.Tree.Nodes[funcIdentID].Kind == ast.KindMemberExpr {
		funcIdentID = doc.Tree.Nodes[funcIdentID].Right
	}

	if funcIdentID == ast.InvalidNode || int(funcIdentID) >= len(doc.Tree.Nodes) {
		return TypeSet{}
	}

	ctx := doc.Server.resolveSymbolNode(doc.URI, doc, funcIdentID)
	if ctx == nil || ctx.TargetDoc == nil || ctx.TargetDefID == ast.InvalidNode {
		return TypeSet{}
	}

	funcDoc := ctx.TargetDoc.GetLuaDoc(ctx.TargetDefID)
	if funcDoc == nil {
		return TypeSet{}
	}

	callbackParamIndex := argIndex + getImplicitSelfOffset(ctx, callNode, ctx.TargetDoc, ctx.TargetDefID)
	if callbackParamIndex < 0 {
		return TypeSet{}
	}

	callbackParamType := ctx.TargetDoc.getLuaDocParamType(ctx.TargetDefID, callbackParamIndex, funcDoc)
	if callbackParamType == "" {
		return TypeSet{}
	}

	callable, ok := parseCallableType(callbackParamType)
	if !ok || paramIndex >= len(callable.Params) {
		return TypeSet{}
	}

	return ParseTypeString(callable.Params[paramIndex].Type)
}

func (doc *Document) inferLoopVariable(defID, nameListID ast.NodeID) TypeSet {
	grandParentID := doc.Tree.Nodes[nameListID].Parent
	if grandParentID == ast.InvalidNode {
		return TypeSet{}
	}

	grandParentNode := doc.Tree.Nodes[grandParentID]
	if grandParentNode.Kind != ast.KindForIn || grandParentNode.Extra == 0 {
		return TypeSet{}
	}

	idx := doc.Tree.IndexOfExtra(nameListID, defID)

	if idx == -1 {
		return TypeSet{}
	}

	exprList := doc.Tree.Nodes[grandParentNode.Extra]
	if exprList.Count == 0 {
		return TypeSet{}
	}

	firstExprID := doc.Tree.ExtraList[exprList.Extra]
	firstExpr := doc.Tree.Nodes[firstExprID]

	if firstExpr.Kind != ast.KindCallExpr || firstExpr.Count == 0 {
		return TypeSet{}
	}

	funcID := firstExpr.Left
	if doc.Tree.Nodes[funcID].Kind != ast.KindIdent {
		return TypeSet{}
	}

	if doc.referenceAt(funcID) != ast.InvalidNode {
		return TypeSet{}
	}

	funcName := doc.nodeSource(funcID)

	if bytes.Equal(funcName, []byte("ipairs")) {
		switch idx {
		case 0:
			return TypeSet{Basics: TypeNumber}
		case 1:
			if firstExpr.Count > 0 {
				argID := doc.Tree.ExtraList[firstExpr.Extra]

				return doc.extractArrayElementType(doc.InferType(argID))
			}
		}
	} else if bytes.Equal(funcName, []byte("pairs")) {
		if inferred, ok := doc.inferPairsMetamethodLoopVariable(firstExpr, idx); ok {
			return inferred
		}
		switch idx {
		case 0:
			return TypeSet{Basics: TypeAny}
		case 1:
			if firstExpr.Count > 0 {
				argID := doc.Tree.ExtraList[firstExpr.Extra]

				return doc.extractArrayElementType(doc.InferType(argID))
			}
		}
	}

	return TypeSet{}
}

// inferPairsMetamethodLoopVariable follows __pairs to its iterator and uses
// the iterator's return tuple for the generic-for variables.
func (doc *Document) inferPairsMetamethodLoopVariable(call ast.Node, idx int) (TypeSet, bool) {
	if call.Count == 0 || call.Extra >= uint32(len(doc.Tree.ExtraList)) {
		return TypeSet{}, false
	}
	sourceID := doc.Tree.ExtraList[call.Extra]
	sourceType := doc.InferType(sourceID)
	metaDoc := doc
	if sourceType.MetaURI != "" && sourceType.MetaURI != doc.URI {
		if doc.Server == nil {
			return TypeSet{}, false
		}
		metaDoc = doc.Server.Documents[sourceType.MetaURI]
	}
	if metaDoc == nil || sourceType.MetaNode == ast.InvalidNode {
		return TypeSet{}, false
	}
	metaValues := metaDoc.metatableFieldAlternativesAt(sourceType.MetaNode, "__pairs", metatableOffsetForDocument(doc, metaDoc, call.Start))
	var result TypeSet
	found := false
	for _, metaValue := range metaValues {
		fnID := metaDoc.resolvePairsFunction(metaValue)
		if fnID == ast.InvalidNode {
			continue
		}
		var iterators functionReturnValueBuffer
		var visited [32]functionReturnVisit
		metaDoc.collectFunctionReturnValues(fnID, 0, &iterators, &visited, 0)
		if iterators.incomplete {
			found = true
			continue
		}
		for iteratorIndex := 0; iteratorIndex < iterators.len(); iteratorIndex++ {
			iteratorExprID := iterators.at(iteratorIndex)
			iteratorID := metaDoc.resolvePairsFunction(iteratorExprID)
			if iteratorID == ast.InvalidNode {
				continue
			}
			var values functionReturnValueBuffer
			visited = [32]functionReturnVisit{}
			metaDoc.collectFunctionReturnValues(iteratorID, idx, &values, &visited, 0)
			if values.incomplete {
				found = true
				continue
			}
			if values.len() == 0 {
				result.Basics |= TypeNil
				found = true
			}
			for valueIndex := 0; valueIndex < values.len(); valueIndex++ {
				valueID := values.at(valueIndex)
				part := TypeSet{Basics: TypeNil}
				if valueID != ast.InvalidNode {
					part = metaDoc.InferType(valueID)
				}
				result.Basics |= part.Basics
				if result.CustomName == "" {
					result.CustomName = part.CustomName
				}
				found = true
			}
		}
	}
	return result, found
}

func (doc *Document) resolvePairsFunction(id ast.NodeID) ast.NodeID {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return ast.InvalidNode
	}
	if doc.Tree.Nodes[id].Kind == ast.KindParenExpr {
		return doc.resolvePairsFunction(doc.Tree.Nodes[id].Left)
	}
	if doc.Tree.Nodes[id].Kind == ast.KindIdent {
		if def := doc.referenceAt(id); def != ast.InvalidNode {
			return doc.getAssignedValue(def)
		}
	}
	if doc.Tree.Nodes[id].Kind == ast.KindFunctionExpr {
		return id
	}
	return ast.InvalidNode
}

func (doc *Document) resolvePairsCallFunction(call ast.Node) ast.NodeID {
	if call.Kind == ast.KindCallExpr {
		return doc.resolvePairsFunction(call.Left)
	}
	if call.Kind != ast.KindMethodCall || doc.Server == nil || call.Right == ast.InvalidNode {
		return ast.InvalidNode
	}
	ctx := doc.Server.resolveSymbolNode(doc.URI, doc, call.Right)
	if ctx == nil || ctx.TargetDoc == nil || ctx.TargetDefID == ast.InvalidNode {
		return ast.InvalidNode
	}
	fnID := ctx.TargetDoc.getAssignedValue(ctx.TargetDefID)
	if fnID != ast.InvalidNode && int(fnID) < len(ctx.TargetDoc.Tree.Nodes) && ctx.TargetDoc.Tree.Nodes[fnID].Kind == ast.KindFunctionExpr {
		return fnID
	}
	if int(ctx.TargetDefID) < len(ctx.TargetDoc.Tree.Nodes) && ctx.TargetDoc.Tree.Nodes[ctx.TargetDefID].Kind == ast.KindFunctionExpr {
		return ctx.TargetDefID
	}
	return ast.InvalidNode
}

type functionReturnVisit struct {
	function ast.NodeID
	index    int
}

type functionReturnValueBuffer struct {
	values     [64]ast.NodeID
	count      int
	spill      []ast.NodeID
	incomplete bool
}

func (buffer *functionReturnValueBuffer) add(id ast.NodeID) {
	if buffer.count < len(buffer.values) {
		buffer.values[buffer.count] = id
		buffer.count++
		return
	}
	buffer.spill = append(buffer.spill, id)
}

func (buffer *functionReturnValueBuffer) len() int {
	return buffer.count + len(buffer.spill)
}

func (buffer *functionReturnValueBuffer) at(index int) ast.NodeID {
	if index < buffer.count {
		return buffer.values[index]
	}
	return buffer.spill[index-buffer.count]
}

func (doc *Document) collectFunctionReturnValues(fnID ast.NodeID, returnIndex int, found *functionReturnValueBuffer, visited *[32]functionReturnVisit, depth int) {
	if fnID == ast.InvalidNode || int(fnID) >= len(doc.Tree.Nodes) {
		return
	}
	key := functionReturnVisit{function: fnID, index: returnIndex}
	if depth >= 32 {
		found.incomplete = true
		return
	}
	for i := 0; i < depth; i++ {
		if visited[i] == key {
			found.incomplete = true
			return
		}
	}
	visited[depth] = key
	var walk func(ast.NodeID)
	walk = func(id ast.NodeID) {
		if id == ast.InvalidNode {
			return
		}
		n := doc.Tree.Nodes[id]
		if n.Kind == ast.KindFunctionExpr && id != fnID {
			return
		}
		if n.Kind == ast.KindReturn {
			if n.Left == ast.InvalidNode {
				found.add(ast.InvalidNode)
				return
			}
			list := doc.Tree.Nodes[n.Left]
			if returnIndex < 0 {
				return
			}
			if list.Count > 0 {
				lastIndex := int(list.Count) - 1
				lastExprID := doc.Tree.ExtraList[list.Extra+uint32(lastIndex)]
				finalExprID := lastExprID
				for finalExprID != ast.InvalidNode && doc.Tree.Nodes[finalExprID].Kind == ast.KindParenExpr {
					finalExprID = doc.Tree.Nodes[finalExprID].Left
				}
				if finalExprID != ast.InvalidNode && (doc.Tree.Nodes[finalExprID].Kind == ast.KindCallExpr || doc.Tree.Nodes[finalExprID].Kind == ast.KindMethodCall) && returnIndex >= lastIndex {
					call := doc.Tree.Nodes[finalExprID]
					calleeID := doc.resolvePairsCallFunction(call)
					if calleeID != ast.InvalidNode {
						doc.collectFunctionReturnValues(calleeID, returnIndex-lastIndex, found, visited, depth+1)
					} else {
						found.incomplete = true
					}
					return
				}
			}
			if returnIndex < int(list.Count) {
				found.add(doc.Tree.ExtraList[list.Extra+uint32(returnIndex)])
			} else {
				found.add(ast.InvalidNode)
			}
			return
		}
		walk(n.Left)
		walk(n.Right)
		for i := uint32(0); i < n.Count; i++ {
			walk(doc.Tree.ExtraList[n.Extra+i])
		}
	}
	walk(fnID)
}

func (doc *Document) getLuaDocParamType(defID ast.NodeID, paramIndex int, funcDoc *LuaDoc) string {
	if funcDoc == nil || paramIndex < 0 {
		return ""
	}

	valID := doc.getAssignedValue(defID)
	if valID != ast.InvalidNode && int(valID) < len(doc.Tree.Nodes) && doc.Tree.Nodes[valID].Kind == ast.KindFunctionExpr {
		funcNode := doc.Tree.Nodes[valID]
		if paramIndex < int(funcNode.Count) {
			paramID := doc.Tree.ExtraList[funcNode.Extra+uint32(paramIndex)]
			if paramID != ast.InvalidNode && int(paramID) < len(doc.Tree.Nodes) {
				if paramName := ast.String(doc.nodeSource(paramID)); paramName != "" {
					for _, param := range funcDoc.Params {
						if param.Name == paramName {
							return param.Type
						}
					}
				}
			}
		}
	}

	if paramIndex < len(funcDoc.Params) {
		return funcDoc.Params[paramIndex].Type
	}

	return ""
}

func (doc *Document) inferMemberExpr(node ast.Node) TypeSet {
	return doc.inferMemberExprWithContext(node, nil, nil, false)
}

func (doc *Document) inferMemberExprWithContext(node ast.Node, symbolCtx *SymbolContext, receiver *symbolReceiverInfo, symbolContextResolved bool) TypeSet {
	var leftType TypeSet
	if receiver != nil && receiver.Known {
		leftType = receiver.Type
	} else {
		leftType = doc.InferType(node.Left)
	}

	var t TypeSet

	if node.Right == ast.InvalidNode || int(node.Right) >= len(doc.Tree.Nodes) {
		return TypeSet{}
	}
	rightNode := doc.Tree.Nodes[node.Right]
	var fieldName []byte
	switch rightNode.Kind {
	case ast.KindIdent:
		if node.Kind == ast.KindIndexExpr {
			return TypeSet{}
		}
		fieldName = doc.nodeSource(node.Right)
	case ast.KindString:
		if result, ok := doc.evalNode(node.Right, 0); ok && result.kind == ast.KindString {
			fieldName = []byte(result.str)
		}
	default:
		return TypeSet{}
	}
	if len(fieldName) == 0 {
		return TypeSet{}
	}
	scopedClassHierarchy := false
	if doc.Server != nil && doc.Server.GlobalIndex != nil && leftType.CustomName != "" {
		resource, scope := doc.Server.globalIndexContext(doc)
		className := luaDocGenericClassName(leftType.CustomName)
		_, rootArgs, _ := splitLuaDocGenericType(leftType.CustomName)
		hierarchy := doc.Server.GlobalIndex.LookupLuaDocClassHierarchy(resource, scope, className)
		fields := doc.Server.luaDocClassMemberCandidates(resource, scope, className, string(fieldName), hierarchy)
		genericBindings := luaDocGenericBindingsForHierarchy(hierarchy, rootArgs)
		reachable := doc.Server.luaDocClassFieldReachability(doc, hierarchy)
		scopedClassHierarchy = len(hierarchy) > 0
		for _, candidate := range fields {
			field := candidate.Entry
			fieldDoc := doc.Server.Documents[field.URI]
			if fieldDoc != nil && doc.Server.canSeeLuaDocClassFieldLookup(fieldDoc, candidate, reachable) {
				if field.LuaDoc != nil && field.LuaDoc.TypeString != "" {
					fieldType := field.LuaDoc.TypeString
					if candidate.ClassIndex >= 0 && candidate.ClassIndex < len(genericBindings) {
						fieldType = substituteLuaDocGenericType(fieldType, genericBindings[candidate.ClassIndex])
					}
					if inferred := ParseTypeString(fieldType); inferred.Basics != TypeUnknown || inferred.CustomName != "" {
						return inferred
					}
				}
				if field.Type.Primitive != TypeUnknown {
					return TypeSet{Basics: field.Type.Primitive}
				}
				if valID := fieldDoc.getAssignedValue(field.NodeID); valID != ast.InvalidNode {
					return fieldDoc.InferType(valID)
				}
			}
		}
	}
	propHash := ast.HashBytes(fieldName)
	var writeCandidates []newIndexWriteCandidate
	var routedWrites map[newIndexCandidateKey]bool
	writeRoutingReady := false
	loadWriteRouting := func() map[newIndexCandidateKey]bool {
		if !writeRoutingReady {
			if doc.Server != nil {
				writeCandidates = doc.Server.newIndexWriteCandidates(string(fieldName))
			} else {
				writeCandidates = buildNewIndexWriteCandidates([]*Document{doc}, string(fieldName))
			}
			routedWrites = routedNewIndexCandidates(writeCandidates, string(fieldName))
			writeRoutingReady = true
		}
		return routedWrites
	}
	isRoutedCandidate := func(candidate newIndexWriteCandidate, receiver TypeSet, offset uint32) bool {
		if routed, ok := loadWriteRouting()[newIndexCandidateKey{doc: candidate.doc, node: candidate.node}]; ok {
			return routed
		}
		return candidate.doc.isNewIndexRoutedWrite(receiver, string(fieldName), offset)
	}

	mergeType := func(rt TypeSet) {
		if rt.Basics == TypeUnknown && rt.CustomName == "" {
			t.Basics |= TypeAny
		} else {
			t.Basics |= rt.Basics
			if t.CustomName == "" {
				t.CustomName = rt.CustomName
			}

			if t.DeclNode == ast.InvalidNode && rt.DeclNode != ast.InvalidNode {
				t.DeclNode = rt.DeclNode
				t.DeclURI = rt.DeclURI
			}

			if t.MetaNode == ast.InvalidNode && rt.MetaNode != ast.InvalidNode {
				t.MetaNode = rt.MetaNode
				t.MetaURI = rt.MetaURI
			}

			if t.CallSig == "" && rt.CallSig != "" {
				t.CallSig = rt.CallSig
				t.CallParams = rt.CallParams
				t.CallRet = rt.CallRet
				t.CallURI = rt.CallURI
				t.CallNode = rt.CallNode
			}
		}
	}

	if doc.Server != nil {
		ctx := symbolCtx
		if !symbolContextResolved {
			ctx = doc.Server.resolveSymbolNode(doc.URI, doc, node.Right)
		}
		if ctx != nil && ctx.FiveMExportRes != "" && ctx.TargetDoc != nil && ctx.TargetDefID != ast.InvalidNode {
			if callProxy := ctx.TargetDoc.makeCallableProxyType(ctx.TargetDefID); callProxy.CallSig != "" || callProxy.CallNode != ast.InvalidNode {
				return callProxy
			}
		}
	}

	checkTableFields := func(tDoc *Document, tableID ast.NodeID) {
		if tableID == ast.InvalidNode || int(tableID) >= len(tDoc.Tree.Nodes) {
			return
		}

		src := tDoc.Source()
		if len(src) == 0 {
			return
		}

		tableNode := tDoc.Tree.Nodes[tableID]
		if tableNode.Kind == ast.KindTableExpr {
			for i := uint32(0); i < tableNode.Count; i++ {
				fieldID := tDoc.Tree.ExtraList[tableNode.Extra+uint32(i)]
				field := tDoc.Tree.Nodes[fieldID]

				if field.Kind == ast.KindRecordField {
					if tDoc.Tree.Nodes[field.Left].Kind == ast.KindIdent {
						keyName := tDoc.nodeSource(field.Left)
						if bytes.Equal(keyName, fieldName) {
							mergeType(tDoc.InferType(field.Right))
							return
						}
					}
				}
			}
		}

		recDef := tDoc.getDefForValue(tableID)
		if recDef != ast.InvalidNode {
			recHash := ast.HashBytes(tDoc.nodeSource(recDef))

			if tDoc == doc && leftType.MetaNode != ast.InvalidNode {
				loadWriteRouting()
				for _, candidate := range writeCandidates {
					if candidate.doc != doc || candidate.tree != doc.Tree || candidate.assignStart >= node.Start || int(candidate.member) >= len(doc.Tree.Nodes) {
						continue
					}
					member := doc.Tree.Nodes[candidate.member]
					if doc.referenceAt(member.Left) != recDef || isRoutedCandidate(candidate, doc.InferType(member.Left), candidate.assignStart) {
						continue
					}
					if assigned := doc.getAssignedValue(candidate.node); assigned != ast.InvalidNode {
						t = doc.InferType(assigned)
						break
					}
				}
				return
			}
			for _, fd := range tDoc.Resolver.FieldDefs {
				if fd.ReceiverDef == recDef && fd.ReceiverHash == recHash && fd.PropHash == propHash {
					valID := tDoc.getAssignedValue(fd.NodeID)
					if valID != ast.InvalidNode {
						mergeType(tDoc.InferType(valID))
					} else {
						t.Basics |= TypeAny
					}
				}
			}
		}
	}

	// 1. Target doc declarations
	if leftType.DeclNode != ast.InvalidNode && leftType.DeclURI != "" {
		targetDoc := doc
		if leftType.DeclURI != doc.URI {
			if doc.Server != nil {
				targetDoc = doc.Server.Documents[leftType.DeclURI]
			} else {
				targetDoc = nil
			}
		}

		if targetDoc != nil {
			checkTableFields(targetDoc, leftType.DeclNode)
		}
	}

	// 2. Current doc reassignments/fields on the local reference
	recDef, recHash, _ := doc.Resolver.GetReceiverContext(node.Left)

	for _, fd := range doc.Resolver.FieldDefs {
		if leftType.MetaNode != ast.InvalidNode && leftType.DeclNode != ast.InvalidNode && leftType.DeclURI == doc.URI {
			continue // The ordered own-write pass above already handled this table.
		}
		if fd.ReceiverHash == recHash && (recDef == ast.InvalidNode || fd.ReceiverDef == recDef) {
			if fd.PropHash == propHash {
				if fd.NodeID != ast.InvalidNode && int(fd.NodeID) < len(doc.Tree.Nodes) {
					prop := doc.Tree.Nodes[fd.NodeID]
					if prop.Parent != ast.InvalidNode && int(prop.Parent) < len(doc.Tree.Nodes) {
						receiverType := doc.InferType(doc.Tree.Nodes[prop.Parent].Left)
						candidate := newIndexWriteCandidate{doc: doc, node: fd.NodeID}
						if isRoutedCandidate(candidate, receiverType, prop.Start) {
							continue
						}
					}
				}
				valID := doc.getAssignedValue(fd.NodeID)
				if valID != ast.InvalidNode {
					mergeType(doc.InferType(valID))
				} else {
					t.Basics |= TypeAny
				}
			}
		}
	}
	for _, writeType := range doc.newIndexWriteValues(leftType, string(fieldName), node.Start) {
		mergeType(writeType)
	}

	// 3. Metatables
	if leftType.MetaNode != ast.InvalidNode && (t.Basics == TypeUnknown || t.Basics&TypeNil != 0) && t.CustomName == "" {
		metaDoc := doc
		if leftType.MetaURI != "" && leftType.MetaURI != doc.URI && doc.Server != nil {
			metaDoc = doc.Server.Documents[leftType.MetaURI]
		}

		if metaDoc != nil {
			if inherited, ok := metaDoc.inferMetatableFieldAt(leftType.MetaNode, fieldName, make(map[metatableVisit]struct{}), 0, metatableOffsetForDocument(doc, metaDoc, node.Start)); ok {
				t.Basics &^= TypeNil
				mergeType(inherited)
			}
		}
	}

	// 4. Global Classes
	if leftType.CustomName != "" && doc.Server != nil && !scopedClassHierarchy {
		classQueue := []string{leftType.CustomName}
		visitedClasses := make(map[string]struct{})
		for len(classQueue) > 0 {
			currClassName := classQueue[0]
			classQueue = classQueue[1:]
			if currClassName == "" {
				continue
			}
			if _, seen := visitedClasses[currClassName]; seen {
				continue
			}
			visitedClasses[currClassName] = struct{}{}

			classHash := ast.HashBytes([]byte(currClassName))
			if syms, ok := doc.Server.inferGlobalSymbols(doc, classHash, propHash); ok && len(syms) > 0 {
				sym := syms[0]
				if gDoc, ok := doc.Server.Documents[sym.URI]; ok {
					foundAnnotation := false
					for _, entry := range doc.Server.GlobalIndex.SymbolsByHash(GlobalKey{ReceiverHash: classHash, PropHash: propHash}) {
						if entry != nil && entry.URI == sym.URI && entry.NodeID == sym.NodeID && entry.LuaDoc != nil && entry.LuaDoc.TypeString != "" {
							mergeType(entry.LuaDoc.ParsedType())
							foundAnnotation = true
							break
						}
					}
					valID := gDoc.getAssignedValue(sym.NodeID)
					if foundAnnotation {
						// The indexed LuaDoc field retains callable details that the
						// synthetic declaration node cannot express.
					} else if valID != ast.InvalidNode {
						mergeType(gDoc.InferType(valID))
					} else {
						foundType := false
						if doc.Server.GlobalIndex != nil {
							for _, entry := range doc.Server.GlobalIndex.SymbolsByHash(GlobalKey{ReceiverHash: classHash, PropHash: propHash}) {
								if entry == nil || entry.URI != sym.URI || entry.NodeID != sym.NodeID {
									continue
								}
								if entry.LuaDoc != nil && entry.LuaDoc.TypeString != "" {
									mergeType(entry.LuaDoc.ParsedType())
									foundType = true
									break
								}
								if entry.Type.Primitive != TypeUnknown {
									t.Basics |= entry.Type.Primitive
									foundType = true
									break
								}
							}
						}
						if !foundType {
							t.Basics |= TypeAny
						}
					}

					break
				}
			}
			classEntry := doc.Server.inferFirstGlobalSymbolEntry(doc, 0, classHash)
			if classEntry == nil {
				continue
			}
			classQueue = append(classQueue, symbolParents(classEntry)...)
		}
	}

	if t.Basics == TypeUnknown && t.CustomName == "" && doc.Server != nil && doc.Server.getDocumentFiveMProfile(doc).IsResourceProfile() {
		if glmType, ok := fiveMGLMMemberType(leftType, string(fieldName)); ok {
			return glmType
		}
		if glmType, ok := fiveMGLMSwizzleType(leftType, string(fieldName)); ok {
			return glmType
		}
	}
	return t
}

func (doc *Document) inferCallExpr(node ast.Node) TypeSet {
	funcIdentID := node.Left

	if node.Kind == ast.KindMethodCall {
		funcIdentID = node.Right
	} else if doc.Tree.Nodes[funcIdentID].Kind == ast.KindMemberExpr {
		funcIdentID = doc.Tree.Nodes[funcIdentID].Right
	}
	if node.Kind == ast.KindCallExpr && doc.Server != nil && doc.Server.getDocumentFiveMProfile(doc).IsResourceProfile() {
		if inferred, ok := doc.fiveMGLMDirectVectorCallType(node, funcIdentID); ok {
			return inferred
		}
	}

	if funcIdentID == ast.InvalidNode {
		return TypeSet{}
	}
	var (
		callSymbolCtx       *SymbolContext
		callSymbolCtxLoaded bool
		callReceiver        *symbolReceiverInfo
	)
	var receiverID ast.NodeID
	if node.Kind == ast.KindMethodCall {
		receiverID = node.Left
	}
	if receiverID != ast.InvalidNode && doc.Server != nil && doc.referenceAt(funcIdentID) == ast.InvalidNode {
		callReceiver = &symbolReceiverInfo{NodeID: receiverID}
		callSymbolCtx = doc.Server.resolveSymbolNodeWithReceiverType(doc.URI, doc, funcIdentID, callReceiver)
		callSymbolCtxLoaded = true
	}
	if node.Kind == ast.KindCallExpr || node.Kind == ast.KindMethodCall {
		if inferred, ok := doc.inferLuaDocGenericCall(node, callSymbolCtx, callSymbolCtxLoaded); ok {
			return inferred
		}
	}
	if node.Kind == ast.KindMethodCall {
		callable := doc.inferMemberExprWithContext(node, callSymbolCtx, callReceiver, callSymbolCtxLoaded)
		if callable.CallSig != "" && len(callable.CallRet) > 0 {
			return ParseTypeString(callable.CallRet[0].Type)
		}
	}
	if node.Kind == ast.KindCallExpr {
		resolveOperator := func(receiver TypeSet, name string) (TypeSet, bool) {
			return doc.inferOperatorMetamethodAt(receiver, name, node.Start)
		}
		callable := doc.InferType(node.Left)
		if result, ok := InferMetatableCallOperator(callable, resolveOperator); ok {
			return result
		}
		if callable.CallSig != "" && len(callable.CallRet) > 0 {
			return ParseTypeString(callable.CallRet[0].Type)
		}
		if callable.CustomName != "" {
			arguments := make([]TypeSet, 0, node.Count)
			for i := uint32(0); i < node.Count && node.Extra+i < uint32(len(doc.Tree.ExtraList)); i++ {
				arguments = append(arguments, doc.InferType(doc.Tree.ExtraList[node.Extra+i]))
			}
			if result, ok := doc.inferLuaDocCallOverload(callable, arguments); ok {
				return result
			}
			var argument TypeSet
			if len(arguments) > 0 {
				argument = arguments[0]
			}
			if result, ok := doc.inferLuaDocOperator(callable, argument, "__call"); ok {
				return result
			}
		}
	}

	if node.Kind == ast.KindCallExpr && doc.Tree.Nodes[node.Left].Kind == ast.KindIdent && doc.referenceAt(node.Left) == ast.InvalidNode && bytes.Equal(doc.nodeSource(node.Left), []byte("getmetatable")) && node.Count > 0 && node.Extra < uint32(len(doc.Tree.ExtraList)) {
		arg := doc.InferType(doc.Tree.ExtraList[node.Extra])
		metaDoc := doc
		if arg.MetaURI != "" && arg.MetaURI != doc.URI {
			if doc.Server == nil {
				return TypeSet{}
			}
			metaDoc = doc.Server.Documents[arg.MetaURI]
		}
		if metaDoc != nil && arg.MetaNode != ast.InvalidNode && int(arg.MetaNode) < len(metaDoc.Tree.Nodes) {
			lookupOffset := metatableOffsetForDocument(doc, metaDoc, node.Start)
			protectedValues := metaDoc.metatableFieldAlternativesAt(arg.MetaNode, "__metatable", lookupOffset)
			if len(protectedValues) != 0 {
				var result TypeSet
				for _, protected := range protectedValues {
					if int(protected) < len(metaDoc.Tree.Nodes) && metaDoc.Tree.Nodes[protected].Kind != ast.KindNil {
						result.Basics |= metaDoc.InferType(protected).Basics
					}
				}
				if result.Basics != 0 {
					return result
				}
			}
			return TypeSet{Basics: TypeTable, DeclNode: arg.MetaNode, DeclURI: metaDoc.URI}
		}
		if arg.Basics == TypeTable {
			return TypeSet{Basics: TypeNil}
		}
	}
	if node.Kind == ast.KindCallExpr && doc.Tree.Nodes[node.Left].Kind == ast.KindIdent {
		if def := doc.referenceAt(node.Left); def != ast.InvalidNode {
			if value := doc.getAssignedValue(def); value != ast.InvalidNode && doc.Tree.Nodes[value].Kind == ast.KindFunctionExpr {
				return doc.inferFunctionReturnType(value)
			}
		}
	}
	if doc.Server != nil || (doc.Tree.Nodes[funcIdentID].Kind == ast.KindIdent && bytes.Equal(doc.nodeSource(funcIdentID), []byte("setmetatable"))) {
		if doc.Tree.Nodes[funcIdentID].Kind == ast.KindIdent {
			funcName := doc.nodeSource(funcIdentID)
			if bytes.Equal(funcName, []byte("require")) && node.Count > 0 && node.Extra < uint32(len(doc.Tree.ExtraList)) {
				argID := doc.Tree.ExtraList[node.Extra]

				res, ok := doc.evalNode(argID, 0)
				if ok && res.kind == ast.KindString {
					targetDoc := doc.Server.resolveModule(doc.URI, res.str)
					if targetDoc != nil && targetDoc.ExportedNode != ast.InvalidNode {
						t := targetDoc.InferType(targetDoc.ExportedNode)

						if t.DeclNode == ast.InvalidNode && targetDoc.Tree.Nodes[targetDoc.ExportedNode].Kind == ast.KindTableExpr {
							t.DeclNode = targetDoc.ExportedNode
							t.DeclURI = targetDoc.URI
						}

						return t
					}
				}
			} else if node.Kind == ast.KindCallExpr && node.Left == funcIdentID && doc.referenceAt(funcIdentID) == ast.InvalidNode && bytes.Equal(funcName, []byte("setmetatable")) && node.Count >= 2 && node.Extra+1 < uint32(len(doc.Tree.ExtraList)) {
				arg1ID := doc.Tree.ExtraList[node.Extra]
				arg2ID := doc.Tree.ExtraList[node.Extra+1]

				t := doc.InferType(arg1ID)
				if doc.Tree.Nodes[arg2ID].Kind == ast.KindNil {
					t.MetaNode = ast.InvalidNode
					t.MetaURI = ""
					return t
				}

				metaNodeID := arg2ID
				metaURI := doc.URI

				if doc.Tree.Nodes[arg2ID].Kind == ast.KindIdent {
					defID := doc.referenceAt(arg2ID)
					if defID != ast.InvalidNode {
						valID := doc.getAssignedValue(defID)
						if valID != ast.InvalidNode {
							metaNodeID = valID
						}
					} else {
						identHash := ast.HashBytes(doc.nodeSource(arg2ID))
						if syms, ok := doc.Server.inferGlobalSymbols(doc, 0, identHash); ok && len(syms) > 0 {
							sym := syms[0]
							if gDoc, ok := doc.Server.Documents[sym.URI]; ok {
								valID := gDoc.getAssignedValue(sym.NodeID)
								if valID != ast.InvalidNode {
									metaNodeID = valID
									metaURI = sym.URI
								}
							}
						}
					}
				}

				t.MetaNode = metaNodeID
				t.MetaURI = metaURI

				return t
			}
		}

		if doc.Server == nil {
			return TypeSet{}
		}
		ctx := callSymbolCtx
		if !callSymbolCtxLoaded {
			ctx = doc.Server.resolveSymbolNode(doc.URI, doc, funcIdentID)
		}
		if ctx != nil && ctx.TargetDefID != ast.InvalidNode && ctx.TargetDoc != nil {
			if inferredCallable := ctx.TargetDoc.InferType(ctx.TargetDefID); inferredCallable.CallSig != "" && len(inferredCallable.CallRet) > 0 {
				return ParseTypeString(inferredCallable.CallRet[0].Type)
			}

			luadoc := ctx.TargetDoc.GetLuaDoc(ctx.TargetDefID)

			if luadoc != nil {
				if len(luadoc.Returns) > 0 {
					return ParseTypeString(luadoc.Returns[0].Type)
				}

				if luadoc.Class != nil {
					return TypeSet{CustomName: luadoc.Class.Name}
				}
			}

			valID := ctx.TargetDoc.getAssignedValue(ctx.TargetDefID)
			if valID != ast.InvalidNode && ctx.TargetDoc.Tree.Nodes[valID].Kind == ast.KindFunctionExpr {
				return ctx.TargetDoc.inferFunctionReturnType(valID)
			}
		}
	}

	return TypeSet{}
}

func (doc *Document) makeCallableProxyType(defID ast.NodeID) TypeSet {
	if defID == ast.InvalidNode || int(defID) >= len(doc.Tree.Nodes) {
		return TypeSet{}
	}

	proxy := TypeSet{
		Basics:   TypeTable,
		CallURI:  doc.URI,
		CallNode: defID,
	}

	if luadoc := doc.GetLuaDoc(defID); luadoc != nil {
		proxy.CallParams = append(proxy.CallParams, luadoc.Params...)
		proxy.CallRet = append(proxy.CallRet, luadoc.Returns...)
		proxy.CallSig = buildCallableSignature(luadoc.Params, luadoc.Returns)
	}

	if proxy.CallSig != "" {
		return proxy
	}

	valID := doc.getAssignedValue(defID)
	if valID == ast.InvalidNode || int(valID) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[valID].Kind != ast.KindFunctionExpr {
		return proxy
	}

	funcNode := doc.Tree.Nodes[valID]
	for i := uint32(0); i < funcNode.Count; i++ {
		if funcNode.Extra+uint32(i) >= uint32(len(doc.Tree.ExtraList)) {
			continue
		}

		paramID := doc.Tree.ExtraList[funcNode.Extra+uint32(i)]
		if paramID == ast.InvalidNode || int(paramID) >= len(doc.Tree.Nodes) {
			continue
		}

		paramNode := doc.Tree.Nodes[paramID]
		if paramNode.Start > paramNode.End || paramNode.End > uint32(len(doc.Source())) {
			continue
		}

		proxy.CallParams = append(proxy.CallParams, LuaDocParam{Name: ast.String(doc.nodeSource(paramID))})
	}

	proxy.CallSig = buildCallableSignature(proxy.CallParams, proxy.CallRet)

	return proxy
}

func (doc *Document) findFieldInTable(tableID ast.NodeID, fieldName string) ast.NodeID {
	if tableID == ast.InvalidNode || int(tableID) >= len(doc.Tree.Nodes) {
		return ast.InvalidNode
	}

	node := doc.Tree.Nodes[tableID]
	if node.Kind != ast.KindTableExpr {
		return ast.InvalidNode
	}

	for i := uint32(0); i < node.Count; i++ {
		if node.Extra+uint32(i) >= uint32(len(doc.Tree.ExtraList)) {
			continue
		}

		fieldID := doc.Tree.ExtraList[node.Extra+uint32(i)]
		if int(fieldID) >= len(doc.Tree.Nodes) {
			continue
		}

		field := doc.Tree.Nodes[fieldID]
		if field.Kind == ast.KindRecordField {
			if doc.Tree.Nodes[field.Left].Kind == ast.KindIdent {
				name := doc.nodeSource(field.Left)
				if string(name) == fieldName {
					return field.Right
				}
			}
		} else if field.Kind == ast.KindIndexField {
			if key, ok := doc.evalNode(field.Left, 0); ok && key.kind == ast.KindString && key.str == fieldName {
				return field.Right
			}
		}
	}

	return ast.InvalidNode
}

func (doc *Document) getDefForValue(valID ast.NodeID) ast.NodeID {
	if valID == ast.InvalidNode {
		return ast.InvalidNode
	}

	parentID := doc.Tree.Nodes[valID].Parent
	if parentID == ast.InvalidNode {
		return ast.InvalidNode
	}

	parentNode := doc.Tree.Nodes[parentID]

	switch parentNode.Kind {
	case ast.KindExprList:
		grandParentID := parentNode.Parent
		if grandParentID != ast.InvalidNode {
			grandParentNode := doc.Tree.Nodes[grandParentID]
			if grandParentNode.Kind == ast.KindLocalAssign || grandParentNode.Kind == ast.KindAssign {
				idx := doc.Tree.IndexOfExtra(parentID, valID)
				if idx != -1 {
					lhsNode := doc.Tree.Nodes[grandParentNode.Left]
					if uint32(idx) < lhsNode.Count {
						lhsID := doc.Tree.ExtraList[lhsNode.Extra+uint32(idx)]

						switch doc.Tree.Nodes[lhsID].Kind {
						case ast.KindIdent:
							if grandParentNode.Kind == ast.KindLocalAssign {
								return lhsID
							}
							return doc.referenceAt(lhsID)
						case ast.KindMemberExpr:
							return doc.Tree.Nodes[lhsID].Right
						case ast.KindIndexExpr:
							return lhsID
						}
					}
				}
			}
		}
	case ast.KindLocalFunction, ast.KindFunctionStmt:
		if parentNode.Right == valID {
			leftNode := doc.Tree.Nodes[parentNode.Left]

			switch leftNode.Kind {
			case ast.KindIdent:
				if parentNode.Kind == ast.KindLocalFunction {
					return parentNode.Left
				}

				return doc.referenceAt(parentNode.Left)
			case ast.KindMethodName, ast.KindMemberExpr:
				return leftNode.Right
			}
		}
	case ast.KindRecordField, ast.KindIndexField:
		if parentNode.Right == valID {
			if doc.Tree.Nodes[parentNode.Left].Kind == ast.KindIdent {
				return parentNode.Left
			}
		}
	}

	return ast.InvalidNode
}

func (doc *Document) getIndexTable(metaNodeID ast.NodeID) (*Document, ast.NodeID) {
	indexValID := doc.findFieldInTable(metaNodeID, "__index")

	if indexValID == ast.InvalidNode {
		recDef := doc.getDefForValue(metaNodeID)
		if recDef != ast.InvalidNode {
			recHash := ast.HashBytes(doc.nodeSource(recDef))

			propHash := ast.HashBytes([]byte("__index"))
			for _, fd := range doc.Resolver.FieldDefs {
				if fd.ReceiverDef == recDef && fd.ReceiverHash == recHash && fd.PropHash == propHash {
					indexValID = doc.getAssignedValue(fd.NodeID)
					if indexValID == ast.InvalidNode {
						indexValID = fd.NodeID
					}

					break
				}
			}
		}
	}

	if indexValID != ast.InvalidNode {
		if doc.Tree.Nodes[indexValID].Kind == ast.KindTableExpr {
			return doc, indexValID
		}

		if doc.Tree.Nodes[indexValID].Kind == ast.KindIdent {
			defID := doc.referenceAt(indexValID)
			if defID != ast.InvalidNode {
				valID := doc.getAssignedValue(defID)
				if valID != ast.InvalidNode && doc.Tree.Nodes[valID].Kind == ast.KindTableExpr {
					return doc, valID
				}
			} else if doc.Server != nil {
				identHash := ast.HashBytes(doc.nodeSource(indexValID))
				if syms, ok := doc.Server.inferGlobalSymbols(doc, 0, identHash); ok && len(syms) > 0 {
					sym := syms[0]
					if gDoc, ok := doc.Server.Documents[sym.URI]; ok {
						valID := gDoc.getAssignedValue(sym.NodeID)
						if valID != ast.InvalidNode && gDoc.Tree.Nodes[valID].Kind == ast.KindTableExpr {
							return gDoc, valID
						}
					}
				}
			}
		}
	}

	return nil, ast.InvalidNode
}

func (doc *Document) extractArrayElementType(t TypeSet) TypeSet {
	if t.DeclNode != ast.InvalidNode && t.DeclURI != "" {
		targetDoc := doc
		if t.DeclURI != doc.URI {
			if doc.Server != nil {
				targetDoc = doc.Server.Documents[t.DeclURI]
			} else {
				targetDoc = nil
			}
		}

		if targetDoc != nil {
			node := targetDoc.Tree.Nodes[t.DeclNode]
			if node.Kind == ast.KindTableExpr {
				var elemType TypeSet

				for i := uint32(0); i < node.Count; i++ {
					childID := targetDoc.Tree.ExtraList[node.Extra+uint32(i)]

					child := targetDoc.Tree.Nodes[childID]
					if child.Kind != ast.KindRecordField && child.Kind != ast.KindIndexField {
						rt := targetDoc.InferType(childID)
						elemType.Basics |= rt.Basics

						if elemType.CustomName == "" {
							elemType.CustomName = rt.CustomName
						}

						if elemType.DeclNode == ast.InvalidNode && rt.DeclNode != ast.InvalidNode {
							elemType.DeclNode = rt.DeclNode
							elemType.DeclURI = rt.DeclURI
						}
					}
				}

				if elemType.Basics != TypeUnknown || elemType.CustomName != "" {
					return elemType
				}
			}
		}
	}

	return TypeSet{Basics: TypeUnknown}
}

func (doc *Document) inferFunctionReturnType(funcExprID ast.NodeID) TypeSet {
	var (
		t    TypeSet
		walk func(id ast.NodeID)
	)

	walk = func(id ast.NodeID) {
		if id == ast.InvalidNode {
			return
		}

		node := doc.Tree.Nodes[id]

		if node.Kind == ast.KindFunctionExpr && id != funcExprID {
			return
		}

		if node.Kind == ast.KindReturn {
			if node.Left != ast.InvalidNode {
				exprList := doc.Tree.Nodes[node.Left]
				if exprList.Count > 0 {
					firstExpr := doc.Tree.ExtraList[exprList.Extra]

					rt := doc.InferType(firstExpr)

					t.Basics |= rt.Basics

					if rt.Basics == TypeUnknown && rt.CustomName == "" {
						t.Basics |= TypeAny
					}

					if t.CustomName == "" {
						t.CustomName = rt.CustomName
					}

					if t.DeclNode == ast.InvalidNode && rt.DeclNode != ast.InvalidNode {
						t.DeclNode = rt.DeclNode
						t.DeclURI = rt.DeclURI
					}

					if t.MetaNode == ast.InvalidNode && rt.MetaNode != ast.InvalidNode {
						t.MetaNode = rt.MetaNode
						t.MetaURI = rt.MetaURI
					}
				}
			} else {
				t.Basics |= TypeNil
			}
		}

		walk(node.Left)
		walk(node.Right)

		for i := uint32(0); i < node.Count; i++ {
			walk(doc.Tree.ExtraList[node.Extra+uint32(i)])
		}
	}

	walk(funcExprID)

	return t
}

// ContextualType checks for control flow narrowing directly above the node.
func (doc *Document) ContextualType(id ast.NodeID, offset uint32, base TypeSet) TypeSet {
	if id == ast.InvalidNode || doc.Tree.Nodes[id].Kind != ast.KindIdent {
		return base
	}

	identName := doc.nodeSource(id)

	curr := id

	for curr != ast.InvalidNode {
		node := doc.Tree.Nodes[curr]

		if node.Kind == ast.KindIf || node.Kind == ast.KindElseIf {
			if node.Right != ast.InvalidNode {
				block := doc.Tree.Nodes[node.Right]

				// Narrow type if we are statically inside a successful type check block
				if offset >= block.Start && offset <= block.End {
					narrowed := doc.checkTypeCondition(node.Left, identName, base)

					if narrowed.Basics != base.Basics || narrowed.CustomName != base.CustomName {
						base = narrowed
					}
				}
			}
		}

		curr = node.Parent
	}

	return base
}

func (doc *Document) checkTypeCondition(condID ast.NodeID, targetName []byte, base TypeSet) TypeSet {
	if condID == ast.InvalidNode {
		return base
	}

	cond := doc.Tree.Nodes[condID]

	if cond.Kind == ast.KindIdent {
		name := doc.nodeSource(condID)
		if bytes.Equal(name, targetName) {
			base.Basics &^= TypeNil

			return base
		}
	}

	if cond.Kind == ast.KindBinaryExpr {
		op := token.Kind(cond.Extra)

		switch op {
		case token.NotEq:
			lNode := doc.Tree.Nodes[cond.Left]
			rNode := doc.Tree.Nodes[cond.Right]

			if lNode.Kind == ast.KindIdent && rNode.Kind == ast.KindNil {
				if bytes.Equal(doc.nodeSource(cond.Left), targetName) {
					base.Basics &^= TypeNil

					return base
				}
			} else if rNode.Kind == ast.KindIdent && lNode.Kind == ast.KindNil {
				if bytes.Equal(doc.nodeSource(cond.Right), targetName) {
					base.Basics &^= TypeNil

					return base
				}
			}
		case token.Eq:
			lNode := doc.Tree.Nodes[cond.Left]
			rNode := doc.Tree.Nodes[cond.Right]

			checkTypeCall := func(callNode, strNode ast.Node) {
				if callNode.Kind == ast.KindCallExpr && strNode.Kind == ast.KindString {
					fnID := callNode.Left
					if doc.Tree.Nodes[fnID].Kind == ast.KindIdent {
						fnName := doc.nodeSource(fnID)

						if bytes.Equal(fnName, []byte("type")) && callNode.Count > 0 {
							argID := doc.Tree.ExtraList[callNode.Extra]

							if doc.Tree.Nodes[argID].Kind == ast.KindIdent {
								argName := doc.nodeSource(argID)

								if bytes.Equal(argName, targetName) {
									res, ok := doc.evalNode(cond.Right, 0)
									if !ok {
										res, ok = doc.evalNode(cond.Left, 0)
									}

									if ok && res.kind == ast.KindString {
										base = ParseTypeString(res.str)
									}
								}
							}
						}
					}
				}
			}

			checkTypeCall(lNode, rNode)
			checkTypeCall(rNode, lNode)
		}
	}

	return base
}

// InferColonMethod resolves a Lua colon call (obj:method()) against the
// receiver structural table. Colon calls bind the function's self type to the
// receiver before returning the first method return type.
func InferColonMethod(doc *Document, callNode ast.NodeID, receiverType Type) Type {
	if doc == nil || doc.Tree == nil || callNode == ast.InvalidNode || !inferValidNode(doc, callNode) {
		return Type{}
	}

	node := doc.Tree.Nodes[callNode]
	if node.Kind != ast.KindMethodCall || node.Right == ast.InvalidNode || receiverType.Structural == nil || receiverType.Structural.Fields == nil {
		return Type{}
	}

	methodName := string(inferNodeSource(doc, node.Right))
	if methodName == "" {
		return Type{}
	}

	methodType, ok := receiverType.Structural.Fields[methodName]
	if !ok || methodType.Structural == nil || methodType.Structural.Function == nil {
		return Type{}
	}

	// Colon syntax supplies the receiver as implicit self. Make a shallow copy of
	// the structural type so we don't mutate the interned/cached original.
	structCopy := *methodType.Structural
	fnCopy := *methodType.Structural.Function
	fnCopy.SelfType = receiverType.Structural
	structCopy.Function = &fnCopy
	methodType.Structural = &structCopy

	if len(methodType.Structural.Function.Returns) == 0 {
		return Type{}
	}

	return methodType.Structural.Function.Returns[0]
}

// InferMethodChain resolves a left-to-right chain such as
// obj:method1():method2():method3(). The first node's receiver expression is
// inferred, then each colon method is applied in order.
func InferMethodChain(doc *Document, chain []ast.NodeID) Type {
	if doc == nil || doc.Tree == nil || len(chain) == 0 {
		return Type{}
	}

	first := chain[0]
	if first == ast.InvalidNode || !inferValidNode(doc, first) {
		return Type{}
	}

	receiver := doc.Tree.Nodes[first].Left
	current := inferExpression(doc, receiver)
	for _, callNode := range chain {
		current = InferColonMethod(doc, callNode, current)
		if current.Primitive == TypeUnknown && current.Structural == nil {
			return Type{}
		}
	}

	return current
}

// InferAssignment infers the first RHS value for local/global assignments.
func InferAssignment(doc *Document, assignNode ast.NodeID) Type {
	if doc == nil || doc.Tree == nil || assignNode == ast.InvalidNode || !inferValidNode(doc, assignNode) {
		return Type{}
	}

	node := doc.Tree.Nodes[assignNode]
	if node.Kind != ast.KindLocalAssign && node.Kind != ast.KindAssign {
		return Type{}
	}

	return inferExpression(doc, inferFirstExpr(doc, node.Right))
}

// InferTableLiteral builds a structural table type from record and string index
// fields in a Lua table literal.
func InferTableLiteral(doc *Document, tableNode ast.NodeID) Type {
	if doc == nil || doc.Tree == nil || tableNode == ast.InvalidNode || !inferValidNode(doc, tableNode) {
		return Type{}
	}

	node := doc.Tree.Nodes[tableNode]
	if node.Kind != ast.KindTableExpr {
		return Type{}
	}

	fields := make(map[string]Type)
	for i := uint32(0); i < node.Count; i++ {
		fieldID := doc.Tree.ExtraList[node.Extra+uint32(i)]
		if !inferValidNode(doc, fieldID) {
			continue
		}

		field := doc.Tree.Nodes[fieldID]
		switch field.Kind {
		case ast.KindRecordField:
			if field.Left != ast.InvalidNode && inferValidNode(doc, field.Left) && doc.Tree.Nodes[field.Left].Kind == ast.KindIdent {
				fields[string(inferNodeSource(doc, field.Left))] = inferExpression(doc, field.Right)
			}
		case ast.KindIndexField:
			if name, ok := inferStringKey(doc, field.Left); ok {
				fields[name] = inferExpression(doc, field.Right)
			}
		}
	}

	return Type{Primitive: TypeTable, Structural: &StructuralType{Fields: fields}}
}

func inferExpression(doc *Document, id ast.NodeID) Type {
	if id == ast.InvalidNode || !inferValidNode(doc, id) {
		return Type{}
	}

	node := doc.Tree.Nodes[id]
	switch node.Kind {
	case ast.KindNumber:
		return Type{Primitive: TypeNumber}
	case ast.KindString:
		return Type{Primitive: TypeString}
	case ast.KindHashedString:
		return Type{Primitive: TypeNumber}
	case ast.KindTrue, ast.KindFalse:
		return Type{Primitive: TypeBoolean}
	case ast.KindNil:
		return Type{Primitive: TypeNil}
	case ast.KindTableExpr:
		return InferTableLiteral(doc, id)
	case ast.KindFunctionExpr:
		return inferFunction(doc, id)
	case ast.KindParenExpr:
		return inferExpression(doc, node.Left)
	case ast.KindExprList:
		return inferExpression(doc, inferFirstExpr(doc, id))
	case ast.KindLocalAssign, ast.KindAssign:
		return InferAssignment(doc, id)
	case ast.KindIdent:
		return inferIdent(doc, id)
	case ast.KindMemberExpr:
		return inferMember(doc, node)
	case ast.KindCallExpr:
		return inferDotCall(doc, node)
	case ast.KindMethodCall:
		return InferColonMethod(doc, id, inferExpression(doc, node.Left))
	}

	return Type{}
}

func inferFunction(doc *Document, id ast.NodeID) Type {
	node := doc.Tree.Nodes[id]
	fn := &FunctionType{Params: make([]Type, int(node.Count))}
	if returns := inferReturnTypes(doc, node.Right); len(returns) > 0 {
		fn.Returns = returns
	}

	return Type{Primitive: TypeFunction, Structural: &StructuralType{Function: fn}}
}

func inferReturnTypes(doc *Document, id ast.NodeID) []Type {
	if id == ast.InvalidNode || !inferValidNode(doc, id) {
		return nil
	}

	node := doc.Tree.Nodes[id]
	if node.Kind == ast.KindReturn {
		return inferExprListTypes(doc, node.Left)
	}

	var out []Type
	forEachInferChild(doc, id, func(child ast.NodeID) bool {
		out = inferReturnTypes(doc, child)
		return len(out) == 0
	})

	return out
}

func inferExprListTypes(doc *Document, id ast.NodeID) []Type {
	if id == ast.InvalidNode || !inferValidNode(doc, id) {
		return nil
	}
	node := doc.Tree.Nodes[id]
	if node.Kind != ast.KindExprList {
		return []Type{inferExpression(doc, id)}
	}

	out := make([]Type, 0, node.Count)
	for i := uint32(0); i < node.Count; i++ {
		out = append(out, inferExpression(doc, doc.Tree.ExtraList[node.Extra+uint32(i)]))
	}

	return out
}

func inferMember(doc *Document, node ast.Node) Type {
	if node.Right == ast.InvalidNode || !inferValidNode(doc, node.Right) {
		return Type{}
	}

	receiver := inferExpression(doc, node.Left)
	if receiver.Structural == nil || receiver.Structural.Fields == nil {
		return Type{}
	}

	return receiver.Structural.Fields[string(inferNodeSource(doc, node.Right))]
}

func inferDotCall(doc *Document, node ast.Node) Type {
	callee := inferExpression(doc, node.Left)
	if callee.Structural == nil || callee.Structural.Function == nil || len(callee.Structural.Function.Returns) == 0 {
		return Type{}
	}

	return callee.Structural.Function.Returns[0]
}

func inferIdent(doc *Document, id ast.NodeID) Type {
	name := string(inferNodeSource(doc, id))
	if name == "" {
		return Type{}
	}

	assigned := inferFindAssignedValue(doc, id, name)
	if assigned == ast.InvalidNode {
		return Type{}
	}

	return inferExpression(doc, assigned)
}

func inferFindAssignedValue(doc *Document, ref ast.NodeID, name string) ast.NodeID {
	refStart := doc.Tree.Nodes[ref].Start
	var assigned ast.NodeID

	for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
		node := doc.Tree.Nodes[id]
		if node.Start >= refStart || (node.Kind != ast.KindLocalAssign && node.Kind != ast.KindAssign) {
			continue
		}

		if value := inferAssignedValueForName(doc, node, name); value != ast.InvalidNode {
			assigned = value
		}
	}

	return assigned
}

func inferAssignedValueForName(doc *Document, assign ast.Node, name string) ast.NodeID {
	if assign.Left == ast.InvalidNode || assign.Right == ast.InvalidNode || !inferValidNode(doc, assign.Left) {
		return ast.InvalidNode
	}

	left := doc.Tree.Nodes[assign.Left]
	if left.Kind != ast.KindNameList && left.Kind != ast.KindExprList {
		if doc.Tree.Nodes[assign.Left].Kind == ast.KindIdent && string(inferNodeSource(doc, assign.Left)) == name {
			return inferFirstExpr(doc, assign.Right)
		}
		return ast.InvalidNode
	}

	for i := uint32(0); i < left.Count; i++ {
		lhsID := doc.Tree.ExtraList[left.Extra+uint32(i)]
		if !inferValidNode(doc, lhsID) || doc.Tree.Nodes[lhsID].Kind != ast.KindIdent || string(inferNodeSource(doc, lhsID)) != name {
			continue
		}

		rhs := doc.Tree.Nodes[assign.Right]
		if rhs.Kind == ast.KindExprList && i < rhs.Count {
			return doc.Tree.ExtraList[rhs.Extra+uint32(i)]
		}
		return inferFirstExpr(doc, assign.Right)
	}

	return ast.InvalidNode
}

func inferFirstExpr(doc *Document, id ast.NodeID) ast.NodeID {
	if id == ast.InvalidNode || !inferValidNode(doc, id) {
		return ast.InvalidNode
	}
	node := doc.Tree.Nodes[id]
	if node.Kind == ast.KindExprList && node.Count > 0 {
		return doc.Tree.ExtraList[node.Extra]
	}

	return id
}

func inferStringKey(doc *Document, id ast.NodeID) (string, bool) {
	if id == ast.InvalidNode || !inferValidNode(doc, id) {
		return "", false
	}
	if kind := doc.Tree.Nodes[id].Kind; kind != ast.KindString && kind != ast.KindHashedString {
		return "", false
	}

	raw := string(inferNodeSource(doc, id))
	if len(raw) >= 2 {
		return strings.Trim(raw, "'\"`"), true
	}

	return raw, raw != ""
}

func inferValidNode(doc *Document, id ast.NodeID) bool {
	return doc != nil && doc.Tree != nil && int(id) > 0 && int(id) < len(doc.Tree.Nodes)
}

func inferNodeSource(doc *Document, id ast.NodeID) []byte {
	if !inferValidNode(doc, id) {
		return nil
	}
	node := doc.Tree.Nodes[id]
	if node.Start > node.End || node.End > uint32(len(doc.Tree.Source)) {
		return nil
	}

	return doc.Tree.Source[node.Start:node.End]
}

func forEachInferChild(doc *Document, id ast.NodeID, visit func(ast.NodeID) bool) {
	if id == ast.InvalidNode || !inferValidNode(doc, id) {
		return
	}

	node := doc.Tree.Nodes[id]
	if node.Left != ast.InvalidNode && !visit(node.Left) {
		return
	}
	if node.Right != ast.InvalidNode && !visit(node.Right) {
		return
	}
	for i := uint32(0); i < node.Count; i++ {
		if !visit(doc.Tree.ExtraList[node.Extra+uint32(i)]) {
			return
		}
	}
}
