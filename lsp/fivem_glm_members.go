package lsp

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// fiveMGLMMemberType infers built-in vector/quaternion properties documented
// by cfxlua-glm's runtime accessors. It is a standalone hook for member
// inference: wire it into member lookup (for example in infer.go) after
// evaluating the receiver type. This helper intentionally does not alter the
// existing inference pipeline.
//
// Runtime basis: cfxlua-glm README.md describes numeric, x/y/z/w and r/g/b/a
// access, reserved n/dim, quaternion angle/axis, and immutable vectors. Its
// lglm_core.h accessor implements n and component aliases; dimensional bounds
// depend on the concrete vector size. Matrices are numeric-indexed columns,
// not named-member vectors. GLM namespace operations and metatable methods are
// not inferred here because their signatures come from binding functions.
func fiveMGLMMemberType(receiver TypeSet, member string) (TypeSet, bool) {
	dimension, quaternion, ok := fiveMGLMVectorShape(receiver.CustomName)
	if !ok {
		return TypeSet{}, false
	}
	if member == "n" || member == "dim" {
		return TypeSet{Basics: TypeNumber}, true
	}
	if quaternion {
		switch member {
		case "angle":
			return TypeSet{Basics: TypeNumber}, true
		case "axis":
			return TypeSet{CustomName: "Vector3"}, true
		}
	}
	if len(member) != 1 {
		return TypeSet{}, false
	}
	component, valid := fiveMGLMComponent(member[0])
	if !valid || component >= dimension {
		return TypeSet{}, false
	}
	return TypeSet{Basics: TypeNumber}, true
}

// fiveMGLMSwizzleType returns the result type for a valid runtime vector
// swizzle. LuaGLM accepts at most four x/y/z/w characters, each within the
// source dimension. A single-character field also supports component aliases.
func fiveMGLMSwizzleType(receiver TypeSet, swizzle string) (TypeSet, bool) {
	dimension, quaternion, ok := fiveMGLMVectorShape(receiver.CustomName)
	if !ok || len(swizzle) == 0 || len(swizzle) > 4 {
		return TypeSet{}, false
	}
	for i := 0; i < len(swizzle); i++ {
		if len(swizzle) > 1 && !strings.ContainsRune("xyzw", rune(swizzle[i])) {
			return TypeSet{}, false
		}
		component, valid := fiveMGLMComponent(swizzle[i])
		if !valid || component >= dimension {
			return TypeSet{}, false
		}
	}
	if len(swizzle) == 1 {
		return TypeSet{Basics: TypeNumber}, true
	}
	if quaternion && len(swizzle) == 4 {
		return TypeSet{Basics: TypeAny}, true
	}
	return TypeSet{CustomName: [...]string{"", "", "Vector2", "Vector3", "Vector4"}[len(swizzle)]}, true
}

func fiveMGLMVectorShape(name string) (dimension int, quaternion bool, ok bool) {
	name = strings.TrimSpace(name)
	switch {
	case strings.EqualFold(name, "vector2"):
		return 2, false, true
	case strings.EqualFold(name, "vector3"):
		return 3, false, true
	case strings.EqualFold(name, "vector4"):
		return 4, false, true
	case strings.EqualFold(name, "quat"):
		return 4, true, true
	default:
		return 0, false, false
	}
}

// fiveMGLMIndexType describes numeric access as implemented by cfxlua-glm:
// matrix indices select 1-based column vectors; polygon indices select
// 1-based Vector3 points. The matrix dimensions are part of its runtime type.
func fiveMGLMIndexType(receiver TypeSet, index float64) (TypeSet, bool) {
	if index != 0 && (index < 1 || index != float64(int(index))) {
		return TypeSet{}, false
	}
	name := strings.TrimSpace(receiver.CustomName)
	if len(name) >= len("matrix") && strings.EqualFold(name[:len("matrix")], "matrix") {
		shape := name[len("matrix"):]
		separator := strings.IndexAny(shape, "xX")
		if separator < 0 {
			return TypeSet{}, false
		}
		columns, errC := strconv.Atoi(shape[:separator])
		rows, errR := strconv.Atoi(shape[separator+1:])
		if errC != nil || errR != nil || columns < 2 || columns > 4 || rows < 2 || rows > 4 || (index != 0 && int(index) > columns) {
			return TypeSet{}, false
		}
		return TypeSet{CustomName: [...]string{"", "", "Vector2", "Vector3", "Vector4"}[rows]}, true
	}
	if strings.EqualFold(name, "polygon") {
		return TypeSet{CustomName: "Vector3"}, true
	}
	if dimension, _, ok := fiveMGLMVectorShape(receiver.CustomName); ok && (index == 0 || int(index) <= dimension) {
		return TypeSet{Basics: TypeNumber}, true
	}
	return TypeSet{}, false
}

