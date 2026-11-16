package lsp

import (
	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/token"
)

// MetatableOperatorResolver looks up a metamethod on receiver and returns the
// inferred result type of calling it. The callback owns metatable traversal,
// function return inference, and any document/workspace lookup.
type MetatableOperatorResolver func(receiver TypeSet, metamethod string) (TypeSet, bool)

// LuaMetatableOperatorName maps Lua 5.4 unary/binary operator tokens to their
// metamethod names. Relational operators are deliberately excluded: Lua's
// __eq/__lt/__le behavior has distinct rules and is not a numeric result.
func LuaMetatableOperatorName(op token.Kind) (string, bool) {
	switch op {
	case token.Plus:
		return "__add", true
	case token.Minus:
		return "__sub", true
	case token.Asterisk:
		return "__mul", true
	case token.Modulo:
		return "__mod", true
	case token.Caret:
		return "__pow", true
	case token.Slash:
		return "__div", true
	case token.FloorSlash:
		return "__idiv", true
	case token.BitAnd:
		return "__band", true
	case token.BitOr:
		return "__bor", true
	case token.BitXor:
		return "__bxor", true
	case token.ShiftLeft:
		return "__shl", true
	case token.ShiftRight:
		return "__shr", true
	case token.Concat:
		return "__concat", true
	case token.Hash:
		return "__len", true
	default:
		return "", false
	}
}

// LuaMetatableUnaryOperatorName maps Lua unary operator tokens to their
// metamethod names. Hash is also supported by LuaMetatableOperatorName for
// callers that represent length as an operator expression.
func LuaMetatableUnaryOperatorName(op token.Kind) (string, bool) {
	switch op {
	case token.Minus:
		return "__unm", true
	case token.BitXor:
		return "__bnot", true
	case token.Hash:
		return "__len", true
	default:
		return "", false
	}
}

// InferMetatableBinaryOperator follows Lua's callbinTM order: try the left
// operand's metamethod first, then the right operand's. The resolver is
// responsible for returning the result type (including first-return behavior).
func InferMetatableBinaryOperator(op token.Kind, left, right TypeSet, resolve MetatableOperatorResolver) (TypeSet, bool) {
	name, ok := LuaMetatableOperatorName(op)
	if !ok || resolve == nil {
		return TypeSet{}, false
	}
	if result, found := resolve(left, name); found {
		return result, true
	}
	return resolve(right, name)
}

// InferMetatableUnaryOperator resolves the metamethod for a unary operator on
// its sole operand. The resolver returns the inferred call result type.
func InferMetatableUnaryOperator(op token.Kind, operand TypeSet, resolve MetatableOperatorResolver) (TypeSet, bool) {
	name, ok := LuaMetatableUnaryOperatorName(op)
	if !ok || resolve == nil {
		return TypeSet{}, false
	}
	return resolve(operand, name)
}

// InferMetatableCallOperator resolves __call on the original callable value.
// Lua passes that value as argument one; the caller can model the extra
// argument when it derives the called function's return type.
func InferMetatableCallOperator(callable TypeSet, resolve MetatableOperatorResolver) (TypeSet, bool) {
	if resolve == nil {
		return TypeSet{}, false
	}
	return resolve(callable, "__call")
}

// inferOperatorMetamethod finds a directly stored metamethod and infers its first return.
func (doc *Document) inferOperatorMetamethod(receiver TypeSet, name string) (TypeSet, bool) {
	return doc.inferOperatorMetamethodAt(receiver, name, ^uint32(0))
}

func (doc *Document) inferOperatorMetamethodAt(receiver TypeSet, name string, offset uint32) (TypeSet, bool) {
	if receiver.MetaNode == 0 {
		return TypeSet{}, false
	}
	metaDoc := doc
	if receiver.MetaURI != "" && receiver.MetaURI != doc.URI {
		if doc.Server == nil {
			return TypeSet{}, false
		}
		metaDoc = doc.Server.Documents[receiver.MetaURI]
	}
	if metaDoc == nil || int(receiver.MetaNode) >= len(metaDoc.Tree.Nodes) {
		return TypeSet{}, false
	}
	lookupOffset := metatableOffsetForDocument(doc, metaDoc, offset)
	fields := metaDoc.metatableFieldAlternativesAt(receiver.MetaNode, name, lookupOffset)
	if len(fields) == 0 {
		return TypeSet{}, false
	}
	var result TypeSet
	found := false
	for _, field := range fields {
		if int(field) >= len(metaDoc.Tree.Nodes) || metaDoc.Tree.Nodes[field].Kind == ast.KindNil {
			continue
		}
		functionDoc, functionID := metaDoc.resolveMetatableIndexValue(field)
		if functionDoc == nil || functionID == 0 || int(functionID) >= len(functionDoc.Tree.Nodes) || functionDoc.Tree.Nodes[functionID].Kind != ast.KindFunctionExpr {
			continue
		}
		part := functionDoc.inferFunctionReturnType(functionID)
		result.Basics |= part.Basics
		found = true
	}
	return result, found
}
