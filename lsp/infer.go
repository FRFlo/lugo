package lsp

import (
	"strings"

	"github.com/coalaura/lugo/ast"
)

// InferColonMethod resolves a Lua colon call (obj:method()) against the
// receiver structural table. Colon calls bind the function's self type to the
// receiver before returning the first method return type.
func InferColonMethod(doc *Document, callNode ast.NodeID, receiverType Type) Type {
	if doc == nil || doc.Tree == nil || callNode == ast.InvalidNode || !inferV2ValidNode(doc, callNode) {
		return Type{}
	}

	node := doc.Tree.Nodes[callNode]
	if node.Kind != ast.KindMethodCall || node.Right == ast.InvalidNode || receiverType.Structural == nil || receiverType.Structural.Fields == nil {
		return Type{}
	}

	methodName := string(inferV2NodeSource(doc, node.Right))
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
	if first == ast.InvalidNode || !inferV2ValidNode(doc, first) {
		return Type{}
	}

	receiver := doc.Tree.Nodes[first].Left
	current := inferV2Expression(doc, receiver)
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
	if doc == nil || doc.Tree == nil || assignNode == ast.InvalidNode || !inferV2ValidNode(doc, assignNode) {
		return Type{}
	}

	node := doc.Tree.Nodes[assignNode]
	if node.Kind != ast.KindLocalAssign && node.Kind != ast.KindAssign {
		return Type{}
	}

	return inferV2Expression(doc, inferV2FirstExpr(doc, node.Right))
}

// InferTableLiteral builds a structural table type from record and string index
// fields in a Lua table literal.
func InferTableLiteral(doc *Document, tableNode ast.NodeID) Type {
	if doc == nil || doc.Tree == nil || tableNode == ast.InvalidNode || !inferV2ValidNode(doc, tableNode) {
		return Type{}
	}

	node := doc.Tree.Nodes[tableNode]
	if node.Kind != ast.KindTableExpr {
		return Type{}
	}

	fields := make(map[string]Type)
	for i := uint16(0); i < node.Count; i++ {
		fieldID := doc.Tree.ExtraList[node.Extra+uint32(i)]
		if !inferV2ValidNode(doc, fieldID) {
			continue
		}

		field := doc.Tree.Nodes[fieldID]
		switch field.Kind {
		case ast.KindRecordField:
			if field.Left != ast.InvalidNode && inferV2ValidNode(doc, field.Left) && doc.Tree.Nodes[field.Left].Kind == ast.KindIdent {
				fields[string(inferV2NodeSource(doc, field.Left))] = inferV2Expression(doc, field.Right)
			}
		case ast.KindIndexField:
			if name, ok := inferV2StringKey(doc, field.Left); ok {
				fields[name] = inferV2Expression(doc, field.Right)
			}
		}
	}

	return Type{Primitive: TypeTable, Structural: &StructuralType{Fields: fields}}
}

func inferV2Expression(doc *Document, id ast.NodeID) Type {
	if id == ast.InvalidNode || !inferV2ValidNode(doc, id) {
		return Type{}
	}

	node := doc.Tree.Nodes[id]
	switch node.Kind {
	case ast.KindNumber:
		return Type{Primitive: TypeNumber}
	case ast.KindString, ast.KindHashedString:
		return Type{Primitive: TypeString}
	case ast.KindTrue, ast.KindFalse:
		return Type{Primitive: TypeBoolean}
	case ast.KindNil:
		return Type{Primitive: TypeNil}
	case ast.KindTableExpr:
		return InferTableLiteral(doc, id)
	case ast.KindFunctionExpr:
		return inferV2Function(doc, id)
	case ast.KindParenExpr:
		return inferV2Expression(doc, node.Left)
	case ast.KindExprList:
		return inferV2Expression(doc, inferV2FirstExpr(doc, id))
	case ast.KindLocalAssign, ast.KindAssign:
		return InferAssignment(doc, id)
	case ast.KindIdent:
		return inferV2Ident(doc, id)
	case ast.KindMemberExpr:
		return inferV2Member(doc, node)
	case ast.KindCallExpr:
		return inferV2DotCall(doc, node)
	case ast.KindMethodCall:
		return InferColonMethod(doc, id, inferV2Expression(doc, node.Left))
	}

	return Type{}
}

func inferV2Function(doc *Document, id ast.NodeID) Type {
	node := doc.Tree.Nodes[id]
	fn := &FunctionType{Params: make([]Type, int(node.Count))}
	if returns := inferV2ReturnTypes(doc, node.Right); len(returns) > 0 {
		fn.Returns = returns
	}

	return Type{Primitive: TypeFunction, Structural: &StructuralType{Function: fn}}
}

func inferV2ReturnTypes(doc *Document, id ast.NodeID) []Type {
	if id == ast.InvalidNode || !inferV2ValidNode(doc, id) {
		return nil
	}

	node := doc.Tree.Nodes[id]
	if node.Kind == ast.KindReturn {
		return inferV2ExprListTypes(doc, node.Left)
	}

	var out []Type
	forEachInferV2Child(doc, id, func(child ast.NodeID) bool {
		out = inferV2ReturnTypes(doc, child)
		return len(out) == 0
	})

	return out
}