// fiveMGLMExprType recovers GLM constructor types for direct and chained
// indexing expressions when the binding's LuaDoc symbol is unavailable.
func (doc *Document) fiveMGLMExprType(id ast.NodeID, depth int) TypeSet {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) || depth > 8 {
		return TypeSet{}
	}
	if inferred := doc.InferType(id); inferred.CustomName != "" {
		return inferred
	}
	node := doc.Tree.Nodes[id]
	if node.Kind == ast.KindIdent {
		if def := doc.referenceAt(id); def != ast.InvalidNode {
			if value := doc.getAssignedValue(def); value != ast.InvalidNode {
				return doc.fiveMGLMExprType(value, depth+1)
			}
		}
		return TypeSet{}
	}
	name, builtin := doc.fiveMGLMCallName(node)
	if !builtin {
		return TypeSet{}
	}
	if name == "new" {
		return TypeSet{CustomName: "Polygon"}
	}
	return doc.fiveMGLMConstructorType(node, name, depth)
}

func (doc *Document) fiveMGLMConstructorType(node ast.Node, name string, depth int) TypeSet {
	switch name {
	case "vec", "ivec", "bvec", "vector":
		return doc.fiveMGLMGenericVectorCallType(node, name)
	case "vec1", "ivec1", "bvec1", "vector1":
		if node.Count == 0 {
			return TypeSet{}
		}
		if name == "bvec1" {
			return TypeSet{Basics: TypeBoolean}
		}
		return TypeSet{Basics: TypeNumber}
	case "vec2", "vector2", "ivec2", "bvec2":
		return TypeSet{CustomName: "Vector2"}
	case "vec3", "vector3", "ivec3", "bvec3":
		return TypeSet{CustomName: "Vector3"}
	case "vec4", "vector4", "ivec4", "bvec4":
		return TypeSet{CustomName: "Vector4"}
	case "mat":
		return doc.fiveMGLMGenericMatrixCallType(node, depth)
	case "mat2", "mat2x2":
		return TypeSet{CustomName: "Matrix2x2"}
	case "mat2x3":
		return TypeSet{CustomName: "Matrix2x3"}
	case "mat2x4":
		return TypeSet{CustomName: "Matrix2x4"}
	case "mat3":
		return TypeSet{CustomName: "Matrix3x3"}
	case "mat3x2":
		return TypeSet{CustomName: "Matrix3x2"}
	case "mat3x3":
		return TypeSet{CustomName: "Matrix3x3"}
	case "mat3x4":
		return TypeSet{CustomName: "Matrix3x4"}
	case "mat4":
		return TypeSet{CustomName: "Matrix4x4"}
	case "mat4x2":
		return TypeSet{CustomName: "Matrix4x2"}
	case "mat4x3":
		return TypeSet{CustomName: "Matrix4x3"}
	case "mat4x4":
		return TypeSet{CustomName: "Matrix4x4"}
	case "quat", "qua":
		return TypeSet{CustomName: "Quat"}
	case "polygon":
		return TypeSet{CustomName: "Polygon"}
	}
	return TypeSet{}
}

