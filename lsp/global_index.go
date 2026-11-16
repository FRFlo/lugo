package lsp

import (
	"iter"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/FRFlo/lugo/ast"
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
	// Identity
	Name SymbolName
	Key  GlobalKey

	// Type Info
	Type   Type
	LuaDoc *LuaDocData

	// FiveM Data
	FiveM  *FiveMData
	Export *ExportData

	// Source Location
	URI     string
	NodeID  ast.NodeID
	Parent  string
	Parents []string

	// Booleans
	IsRoot        bool
	IsDeprecated  bool
	DeprecatedMsg string
}

func symbolParents(entry *SymbolEntry) []string {
	if entry == nil {
		return nil
	}
	if len(entry.Parents) > 0 {
		return entry.Parents
	}
	if entry.Parent != "" {
		return []string{entry.Parent}
	}
	return nil
}

type ResourceScope struct {
	// Identity
	URI      ResourceURI
	Identity ResourceURI

	// Symbol Tables
	Client     SymbolTable
	Server     SymbolTable
	Shared     SymbolTable
	Candidates map[GlobalIndexScope]map[SymbolName][]*SymbolEntry

	// Metadata
	Dependencies []ResourceURI
	Dependents   []ResourceURI
	ScriptScopes map[ResourceURI]GlobalIndexScope
	Source       []byte
	AST          *ast.Tree
	SourceBytes  uint64
	lastAccess   uint64
}

type GlobalIndex struct {
	mu                 sync.RWMutex
	Resources          map[ResourceURI]*ResourceScope
	HashIndex          map[GlobalKey][]*SymbolEntry
	DepGraph           *DependencyGraph
	resourceIdentities map[ResourceURI]map[ResourceURI]*ResourceScope
	MaxMemory          uint64
	memoryUsage        uint64
	clock              uint64
}

func NewGlobalIndex(maxMemory ...uint64) *GlobalIndex {
	limit := DefaultGlobalIndexMaxMemory
	if len(maxMemory) > 0 && maxMemory[0] > 0 {
		limit = maxMemory[0]
	}

	return &GlobalIndex{
		Resources:          make(map[ResourceURI]*ResourceScope),
		HashIndex:          make(map[GlobalKey][]*SymbolEntry),
		DepGraph:           NewDependencyGraph(),
		resourceIdentities: make(map[ResourceURI]map[ResourceURI]*ResourceScope),
		MaxMemory:          limit,
	}
}

