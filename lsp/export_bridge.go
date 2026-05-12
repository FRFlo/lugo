package lsp

import "slices"

type ExportBridge struct {
	Index *GlobalIndex
}

func (eb *ExportBridge) RegisterExports(resource ResourceURI, scope GlobalIndexScope, exports []ExportData) {
	for _, data := range exports {
		eb.RegisterExport(resource, scope, data)
	}
}

func (eb *ExportBridge) RegisterExport(resource ResourceURI, scope GlobalIndexScope, data ExportData) *SymbolEntry {
	if eb == nil || eb.Index == nil || data.Name == "" {
		return nil
	}

	data.Resource = resource
	data.Scope = scope

	entry := &SymbolEntry{Export: &data}
	return eb.Index.AddSymbol(resource, scope, SymbolName(data.Name), entry)
}

func (eb *ExportBridge) LookupExport(name SymbolName, fromResource ResourceURI, desiredScope GlobalIndexScope) *SymbolEntry {
	if eb == nil || eb.Index == nil || name == "" || fromResource == "" {
		return nil
	}

	for _, dep := range eb.lookupDependencies(fromResource) {
		for _, scope := range lookupScopesForExport(desiredScope) {
			if entry := eb.Index.LookupByScope(dep, scope, name); entry != nil && entry.Export != nil {
				return entry
			}
		}
	}

	return nil
}

func (eb *ExportBridge) lookupDependencies(fromResource ResourceURI) []ResourceURI {
	eb.Index.mu.RLock()
	defer eb.Index.mu.RUnlock()

	seen := make(map[ResourceURI]struct{})
	deps := make([]ResourceURI, 0, 4)
	add := func(uri ResourceURI) {
		if uri == "" || uri == fromResource {
			return
		}
		if _, ok := seen[uri]; ok {
			return
		}
		seen[uri] = struct{}{}
		deps = append(deps, uri)
	}

	if res := eb.Index.Resources[fromResource]; res != nil {
		for _, dep := range res.Dependencies {
			add(dep)
		}
	}
	for uri, res := range eb.Index.Resources {
		if res == nil {
			continue
		}
		if slices.Contains(res.Dependents, fromResource) {
			add(uri)
		}
	}

	return deps
}

func lookupScopesForExport(scope GlobalIndexScope) []GlobalIndexScope {
	switch scope {
	case GlobalIndexScopeClient:
		return []GlobalIndexScope{GlobalIndexScopeClient, GlobalIndexScopeShared}
	case GlobalIndexScopeServer:
		return []GlobalIndexScope{GlobalIndexScopeServer, GlobalIndexScopeShared}
	default:
		return []GlobalIndexScope{GlobalIndexScopeShared}
	}
}
