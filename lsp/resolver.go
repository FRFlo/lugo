package lsp

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/coalaura/lugo/ast"
)

type ResolverPhase uint8

const (
	ResolverPhaseNone ResolverPhase = iota
	ResolverPhaseDeclarations
	ResolverPhaseTypes
)

type ResolverOptions struct {
	FeatureFiveM bool
	ResourceURI  ResourceURI
	Scope        GlobalIndexScope
	Index        *GlobalIndex
	SemanticData *SemanticDataTable
	PhaseState   *ResolverPhaseState
}

type ResolverPhaseState struct {
	mu       sync.RWMutex
	complete map[ResourceURI]bool
}

func NewResolverPhaseState() *ResolverPhaseState {
	return &ResolverPhaseState{complete: make(map[ResourceURI]bool)}
}

func (s *ResolverPhaseState) MarkPhase1Complete(uri ResourceURI) {
	if s == nil || uri == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.complete == nil {
		s.complete = make(map[ResourceURI]bool)
	}
	s.complete[uri] = true
}

func (s *ResolverPhaseState) IsPhase1Complete(uri ResourceURI) bool {
	if s == nil || uri == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.complete[uri]
}

type Resolver struct {
	Tree *ast.Tree

	References    []ast.NodeID
	GlobalRefs    []ast.NodeID
	GlobalDefs    []ast.NodeID
	LocalDefs     []ast.NodeID
	FieldDefs     []resolverFieldDef
	PendingFields []resolverFieldRef

	Data *SemanticDataTable

	FeatureFiveM bool
	ResourceURI  ResourceURI
	Scope        GlobalIndexScope
	Index        *GlobalIndex

	Phase      ResolverPhase
	PhaseState *ResolverPhaseState

	rootScope *resolverScope
	scopes    map[ast.NodeID]*resolverScope
	globals   map[string][]resolverDecl
	fieldMap  map[resolverFieldKey]ast.NodeID
	types     []Type
	inferring []bool
}

type resolverScope struct {
	data     *Scope
	nodeID   ast.NodeID
	parent   *resolverScope
	children []*resolverScope
	decls    map[string][]resolverDecl
}

type resolverDecl struct {
	Name   string
	NodeID ast.NodeID
	Scope  *resolverScope
	Global bool
}

type resolverFieldDef struct {
	ReceiverName []byte
	ReceiverHash uint64
	PropHash     uint64
	ReceiverDef  ast.NodeID
	NodeID       ast.NodeID
}

type resolverFieldRef struct {
	PropNodeID   ast.NodeID
	ReceiverDef  ast.NodeID
	ReceiverHash uint64
	ReceiverName []byte
	PropHash     uint64
}

type resolverFieldKey struct {
	RecDef   ast.NodeID
	RecHash  uint64
	PropHash uint64
}

func NewResolver(tree *ast.Tree, opts ResolverOptions) *Resolver {
	data := opts.SemanticData
	if data == nil {
		data = NewSemanticDataTable()
	}

	scope := opts.Scope
	if scope == "" {
		scope = GlobalIndexScopeShared
	}

	return &Resolver{
		Tree:          tree,
		References:    make([]ast.NodeID, len(tree.Nodes)),
		GlobalRefs:    make([]ast.NodeID, 0, 256),
		GlobalDefs:    make([]ast.NodeID, 0, 256),
		LocalDefs:     make([]ast.NodeID, 0, 512),
		FieldDefs:     make([]resolverFieldDef, 0, 512),
		PendingFields: make([]resolverFieldRef, 0, 128),
		Data:          data,
		FeatureFiveM:  opts.FeatureFiveM,
		ResourceURI:   opts.ResourceURI,
		Scope:         scope,
		Index:         opts.Index,
		PhaseState:    opts.PhaseState,
		scopes:        make(map[ast.NodeID]*resolverScope, 64),
		globals:       make(map[string][]resolverDecl, 256),
		fieldMap:      make(map[resolverFieldKey]ast.NodeID, 512),
	}
}

func (r *Resolver) Resolve(root ast.NodeID) error {
	if err := r.Phase1(root); err != nil {
		return err
	}

	return r.Phase2(root)
}

func (r *Resolver) Phase1(root ast.NodeID) error {
	if r == nil || r.Tree == nil || root == ast.InvalidNode {
		return nil
	}

	r.resetPhase1()
	r.rootScope = r.newScope(ast.InvalidNode, nil)
	r.collectDeclarations(root, r.rootScope)
	r.collectReferences(root, r.rootScope)
	r.bindReferences()
	r.bindPendingFields()

	r.PhaseState.MarkPhase1Complete(r.ResourceURI)
	r.Phase = ResolverPhaseDeclarations

	return nil
}

func (r *Resolver) Phase2(root ast.NodeID) error {
	if r == nil || r.Tree == nil || root == ast.InvalidNode {
		return nil
	}
	if r.Phase < ResolverPhaseDeclarations {
		if err := r.Phase1(root); err != nil {
			return err
		}
	}
	if err := r.ensurePhase2DependenciesReady(); err != nil {
		return err
	}

	r.types = make([]Type, len(r.Tree.Nodes))
	r.inferring = make([]bool, len(r.Tree.Nodes))
	r.resolveTypes(root)
	r.publishGlobalsToIndex()
	r.Phase = ResolverPhaseTypes

	return nil
}

func (r *Resolver) resetPhase1() {
	if cap(r.References) >= len(r.Tree.Nodes) {
		r.References = r.References[:len(r.Tree.Nodes)]
		clear(r.References)
	} else {
		r.References = make([]ast.NodeID, len(r.Tree.Nodes))
	}
	r.GlobalRefs = r.GlobalRefs[:0]
	r.GlobalDefs = r.GlobalDefs[:0]
	r.LocalDefs = r.LocalDefs[:0]
	r.FieldDefs = r.FieldDefs[:0]
	r.PendingFields = r.PendingFields[:0]
	r.Phase = ResolverPhaseNone
	r.rootScope = nil
	clear(r.scopes)
	clear(r.globals)
	clear(r.fieldMap)
	if r.Data != nil {
		r.Data.Clear()
	}
}