// fiveMGLMGenericVectorCallType follows glm_createVector's input packing:
// scalars add one component, vectors and array literals add their known
// component count, and the generic constructor returns a scalar for one.
func (doc *Document) fiveMGLMGenericVectorCallType(node ast.Node, name string) TypeSet {
	if node.Count == 0 || node.Count > 4 || node.Extra+node.Count > uint32(len(doc.Tree.ExtraList)) {
		return TypeSet{}
	}
	dimension := 0
	for i := uint32(0); i < node.Count; i++ {
		arg := doc.Tree.ExtraList[node.Extra+i]
		argNode := doc.Tree.Nodes[arg]
		if argNode.Kind == ast.KindTableExpr {
			// Only plain array literals have a statically knowable GLM width.
			if argNode.Count == 0 || argNode.Count > 4 || argNode.Extra+argNode.Count > uint32(len(doc.Tree.ExtraList)) {
				return TypeSet{}
			}
			for j := uint32(0); j < argNode.Count; j++ {
				field := doc.Tree.Nodes[doc.Tree.ExtraList[argNode.Extra+j]]
				if field.Kind != ast.KindNumber && field.Kind != ast.KindTrue && field.Kind != ast.KindFalse {
					return TypeSet{}
				}
			}
			dimension += int(argNode.Count)
		} else if typ := doc.fiveMGLMExprType(arg, 1); typ.CustomName != "" {
			d, _, ok := fiveMGLMVectorShape(typ.CustomName)
			if !ok {
				return TypeSet{}
			}
			dimension += d
		} else if typ.Basics&(TypeNumber|TypeBoolean) != 0 || argNode.Kind == ast.KindNumber || argNode.Kind == ast.KindTrue || argNode.Kind == ast.KindFalse {
			dimension++
		} else {
			return TypeSet{}
		}
		if dimension > 4 {
			return TypeSet{}
		}
	}
	if dimension == 1 {
		if name == "bvec" {
			return TypeSet{Basics: TypeBoolean}
		}
		return TypeSet{Basics: TypeNumber}
	}
	if dimension >= 2 {
		return TypeSet{CustomName: [...]string{"", "", "Vector2", "Vector3", "Vector4"}[dimension]}
	}
	return TypeSet{}
}

func (doc *Document) fiveMGLMGenericMatrixCallType(node ast.Node, depth int) TypeSet {
	if node.Count == 0 {
		return TypeSet{CustomName: "Matrix4x4"}
	}
	if node.Count == 1 {
		if node.Extra >= uint32(len(doc.Tree.ExtraList)) {
			return TypeSet{}
		}
		arg := doc.Tree.ExtraList[node.Extra]
		argNode := doc.Tree.Nodes[arg]
		typ := doc.fiveMGLMExprType(arg, depth+1)
		if typ.CustomName != "" {
			if _, _, ok := fiveMGLMMatrixShape(typ.CustomName); ok {
				return typ
			}
			if typ.CustomName == "Quat" {
				return TypeSet{CustomName: "Matrix4x4"}
			}
		}
		if argNode.Kind == ast.KindNumber {
			return TypeSet{CustomName: "Matrix4x4"}
		}
		if argNode.Kind == ast.KindTableExpr {
			return doc.fiveMGLMMatrixColumnsType(argNode, argNode.Count, depth+1)
		}
		return TypeSet{}
	}
	return doc.fiveMGLMMatrixColumnsType(node, node.Count, depth+1)
}

func (doc *Document) fiveMGLMMatrixColumnsType(container ast.Node, count uint32, depth int) TypeSet {
	if count < 2 || count > 4 || container.Extra+count > uint32(len(doc.Tree.ExtraList)) || depth > 8 {
		return TypeSet{}
	}
	rows := 0
	for i := uint32(0); i < count; i++ {
		arg := doc.Tree.ExtraList[container.Extra+i]
		argNode := doc.Tree.Nodes[arg]
		width := 0
		if argNode.Kind == ast.KindTableExpr {
			if argNode.Count < 2 || argNode.Count > 4 || argNode.Extra+argNode.Count > uint32(len(doc.Tree.ExtraList)) {
				return TypeSet{}
			}
			width = int(argNode.Count)
			for j := uint32(0); j < argNode.Count; j++ {
				field := doc.Tree.Nodes[doc.Tree.ExtraList[argNode.Extra+j]]
				if field.Kind != ast.KindNumber && field.Kind != ast.KindTrue && field.Kind != ast.KindFalse {
					return TypeSet{}
				}
			}
		} else if typ := doc.fiveMGLMExprType(arg, depth+1); typ.CustomName != "" {
			width, _, _ = fiveMGLMVectorShape(typ.CustomName)
		}
		if width < 2 || width > 4 || (rows != 0 && rows != width) {
			return TypeSet{}
		}
		rows = width
	}
	if rows == 0 {
		return TypeSet{}
	}
	return TypeSet{CustomName: "Matrix" + strconv.Itoa(int(count)) + "x" + strconv.Itoa(rows)}
}

