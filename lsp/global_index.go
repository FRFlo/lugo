package lsp

import (
	"slices"
	"strings"
	"sync"

	"github.com/coalaura/lugo/ast"
)

const DefaultGlobalIndexV2MaxMemory uint64 = 256 * 1024 * 1024

type ResourceURI string
type SymbolName string
type GlobalIndexScope string

const (
	GlobalIndexScopeClient GlobalIndexScope = "client"
	GlobalIndexScopeServer GlobalIndexScope = "server"
	GlobalIndexScopeShared GlobalIndexScope = "shared"
)

type SymbolTable map[SymbolName]*SymbolEntry

type SymbolEntry struct {
	Name   SymbolName
	Key    GlobalKey
	Type   Type
	LuaDoc *LuaDocData
	FiveM  *FiveMData
	Export *ExportData
}

type ResourceScope struct {
	URI          ResourceURI
	Client       SymbolTable
	Server       SymbolTable
	Shared       SymbolTable
	Dependencies []ResourceURI
	Dependents   []ResourceURI
	ScriptScopes map[ResourceURI]GlobalIndexScope
	Source       []byte
	AST          *ast.Tree
	SourceBytes  uint64
	lastAccess   uint64
}

type GlobalIndexV2 struct {
	mu          sync.RWMutex
	Resources   map[ResourceURI]*ResourceScope
	HashIndex   map[GlobalKey][]*SymbolEntry
	DepGraph    *DependencyGraph
	MaxMemory   uint64
	memoryUsage uint64
	clock       uint64
}

func NewGlobalIndexV2(maxMemory ...uint64) *GlobalIndexV2 {
	limit := DefaultGlobalIndexV2MaxMemory
	if len(maxMemory) > 0 && maxMemory[0] > 0 {
		limit = maxMemory[0]
	}

	return &GlobalIndexV2{
		Resources: make(map[ResourceURI]*ResourceScope),
		HashIndex: make(map[GlobalKey][]*SymbolEntry),
		DepGraph:  NewDependencyGraph(),
		MaxMemory: limit,
	}
}

func NewResourceScope(uri ResourceURI) *ResourceScope {
	return &ResourceScope{
		URI:          uri,
		Client:       make(SymbolTable),
		Server:       make(SymbolTable),
		Shared:       make(SymbolTable),
		ScriptScopes: make(map[ResourceURI]GlobalIndexScope),
	}
}