func (r *Resolver) newScope(nodeID ast.NodeID, parent *resolverScope) *resolverScope {
	s := &resolverScope{
		nodeID: nodeID,
		parent: parent,
		decls:  make(map[string][]resolverDecl),
		data: &Scope{
			Symbols: make(map[string]NodeID),
		},
	}
	if parent != nil {
		s.data.Parent = parent.data
		parent.children = append(parent.children, s)
	}
	if nodeID != ast.InvalidNode {
		r.scopes[nodeID] = s
		r.mergeSemantic(nodeID, SemanticData{Scope: s.data})
	}

	return s
}

func (r *Resolver) declareLocal(scope *resolverScope, identID ast.NodeID) {
	if scope == nil || identID == ast.InvalidNode || !r.validNode(identID) {
		return
	}
	name := string(r.source(identID))
	if name == "" {
		return
	}

	decl := resolverDecl{Name: name, NodeID: identID, Scope: scope}
	scope.decls[name] = append(scope.decls[name], decl)
	scope.data.Symbols[name] = NodeID(identID)
	r.LocalDefs = append(r.LocalDefs, identID)
	r.References[identID] = identID
	r.mergeSemantic(identID, SemanticData{Scope: scope.data, Bindings: []Binding{{Name: name, NodeID: NodeID(identID)}}})
}

func (r *Resolver) declareGlobal(identID ast.NodeID) {
	if identID == ast.InvalidNode || !r.validNode(identID) {
		return
	}
	name := string(r.source(identID))
	if name == "" {
		return
	}

	decl := resolverDecl{Name: name, NodeID: identID, Scope: r.rootScope, Global: true}
	r.globals[name] = append(r.globals[name], decl)
	r.GlobalDefs = append(r.GlobalDefs, identID)
	r.References[identID] = identID
	r.mergeSemantic(identID, SemanticData{Scope: r.rootScope.data, Bindings: []Binding{{Name: name, NodeID: NodeID(identID)}}})
}

func (r *Resolver) collectDeclarations(id ast.NodeID, scope *resolverScope) {
	if id == ast.InvalidNode || !r.validNode(id) {
		return
	}
	node := r.Tree.Nodes[id]

	switch node.Kind {
	case ast.KindFile:
		r.collectDeclarations(node.Left, scope)
	case ast.KindBlock:
		child := r.newScope(id, scope)
		for i := uint16(0); i < node.Count; i++ {
			r.collectDeclarations(r.Tree.ExtraList[node.Extra+uint32(i)], child)
		}
	case ast.KindDo, ast.KindWhile, ast.KindElseIf, ast.KindElse:
		r.collectDeclarations(node.Left, scope)
		r.collectDeclarations(node.Right, scope)
	case ast.KindLocalAssign:
		forEachExtra(r.Tree, node.Left, func(child ast.NodeID) { r.declareLocal(scope, child) })
		r.collectDeclarations(node.Right, scope)
	case ast.KindLocalFunction:
		r.declareLocal(scope, node.Left)
		r.collectFunctionDeclarations(node.Right, scope)
	case ast.KindFunctionStmt:
		r.collectFunctionNameDeclaration(node.Left, scope)
		r.collectFunctionDeclarations(node.Right, scope)
	case ast.KindFunctionExpr:
		r.collectFunctionDeclarations(id, scope)
	case ast.KindAssign:
		forEachExtra(r.Tree, node.Left, func(child ast.NodeID) {
			if r.Tree.Nodes[child].Kind == ast.KindIdent {
				r.declareGlobal(child)
			} else {
				r.collectFieldDeclaration(child)
				r.collectDeclarations(child, scope)
			}
		})
		r.collectDeclarations(node.Right, scope)
	case ast.KindForNum:
		for i := uint16(0); i < node.Count; i++ {
			r.collectDeclarations(r.Tree.ExtraList[node.Extra+uint32(i)], scope)
		}
		child := r.newScope(id, scope)
		r.declareLocal(child, node.Left)
		r.collectDeclarations(node.Right, child)
	case ast.KindForIn:
		r.collectDeclarations(ast.NodeID(node.Extra), scope)
		child := r.newScope(id, scope)
		forEachExtra(r.Tree, node.Left, func(name ast.NodeID) { r.declareLocal(child, name) })
		r.collectDeclarations(node.Right, child)
	case ast.KindRepeat:
		child := r.newScope(id, scope)
		r.collectDeclarations(node.Left, child)
		r.collectDeclarations(node.Right, child)
	case ast.KindTableExpr:
		forEachExtra(r.Tree, id, func(fieldID ast.NodeID) {
			field := r.Tree.Nodes[fieldID]
			if field.Kind == ast.KindRecordField {
				r.collectTableFieldDeclaration(id, fieldID)
				r.collectDeclarations(field.Right, scope)
			} else {
				r.collectDeclarations(fieldID, scope)
			}
		})
	case ast.KindIf:
		r.collectDeclarations(node.Left, scope)
		r.collectDeclarations(node.Right, scope)
		for i := uint16(0); i < node.Count; i++ {
			r.collectDeclarations(r.Tree.ExtraList[node.Extra+uint32(i)], scope)
		}
	case ast.KindExprList, ast.KindReturn:
		forEachExtra(r.Tree, id, func(child ast.NodeID) { r.collectDeclarations(child, scope) })
		r.collectDeclarations(node.Left, scope)
		r.collectDeclarations(node.Right, scope)
	case ast.KindCallExpr, ast.KindMethodCall:
		r.collectDeclarations(node.Left, scope)
		r.collectDeclarations(node.Right, scope)
		for i := uint16(0); i < node.Count; i++ {
			r.collectDeclarations(r.Tree.ExtraList[node.Extra+uint32(i)], scope)
		}
	case ast.KindBinaryExpr, ast.KindUnaryExpr, ast.KindParenExpr, ast.KindIndexExpr, ast.KindMemberExpr, ast.KindMethodName, ast.KindRecordField, ast.KindIndexField:
		r.collectDeclarations(node.Left, scope)
		r.collectDeclarations(node.Right, scope)
	}
}