func NewResourceScope(uri ResourceURI) *ResourceScope {
	return &ResourceScope{
		URI:    uri,
		Client: make(SymbolTable),
		Server: make(SymbolTable),
		Shared: make(SymbolTable),
		Candidates: map[GlobalIndexScope]map[SymbolName][]*SymbolEntry{
			GlobalIndexScopeClient: make(map[SymbolName][]*SymbolEntry),
			GlobalIndexScopeServer: make(map[SymbolName][]*SymbolEntry),
			GlobalIndexScopeShared: make(map[SymbolName][]*SymbolEntry),
		},
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
	if idx.resourceIdentities == nil {
		idx.resourceIdentities = make(map[ResourceURI]map[ResourceURI]*ResourceScope)
	}
	if idx.MaxMemory == 0 {
		idx.MaxMemory = DefaultGlobalIndexMaxMemory
	}

	res := idx.Resources[uri]
	if res == nil {
		res = NewResourceScope(uri)
		idx.Resources[uri] = res
	}
	idx.addResourceIdentityLocked(res, resourceIdentity(res))
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
	entry.Name = name
	if entry.URI == "" {
		entry.URI = string(resource)
	}
	for _, old := range append([]*SymbolEntry(nil), res.Candidates[scope][name]...) {
		if old != nil && old.URI == entry.URI && old.NodeID == entry.NodeID && old.Key == entry.Key && old.Parent == entry.Parent && (old.Export != nil) == (entry.Export != nil) && (old.Export == nil || old.Export.SourceURI == entry.Export.SourceURI) {
			idx.removeCandidateLocked(res, scope, name, old)
		}
	}
	idx.addCandidateLocked(res, scope, name, entry)

	return entry
}

func candidateLess(a, b *SymbolEntry) bool {
	if a.IsRoot != b.IsRoot {
		return a.IsRoot
	}
	if a.URI != b.URI {
		return a.URI < b.URI
	}
	if a.NodeID != b.NodeID {
		return a.NodeID < b.NodeID
	}
	if a.Key.ReceiverHash != b.Key.ReceiverHash {
		return a.Key.ReceiverHash < b.Key.ReceiverHash
	}
	if a.Key.PropHash != b.Key.PropHash {
		return a.Key.PropHash < b.Key.PropHash
	}
	if a.Parent != b.Parent {
		return a.Parent < b.Parent
	}
	if (a.Export != nil) != (b.Export != nil) {
		return a.Export != nil
	}
	if a.Export != nil && b.Export != nil {
		return a.Export.SourceURI < b.Export.SourceURI
	}
	return false
}

func (idx *GlobalIndex) addCandidateLocked(res *ResourceScope, scope GlobalIndexScope, name SymbolName, entry *SymbolEntry) {
	if res.Candidates == nil {
		res.Candidates = NewResourceScope(res.URI).Candidates
	}
	byName := res.Candidates[scope]
	if byName == nil {
		byName = make(map[SymbolName][]*SymbolEntry)
		res.Candidates[scope] = byName
	}
	list := byName[name]
	for i, old := range list {
		if old == entry || (old != nil && old.URI == entry.URI && old.NodeID == entry.NodeID && old.Key == entry.Key && old.Parent == entry.Parent && (old.Export != nil) == (entry.Export != nil) && (old.Export == nil || old.Export.SourceURI == entry.Export.SourceURI)) {
			list = append(list[:i], list[i+1:]...)
			break
		}
	}
	list = append(list, entry)
	slices.SortFunc(list, func(a, b *SymbolEntry) int {
		if candidateLess(a, b) {
			return -1
		}
		if candidateLess(b, a) {
			return 1
		}
		return 0
	})
	byName[name] = list
	res.tableForScope(scope)[name] = list[0]
	if entry.Key != (GlobalKey{}) {
		for _, old := range idx.HashIndex[entry.Key] {
			if old == entry {
				return
			}
		}
		idx.HashIndex[entry.Key] = append(idx.HashIndex[entry.Key], entry)
		slices.SortFunc(idx.HashIndex[entry.Key], func(a, b *SymbolEntry) int {
			if candidateLess(a, b) {
				return -1
			}
			if candidateLess(b, a) {
				return 1
			}
			return 0
		})
	}
}

func (idx *GlobalIndex) removeCandidateLocked(res *ResourceScope, scope GlobalIndexScope, name SymbolName, entry *SymbolEntry) {
	byName := res.Candidates[scope]
	list := byName[name]
	for i, old := range list {
		if old == entry {
			list = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(list) == 0 {
		delete(byName, name)
		delete(res.tableForScope(scope), name)
	} else {
		byName[name] = list
		res.tableForScope(scope)[name] = list[0]
	}
	if entry.Key != (GlobalKey{}) {
		entries := removeSymbolEntry(idx.HashIndex[entry.Key], entry)
		if len(entries) == 0 {
			delete(idx.HashIndex, entry.Key)
		} else {
			idx.HashIndex[entry.Key] = entries
		}
	}
}

func candidateEntries(res *ResourceScope, scope GlobalIndexScope) []*SymbolEntry {
	if res == nil {
		return nil
	}
	byName := candidateBuckets(res, scope)
	entries := make([]*SymbolEntry, 0)
	for _, name := range sortedCandidateNames(byName) {
		list := byName[name]
		for _, entry := range list {
			if entry != nil {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

func candidateBuckets(res *ResourceScope, scope GlobalIndexScope) map[SymbolName][]*SymbolEntry {
	buckets := make(map[SymbolName][]*SymbolEntry)
	if res == nil {
		return buckets
	}
	for name, entries := range res.Candidates[scope] {
		buckets[name] = append([]*SymbolEntry(nil), entries...)
	}
	// Keep direct writes through the public compatibility tables visible.
	for name, entry := range res.tableForScopeRead(scope) {
		if entry == nil {
			continue
		}
		found := false
		for _, existing := range buckets[name] {
			if existing == entry {
				found = true
				break
			}
		}
		if !found {
			buckets[name] = append(buckets[name], entry)
		}
	}
	for name := range buckets {
		slices.SortFunc(buckets[name], func(a, b *SymbolEntry) int {
			if candidateLess(a, b) {
				return -1
			}
			if candidateLess(b, a) {
				return 1
			}
			return 0
		})
	}
	return buckets
}

func sortedCandidateNames(m map[SymbolName][]*SymbolEntry) []SymbolName {
	names := make([]SymbolName, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
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
		// Do not call symbolCountLocked here: it used to build a complete
		// candidate snapshot solely to size this snapshot, doubling the work.
		var symbols []indexedSymbol
		resources := sortedResourceKeys(idx.Resources)
		for _, uri := range resources {
			res := idx.Resources[uri]
			for _, scope := range [...]GlobalIndexScope{GlobalIndexScopeClient, GlobalIndexScopeServer, GlobalIndexScopeShared} {
				for _, entry := range candidateEntries(res, scope) {
					symbols = append(symbols, indexedSymbol{resource: uri, entry: entry})
				}
			}
		}
		idx.mu.RUnlock()

		for _, symbol := range symbols {
			if !yield(symbol.resource, symbol.entry) {
				return
			}
		}
	}
}

// SymbolsForReceivers returns symbols whose receiver hash is requested, keeping
// the same resource, scope, and name ordering as AllSymbols. It snapshots only
// matching entries while holding the read lock so callers can inspect them
// after the lock is released.
func (idx *GlobalIndex) SymbolsForReceivers(receivers map[uint64]bool) []*SymbolEntry {
	if idx == nil || len(receivers) == 0 {
		return nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var symbols []*SymbolEntry
	for _, uri := range sortedResourceKeys(idx.Resources) {
		res := idx.Resources[uri]
		if res == nil {
			continue
		}
		for _, scope := range [...]GlobalIndexScope{GlobalIndexScopeClient, GlobalIndexScopeServer, GlobalIndexScopeShared} {
			nameSet := make(map[SymbolName][]*SymbolEntry, len(res.Candidates[scope]))
			for name := range res.Candidates[scope] {
				nameSet[name] = nil
			}
			for name := range res.tableForScopeRead(scope) {
				nameSet[name] = nil
			}
			for _, name := range sortedCandidateNames(nameSet) {
				entries := res.Candidates[scope][name]
				matches := false
				for _, entry := range entries {
					if entry != nil && receivers[entry.Key.ReceiverHash] {
						matches = true
						break
					}
				}
				representative := res.tableForScopeRead(scope)[name]
				if representative != nil && receivers[representative.Key.ReceiverHash] {
					present := false
					for _, entry := range entries {
						if entry == representative {
							present = true
							break
						}
					}
					matches = matches || !present
				}
				if !matches {
					continue
				}
				// Dynamic SymbolEntry fields can invalidate the stored candidate
				// order, so rebuild and sort only buckets that can contribute.
				entries = append([]*SymbolEntry(nil), entries...)
				if representative != nil {
					present := false
					for _, entry := range entries {
						if entry == representative {
							present = true
							break
						}
					}
					if !present {
						entries = append(entries, representative)
					}
				}
				slices.SortFunc(entries, func(a, b *SymbolEntry) int {
					if candidateLess(a, b) {
						return -1
					}
					if candidateLess(b, a) {
						return 1
					}
					return 0
				})
				for _, entry := range entries {
					if entry != nil && receivers[entry.Key.ReceiverHash] {
						symbols = append(symbols, entry)
					}
				}
			}
		}
	}
	return symbols
}

// FieldKeysForReceivers returns indexed field keys for the requested receivers.
// The returned slice is detached from the index and ordered deterministically.
func (idx *GlobalIndex) FieldKeysForReceivers(receivers map[uint64]bool) []GlobalKey {
	if idx == nil || len(receivers) == 0 {
		return nil
	}
	idx.mu.RLock()
	var keys []GlobalKey
	for key := range idx.HashIndex {
		if key.PropHash != 0 && receivers[key.ReceiverHash] {
			keys = append(keys, key)
		}
	}
	idx.mu.RUnlock()
	slices.SortFunc(keys, func(a, b GlobalKey) int {
		if a.ReceiverHash < b.ReceiverHash {
			return -1
		}
		if a.ReceiverHash > b.ReceiverHash {
			return 1
		}
		if a.PropHash < b.PropHash {
			return -1
		}
		if a.PropHash > b.PropHash {
			return 1
		}
		return 0
	})
	return keys
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
			for _, entry := range candidateEntries(res, scope) {
				if _, ok := seen[entry]; !ok {
					seen[entry] = struct{}{}
					visible = append(visible, entry)
				}
			}
		}
		for _, entry := range candidateEntries(res, GlobalIndexScopeShared) {
			if _, ok := seen[entry]; !ok {
				seen[entry] = struct{}{}
				visible = append(visible, entry)
			}
		}
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

// VisibleSymbolsByKey returns the live visible entries matching key in the
// same order as VisibleSymbols. It scans the current tables on every call so
// direct edits to public tables or SymbolEntry fields cannot leave a stale
// lookup cache behind.
func (idx *GlobalIndex) VisibleSymbolsByKey(fromResource ResourceURI, scope GlobalIndexScope, key GlobalKey) []*SymbolEntry {
	if idx == nil {
		return nil
	}

	type candidate struct {
		entry *SymbolEntry
		order int
		name  SymbolName
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var candidates []candidate
	order := 0
	res := idx.Resources[fromResource]
	appendScopeEntries := func(res *ResourceScope, scope GlobalIndexScope) {
		for name, entries := range res.Candidates[scope] {
			for _, entry := range entries {
				if entry != nil && entry.Key == key {
					candidates = append(candidates, candidate{entry: entry, order: order, name: name})
				}
			}
		}
		// Public compatibility tables can be mutated directly. Add those live
		// entries as well; pointer deduplication below removes representatives
		// that also appear in the candidate buckets.
		for name, entry := range res.tableForScopeRead(scope) {
			if entry != nil && entry.Key == key {
				candidates = append(candidates, candidate{entry: entry, order: order, name: name})
			}
		}
		order++
	}
	appendScope := func(res *ResourceScope, includeScoped bool) {
		if res == nil {
			return
		}
		if includeScoped {
			appendScopeEntries(res, scope)
		}
		if scope != GlobalIndexScopeShared {
			appendScopeEntries(res, GlobalIndexScopeShared)
		}
	}
	appendScope(res, true)
	if res != nil {
		for _, dep := range res.Dependencies {
			appendScope(idx.Resources[dep], true)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	slices.SortStableFunc(candidates, func(a, b candidate) int {
		if a.order != b.order {
			return a.order - b.order
		}
		if nameOrder := strings.Compare(string(a.name), string(b.name)); nameOrder != 0 {
			return nameOrder
		}
		if candidateLess(a.entry, b.entry) {
			return -1
		}
		if candidateLess(b.entry, a.entry) {
			return 1
		}
		return 0
	})
	visible := make([]*SymbolEntry, 0, len(candidates))
	seen := make(map[*SymbolEntry]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, ok := seen[candidate.entry]; ok {
			continue
		}
		seen[candidate.entry] = struct{}{}
		visible = append(visible, candidate.entry)
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
		order int
		key   SymbolName
	}

	idx.mu.RLock()
	query = strings.ToLower(query)
	matches := make([]scoredSymbol, 0)
	if limit > 0 {
		matches = make([]scoredSymbol, 0, min(limit, 100))
	}
	order := 0
	less := func(a, b scoredSymbol) bool {
		if a.score != b.score {
			return a.score < b.score
		}
		if names := strings.Compare(string(a.entry.Name), string(b.entry.Name)); names != 0 {
			return names < 0
		}
		if a.order != b.order {
			return a.order < b.order
		}
		return a.key < b.key
	}
	keepCandidate := func(candidate scoredSymbol) {
		if len(matches) < limit {
			matches = append(matches, candidate)
			child := len(matches) - 1
			for child > 0 {
				parent := (child - 1) / 2
				if !less(matches[parent], matches[child]) {
					break
				}
				matches[parent], matches[child] = matches[child], matches[parent]
				child = parent
			}
			return
		}
		if !less(candidate, matches[0]) {
			return
		}
		matches[0] = candidate
		parent := 0
		for {
			left := parent*2 + 1
			if left >= len(matches) {
				return
			}
			worst := left
			right := left + 1
			if right < len(matches) && less(matches[left], matches[right]) {
				worst = right
			}
			if !less(matches[parent], matches[worst]) {
				return
			}
			matches[parent], matches[worst] = matches[worst], matches[parent]
			parent = worst
		}
	}
	for _, uri := range sortedResourceKeys(idx.Resources) {
		res := idx.Resources[uri]
		appendMatches := func(scope GlobalIndexScope) {
			if res == nil {
				return
			}
			// Candidate names are already the keys we need to score. Avoid
			// copying every bucket before knowing whether the query matches it.
			nameSet := make(map[SymbolName][]*SymbolEntry, len(res.Candidates[scope]))
			for name := range res.Candidates[scope] {
				nameSet[name] = nil
			}
			for name := range res.tableForScopeRead(scope) {
				nameSet[name] = nil
			}
			names := sortedCandidateNames(nameSet)
			if limit > 0 {
				// Candidate ordering is determined by score and name. For identical
				// names, keep traversal order (resource, then scope) as the stable
				// sort below did.
				for _, name := range names {
					if score, ok := workspaceSymbolScore(strings.ToLower(string(name)), query); ok {
						entries := append([]*SymbolEntry(nil), res.Candidates[scope][name]...)
						if entry := res.tableForScopeRead(scope)[name]; entry != nil {
							found := false
							for _, existing := range entries {
								if existing == entry {
									found = true
									break
								}
							}
							if !found {
								entries = append(entries, entry)
							}
						}
						slices.SortFunc(entries, func(a, b *SymbolEntry) int {
							if candidateLess(a, b) {
								return -1
							}
							if candidateLess(b, a) {
								return 1
							}
							return 0
						})
						for _, entry := range entries {
							if entry != nil {
								keepCandidate(scoredSymbol{entry: entry, score: score, order: order, key: name})
							}
						}
					}
				}
				// Public compatibility tables may contain names outside the
				// candidate index. Score those representatives individually.
				return
			}
			for _, name := range names {
				if score, ok := workspaceSymbolScore(strings.ToLower(string(name)), query); ok {
					entries := append([]*SymbolEntry(nil), res.Candidates[scope][name]...)
					if entry := res.tableForScopeRead(scope)[name]; entry != nil {
						found := false
						for _, existing := range entries {
							if existing == entry {
								found = true
								break
							}
						}
						if !found {
							entries = append(entries, entry)
						}
					}
					slices.SortFunc(entries, func(a, b *SymbolEntry) int {
						if candidateLess(a, b) {
							return -1
						}
						if candidateLess(b, a) {
							return 1
						}
						return 0
					})
					for _, entry := range entries {
						if entry != nil {
							matches = append(matches, scoredSymbol{entry: entry, score: score})
						}
					}
				}
			}
		}
		appendMatches(GlobalIndexScopeClient)
		order++
		appendMatches(GlobalIndexScopeServer)
		order++
		appendMatches(GlobalIndexScopeShared)
		order++
	}
	idx.mu.RUnlock()

	slices.SortStableFunc(matches, func(a, b scoredSymbol) int {
		if less(a, b) {
			return -1
		}
		if less(b, a) {
			return 1
		}
		return 0
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
	needleASCII := true
	for i := 0; i < len(needle); i++ {
		if needle[i] >= utf8.RuneSelf {
			needleASCII = false
			break
		}
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	seen := make(map[SymbolName]struct{})
	suggestions := make([]suggestion, 0, max)
	threshold := 3
	if len(needle) > 8 {
		threshold = 4
	}
	asciiLower := func(b byte) byte {
		if b >= 'A' && b <= 'Z' {
			return b + ('a' - 'A')
		}
		return b
	}
	consider := func(entry *SymbolEntry) {
		if entry == nil || entry.Name == "" || entry.Name == name {
			return
		}
		candidateName := entry.Name
		isASCII := needleASCII
		if isASCII {
			for i := 0; i < len(candidateName); i++ {
				if candidateName[i] >= utf8.RuneSelf {
					isASCII = false
					break
				}
			}
		}
		var dist int
		if isASCII {
			lengthDiff := len(needle) - len(candidateName)
			if lengthDiff < 0 {
				lengthDiff = -lengthDiff
			}
			maxLen := len(needle)
			if len(candidateName) > maxLen {
				maxLen = len(candidateName)
			}
			if lengthDiff > threshold || maxLen > 63 {
				return
			}
			var lowered [63]byte
			for i := range candidateName {
				lowered[i] = asciiLower(candidateName[i])
			}
			dist = levenshteinFast(needle, string(lowered[:len(candidateName)]), threshold)
		} else {
			candidate := strings.ToLower(string(candidateName))
			// Lowercasing Unicode can change byte length, so only reject after it.
			lengthDiff := len(needle) - len(candidate)
			if lengthDiff < 0 {
				lengthDiff = -lengthDiff
			}
			if lengthDiff > threshold {
				return
			}
			dist = levenshteinFast(needle, candidate, threshold)
		}
		if dist > threshold {
			return
		}
		if _, ok := seen[entry.Name]; ok {
			return
		}
		seen[entry.Name] = struct{}{}
		suggestions = append(suggestions, suggestion{name: entry.Name, dist: dist})
	}
	appendScope := func(res *ResourceScope, includeScoped bool) {
		if res == nil {
			return
		}
		visit := func(candidateScope GlobalIndexScope) {
			for _, entries := range res.Candidates[candidateScope] {
				for _, entry := range entries {
					consider(entry)
				}
			}
			// Compatibility tables are public and may contain live entries that
			// are absent from Candidates.
			for _, entry := range res.tableForScopeRead(candidateScope) {
				consider(entry)
			}
		}
		if includeScoped {
			visit(scope)
		}
		visit(GlobalIndexScopeShared)
	}
	res := idx.Resources[fromResource]
	appendScope(res, true)
	if res != nil {
		for _, dep := range res.Dependencies {
			appendScope(idx.Resources[dep], true)
		}
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

	if candidates := res.Candidates[scope][name]; len(candidates) > 0 {
		return candidates[0]
	}
	return res.tableForScope(scope)[name]
}

// LookupCandidatesByScope returns every indexed definition for a name in a
// resource scope, in deterministic priority order.
func (idx *GlobalIndex) LookupCandidatesByScope(resource ResourceURI, scope GlobalIndexScope, name SymbolName) []*SymbolEntry {
	if idx == nil || name == "" {
		return nil
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	res := idx.Resources[resource]
	if res == nil {
		return nil
	}
	entries := append([]*SymbolEntry(nil), res.Candidates[scope][name]...)
	slices.SortFunc(entries, func(a, b *SymbolEntry) int {
		if candidateLess(a, b) {
			return -1
		}
		if candidateLess(b, a) {
			return 1
		}
		return 0
	})
	if len(entries) == 0 {
		if entry := res.tableForScopeRead(scope)[name]; entry != nil {
			entries = append(entries, entry)
		}
	}
	return entries
}

// LookupLuaDocClassField finds a field declared on a LuaDoc class or one of
// its parents. A field declared on the requested class takes precedence over
// inherited fields. Parent walks are bounded by a visited-name set so invalid
// cyclic @class declarations cannot loop forever.
func (idx *GlobalIndex) LookupLuaDocClassField(resource ResourceURI, scope GlobalIndexScope, className, fieldName string) *SymbolEntry {
	if idx == nil || className == "" || fieldName == "" {
		return nil
	}
	idx.mu.RLock()
	res := idx.Resources[resource]
	if res != nil {
		// Most class lookups stay within one resource. Keep this common path
		// allocation-free; cross-resource and unusually deep walks use the
		// context-aware path below.
		var queue [64]string
		var visited [64]string
		head, tail, visitedCount := 0, 1, 0
		queue[0] = className
		fallback := false
		for head < tail {
			current := queue[head]
			head++
			alreadyVisited := false
			for i := 0; i < visitedCount; i++ {
				if visited[i] == current {
					alreadyVisited = true
					break
				}
			}
			if alreadyVisited {
				continue
			}
			if visitedCount == len(visited) {
				fallback = true
				break
			}
			visited[visitedCount] = current
			visitedCount++

			class := res.tableForScopeRead(scope)[SymbolName(current)]
			if class == nil {
				class = res.Shared[SymbolName(current)]
			}
			if class == nil {
				fallback = true
				break
			}
			if len(current)+1+len(fieldName) > 256 {
				fallback = true
				break
			}
			if field := lookupLocalLuaDocClassField(res, scope, current, fieldName); field != nil {
				idx.mu.RUnlock()
				return field
			}

			parents := class.Parents
			if len(parents) == 0 && class.Parent != "" {
				if tail == len(queue) {
					fallback = true
					break
				}
				queue[tail] = class.Parent
				tail++
				continue
			}
			for _, parent := range parents {
				if tail == len(queue) {
					fallback = true
					break
				}
				queue[tail] = parent
				tail++
			}
			if fallback {
				break
			}
		}
		if !fallback {
			idx.mu.RUnlock()
			return nil
		}
	}
	idx.mu.RUnlock()
	entry, _ := idx.LookupLuaDocClassFieldWithPath(resource, scope, className, fieldName)
	return entry
}

func lookupLocalLuaDocClassField(res *ResourceScope, scope GlobalIndexScope, className, fieldName string) *SymbolEntry {
	if res == nil {
		return nil
	}
	var keyBytes [256]byte
	keyLen := len(className) + 1 + len(fieldName)
	if keyLen > len(keyBytes) {
		return nil
	}
	classLen := copy(keyBytes[:], className)
	keyBytes[classLen] = '.'
	copy(keyBytes[classLen+1:keyLen], fieldName)
	key := SymbolName(string(keyBytes[:keyLen]))
	if field := res.tableForScopeRead(scope)[key]; field != nil {
		return field
	}
	if field := res.Shared[key]; field != nil {
		return field
	}
	keyBytes[classLen] = ':'
	key = SymbolName(string(keyBytes[:keyLen]))
	if field := res.tableForScopeRead(scope)[key]; field != nil {
		return field
	}
	return res.Shared[key]
}

// LuaDocClassHierarchyEntry identifies a class and the resource that owns it.
// Resource context matters because a class may inherit from a type visible to
// its declaring resource, even when that type is not directly visible to the
// original consumer.
type LuaDocClassHierarchyEntry struct {
	Name          string
	Entry         *SymbolEntry
	Resource      ResourceURI
	Depth         int
	ParentIndex   int
	FirstParent   int
	ParentIndices []int
}

type LuaDocClassFieldLookup struct {
	Entry      *SymbolEntry
	Path       []LuaDocClassHierarchyEntry
	Hierarchy  []LuaDocClassHierarchyEntry
	ClassIndex int
}

// LookupLuaDocClassHierarchy follows parents using the visibility context of
// each class declaration. It only crosses the current resource's direct
// dependencies and keeps the requested execution scope throughout the walk.
func (idx *GlobalIndex) LookupLuaDocClassHierarchy(resource ResourceURI, scope GlobalIndexScope, className string) []LuaDocClassHierarchyEntry {
	if idx == nil || className == "" {
		return nil
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	type pendingClass struct {
		name        string
		resource    ResourceURI
		depth       int
		parentIndex int
	}
	lookup := func(from ResourceURI, name SymbolName) (*SymbolEntry, ResourceURI) {
		res := idx.Resources[from]
		if res == nil {
			return nil, ""
		}
		if entry := res.tableForScopeRead(scope)[name]; entry != nil {
			return entry, from
		}
		if entry := res.Shared[name]; entry != nil {
			return entry, from
		}
		for _, dependency := range res.Dependencies {
			dep := idx.Resources[dependency]
			if dep == nil {
				continue
			}
			if entry := dep.tableForScopeRead(scope)[name]; entry != nil {
				return entry, dependency
			}
			if entry := dep.Shared[name]; entry != nil {
				return entry, dependency
			}
		}
		// Non-FiveM workspace files are indexed under their own document URI
		// and do not have resource dependency edges. Let their class graph find
		// declarations anywhere in the workspace; LSP callers validate every
		// cross-document edge against canSeeSymbol before exposing a member.
		for _, candidate := range sortedResourceKeys(idx.Resources) {
			if candidate == from {
				continue
			}
			dep := idx.Resources[candidate]
			if dep == nil {
				continue
			}
			if entry := dep.tableForScopeRead(scope)[name]; entry != nil {
				return entry, candidate
			}
			if entry := dep.Shared[name]; entry != nil {
				return entry, candidate
			}
		}
		return nil, ""
	}

	queue := []pendingClass{{name: className, resource: resource, parentIndex: -1}}
	type hierarchyKey struct {
		name     string
		resource ResourceURI
	}
	visited := make(map[hierarchyKey]int)
	out := make([]LuaDocClassHierarchyEntry, 0, 4)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		entry, owner := lookup(current.resource, SymbolName(current.name))
		if entry == nil {
			continue
		}
		key := hierarchyKey{name: current.name, resource: owner}
		if existingIndex, seen := visited[key]; seen {
			if current.parentIndex >= 0 {
				addLuaDocClassParent(&out[current.parentIndex], existingIndex)
			}
			continue
		}
		classIndex := len(out)
		out = append(out, LuaDocClassHierarchyEntry{Name: current.name, Entry: entry, Resource: owner, Depth: current.depth, ParentIndex: current.parentIndex, FirstParent: -1})
		visited[key] = classIndex
		if current.parentIndex >= 0 {
			addLuaDocClassParent(&out[current.parentIndex], classIndex)
		}
		if len(entry.Parents) > 0 {
			for _, parent := range entry.Parents {
				queue = append(queue, pendingClass{name: parent, resource: owner, depth: current.depth + 1, parentIndex: classIndex})
			}
		} else if entry.Parent != "" {
			queue = append(queue, pendingClass{name: entry.Parent, resource: owner, depth: current.depth + 1, parentIndex: classIndex})
		}
	}
	return out
}

func addLuaDocClassParent(class *LuaDocClassHierarchyEntry, parentIndex int) {
	if class == nil || parentIndex < 0 {
		return
	}
	if class.FirstParent < 0 {
		class.FirstParent = parentIndex
		return
	}
	if class.FirstParent == parentIndex || slices.Contains(class.ParentIndices, parentIndex) {
		return
	}
	class.ParentIndices = append(class.ParentIndices, parentIndex)
}

func luaDocClassPath(hierarchy []LuaDocClassHierarchyEntry, index int) []LuaDocClassHierarchyEntry {
	if index < 0 || index >= len(hierarchy) {
		return nil
	}
	path := make([]LuaDocClassHierarchyEntry, 0, hierarchy[index].Depth+1)
	for index >= 0 && index < len(hierarchy) {
		class := hierarchy[index]
		path = append(path, class)
		index = class.ParentIndex
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path
}

// LookupLuaDocClassFieldWithPath returns the first field in the inherited
// class hierarchy, along with the class declarations traversed to reach it.
// The path lets LSP callers validate each cross-resource inheritance edge
// without making the parent's other symbols visible to the original resource.
func (idx *GlobalIndex) LookupLuaDocClassFieldWithPath(resource ResourceURI, scope GlobalIndexScope, className, fieldName string) (*SymbolEntry, []LuaDocClassHierarchyEntry) {
	candidates := idx.LookupLuaDocClassFieldCandidatesWithPath(resource, scope, className, fieldName)
	if len(candidates) == 0 {
		return nil, idx.LookupLuaDocClassHierarchy(resource, scope, className)
	}
	return candidates[0].Entry, luaDocClassPath(candidates[0].Hierarchy, candidates[0].ClassIndex)
}

// LookupLuaDocClassFieldCandidatesWithPath returns every declaration path for
// the requested field. The same parent class may be reached through distinct
// parents, and callers must validate each path independently.
func (idx *GlobalIndex) LookupLuaDocClassFieldCandidatesWithPath(resource ResourceURI, scope GlobalIndexScope, className, fieldName string) []LuaDocClassFieldLookup {
	if idx == nil || className == "" || fieldName == "" {
		return nil
	}
	path := idx.LookupLuaDocClassHierarchy(resource, scope, className)
	return idx.LookupLuaDocClassFieldCandidatesInHierarchy(scope, fieldName, path)
}

func (idx *GlobalIndex) LookupLuaDocClassFieldCandidatesInHierarchy(scope GlobalIndexScope, fieldName string, path []LuaDocClassHierarchyEntry) []LuaDocClassFieldLookup {
	if idx == nil || fieldName == "" || len(path) == 0 {
		return nil
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var candidates []LuaDocClassFieldLookup
	for i, class := range path {
		if field := lookupLuaDocClassMember(idx, class.Resource, scope, class.Name, fieldName); field != nil {
			candidates = append(candidates, LuaDocClassFieldLookup{Entry: field, Hierarchy: path, ClassIndex: i})
		}
	}
	return candidates
}

func lookupLuaDocClassMember(idx *GlobalIndex, resource ResourceURI, scope GlobalIndexScope, className, fieldName string) *SymbolEntry {
	if entry, _ := idx.lookupLuaDocSymbol(resource, scope, SymbolName(className+"."+fieldName)); entry != nil {
		return entry
	}
	entry, _ := idx.lookupLuaDocSymbol(resource, scope, SymbolName(className+":"+fieldName))
	return entry
}

// LookupLuaDocClassFieldsWithPath returns indexed fields declared along the
// visible parent chain, with the declarations needed to validate each path.
func (idx *GlobalIndex) LookupLuaDocClassFieldsWithPath(resource ResourceURI, scope GlobalIndexScope, className string) []LuaDocClassFieldLookup {
	path := idx.LookupLuaDocClassHierarchy(resource, scope, className)
	return idx.LookupLuaDocClassFieldsInHierarchy(scope, path)
}

func (idx *GlobalIndex) LookupLuaDocClassFieldsInHierarchy(scope GlobalIndexScope, path []LuaDocClassHierarchyEntry) []LuaDocClassFieldLookup {
	if idx == nil || len(path) == 0 {
		return nil
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	result := make([]LuaDocClassFieldLookup, 0)
	for i, class := range path {
		dotPrefix := class.Name + "."
		methodPrefix := class.Name + ":"
		appendFields := func(res *ResourceScope) {
			if res == nil {
				return
			}
			appendTable := func(table SymbolTable) {
				for name, entry := range table {
					nameString := string(name)
					if (!strings.HasPrefix(nameString, dotPrefix) && !strings.HasPrefix(nameString, methodPrefix)) || entry == nil {
						continue
					}
					result = append(result, LuaDocClassFieldLookup{Entry: entry, Hierarchy: path, ClassIndex: i})
				}
			}
			appendTable(res.tableForScopeRead(scope))
			appendTable(res.Shared)
		}
		res := idx.Resources[class.Resource]
		appendFields(res)
		if res != nil {
			for _, dependency := range res.Dependencies {
				appendFields(idx.Resources[dependency])
			}
		}
	}
	return result
}

func (idx *GlobalIndex) lookupLuaDocSymbol(resource ResourceURI, scope GlobalIndexScope, name SymbolName) (*SymbolEntry, ResourceURI) {
	// Caller must hold idx.mu.RLock or idx.mu.Lock.
	res := idx.Resources[resource]
	if res == nil {
		return nil, ""
	}
	if entry := res.tableForScopeRead(scope)[name]; entry != nil {
		return entry, resource
	}
	if entry := res.Shared[name]; entry != nil {
		return entry, resource
	}
	for _, dependency := range res.Dependencies {
		dep := idx.Resources[dependency]
		if dep == nil {
			continue
		}
		if entry := dep.tableForScopeRead(scope)[name]; entry != nil {
			return entry, dependency
		}
		if entry := dep.Shared[name]; entry != nil {
			return entry, dependency
		}
	}
	return nil, ""
}

func (idx *GlobalIndex) RegisterFiveMResource(res *FiveMResource) *ResourceScope {
	if idx == nil || res == nil {
		return nil
	}

	// Dependency edges use the stable resource identity (the manifest name),
	// rather than a workspace-specific root URI. This keeps graph diagnostics
	// consistent across machines and allows aliases to be resolved canonically.
	rootURI := ResourceURI(res.RootURI)
	identity := ResourceURI(normalizeFiveMResourceAlias(res.Name))
	if identity == "" {
		identity = rootURI
	}
	if rootURI == "" {
		rootURI = identity
	}
	if identity == "" {
		return nil
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	scope := idx.ensureResourceLocked(rootURI)
	previousIdentity := resourceIdentity(scope)
	if previousIdentity != identity {
		idx.removeResourceIdentityLocked(scope, previousIdentity)
		scope.Identity = identity
		idx.addResourceIdentityLocked(scope, identity)
	}

	// Update only graph vertices affected by this resource. Rebuilding every
	// scope here made registering N resources quadratic while holding idx.mu.
	previousDeps := idx.DepGraph.DependencyList(identity)
	deps := fiveMResourceDependencies(res)
	idx.DepGraph.SetDependencies(identity, deps)
	idx.refreshResourceEdgesLocked(previousIdentity)
	idx.refreshResourceEdgesLocked(identity)
	for _, dep := range previousDeps {
		idx.refreshResourceEdgesLocked(dep)
	}
	for _, dep := range deps {
		idx.refreshResourceEdgesLocked(dep)
	}
	idx.registerFiveMScriptScopesLocked(scope, res)

	return scope
}

// UnregisterFiveMResource removes a manifest's dependency topology while
// preserving the scope itself for any remaining document-local symbols.
func (idx *GlobalIndex) UnregisterFiveMResource(root ResourceURI) {
	if idx == nil || root == "" {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()

	scope := idx.Resources[root]
	if scope == nil {
		return
	}
	identity := resourceIdentity(scope)
	idx.removeResourceIdentityLocked(scope, identity)
	if idx.DepGraph != nil && len(idx.resourceIdentities[identity]) == 0 {
		for dependency := range idx.DepGraph.Dependencies[identity] {
			delete(idx.DepGraph.Dependents[dependency], identity)
		}
		for dependent := range idx.DepGraph.Dependents[identity] {
			delete(idx.DepGraph.Dependencies[dependent], identity)
		}
		delete(idx.DepGraph.Dependencies, identity)
		delete(idx.DepGraph.Dependents, identity)
	}

	scope.Identity = root
	scope.Dependencies = nil
	scope.Dependents = nil
	clear(scope.ScriptScopes)
	idx.addResourceIdentityLocked(scope, root)
	if idx.DepGraph != nil {
		idx.DepGraph.AddResource(root)
	}
	idx.refreshResourceEdgesLocked(identity)
	idx.refreshResourceEdgesLocked(root)
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

// PruneResource releases a document's source and removes its empty index scope.
// Resource scopes that still contain symbols or participate in the dependency
// graph are retained: FiveM resources can outlive any one document.
func (idx *GlobalIndex) PruneResource(uri ResourceURI) bool {
	if idx == nil || uri == "" {
		return false
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	res := idx.Resources[uri]
	if res == nil {
		return false
	}
	idx.evictSourceLocked(uri)
	if idx.symbolCountForResourceLocked(res) != 0 {
		return false
	}
	if idx.DepGraph != nil {
		if len(idx.DepGraph.Dependencies[uri]) != 0 || len(idx.DepGraph.Dependents[uri]) != 0 {
			return false
		}
		delete(idx.DepGraph.Dependencies, uri)
		delete(idx.DepGraph.Dependents, uri)
	}
	idx.removeResourceIdentityLocked(res, resourceIdentity(res))
	delete(idx.Resources, uri)
	idx.syncResourceEdgesLocked()
	return true
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

func resourceIdentity(res *ResourceScope) ResourceURI {
	if res == nil || res.Identity == "" {
		if res == nil {
			return ""
		}
		return res.URI
	}
	return res.Identity
}

func (idx *GlobalIndex) addResourceIdentityLocked(res *ResourceScope, identity ResourceURI) {
	if res == nil || identity == "" {
		return
	}
	if idx.resourceIdentities == nil {
		idx.resourceIdentities = make(map[ResourceURI]map[ResourceURI]*ResourceScope)
	}
	scopes := idx.resourceIdentities[identity]
	if scopes == nil {
		scopes = make(map[ResourceURI]*ResourceScope)
		idx.resourceIdentities[identity] = scopes
	}
	scopes[res.URI] = res
}

func (idx *GlobalIndex) removeResourceIdentityLocked(res *ResourceScope, identity ResourceURI) {
	if res == nil || identity == "" {
		return
	}
	scopes := idx.resourceIdentities[identity]
	delete(scopes, res.URI)
	if len(scopes) == 0 {
		delete(idx.resourceIdentities, identity)
	}
}

func (idx *GlobalIndex) refreshResourceEdgesLocked(identity ResourceURI) {
	if identity == "" {
		return
	}
	for _, scope := range idx.resourceIdentities[identity] {
		scope.Dependencies = idx.DepGraph.DependencyList(identity)
		scope.Dependents = idx.DepGraph.DependentList(identity)
	}
}

func (idx *GlobalIndex) syncResourceEdgesLocked() {
	// This full reconciliation is retained for callers that construct resource
	// scopes directly. RegisterFiveMResource uses the incremental path above.
	for _, res := range idx.Resources {
		idx.addResourceIdentityLocked(res, resourceIdentity(res))
	}
	for identity := range idx.resourceIdentities {
		idx.refreshResourceEdgesLocked(identity)
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
		count += idx.symbolCountForResourceLocked(res)
	}

	return count
}

func (idx *GlobalIndex) symbolCountForResourceLocked(res *ResourceScope) int {
	count := 0
	for _, scope := range [...]GlobalIndexScope{GlobalIndexScopeClient, GlobalIndexScopeServer, GlobalIndexScopeShared} {
		count += len(candidateEntries(res, scope))
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

func (graph *DependencyGraph) DependentList(uri ResourceURI) []ResourceURI {
	if graph == nil {
		return nil
	}
	dependents := make([]ResourceURI, 0, len(graph.Dependents[uri]))
	for dependent := range graph.Dependents[uri] {
		dependents = append(dependents, dependent)
	}
	sortResourceURIs(dependents)

	return dependents
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