func (idx *GlobalIndexV2) EnsureResource(uri ResourceURI) *ResourceScope {
	if idx == nil {
		return nil
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	return idx.ensureResourceLocked(uri)
}

func (idx *GlobalIndexV2) ensureResourceLocked(uri ResourceURI) *ResourceScope {
	if idx.Resources == nil {
		idx.Resources = make(map[ResourceURI]*ResourceScope)
	}
	if idx.HashIndex == nil {
		idx.HashIndex = make(map[GlobalKey][]*SymbolEntry)
	}
	if idx.DepGraph == nil {
		idx.DepGraph = NewDependencyGraph()
	}
	if idx.MaxMemory == 0 {
		idx.MaxMemory = DefaultGlobalIndexV2MaxMemory
	}

	res := idx.Resources[uri]
	if res == nil {
		res = NewResourceScope(uri)
		idx.Resources[uri] = res
	}
	idx.DepGraph.AddResource(uri)
	idx.touchLocked(res)

	return res
}

func (idx *GlobalIndexV2) AddSymbol(resource ResourceURI, scope GlobalIndexScope, name SymbolName, entry *SymbolEntry) *SymbolEntry {
	if idx == nil || name == "" || entry == nil {
		return nil
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	res := idx.ensureResourceLocked(resource)
	previous := res.tableForScope(scope)[name]
	entry.Name = name
	table := res.tableForScope(scope)
	if previous != nil && previous != entry && entry.Key != (GlobalKey{}) && previous.Key == entry.Key {
		idx.HashIndex[entry.Key] = removeSymbolEntry(idx.HashIndex[entry.Key], previous)
	}
	table[name] = entry
	if entry.Key != (GlobalKey{}) {
		idx.HashIndex[entry.Key] = append(idx.HashIndex[entry.Key], entry)
	}

	return entry
}

func (idx *GlobalIndexV2) LookupByHash(key GlobalKey) []SymbolEntry {
	if idx == nil {
		return nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entries := idx.HashIndex[key]
	if len(entries) == 0 {
		return nil
	}

	out := make([]SymbolEntry, 0, len(entries))
	for _, entry := range entries {
		if entry != nil {
			out = append(out, *entry)
		}
	}

	return out
}

func (idx *GlobalIndexV2) LookupByScope(resource ResourceURI, scope GlobalIndexScope, name SymbolName) *SymbolEntry {
	if idx == nil || name == "" {
		return nil
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	res := idx.Resources[resource]
	if res == nil {
		return nil
	}
	idx.touchLocked(res)

	return res.tableForScope(scope)[name]
}

func (idx *GlobalIndexV2) RegisterFiveMResource(res *FiveMResource, featureFiveM bool) *ResourceScope {
	if idx == nil || res == nil {
		return nil
	}

	uri := ResourceURI(res.RootURI)
	if uri == "" {
		uri = ResourceURI(normalizeFiveMResourceAlias(res.Name))
	}
	if uri == "" {
		return nil
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	scope := idx.ensureResourceLocked(uri)
	if !featureFiveM {
		return scope
	}

	deps := fiveMResourceDependencies(res)
	scope.Dependencies = cloneResourceURIs(deps)
	idx.DepGraph.SetDependencies(uri, deps)
	idx.syncResourceEdgesLocked()
	idx.registerFiveMScriptScopesLocked(scope, res)
	idx.registerFiveMRuntimeMetadataLocked(scope, res)

	return scope
}

func (idx *GlobalIndexV2) TopologicalSort() ([]ResourceURI, []Diagnostic) {
	if idx == nil {
		return nil, nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.DepGraph.TopologicalSort()
}

func (idx *GlobalIndexV2) MemoryUsage() uint64 {
	if idx == nil {
		return 0
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.memoryUsage
}

func (idx *GlobalIndexV2) SetSource(uri ResourceURI, source []byte, tree *ast.Tree) {
	if idx == nil {
		return
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	res := idx.ensureResourceLocked(uri)
	idx.memoryUsage -= res.SourceBytes
	res.Source = source
	res.AST = tree
	res.SourceBytes = estimateSourceBytes(source, tree)
	idx.memoryUsage += res.SourceBytes
	idx.touchLocked(res)
	idx.evictLRULocked()
}

func (idx *GlobalIndexV2) EvictSource(uri ResourceURI) bool {
	if idx == nil {
		return false
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	return idx.evictSourceLocked(uri)
}

func (idx *GlobalIndexV2) evictSourceLocked(uri ResourceURI) bool {
	res := idx.Resources[uri]
	if res == nil || res.SourceBytes == 0 {
		return false
	}

	idx.memoryUsage -= res.SourceBytes
	res.Source = nil
	res.AST = nil
	res.SourceBytes = 0

	return true
}

func (idx *GlobalIndexV2) evictLRULocked() {
	if idx.MaxMemory == 0 || idx.memoryUsage <= idx.MaxMemory {
		return
	}

	for idx.memoryUsage > idx.MaxMemory {
		var victim ResourceURI
		var oldest uint64
		for uri, res := range idx.Resources {
			if res.SourceBytes == 0 {
				continue
			}
			if victim == "" || res.lastAccess < oldest {
				victim = uri
				oldest = res.lastAccess
			}
		}
		if victim == "" || !idx.evictSourceLocked(victim) {
			return
		}
	}
}

func (idx *GlobalIndexV2) touchLocked(res *ResourceScope) {
	idx.clock++
	res.lastAccess = idx.clock
}

func (idx *GlobalIndexV2) syncResourceEdgesLocked() {
	for uri, res := range idx.Resources {
		res.Dependents = res.Dependents[:0]
		res.Dependencies = idx.DepGraph.DependencyList(uri)
	}
	for dependent, deps := range idx.DepGraph.Dependencies {
		for dep := range deps {
			if res := idx.Resources[dep]; res != nil {
				res.Dependents = appendUniqueResourceURI(res.Dependents, dependent)
			}
		}
	}
	for _, res := range idx.Resources {
		sortResourceURIs(res.Dependencies)
		sortResourceURIs(res.Dependents)
	}
}

func (idx *GlobalIndexV2) registerFiveMScriptScopesLocked(scope *ResourceScope, res *FiveMResource) {
	client := make(map[ResourceURI]struct{})
	server := make(map[ResourceURI]struct{})
	shared := make(map[ResourceURI]struct{})
	for _, script := range res.ClientGlobs {
		if script != "" {
			client[ResourceURI(script)] = struct{}{}
		}
	}
	for _, script := range res.ServerGlobs {
		if script != "" {
			server[ResourceURI(script)] = struct{}{}
		}
	}
	for _, script := range res.SharedGlobs {
		if script != "" {
			shared[ResourceURI(script)] = struct{}{}
		}
	}

	if res.Manifest != nil {
		for _, entry := range res.Manifest.Entries {
			if entry.LoaderInjected || entry.ReservedKey || strings.HasPrefix(entry.Value, "@") || entry.Value == "" {
				continue
			}
			script := ResourceURI(entry.Value)
			switch entry.EmittedName {
			case "client_script":
				client[script] = struct{}{}
			case "server_script":
				server[script] = struct{}{}
			case "shared_script", "file":
				shared[script] = struct{}{}
			}
		}
	}

	clear(scope.ScriptScopes)
	for script := range client {
		if _, ok := server[script]; ok {
			scope.ScriptScopes[script] = GlobalIndexScopeShared
			continue
		}
		scope.ScriptScopes[script] = GlobalIndexScopeClient
	}
	for script := range server {
		if _, ok := client[script]; ok {
			scope.ScriptScopes[script] = GlobalIndexScopeShared
			continue
		}
		scope.ScriptScopes[script] = GlobalIndexScopeServer
	}
	for script := range shared {
		scope.ScriptScopes[script] = GlobalIndexScopeShared
	}
}

func (res *ResourceScope) tableForScope(scope GlobalIndexScope) SymbolTable {
	if res.Client == nil {
		res.Client = make(SymbolTable)
	}
	if res.Server == nil {
		res.Server = make(SymbolTable)
	}
	if res.Shared == nil {
		res.Shared = make(SymbolTable)
	}

	switch scope {
	case GlobalIndexScopeClient:
		return res.Client
	case GlobalIndexScopeServer:
		return res.Server
	default:
		return res.Shared
	}
}

func removeSymbolEntry(entries []*SymbolEntry, stale *SymbolEntry) []*SymbolEntry {
	if len(entries) == 0 || stale == nil {
		return entries
	}
	for i, entry := range entries {
		if entry == stale {
			copy(entries[i:], entries[i+1:])
			entries[len(entries)-1] = nil
			return entries[:len(entries)-1]
		}
	}
	return entries
}

type DependencyGraph struct {
	Dependencies map[ResourceURI]map[ResourceURI]struct{}
	Dependents   map[ResourceURI]map[ResourceURI]struct{}
}

func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		Dependencies: make(map[ResourceURI]map[ResourceURI]struct{}),
		Dependents:   make(map[ResourceURI]map[ResourceURI]struct{}),
	}
}

func (graph *DependencyGraph) AddResource(uri ResourceURI) {
	if graph == nil || uri == "" {
		return
	}
	if graph.Dependencies == nil {
		graph.Dependencies = make(map[ResourceURI]map[ResourceURI]struct{})
	}
	if graph.Dependents == nil {
		graph.Dependents = make(map[ResourceURI]map[ResourceURI]struct{})
	}
	if graph.Dependencies[uri] == nil {
		graph.Dependencies[uri] = make(map[ResourceURI]struct{})
	}
	if graph.Dependents[uri] == nil {
		graph.Dependents[uri] = make(map[ResourceURI]struct{})
	}
}

func (graph *DependencyGraph) SetDependencies(uri ResourceURI, deps []ResourceURI) {
	if graph == nil || uri == "" {
		return
	}
	graph.AddResource(uri)
	for dep := range graph.Dependencies[uri] {
		delete(graph.Dependents[dep], uri)
	}
	clear(graph.Dependencies[uri])

	for _, dep := range deps {
		if dep == "" || dep == uri {
			continue
		}
		graph.AddResource(dep)
		graph.Dependencies[uri][dep] = struct{}{}
		graph.Dependents[dep][uri] = struct{}{}
	}
}

func (graph *DependencyGraph) DependencyList(uri ResourceURI) []ResourceURI {
	if graph == nil {
		return nil
	}
	deps := make([]ResourceURI, 0, len(graph.Dependencies[uri]))
	for dep := range graph.Dependencies[uri] {
		deps = append(deps, dep)
	}
	sortResourceURIs(deps)

	return deps
}

func (graph *DependencyGraph) TopologicalSort() ([]ResourceURI, []Diagnostic) {
	if graph == nil {
		return nil, nil
	}

	cycles := graph.detectCycles()
	remaining := make(map[ResourceURI]int, len(graph.Dependencies))
	for uri, deps := range graph.Dependencies {
		remaining[uri] = len(deps)
	}

	ready := make([]ResourceURI, 0, len(remaining))
	for uri, count := range remaining {
		if count == 0 {
			ready = append(ready, uri)
		}
	}
	sortResourceURIs(ready)

	ordered := make([]ResourceURI, 0, len(remaining))
	for len(ready) > 0 {
		uri := ready[0]
		ready = ready[1:]
		ordered = append(ordered, uri)

		dependents := make([]ResourceURI, 0, len(graph.Dependents[uri]))
		for dependent := range graph.Dependents[uri] {
			dependents = append(dependents, dependent)
		}
		sortResourceURIs(dependents)
		for _, dependent := range dependents {
			remaining[dependent]--
			if remaining[dependent] == 0 {
				ready = append(ready, dependent)
				sortResourceURIs(ready)
			}
		}
	}

	if len(ordered) < len(remaining) {
		seen := make(map[ResourceURI]struct{}, len(ordered))
		for _, uri := range ordered {
			seen[uri] = struct{}{}
		}
		leftovers := make([]ResourceURI, 0, len(remaining)-len(ordered))
		for uri := range remaining {
			if _, ok := seen[uri]; !ok {
				leftovers = append(leftovers, uri)
			}
		}
		sortResourceURIs(leftovers)
		ordered = append(ordered, leftovers...)
	}

	return ordered, cycleDiagnostics(cycles)
}

func (graph *DependencyGraph) detectCycles() [][]ResourceURI {
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)

	state := make(map[ResourceURI]int, len(graph.Dependencies))
	stack := make([]ResourceURI, 0, len(graph.Dependencies))
	var cycles [][]ResourceURI

	var visit func(ResourceURI)
	visit = func(uri ResourceURI) {
		state[uri] = visiting
		stack = append(stack, uri)

		deps := graph.DependencyList(uri)
		for _, dep := range deps {
			switch state[dep] {
			case unvisited:
				visit(dep)
			case visiting:
				cycle := []ResourceURI{dep}
				for i := len(stack) - 1; i >= 0; i-- {
					cycle = append(cycle, stack[i])
					if stack[i] == dep {
						break
					}
				}
				reverseResourceURIs(cycle)
				cycles = append(cycles, cycle)
			}
		}

		stack = stack[:len(stack)-1]
		state[uri] = visited
	}

	resources := make([]ResourceURI, 0, len(graph.Dependencies))
	for uri := range graph.Dependencies {
		resources = append(resources, uri)
	}
	sortResourceURIs(resources)
	for _, uri := range resources {
		if state[uri] == unvisited {
			visit(uri)
		}
	}

	return cycles
}

func fiveMResourceDependencies(res *FiveMResource) []ResourceURI {
	seen := make(map[ResourceURI]struct{})
	deps := make([]ResourceURI, 0, len(res.Dependencies))
	add := func(value string) {
		dep := ResourceURI(normalizeFiveMResourceAlias(value))
		if dep == "" {
			return
		}
		if _, ok := seen[dep]; ok {
			return
		}
		seen[dep] = struct{}{}
		deps = append(deps, dep)
	}

	if res.Manifest != nil {
		for _, entry := range res.Manifest.Entries {
			if entry.LoaderInjected || entry.ReservedKey {
				continue
			}
			if entry.EmittedName == "dependency" || entry.EmittedName == "dependencie" {
				add(entry.Value)
			}
		}
	} else {
		for _, dep := range res.Dependencies {
			add(dep)
		}
	}

	return deps
}

func cycleDiagnostics(cycles [][]ResourceURI) []Diagnostic {
	if len(cycles) == 0 {
		return nil
	}

	diagnostics := make([]Diagnostic, 0, len(cycles))
	for _, cycle := range cycles {
		parts := make([]string, 0, len(cycle))
		for _, uri := range cycle {
			parts = append(parts, string(uri))
		}
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityWarning,
			Code:     "fivem-circular-dependency",
			Message:  "Circular FiveM resource dependency detected; indexing will continue with the cycle broken: " + strings.Join(parts, " -> "),
		})
	}

	return diagnostics
}

func estimateSourceBytes(source []byte, tree *ast.Tree) uint64 {
	bytes := uint64(len(source))
	if tree == nil {
		return bytes
	}
	bytes += uint64(len(tree.Source))
	bytes += uint64(len(tree.Nodes)) * 64
	bytes += uint64(len(tree.Comments)) * 32
	bytes += uint64(len(tree.ExtraList)) * 8
	bytes += uint64(len(tree.LineOffsets)) * 4

	return bytes
}

func cloneResourceURIs(in []ResourceURI) []ResourceURI {
	out := append([]ResourceURI(nil), in...)
	sortResourceURIs(out)

	return out
}

func appendUniqueResourceURI(list []ResourceURI, uri ResourceURI) []ResourceURI {
	if slices.Contains(list, uri) {
		return list
	}

	return append(list, uri)
}

func sortResourceURIs(uris []ResourceURI) {
	slices.Sort(uris)
}

func reverseResourceURIs(uris []ResourceURI) {
	for i, j := 0, len(uris)-1; i < j; i, j = i+1, j-1 {
		uris[i], uris[j] = uris[j], uris[i]
	}
}