func (r *Resolver) collectFunctionDeclarations(funcID ast.NodeID, parent *resolverScope) {
	if funcID == ast.InvalidNode || !r.validNode(funcID) {
		return
	}
	node := r.Tree.Nodes[funcID]
	if node.Kind != ast.KindFunctionExpr {
		r.collectDeclarations(funcID, parent)
		return
	}

	fnScope := r.newScope(funcID, parent)
	for i := uint16(0); i < node.Count; i++ {
		r.declareLocal(fnScope, r.Tree.ExtraList[node.Extra+uint32(i)])
	}
	r.collectDeclarations(node.Right, fnScope)
}

func (r *Resolver) collectFunctionNameDeclaration(nameID ast.NodeID, scope *resolverScope) {
	if nameID == ast.InvalidNode || !r.validNode(nameID) {
		return
	}
	switch r.Tree.Nodes[nameID].Kind {
	case ast.KindIdent:
		if r.lookupLocalByPosition(scope, string(r.source(nameID)), nameID) == ast.InvalidNode {
			r.declareGlobal(nameID)
		} else {
			r.References[nameID] = r.lookupLocalByPosition(scope, string(r.source(nameID)), nameID)
		}
	case ast.KindMemberExpr, ast.KindMethodName:
		r.collectFieldDeclaration(nameID)
	}
}

func (r *Resolver) collectFieldDeclaration(memberNodeID ast.NodeID) {
	if memberNodeID == ast.InvalidNode || !r.validNode(memberNodeID) {
		return
	}
	node := r.Tree.Nodes[memberNodeID]
	if (node.Kind != ast.KindMemberExpr && node.Kind != ast.KindMethodName) || node.Right == ast.InvalidNode || !r.validNode(node.Right) || r.Tree.Nodes[node.Right].Kind != ast.KindIdent {
		return
	}
	recDef, recHash, recName := r.receiverContext(node.Left)
	if len(recName) == 0 {
		return
	}
	propHash := ast.HashBytes(r.source(node.Right))
	fk := resolverFieldKey{RecDef: recDef, RecHash: recHash, PropHash: propHash}
	if existing, ok := r.fieldMap[fk]; ok {
		r.References[node.Right] = existing
		r.mergeSemantic(node.Right, SemanticData{Bindings: []Binding{{Name: string(r.source(node.Right)), NodeID: NodeID(existing)}}})
		return
	}
	r.FieldDefs = append(r.FieldDefs, resolverFieldDef{ReceiverDef: recDef, ReceiverHash: recHash, ReceiverName: recName, PropHash: propHash, NodeID: node.Right})
	r.fieldMap[fk] = node.Right
	r.References[node.Right] = node.Right
	r.mergeSemantic(node.Right, SemanticData{Bindings: []Binding{{Name: string(r.source(node.Right)), NodeID: NodeID(node.Right)}}})
}

func (r *Resolver) collectTableFieldDeclaration(tableID, fieldID ast.NodeID) {
	parentDef, parentRec := r.tableReceiver(tableID)
	if len(parentRec) == 0 || !r.validNode(fieldID) {
		return
	}
	field := r.Tree.Nodes[fieldID]
	if field.Kind != ast.KindRecordField || field.Left == ast.InvalidNode || !r.validNode(field.Left) || r.Tree.Nodes[field.Left].Kind != ast.KindIdent {
		return
	}
	propHash := ast.HashBytes(r.source(field.Left))
	recHash := ast.HashBytes(parentRec)
	fk := resolverFieldKey{RecDef: parentDef, RecHash: recHash, PropHash: propHash}
	if existing, ok := r.fieldMap[fk]; ok {
		r.References[field.Left] = existing
		r.mergeSemantic(field.Left, SemanticData{Bindings: []Binding{{Name: string(r.source(field.Left)), NodeID: NodeID(existing)}}})
		return
	}
	r.FieldDefs = append(r.FieldDefs, resolverFieldDef{ReceiverDef: parentDef, ReceiverHash: recHash, ReceiverName: parentRec, PropHash: propHash, NodeID: field.Left})
	r.fieldMap[fk] = field.Left
	r.References[field.Left] = field.Left
	r.mergeSemantic(field.Left, SemanticData{Bindings: []Binding{{Name: string(r.source(field.Left)), NodeID: NodeID(field.Left)}}})
}

