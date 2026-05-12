package lsp

func GotoExportTarget(name SymbolName, fromResource ResourceURI, index *GlobalIndex, desiredScope ...GlobalIndexScope) *SymbolEntry {
	if name == "" || fromResource == "" || index == nil {
		return nil
	}

	bridge := &ExportBridge{Index: index}
	if len(desiredScope) == 0 || desiredScope[0] == "" {
		for _, scope := range []GlobalIndexScope{GlobalIndexScopeShared, GlobalIndexScopeClient, GlobalIndexScopeServer} {
			if entry := bridge.LookupExport(name, fromResource, scope); entry != nil {
				return entry
			}
		}
		return nil
	}

	if entry := bridge.LookupExport(name, fromResource, desiredScope[0]); entry != nil {
		return entry
	}

	return nil
}

func GotoLocalTarget(name SymbolName, resource ResourceURI, index *GlobalIndex) *SymbolEntry {
	if name == "" || resource == "" || index == nil {
		return nil
	}

	for _, scope := range []GlobalIndexScope{GlobalIndexScopeClient, GlobalIndexScopeServer, GlobalIndexScopeShared} {
		if entry := index.LookupByScope(resource, scope, name); entry != nil {
			return entry
		}
	}

	return nil
}