func fiveMGLMMatrixShape(name string) (columns, rows int, ok bool) {
	if !strings.HasPrefix(strings.ToLower(name), "matrix") {
		return 0, 0, false
	}
	shape := name[len("Matrix"):]
	x := strings.IndexByte(shape, 'x')
	if x < 1 {
		return 0, 0, false
	}
	columns, err1 := strconv.Atoi(shape[:x])
	rows, err2 := strconv.Atoi(shape[x+1:])
	return columns, rows, err1 == nil && err2 == nil && columns >= 2 && columns <= 4 && rows >= 2 && rows <= 4
}

func (doc *Document) fiveMGLMDirectVectorCallType(node ast.Node, callee ast.NodeID) (TypeSet, bool) {
	if node.Kind != ast.KindCallExpr || callee == ast.InvalidNode || int(callee) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[callee].Kind != ast.KindIdent {
		return TypeSet{}, false
	}
	if node.Left != callee && (int(node.Left) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[node.Left].Kind != ast.KindMemberExpr || doc.Tree.Nodes[node.Left].Right != callee) {
		return TypeSet{}, false
	}
	name, ok := doc.fiveMGLMCallName(node)
	if !ok {
		return TypeSet{}, false
	}
	return doc.fiveMGLMConstructorType(node, name, 0), true
}

// fiveMGLMCallName recognizes only runtime constructor identities: bare
// unshadowed globals and the explicitly namespaced glm.polygon API.
func (doc *Document) fiveMGLMCallName(node ast.Node) (string, bool) {
	if node.Kind != ast.KindCallExpr && node.Kind != ast.KindMethodCall {
		return "", false
	}
	callee := node.Left
	if node.Kind == ast.KindMethodCall {
		root := doc.fiveMGLMMemberPathRoot(node.Left, "glm", "polygon")
		if root != ast.InvalidNode && node.Right != ast.InvalidNode && bytes.Equal(doc.nodeSource(node.Right), []byte("new")) && doc.fiveMGLMIsUnshadowedGlobal(root, "glm") {
			return "new", true
		}
		return "", false
	}
	if callee == ast.InvalidNode || int(callee) >= len(doc.Tree.Nodes) {
		return "", false
	}
	calleeNode := doc.Tree.Nodes[callee]
	if calleeNode.Kind == ast.KindIdent {
		name := fiveMGLMGlobalConstructorName(doc.nodeSource(callee))
		if name == "" {
			return "", false
		}
		if !doc.fiveMGLMIsUnshadowedGlobal(callee, name) {
			return "", false
		}
		return name, true
	}
	if node.Kind == ast.KindCallExpr && calleeNode.Kind == ast.KindMemberExpr {
		root := doc.fiveMGLMMemberPathRoot(callee, "glm", "polygon", "new")
		if root != ast.InvalidNode && doc.fiveMGLMIsUnshadowedGlobal(root, "glm") {
			return "new", true
		}
		// Namespaced constructors are exactly one member below the global glm
		// table; e.g. glm.vec3, never glm.custom.vec3.
		root = doc.fiveMGLMMemberPathRoot(callee, "glm", string(doc.nodeSource(calleeNode.Right)))
		if root == ast.InvalidNode || !doc.fiveMGLMIsUnshadowedGlobal(root, "glm") {
			return "", false
		}
		if name := fiveMGLMNamespaceConstructorName(doc.nodeSource(calleeNode.Right)); name != "" && !doc.fiveMGLMFieldOverridden(name, node.Start) {
			return name, true
		}
	}
	return "", false
}

func (doc *Document) fiveMGLMExactMemberPath(id ast.NodeID, names ...string) bool {
	return doc.fiveMGLMMemberPathRoot(id, names...) != ast.InvalidNode
}

func (doc *Document) fiveMGLMMemberPathRoot(id ast.NodeID, names ...string) ast.NodeID {
	if len(names) == 0 || id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return ast.InvalidNode
	}
	current := id
	for i := len(names) - 1; i > 0; i-- {
		if int(current) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[current].Kind != ast.KindMemberExpr {
			return ast.InvalidNode
		}
		node := doc.Tree.Nodes[current]
		if node.Right == ast.InvalidNode || !bytes.Equal(doc.nodeSource(node.Right), []byte(names[i])) {
			return ast.InvalidNode
		}
		current = node.Left
	}
	if int(current) < len(doc.Tree.Nodes) && doc.Tree.Nodes[current].Kind == ast.KindIdent && bytes.Equal(doc.nodeSource(current), []byte(names[0])) {
		return current
	}
	return ast.InvalidNode
}