func (r *Resolver) collectReferences(id ast.NodeID, scope *resolverScope) {
	if id == ast.InvalidNode || !r.validNode(id) {
		return
	}
	node := r.Tree.Nodes[id]

	switch node.Kind {
	case ast.KindFile:
		r.collectReferences(node.Left, scope)
	case ast.KindBlock, ast.KindForNum, ast.KindForIn, ast.KindRepeat, ast.KindFunctionExpr:
		if s := r.scopes[id]; s != nil {
			scope = s
		}
		if node.Kind == ast.KindFunctionExpr {
			r.collectReferences(node.Right, scope)
			return
		}
		if node.Kind == ast.KindForNum {
			for i := uint16(0); i < node.Count; i++ {
				r.collectReferences(r.Tree.ExtraList[node.Extra+uint32(i)], scope.parentOrSelf())
			}
			r.collectReferences(node.Right, scope)
			return
		}
		if node.Kind == ast.KindForIn {
			r.collectReferences(ast.NodeID(node.Extra), scope.parentOrSelf())
			r.collectReferences(node.Right, scope)
			return
		}
		if node.Kind == ast.KindRepeat {
			r.collectReferences(node.Left, scope)
			r.collectReferences(node.Right, scope)
			return
		}
		for i := uint16(0); i < node.Count; i++ {
			r.collectReferences(r.Tree.ExtraList[node.Extra+uint32(i)], scope)
		}
	case ast.KindLocalAssign:
		r.collectReferences(node.Right, scope)
	case ast.KindLocalFunction:
		r.collectReferences(node.Right, scope)
	case ast.KindFunctionStmt:
		if node.Left != ast.InvalidNode && r.validNode(node.Left) {
			left := r.Tree.Nodes[node.Left]
			if left.Kind == ast.KindMemberExpr || left.Kind == ast.KindMethodName {
				r.collectReferences(left.Left, scope)
			}
		}
		r.collectReferences(node.Right, scope)
	case ast.KindAssign:
		forEachExtra(r.Tree, node.Left, func(child ast.NodeID) {
			childNode := r.Tree.Nodes[child]
			if childNode.Kind == ast.KindIdent {
				if r.References[child] == ast.InvalidNode {
					r.referencesIdent(child, scope, true)
				}
			} else {
				r.collectReferences(child, scope)
			}
		})
		r.collectReferences(node.Right, scope)
	case ast.KindIdent, ast.KindVararg:
		r.referencesIdent(id, scope, false)
	case ast.KindMemberExpr, ast.KindMethodCall:
		r.collectReferences(node.Left, scope)
		if node.Right != ast.InvalidNode && r.validNode(node.Right) && r.Tree.Nodes[node.Right].Kind == ast.KindIdent {
			recDef, recHash, recName := r.receiverContext(node.Left)
			if len(recName) > 0 {
				r.PendingFields = append(r.PendingFields, resolverFieldRef{PropNodeID: node.Right, ReceiverDef: recDef, ReceiverHash: recHash, ReceiverName: recName, PropHash: ast.HashBytes(r.source(node.Right))})
			}
		}
		if node.Kind == ast.KindMethodCall {
			for i := uint16(0); i < node.Count; i++ {
				r.collectReferences(r.Tree.ExtraList[node.Extra+uint32(i)], scope)
			}
		}
	case ast.KindTableExpr:
		forEachExtra(r.Tree, id, func(fieldID ast.NodeID) {
			field := r.Tree.Nodes[fieldID]
			if field.Kind == ast.KindRecordField {
				r.collectReferences(field.Right, scope)
			} else {
				r.collectReferences(fieldID, scope)
			}
		})
	case ast.KindIf:
		r.collectReferences(node.Left, scope)
		r.collectReferences(node.Right, scope)
		for i := uint16(0); i < node.Count; i++ {
			r.collectReferences(r.Tree.ExtraList[node.Extra+uint32(i)], scope)
		}
	case ast.KindCallExpr, ast.KindExprList, ast.KindReturn:
		r.collectReferences(node.Left, scope)
		r.collectReferences(node.Right, scope)
		for i := uint16(0); i < node.Count; i++ {
			r.collectReferences(r.Tree.ExtraList[node.Extra+uint32(i)], scope)
		}
	case ast.KindDo, ast.KindWhile, ast.KindElseIf, ast.KindElse, ast.KindBinaryExpr, ast.KindUnaryExpr, ast.KindParenExpr, ast.KindIndexExpr, ast.KindMethodName, ast.KindRecordField, ast.KindIndexField:
		r.collectReferences(node.Left, scope)
		r.collectReferences(node.Right, scope)
	}
}

func (s *resolverScope) parentOrSelf() *resolverScope {
	if s != nil && s.parent != nil {
		return s.parent
	}
	return s
}

func (r *Resolver) referencesIdent(id ast.NodeID, scope *resolverScope, isDef bool) {
	if id == ast.InvalidNode || !r.validNode(id) || r.References[id] != ast.InvalidNode {
		return
	}
	name := string(r.source(id))
	if name == "" || name == "self" {
		return
	}

	if defID := r.lookupLocalByPosition(scope, name, id); defID != ast.InvalidNode {
		r.References[id] = defID
		r.mergeSemantic(id, SemanticData{Bindings: []Binding{{Name: name, NodeID: NodeID(defID)}}})
		return
	}
	if isDef {
		r.declareGlobal(id)
		return
	}
	r.GlobalRefs = append(r.GlobalRefs, id)
}

func (r *Resolver) bindReferences() {
	for _, refID := range r.GlobalRefs {
		if r.References[refID] != ast.InvalidNode {
			continue
		}
		name := string(r.source(refID))
		if defs := r.globals[name]; len(defs) > 0 {
			r.References[refID] = defs[0].NodeID
			r.mergeSemantic(refID, SemanticData{Bindings: []Binding{{Name: name, NodeID: NodeID(defs[0].NodeID)}}})
		}
	}
}

func (r *Resolver) bindPendingFields() {
	for _, pref := range r.PendingFields {
		fk := resolverFieldKey{RecDef: pref.ReceiverDef, RecHash: pref.ReceiverHash, PropHash: pref.PropHash}
		if defID, ok := r.fieldMap[fk]; ok {
			r.References[pref.PropNodeID] = defID
			r.mergeSemantic(pref.PropNodeID, SemanticData{Bindings: []Binding{{Name: string(r.source(pref.PropNodeID)), NodeID: NodeID(defID)}}})
		}
	}
}

func (r *Resolver) lookupLocalByPosition(scope *resolverScope, name string, refID ast.NodeID) ast.NodeID {
	if scope == nil || name == "" || !r.validNode(refID) {
		return ast.InvalidNode
	}
	refStart := r.Tree.Nodes[refID].Start
	for s := scope; s != nil; s = s.parent {
		decls := s.decls[name]
		for i := len(decls) - 1; i >= 0; i-- {
			decl := decls[i]
			if decl.NodeID == refID || !r.validNode(decl.NodeID) {
				continue
			}
			if r.isLocalAssignInitializerReference(decl.NodeID, refID) {
				continue
			}
			if r.Tree.Nodes[decl.NodeID].Start <= refStart {
				return decl.NodeID
			}
		}
	}

	return ast.InvalidNode
}

