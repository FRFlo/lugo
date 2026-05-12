package lsp

import "github.com/coalaura/lugo/ast"

const semanticDataArenaSize = 1024

// NodeID aliases AST node identifiers for semantic side-table keys.
type NodeID = ast.NodeID

// SemanticDataTable stores semantic annotations outside the packed AST node
// arena, keyed by NodeID so ast.Node remains pointer-free.
type SemanticDataTable struct {
	data        map[NodeID]*SemanticData
	arena       []SemanticData
	arenaPtr    int
	arenaChunks [][]SemanticData
}

// SemanticData holds optional semantic annotations for a single AST node.
type SemanticData struct {
	Type     Type
	Scope    *Scope
	Bindings []Binding
	LuaDoc   *LuaDocData
	FiveM    *FiveMData
	Export   *ExportData
}

type Scope struct {
	Parent  *Scope
	Symbols map[string]NodeID
}

type Binding struct {
	Name   string
	NodeID NodeID
	Type   Type
}

type LuaDocData struct {
	Description string
	Params      []LuaDocParam
	Returns     []LuaDocReturn
}

type FiveMData struct {
	Kind               string
	Scope              GlobalIndexScope
	Bundle             string
	Family             FiveMNativeFamily
	UseExperimentalOAL bool
}

type ExportData struct {
	Name         string
	Resource     ResourceURI
	ResourceName string
	Scope        GlobalIndexScope
	SourceURI    ResourceURI
	NodeID       NodeID
}

func NewSemanticDataTable() *SemanticDataTable {
	return &SemanticDataTable{data: make(map[NodeID]*SemanticData)}
}

func (t *SemanticDataTable) Get(nodeID NodeID) *SemanticData {
	if t == nil || t.data == nil {
		return nil
	}

	return t.data[nodeID]
}

func (t *SemanticDataTable) Set(nodeID NodeID, data *SemanticData) {
	if t.data == nil {
		t.data = make(map[NodeID]*SemanticData)
	}
	if data == nil {
		delete(t.data, nodeID)

		return
	}
	if existing := t.data[nodeID]; existing != nil {
		*existing = *data

		return
	}

	entry := t.allocateFromArena()
	*entry = *data
	t.data[nodeID] = entry
}

func (t *SemanticDataTable) Clear() {
	if t == nil {
		return
	}
	if t.data != nil {
		clear(t.data)
	}

	t.arena = nil
	t.arenaPtr = 0
	t.arenaChunks = nil
}

func (t *SemanticDataTable) allocateFromArena() *SemanticData {
	if len(t.arena) == 0 || t.arenaPtr >= len(t.arena) {
		t.arena = make([]SemanticData, semanticDataArenaSize)
		t.arenaChunks = append(t.arenaChunks, t.arena)
		t.arenaPtr = 0
	}

	entry := &t.arena[t.arenaPtr]
	*entry = SemanticData{}
	t.arenaPtr++

	return entry
}