func (doc *Document) fiveMGLMFieldOverridden(name string, before uint32) bool {
	propHash := ast.HashBytes([]byte(name))
	glmHash := ast.HashBytes([]byte("glm"))
	if doc.Resolver != nil {
		for _, field := range doc.Resolver.FieldDefs {
			if field.PropHash != propHash || field.ReceiverHash != glmHash || !bytes.Equal(field.ReceiverName, []byte("glm")) || field.ReceiverDef != ast.InvalidNode || field.NodeID == ast.InvalidNode || int(field.NodeID) >= len(doc.Tree.Nodes) {
				continue
			}
			if doc.Tree.Nodes[field.NodeID].Start < before && doc.getAssignedValue(field.NodeID) != ast.InvalidNode {
				return true
			}
		}
	}
	// Reuse the server's versioned, decoded-key index instead of scanning every
	// AST node on each lookup. It is invalidated as part of document reindexing.
	var candidates []newIndexWriteCandidate
	if doc.Server != nil {
		candidates = doc.Server.newIndexWriteCandidates(name)
	} else {
		candidates = buildNewIndexWriteCandidates([]*Document{doc}, name)
	}
	for _, candidate := range candidates {
		if candidate.doc != doc || candidate.tree != doc.Tree || candidate.assignStart >= before || candidate.member == ast.InvalidNode || int(candidate.member) >= len(doc.Tree.Nodes) {
			continue
		}
		member := doc.Tree.Nodes[candidate.member]
		if member.Kind == ast.KindIndexExpr && doc.fiveMGLMExactMemberPath(member.Left, "glm") && doc.fiveMGLMIsUnshadowedGlobal(member.Left, "glm") {
			return true
		}
	}
	return false
}

func fiveMGLMGlobalConstructorName(name []byte) string {
	for _, candidate := range [...]string{"vec", "vec1", "vec2", "vec3", "vec4", "vector", "vector1", "vector2", "vector3", "vector4", "ivec", "ivec1", "ivec2", "ivec3", "ivec4", "bvec", "bvec1", "bvec2", "bvec3", "bvec4", "mat", "mat2", "mat2x2", "mat2x3", "mat2x4", "mat3", "mat3x2", "mat3x3", "mat3x4", "mat4", "mat4x2", "mat4x3", "mat4x4", "qua", "quat"} {
		if bytes.Equal(name, []byte(candidate)) {
			return candidate
		}
	}
	return ""
}

func fiveMGLMNamespaceConstructorName(name []byte) string {
	for _, candidate := range [...]string{"vec", "vec1", "vec2", "vec3", "vec4", "ivec", "ivec1", "ivec2", "ivec3", "ivec4", "bvec", "bvec1", "bvec2", "bvec3", "bvec4", "qua", "quat", "mat", "mat2", "mat2x2", "mat2x3", "mat2x4", "mat3", "mat3x2", "mat3x3", "mat3x4", "mat4", "mat4x2", "mat4x3", "mat4x4"} {
		if bytes.Equal(name, []byte(candidate)) {
			return candidate
		}
	}
	return ""
}

func (doc *Document) fiveMGLMIsUnshadowedGlobal(ident ast.NodeID, name string) bool {
	if def := doc.referenceAt(ident); def != ast.InvalidNode && int(def) < len(doc.Tree.Nodes) {
		definition := doc.Tree.Nodes[def]
		if definition.Start < doc.Tree.Nodes[ident].Start && bytes.Equal(doc.nodeSource(def), []byte(name)) {
			return false
		}
	}
	// A global assignment is not connected to a later global reference by the
	// local resolver, so check its ordered global definition list as well.
	if doc.Resolver != nil {
		for _, definition := range doc.Resolver.GlobalDefs {
			if definition != ast.InvalidNode && int(definition) < len(doc.Tree.Nodes) && doc.Tree.Nodes[definition].Start < doc.Tree.Nodes[ident].Start && bytes.Equal(doc.nodeSource(definition), []byte(name)) {
				return false
			}
		}
	}
	return true
}

func fiveMGLMComponent(char byte) (int, bool) {
	switch char {
	case 'x', 'r', '1':
		return 0, true
	case 'y', 'g', '2':
		return 1, true
	case 'z', 'b', '3':
		return 2, true
	case 'w', 'a', '4':
		return 3, true
	default:
		return 0, false
	}
}
