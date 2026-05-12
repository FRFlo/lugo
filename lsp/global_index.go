package lsp

import (
	"iter"
	"slices"
	"strings"
	"sync"

	"github.com/coalaura/lugo/ast"
)

const DefaultGlobalIndexMaxMemory uint64 = 256 * 1024 * 1024

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
	Name          SymbolName
	Key           GlobalKey
	Type          Type
	LuaDoc        *LuaDocData
	FiveM         *FiveMData
	Export        *ExportData
	URI           string
	NodeID        ast.NodeID
	Parent        string
	IsRoot        bool
	IsDeprecated  bool
	DeprecatedMsg string
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

type GlobalIndex struct {
	mu          sync.RWMutex
	Resources   map[ResourceURI]*ResourceScope
	HashIndex   map[GlobalKey][]*SymbolEntry
	DepGraph    *DependencyGraph
	MaxMemory   uint64
	memoryUsage uint64
	clock       uint64
}

func NewGlobalIndex(maxMemory ...uint64) *GlobalIndex {
	limit := DefaultGlobalIndexMaxMemory
	if len(maxMemory) > 0 && maxMemory[0] > 0 {
		limit = maxMemory[0]
	}

	return &GlobalIndex{
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

func (idx *GlobalIndex) EnsureResource(uri ResourceURI) *ResourceScope {
	if idx == nil {
		return nil
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	return idx.ensureResourceLocked(uri)
}

func (idx *GlobalIndex) ensureResourceLocked(uri ResourceURI) *ResourceScope {
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
		idx.MaxMemory = DefaultGlobalIndexMaxMemory
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

func (idx *GlobalIndex) AddSymbol(resource ResourceURI, scope GlobalIndexScope, name SymbolName, entry *SymbolEntry) *SymbolEntry {
	if idx == nil || name == "" || entry == nil {
		return nil
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	res := idx.ensureResourceLocked(resource)
	previous := res.tableForScope(scope)[name]
	entry.Name = name
	if entry.URI == "" {
		entry.URI = string(resource)
	}
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

func (idx *GlobalIndex) LookupByHash(key GlobalKey) []SymbolEntry {
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

func (idx *GlobalIndex) AllSymbols() iter.Seq2[ResourceURI, *SymbolEntry] {
	return func(yield func(ResourceURI, *SymbolEntry) bool) {
		if idx == nil {
			return
		}

		type indexedSymbol struct {
			resource ResourceURI
			entry    *SymbolEntry
		}

		idx.mu.RLock()
		symbols := make([]indexedSymbol, 0, idx.symbolCountLocked())
		resources := sortedResourceKeys(idx.Resources)
		for _, uri := range resources {
			res := idx.Resources[uri]
			appendTableSymbols := func(table SymbolTable) {
				for _, name := range sortedSymbolNames(table) {
					if entry := table[name]; entry != nil {
						symbols = append(symbols, indexedSymbol{resource: uri, entry: entry})
					}
				}
			}
			appendTableSymbols(res.Client)
			appendTableSymbols(res.Server)
			appendTableSymbols(res.Shared)
		}
		idx.mu.RUnlock()

		for _, symbol := range symbols {
			if !yield(symbol.resource, symbol.entry) {
				return
			}
		}
	}
}

func (idx *GlobalIndex) SymbolsByHash(key GlobalKey) []*SymbolEntry {
	if idx == nil {
		return nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entries := idx.HashIndex[key]
	if len(entries) == 0 {
		return nil
	}

	out := make([]*SymbolEntry, 0, len(entries))
	for _, entry := range entries {
		if entry != nil {
			out = append(out, entry)
		}
	}

	return out
}

func (idx *GlobalIndex) VisibleSymbols(fromResource ResourceURI, scope GlobalIndexScope) []*SymbolEntry {
	if idx == nil {
		return nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	seen := make(map[*SymbolEntry]struct{})
	visible := make([]*SymbolEntry, 0)
	appendScope := func(res *ResourceScope, includeScoped bool) {
		if res == nil {
			return
		}
		if includeScoped {
			visible = appendUniqueEntries(visible, seen, res.tableForScopeRead(scope))
		}
		visible = appendUniqueEntries(visible, seen, res.Shared)
	}

	res := idx.Resources[fromResource]
	appendScope(res, true)
	if res != nil {
		for _, dep := range res.Dependencies {
			appendScope(idx.Resources[dep], true)
		}
	}

	return visible
}

func (idx *GlobalIndex) WorkspaceSymbols(query string, limit int) []*SymbolEntry {
	if idx == nil || limit == 0 {
		return nil
	}

	type scoredSymbol struct {
		entry *SymbolEntry
		score int
	}

	idx.mu.RLock()
	query = strings.ToLower(query)
	matches := make([]scoredSymbol, 0)
	for _, uri := range sortedResourceKeys(idx.Resources) {
		res := idx.Resources[uri]
		appendMatches := func(table SymbolTable) {
			for _, name := range sortedSymbolNames(table) {
				entry := table[name]
				if entry == nil {
					continue
				}
				if score, ok := workspaceSymbolScore(strings.ToLower(string(name)), query); ok {
					matches = append(matches, scoredSymbol{entry: entry, score: score})
				}
			}
		}
		appendMatches(res.Client)
		appendMatches(res.Server)
		appendMatches(res.Shared)
	}
	idx.mu.RUnlock()

	slices.SortStableFunc(matches, func(a, b scoredSymbol) int {
		if a.score != b.score {
			return a.score - b.score
		}
		return strings.Compare(string(a.entry.Name), string(b.entry.Name))
	})
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}

	out := make([]*SymbolEntry, 0, len(matches))
	for _, match := range matches {
		out = append(out, match.entry)
	}

	return out
}

func (idx *GlobalIndex) TypoSuggestions(name SymbolName, fromResource ResourceURI, scope GlobalIndexScope, max int) []SymbolName {
	if idx == nil || name == "" || max == 0 {
		return nil
	}

	type suggestion struct {
		name SymbolName
		dist int
	}

	needle := strings.ToLower(string(name))
	candidates := idx.VisibleSymbols(fromResource, scope)
	seen := make(map[SymbolName]struct{}, len(candidates))
	suggestions := make([]suggestion, 0, max)
	for _, entry := range candidates {
		if entry == nil || entry.Name == "" || entry.Name == name {
			continue
		}
		if _, ok := seen[entry.Name]; ok {
			continue
		}
		seen[entry.Name] = struct{}{}

		candidate := strings.ToLower(string(entry.Name))
		threshold := 3
		if len(needle) > 8 {
			threshold = 4
		}
		dist := levenshteinFast(needle, candidate, threshold)
		if dist > threshold {
			continue
		}
		suggestions = append(suggestions, suggestion{name: entry.Name, dist: dist})
	}

	slices.SortStableFunc(suggestions, func(a, b suggestion) int {
		if a.dist != b.dist {
			return a.dist - b.dist
		}
		return strings.Compare(string(a.name), string(b.name))
	})
	if max > 0 && len(suggestions) > max {
		suggestions = suggestions[:max]
	}

	out := make([]SymbolName, 0, len(suggestions))
	for _, suggestion := range suggestions {
		out = append(out, suggestion.name)
	}

	return out
}

func (idx *GlobalIndex) LookupByScope(resource ResourceURI, scope GlobalIndexScope, name SymbolName) *SymbolEntry {
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

func (idx *GlobalIndex) RegisterFiveMResource(res *FiveMResource, featureFiveM bool) *ResourceScope {
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

func (idx *GlobalIndex) TopologicalSort() ([]ResourceURI, []Diagnostic) {
	if idx == nil {
		return nil, nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.DepGraph.TopologicalSort()
}

func (idx *GlobalIndex) MemoryUsage() uint64 {
	if idx == nil {
		return 0
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.memoryUsage
}

func (idx *GlobalIndex) SetSource(uri ResourceURI, source []byte, tree *ast.Tree) {
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

func (idx *GlobalIndex) EvictSource(uri ResourceURI) bool {
	if idx == nil {
		return false
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	return idx.evictSourceLocked(uri)
}

func (idx *GlobalIndex) evictSourceLocked(uri ResourceURI) bool {
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

func (idx *GlobalIndex) evictLRULocked() {
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

func (idx *GlobalIndex) touchLocked(res *ResourceScope) {
	idx.clock++
	res.lastAccess = idx.clock
}

func (idx *GlobalIndex) syncResourceEdgesLocked() {
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

func (idx *GlobalIndex) registerFiveMScriptScopesLocked(scope *ResourceScope, res *FiveMResource) {
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

func (res *ResourceScope) tableForScopeRead(scope GlobalIndexScope) SymbolTable {
	if res == nil {
		return nil
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

func (idx *GlobalIndex) symbolCountLocked() int {
	count := 0
	for _, res := range idx.Resources {
		if res == nil {
			continue
		}
		count += len(res.Client) + len(res.Server) + len(res.Shared)
	}

	return count
}

func sortedResourceKeys(resources map[ResourceURI]*ResourceScope) []ResourceURI {
	keys := make([]ResourceURI, 0, len(resources))
	for uri := range resources {
		keys = append(keys, uri)
	}
	sortResourceURIs(keys)

	return keys
}

func sortedSymbolNames(table SymbolTable) []SymbolName {
	names := make([]SymbolName, 0, len(table))
	for name := range table {
		names = append(names, name)
	}
	slices.Sort(names)

	return names
}

func appendUniqueEntries(out []*SymbolEntry, seen map[*SymbolEntry]struct{}, table SymbolTable) []*SymbolEntry {
	for _, name := range sortedSymbolNames(table) {
		entry := table[name]
		if entry == nil {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}

	return out
}

func workspaceSymbolScore(name, query string) (int, bool) {
	if query == "" {
		return 0, true
	}
	if name == query {
		return 0, true
	}
	if strings.HasPrefix(name, query) {
		return 1, true
	}
	if strings.Contains(name, query) {
		return 2, true
	}
	if fuzzyMatch(name, query) {
		return 3, true
	}

	return 0, false
}

func fuzzyMatch(name, query string) bool {
	if query == "" {
		return true
	}
	idx := 0
	for i := 0; i < len(name) && idx < len(query); i++ {
		if name[i] == query[idx] {
			idx++
		}
	}

	return idx == len(query)
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