func (r *Resolver) isLocalAssignInitializerReference(defID, refID ast.NodeID) bool {
	if defID == ast.InvalidNode || refID == ast.InvalidNode || !r.validNode(defID) || !r.validNode(refID) {
		return false
	}
	parentID := r.Tree.Nodes[defID].Parent
	if parentID == ast.InvalidNode || !r.validNode(parentID) || r.Tree.Nodes[parentID].Kind != ast.KindNameList {
		return false
	}
	stmtID := r.Tree.Nodes[parentID].Parent
	if stmtID == ast.InvalidNode || !r.validNode(stmtID) {
		return false
	}
	stmt := r.Tree.Nodes[stmtID]
	if stmt.Kind != ast.KindLocalAssign || stmt.Right == ast.InvalidNode || !r.validNode(stmt.Right) {
		return false
	}
	rhs := r.Tree.Nodes[stmt.Right]
	ref := r.Tree.Nodes[refID]

	return ref.Start >= rhs.Start && ref.End <= rhs.End
}

func (r *Resolver) ensurePhase2DependenciesReady() error {
	if !r.FeatureFiveM || r.Index == nil || r.ResourceURI == "" || r.PhaseState == nil {
		return nil
	}
	r.Index.mu.RLock()
	res := r.Index.Resources[r.ResourceURI]
	var deps []ResourceURI
	if res != nil {
		deps = append(deps, res.Dependencies...)
	}
	r.Index.mu.RUnlock()

	for _, dep := range deps {
		if !r.PhaseState.IsPhase1Complete(dep) {
			return fmt.Errorf("resolver phase 2 waiting for dependency phase 1: %s", dep)
		}
	}

	return nil
}

func (r *Resolver) resolveTypes(id ast.NodeID) Type {
	t := r.inferType(id)
	if id != ast.InvalidNode && r.validNode(id) {
		r.mergeSemantic(id, SemanticData{Type: t})
	}
	node := ast.Node{}
	if r.validNode(id) {
		node = r.Tree.Nodes[id]
	}

	switch node.Kind {
	case ast.KindLocalAssign, ast.KindAssign:
		r.assignDeclarationTypes(node.Left, node.Right)
	case ast.KindLocalFunction:
		fnType := r.inferType(node.Right)
		r.setNodeType(node.Left, fnType)
	case ast.KindFunctionStmt:
		fnType := r.inferType(node.Right)
		r.setNodeType(node.Left, fnType)
	}

	r.walkChildren(id, r.resolveTypes)

	return t
}

func (r *Resolver) inferType(id ast.NodeID) Type {
	if id == ast.InvalidNode || !r.validNode(id) {
		return Type{}
	}
	if len(r.types) == 0 {
		r.types = make([]Type, len(r.Tree.Nodes))
		r.inferring = make([]bool, len(r.Tree.Nodes))
	}
	if r.types[id].Primitive != TypeUnknown || r.types[id].Structural != nil {
		return r.types[id]
	}
	if r.inferring[id] {
		return Type{}
	}
	r.inferring[id] = true
	defer func() { r.inferring[id] = false }()

	node := r.Tree.Nodes[id]
	var t Type
	switch node.Kind {
	case ast.KindNumber:
		t.Primitive = TypeNumber
	case ast.KindString, ast.KindHashedString:
		t.Primitive = TypeString
	case ast.KindTrue, ast.KindFalse:
		t.Primitive = TypeBoolean
	case ast.KindNil:
		t.Primitive = TypeNil
	case ast.KindFunctionExpr, ast.KindLocalFunction, ast.KindFunctionStmt:
		t = r.functionType(id)
	case ast.KindTableExpr:
		t = r.tableType(id)
	case ast.KindIdent:
		t = r.identType(id)
	case ast.KindParenExpr:
		t = r.inferType(node.Left)
	case ast.KindUnaryExpr:
		src := r.source(id)
		if bytes.HasPrefix(src, []byte("not")) {
			t.Primitive = TypeBoolean
		} else {
			t.Primitive = TypeNumber
		}
	case ast.KindBinaryExpr:
		t = r.binaryType(node)
	case ast.KindMemberExpr:
		t = r.memberType(node)
	case ast.KindCallExpr, ast.KindMethodCall:
		t = r.callType(node)
	case ast.KindLocalAssign, ast.KindAssign:
		t = r.inferType(node.Right)
	case ast.KindExprList:
		if node.Count > 0 && node.Extra < uint32(len(r.Tree.ExtraList)) {
			t = r.inferType(r.Tree.ExtraList[node.Extra])
		} else {
			t = r.inferType(node.Left)
		}
	}
	r.types[id] = t

	return t
}

func (r *Resolver) identType(id ast.NodeID) Type {
	name := string(r.source(id))
	defID := ast.InvalidNode
	if int(id) < len(r.References) {
		defID = r.References[id]
	}
	if defID == id {
		if assigned := r.assignedValue(id); assigned != ast.InvalidNode && assigned != id {
			return r.inferType(assigned)
		}
	}
	if defID != ast.InvalidNode && defID != id {
		if assigned := r.assignedValue(defID); assigned != ast.InvalidNode {
			return r.inferType(assigned)
		}
		if int(defID) < len(r.types) && (r.types[defID].Primitive != TypeUnknown || r.types[defID].Structural != nil) {
			return r.types[defID]
		}
	}
	if r.FeatureFiveM {
		switch name {
		case "source":
			return Type{Primitive: TypeNumber}
		case "exports":
			return Type{Primitive: TypeTable, Structural: &StructuralType{Fields: map[string]Type{}}}
		}
	}
	if r.Index != nil && name != "" {
		if entry := r.lookupIndexSymbol(SymbolName(name)); entry != nil {
			return entry.Type
		}
	}

	return Type{}
}