func inferV2ExprListTypes(doc *Document, id ast.NodeID) []Type {
	if id == ast.InvalidNode || !inferV2ValidNode(doc, id) {
		return nil
	}
	node := doc.Tree.Nodes[id]
	if node.Kind != ast.KindExprList {
		return []Type{inferV2Expression(doc, id)}
	}

	out := make([]Type, 0, node.Count)
	for i := uint16(0); i < node.Count; i++ {
		out = append(out, inferV2Expression(doc, doc.Tree.ExtraList[node.Extra+uint32(i)]))
	}

	return out
}

func inferV2Member(doc *Document, node ast.Node) Type {
	if node.Right == ast.InvalidNode || !inferV2ValidNode(doc, node.Right) {
		return Type{}
	}

	receiver := inferV2Expression(doc, node.Left)
	if receiver.Structural == nil || receiver.Structural.Fields == nil {
		return Type{}
	}

	return receiver.Structural.Fields[string(inferV2NodeSource(doc, node.Right))]
}

func inferV2DotCall(doc *Document, node ast.Node) Type {
	callee := inferV2Expression(doc, node.Left)
	if callee.Structural == nil || callee.Structural.Function == nil || len(callee.Structural.Function.Returns) == 0 {
		return Type{}
	}

	return callee.Structural.Function.Returns[0]
}

func inferV2Ident(doc *Document, id ast.NodeID) Type {
	name := string(inferV2NodeSource(doc, id))
	if name == "" {
		return Type{}
	}

	assigned := inferV2FindAssignedValue(doc, id, name)
	if assigned == ast.InvalidNode {
		return Type{}
	}

	return inferV2Expression(doc, assigned)
}

func inferV2FindAssignedValue(doc *Document, ref ast.NodeID, name string) ast.NodeID {
	refStart := doc.Tree.Nodes[ref].Start
	var assigned ast.NodeID

	for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
		node := doc.Tree.Nodes[id]
		if node.Start >= refStart || (node.Kind != ast.KindLocalAssign && node.Kind != ast.KindAssign) {
			continue
		}

		if value := inferV2AssignedValueForName(doc, node, name); value != ast.InvalidNode {
			assigned = value
		}
	}

	return assigned
}

func inferV2AssignedValueForName(doc *Document, assign ast.Node, name string) ast.NodeID {
	if assign.Left == ast.InvalidNode || assign.Right == ast.InvalidNode || !inferV2ValidNode(doc, assign.Left) {
		return ast.InvalidNode
	}

	left := doc.Tree.Nodes[assign.Left]
	if left.Kind != ast.KindNameList && left.Kind != ast.KindExprList {
		if doc.Tree.Nodes[assign.Left].Kind == ast.KindIdent && string(inferV2NodeSource(doc, assign.Left)) == name {
			return inferV2FirstExpr(doc, assign.Right)
		}
		return ast.InvalidNode
	}

	for i := uint16(0); i < left.Count; i++ {
		lhsID := doc.Tree.ExtraList[left.Extra+uint32(i)]
		if !inferV2ValidNode(doc, lhsID) || doc.Tree.Nodes[lhsID].Kind != ast.KindIdent || string(inferV2NodeSource(doc, lhsID)) != name {
			continue
		}

		rhs := doc.Tree.Nodes[assign.Right]
		if rhs.Kind == ast.KindExprList && i < rhs.Count {
			return doc.Tree.ExtraList[rhs.Extra+uint32(i)]
		}
		return inferV2FirstExpr(doc, assign.Right)
	}

	return ast.InvalidNode
}

func inferV2FirstExpr(doc *Document, id ast.NodeID) ast.NodeID {
	if id == ast.InvalidNode || !inferV2ValidNode(doc, id) {
		return ast.InvalidNode
	}
	node := doc.Tree.Nodes[id]
	if node.Kind == ast.KindExprList && node.Count > 0 {
		return doc.Tree.ExtraList[node.Extra]
	}

	return id
}

func inferV2StringKey(doc *Document, id ast.NodeID) (string, bool) {
	if id == ast.InvalidNode || !inferV2ValidNode(doc, id) {
		return "", false
	}
	if kind := doc.Tree.Nodes[id].Kind; kind != ast.KindString && kind != ast.KindHashedString {
		return "", false
	}

	raw := string(inferV2NodeSource(doc, id))
	if len(raw) >= 2 {
		return strings.Trim(raw, "'\"`"), true
	}

	return raw, raw != ""
}

func inferV2ValidNode(doc *Document, id ast.NodeID) bool {
	return doc != nil && doc.Tree != nil && int(id) > 0 && int(id) < len(doc.Tree.Nodes)
}

func inferV2NodeSource(doc *Document, id ast.NodeID) []byte {
	if !inferV2ValidNode(doc, id) {
		return nil
	}
	node := doc.Tree.Nodes[id]
	if node.Start > node.End || node.End > uint32(len(doc.Tree.Source)) {
		return nil
	}

	return doc.Tree.Source[node.Start:node.End]
}

func forEachInferV2Child(doc *Document, id ast.NodeID, visit func(ast.NodeID) bool) {
	if id == ast.InvalidNode || !inferV2ValidNode(doc, id) {
		return
	}

	node := doc.Tree.Nodes[id]
	if node.Left != ast.InvalidNode && !visit(node.Left) {
		return
	}
	if node.Right != ast.InvalidNode && !visit(node.Right) {
		return
	}
	for i := uint16(0); i < node.Count; i++ {
		if !visit(doc.Tree.ExtraList[node.Extra+uint32(i)]) {
			return
		}
	}
}
