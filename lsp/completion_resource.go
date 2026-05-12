package lsp

import (
	"sort"
	"sync"
)

type CompletionResourceCache struct {
	mu    sync.RWMutex
	items map[ResourceURI][]CompletionItem
}

func NewCompletionResourceCache() *CompletionResourceCache {
	return &CompletionResourceCache{items: make(map[ResourceURI][]CompletionItem)}
}

func (cache *CompletionResourceCache) Build(scope *ResourceScope) []CompletionItem {
	if cache == nil || scope == nil {
		return nil
	}

	items := buildCompletionItems(scope)

	cache.mu.Lock()
	if cache.items == nil {
		cache.items = make(map[ResourceURI][]CompletionItem)
	}
	cache.items[scope.URI] = cloneCompletionItems(items)
	cache.mu.Unlock()

	return items
}

func (cache *CompletionResourceCache) Get(uri ResourceURI) []CompletionItem {
	if cache == nil {
		return nil
	}

	cache.mu.RLock()
	items := cloneCompletionItems(cache.items[uri])
	cache.mu.RUnlock()

	return items
}

func (cache *CompletionResourceCache) Invalidate(uri ResourceURI) {
	if cache == nil {
		return
	}

	cache.mu.Lock()
	delete(cache.items, uri)
	cache.mu.Unlock()
}

func (cache *CompletionResourceCache) ChainComplete(objType *Type, prefix string) []CompletionItem {
	if objType == nil || objType.Structural == nil || len(objType.Structural.Fields) == 0 {
		return nil
	}

	items := make([]CompletionItem, 0, len(objType.Structural.Fields))
	for name, fieldType := range objType.Structural.Fields {
		if prefix != "" && len(name) < len(prefix) {
			continue
		}
		if prefix != "" && name[:len(prefix)] != prefix {
			continue
		}

		entry := &SymbolEntry{Name: SymbolName(name), Type: fieldType}
		items = append(items, completionItemForEntry(name, "", entry))
	}

	if len(items) == 0 {
		return nil
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Label < items[j].Label
	})

	return items
}

func (cache *CompletionResourceCache) ChainCompleteFromEntry(entry *SymbolEntry, prefix string) []CompletionItem {
	if entry == nil {
		return nil
	}

	return cache.ChainComplete(&entry.Type, prefix)
}

func buildCompletionItems(scope *ResourceScope) []CompletionItem {
	if scope == nil {
		return nil
	}

	seen := make(map[string]struct{})
	items := make([]CompletionItem, 0, len(scope.Client)+len(scope.Server)+len(scope.Shared))
	appendTable := func(table SymbolTable, scopeName string) {
		if len(table) == 0 {
			return
		}

		names := make([]string, 0, len(table))
		for name := range table {
			names = append(names, string(name))
		}
		sort.Strings(names)

		for _, name := range names {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}

			entry := table[SymbolName(name)]
			items = append(items, completionItemForEntry(name, scopeName, entry))
		}
	}

	appendTable(scope.Client, string(GlobalIndexScopeClient))
	appendTable(scope.Server, string(GlobalIndexScopeServer))
	appendTable(scope.Shared, string(GlobalIndexScopeShared))

	return items
}

func completionItemForEntry(label, detail string, entry *SymbolEntry) CompletionItem {
	kind := VariableCompletion
	if entry != nil && entry.Type.Structural != nil && entry.Type.Structural.Function != nil {
		kind = FunctionCompletion
	}

	return CompletionItem{Label: label, Kind: kind, Detail: detail}
}

func cloneCompletionItems(items []CompletionItem) []CompletionItem {
	if len(items) == 0 {
		return nil
	}

	out := make([]CompletionItem, len(items))
	copy(out, items)

	return out
}