func (r *Resolver) functionType(id ast.NodeID) Type {
	if id != ast.InvalidNode && r.validNode(id) && r.Tree.Nodes[id].Kind != ast.KindFunctionExpr {
		if val := r.assignedValue(id); val != ast.InvalidNode {
			id = val
		}
	}
	fn := &FunctionType{}
	if id != ast.InvalidNode && r.validNode(id) && r.Tree.Nodes[id].Kind == ast.KindFunctionExpr {
		node := r.Tree.Nodes[id]
		fn.Params = make([]Type, int(node.Count))
		returns := r.returnTypes(node.Right)
		if len(returns) > 0 {
			fn.Returns = returns
		}
	}

	return Type{Primitive: TypeFunction, Structural: &StructuralType{Function: fn}}
}

func (r *Resolver) returnTypes(id ast.NodeID) []Type {
	if id == ast.InvalidNode || !r.validNode(id) {
		return nil
	}
	node := r.Tree.Nodes[id]
	if node.Kind == ast.KindReturn {
		out := make([]Type, 0, node.Count)
		if node.Left != ast.InvalidNode && r.validNode(node.Left) {
			left := r.Tree.Nodes[node.Left]
			if left.Kind == ast.KindExprList {
				for i := uint16(0); i < left.Count; i++ {
					out = append(out, r.inferType(r.Tree.ExtraList[left.Extra+uint32(i)]))
				}
			} else {
				out = append(out, r.inferType(node.Left))
			}
		}
		for i := uint16(0); i < node.Count; i++ {
			out = append(out, r.inferType(r.Tree.ExtraList[node.Extra+uint32(i)]))
		}
		return out
	}
	var out []Type
	r.walkChildren(id, func(child ast.NodeID) Type {
		if len(out) == 0 {
			out = r.returnTypes(child)
		}
		return Type{}
	})

	return out
}

func (r *Resolver) tableType(id ast.NodeID) Type {
	fields := make(map[string]Type)
	node := r.Tree.Nodes[id]
	for i := uint16(0); i < node.Count; i++ {
		fieldID := r.Tree.ExtraList[node.Extra+uint32(i)]
		if !r.validNode(fieldID) {
			continue
		}
		field := r.Tree.Nodes[fieldID]
		if field.Kind == ast.KindRecordField && field.Left != ast.InvalidNode && r.validNode(field.Left) && r.Tree.Nodes[field.Left].Kind == ast.KindIdent {
			fields[string(r.source(field.Left))] = r.inferType(field.Right)
		}
	}

	return Type{Primitive: TypeTable, Structural: &StructuralType{Fields: fields}}
}

func (r *Resolver) binaryType(node ast.Node) Type {
	left := r.inferType(node.Left)
	right := r.inferType(node.Right)
	if left.Primitive != TypeUnknown || left.Structural != nil || right.Primitive != TypeUnknown || right.Structural != nil {
		return left.Union(right)
	}

	return Type{}
}

func (r *Resolver) memberType(node ast.Node) Type {
	if node.Right == ast.InvalidNode || !r.validNode(node.Right) {
		return Type{}
	}
	prop := string(r.source(node.Right))
	left := r.inferType(node.Left)
	if left.Structural != nil && left.Structural.Fields != nil {
		if t, ok := left.Structural.Fields[prop]; ok {
			return t
		}
	}
	recDef, recHash, _ := r.receiverContext(node.Left)
	if recDef != ast.InvalidNode {
		if assigned := r.assignedValue(recDef); assigned != ast.InvalidNode {
			_, recHash, _ = r.receiverContext(assigned)
		}
	}
	if r.Index != nil {
		entries := r.Index.LookupByHash(GlobalKey{ReceiverHash: recHash, PropHash: ast.HashBytes(r.source(node.Right))})
		if len(entries) > 0 {
			return entries[0].Type
		}
	}

	return Type{}
}

func (r *Resolver) callType(node ast.Node) Type {
	callee := node.Left
	if node.Kind == ast.KindMethodCall {
		callee = node.Right
	}
	calleeType := r.inferType(callee)
	if calleeType.Structural != nil && calleeType.Structural.Function != nil && len(calleeType.Structural.Function.Returns) > 0 {
		return calleeType.Structural.Function.Returns[0]
	}
	if callee != ast.InvalidNode && r.validNode(callee) && r.Tree.Nodes[callee].Kind == ast.KindIdent {
		defID := r.References[callee]
		if defID != ast.InvalidNode {
			return r.firstReturnFromDefinition(defID)
		}
	}

	return Type{}
}

func (r *Resolver) firstReturnFromDefinition(defID ast.NodeID) Type {
	if val := r.assignedValue(defID); val != ast.InvalidNode {
		fn := r.functionType(val)
		if fn.Structural != nil && fn.Structural.Function != nil && len(fn.Structural.Function.Returns) > 0 {
			return fn.Structural.Function.Returns[0]
		}
	}

	return Type{}
}

func (r *Resolver) assignDeclarationTypes(leftID, rightID ast.NodeID) {
	if leftID == ast.InvalidNode || rightID == ast.InvalidNode || !r.validNode(leftID) || !r.validNode(rightID) {
		return
	}
	left := r.Tree.Nodes[leftID]
	right := r.Tree.Nodes[rightID]
	for i := uint16(0); i < left.Count; i++ {
		if left.Extra+uint32(i) >= uint32(len(r.Tree.ExtraList)) {
			continue
		}
		lhs := r.Tree.ExtraList[left.Extra+uint32(i)]
		if right.Extra+uint32(i) >= uint32(len(r.Tree.ExtraList)) {
			continue
		}
		rhs := r.Tree.ExtraList[right.Extra+uint32(i)]
		r.setNodeType(lhs, r.inferType(rhs))
	}
}

func (r *Resolver) setNodeType(id ast.NodeID, typ Type) {
	if id == ast.InvalidNode || !r.validNode(id) {
		return
	}
	if int(id) < len(r.types) {
		r.types[id] = typ
	}
	r.mergeSemantic(id, SemanticData{Type: typ})
}

func (r *Resolver) publishGlobalsToIndex() {
	if r.Index == nil || r.ResourceURI == "" {
		return
	}
	for _, defID := range r.GlobalDefs {
		if !r.validNode(defID) {
			continue
		}
		name := string(r.source(defID))
		if name == "" {
			continue
		}
		isDep, depMsg := r.deprecatedTag(defID)
		r.Index.AddSymbol(r.ResourceURI, r.Scope, SymbolName(name), &SymbolEntry{
			Key:           GlobalKey{PropHash: ast.HashBytes(r.source(defID))},
			Type:          r.inferType(defID),
			URI:           string(r.ResourceURI),
			NodeID:        defID,
			IsRoot:        isRootLevel(r.Tree, defID),
			IsDeprecated:  isDep,
			DeprecatedMsg: depMsg,
		})
	}
	for _, fd := range r.FieldDefs {
		if !r.validNode(fd.NodeID) || len(fd.ReceiverName) == 0 {
			continue
		}
		prop := string(r.source(fd.NodeID))
		if prop == "" {
			continue
		}
		name := string(fd.ReceiverName) + "." + prop
		isDep, depMsg := r.deprecatedTag(fd.NodeID)
		r.Index.AddSymbol(r.ResourceURI, r.Scope, SymbolName(name), &SymbolEntry{
			Key:           GlobalKey{ReceiverHash: fd.ReceiverHash, PropHash: fd.PropHash},
			Type:          r.inferType(fd.NodeID),
			URI:           string(r.ResourceURI),
			NodeID:        fd.NodeID,
			Parent:        string(fd.ReceiverName),
			IsRoot:        isRootLevel(r.Tree, fd.NodeID),
			IsDeprecated:  isDep,
			DeprecatedMsg: depMsg,
		})
	}
}

func (r *Resolver) deprecatedTag(id ast.NodeID) (bool, string) {
	if r == nil || r.Tree == nil || !r.validNode(id) {
		return false, ""
	}

	node := r.Tree.Nodes[id]
	if node.Start > uint32(len(r.Tree.Source)) {
		return false, ""
	}

	limit := node.Start
	for i := len(r.Tree.Comments) - 1; i >= 0; i-- {
		comment := r.Tree.Comments[i]
		if comment.End > limit {
			continue
		}
		if comment.End > uint32(len(r.Tree.Source)) || comment.Start > comment.End {
			continue
		}
		if len(bytes.TrimSpace(r.Tree.Source[comment.End:limit])) != 0 {
			break
		}

		raw := r.Tree.Source[comment.Start:comment.End]
		if _, after, ok := bytes.Cut(raw, []byte("@deprecated")); ok {
			endIdx := bytes.IndexByte(after, '\n')
			if endIdx == -1 {
				endIdx = len(after)
			}
			return true, string(bytes.TrimSpace(cleanLuaCommentBytes(nil, after[:endIdx])))
		}
		limit = comment.Start
	}

	return false, ""
}

func (r *Resolver) lookupIndexSymbol(name SymbolName) *SymbolEntry {
	if r.Index == nil || name == "" {
		return nil
	}
	if r.ResourceURI != "" {
		if entry := r.Index.LookupByScope(r.ResourceURI, r.Scope, name); entry != nil {
			return entry
		}
	}
	entries := r.Index.LookupByHash(GlobalKey{PropHash: ast.HashBytes([]byte(name))})
	if len(entries) > 0 {
		return &entries[0]
	}

	return nil
}

func (r *Resolver) assignedValue(id ast.NodeID) ast.NodeID {
	if id == ast.InvalidNode || !r.validNode(id) {
		return ast.InvalidNode
	}
	if kind := r.Tree.Nodes[id].Kind; kind == ast.KindFunctionExpr || kind == ast.KindTableExpr || kind == ast.KindString || kind == ast.KindHashedString || kind == ast.KindNumber || kind == ast.KindTrue || kind == ast.KindFalse || kind == ast.KindNil {
		return id
	}
	curr := id
	for {
		parentID := r.Tree.Nodes[curr].Parent
		if parentID == ast.InvalidNode || !r.validNode(parentID) {
			return ast.InvalidNode
		}
		parent := r.Tree.Nodes[parentID]
		switch parent.Kind {
		case ast.KindLocalFunction, ast.KindFunctionStmt:
			if parent.Left == curr {
				return parent.Right
			}
			return ast.InvalidNode
		case ast.KindRecordField, ast.KindIndexField:
			if parent.Left == curr {
				return parent.Right
			}
			return ast.InvalidNode
		case ast.KindNameList:
			grandParentID := r.Tree.Nodes[parentID].Parent
			if grandParentID == ast.InvalidNode || !r.validNode(grandParentID) {
				return ast.InvalidNode
			}
			grandParent := r.Tree.Nodes[grandParentID]
			if grandParent.Kind != ast.KindLocalAssign || grandParent.Right == ast.InvalidNode || !r.validNode(grandParent.Right) {
				return ast.InvalidNode
			}
			idx := r.Tree.IndexOfExtra(parentID, curr)
			if idx == -1 {
				return ast.InvalidNode
			}
			rhs := r.Tree.Nodes[grandParent.Right]
			if uint16(idx) >= rhs.Count {
				return ast.InvalidNode
			}
			return r.Tree.ExtraList[rhs.Extra+uint32(idx)]
		case ast.KindExprList:
			grandParentID := r.Tree.Nodes[parentID].Parent
			if grandParentID == ast.InvalidNode || !r.validNode(grandParentID) {
				return ast.InvalidNode
			}
			grandParent := r.Tree.Nodes[grandParentID]
			if grandParent.Kind != ast.KindAssign || grandParent.Left == ast.InvalidNode || grandParent.Right == ast.InvalidNode || !r.validNode(grandParent.Left) || !r.validNode(grandParent.Right) {
				return ast.InvalidNode
			}
			idx := r.Tree.IndexOfExtra(parentID, curr)
			if idx == -1 {
				return ast.InvalidNode
			}
			rhs := r.Tree.Nodes[grandParent.Right]
			if uint16(idx) >= rhs.Count {
				return ast.InvalidNode
			}
			return r.Tree.ExtraList[rhs.Extra+uint32(idx)]
		default:
			curr = parentID
		}
	}
}

func (r *Resolver) receiverContext(recID ast.NodeID) (ast.NodeID, uint64, []byte) {
	if recID == ast.InvalidNode || !r.validNode(recID) {
		return ast.InvalidNode, 0, nil
	}
	curr := recID
	rootDef := ast.InvalidNode
	for curr != ast.InvalidNode && r.validNode(curr) {
		node := r.Tree.Nodes[curr]
		if node.Kind == ast.KindIdent {
			if int(curr) < len(r.References) {
				rootDef = r.References[curr]
			}
			break
		}
		if node.Kind != ast.KindMemberExpr {
			return ast.InvalidNode, 0, nil
		}
		curr = node.Left
	}
	recBytes := r.buildMemberName(recID, nil)

	return rootDef, ast.HashBytes(recBytes), recBytes
}

func (r *Resolver) buildMemberName(id ast.NodeID, buf []byte) []byte {
	if id == ast.InvalidNode || !r.validNode(id) {
		return buf
	}
	node := r.Tree.Nodes[id]
	switch node.Kind {
	case ast.KindIdent:
		buf = append(buf, r.source(id)...)
	case ast.KindMemberExpr, ast.KindMethodName:
		buf = r.buildMemberName(node.Left, buf)
		buf = append(buf, '.')
		buf = r.buildMemberName(node.Right, buf)
	}

	return buf
}

func (r *Resolver) tableReceiver(id ast.NodeID) (ast.NodeID, []byte) {
	if id == ast.InvalidNode || !r.validNode(id) {
		return ast.InvalidNode, nil
	}
	parentID := r.Tree.Nodes[id].Parent
	if parentID == ast.InvalidNode || !r.validNode(parentID) {
		return ast.InvalidNode, nil
	}
	parent := r.Tree.Nodes[parentID]
	if parent.Kind == ast.KindExprList {
		grandParentID := parent.Parent
		if grandParentID == ast.InvalidNode || !r.validNode(grandParentID) {
			return ast.InvalidNode, nil
		}
		grandParent := r.Tree.Nodes[grandParentID]
		if (grandParent.Kind != ast.KindAssign && grandParent.Kind != ast.KindLocalAssign) || grandParent.Right != parentID {
			return ast.InvalidNode, nil
		}
		idx := r.Tree.IndexOfExtra(parentID, id)
		if idx == -1 || grandParent.Left == ast.InvalidNode || !r.validNode(grandParent.Left) {
			return ast.InvalidNode, nil
		}
		lhs := r.Tree.Nodes[grandParent.Left]
		if uint16(idx) >= lhs.Count {
			return ast.InvalidNode, nil
		}
		leftID := r.Tree.ExtraList[lhs.Extra+uint32(idx)]
		if grandParent.Kind == ast.KindLocalAssign {
			return leftID, r.source(leftID)
		}
		if r.Tree.Nodes[leftID].Kind == ast.KindIdent {
			return r.References[leftID], r.source(leftID)
		}
		if r.Tree.Nodes[leftID].Kind == ast.KindMemberExpr {
			defID, _, recBytes := r.receiverContext(leftID)
			return defID, recBytes
		}
	}

	return ast.InvalidNode, nil
}

func (r *Resolver) walkChildren(id ast.NodeID, visit func(ast.NodeID) Type) {
	if id == ast.InvalidNode || !r.validNode(id) {
		return
	}
	node := r.Tree.Nodes[id]
	if node.Left != ast.InvalidNode {
		visit(node.Left)
	}
	if node.Right != ast.InvalidNode {
		visit(node.Right)
	}
	if node.Kind == ast.KindForIn && node.Extra != 0 {
		visit(ast.NodeID(node.Extra))
	}
	for i := uint16(0); i < node.Count; i++ {
		visit(r.Tree.ExtraList[node.Extra+uint32(i)])
	}
}

func (r *Resolver) mergeSemantic(id ast.NodeID, data SemanticData) {
	if r.Data == nil || id == ast.InvalidNode {
		return
	}
	current := r.Data.Get(NodeID(id))
	if current != nil {
		merged := *current
		if data.Type.Primitive != TypeUnknown || data.Type.Structural != nil {
			merged.Type = data.Type
		}
		if data.Scope != nil {
			merged.Scope = data.Scope
		}
		if data.Bindings != nil {
			merged.Bindings = data.Bindings
		}
		if data.LuaDoc != nil {
			merged.LuaDoc = data.LuaDoc
		}
		if data.FiveM != nil {
			merged.FiveM = data.FiveM
		}
		if data.Export != nil {
			merged.Export = data.Export
		}
		r.Data.Set(NodeID(id), &merged)
		return
	}
	r.Data.Set(NodeID(id), &data)
}

func (r *Resolver) validNode(id ast.NodeID) bool {
	return int(id) > 0 && int(id) < len(r.Tree.Nodes)
}

func (r *Resolver) source(id ast.NodeID) []byte {
	if r.validNode(id) {
		node := r.Tree.Nodes[id]
		if node.Start <= node.End && node.End <= uint32(len(r.Tree.Source)) {
			return r.Tree.Source[node.Start:node.End]
		}
	}

	return nil
}

func forEachExtra(tree *ast.Tree, listID ast.NodeID, visit func(ast.NodeID)) {
	if listID == ast.InvalidNode || int(listID) >= len(tree.Nodes) {
		return
	}
	node := tree.Nodes[listID]
	for i := uint16(0); i < node.Count; i++ {
		visit(tree.ExtraList[node.Extra+uint32(i)])
	}
}
