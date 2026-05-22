package lsp

import (
	"errors"
	"strings"
	"sync"

	"github.com/coalaura/lugo/ast"
)

const fiveMIndexKindNative = "native"

type FiveMNativeCatalog struct {
	mu      sync.Mutex
	loader  func(name string) ([]byte, error)
	bundles map[string]map[string]fiveMNativeCatalogEntry
}

type fiveMNativeCatalogEntry struct {
	Name       string
	Bundle     string
	LuaDoc     LuaDocData
	Type       Type
	ParamNames []string
}

func NewFiveMNativeCatalog(loader func(name string) ([]byte, error)) *FiveMNativeCatalog {
	return &FiveMNativeCatalog{loader: loader, bundles: make(map[string]map[string]fiveMNativeCatalogEntry)}
}

func (catalog *FiveMNativeCatalog) Bundle(name string) (map[string]fiveMNativeCatalogEntry, error) {
	if catalog == nil || name == "" {
		return nil, errors.New("empty FiveM native catalog")
	}
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	if entries, ok := catalog.bundles[name]; ok {
		return entries, nil
	}
	if catalog.loader == nil {
		catalog.loader = readEmbeddedFiveMNativeBundle
	}
	b, err := catalog.loader(name)
	if err != nil {
		return nil, err
	}
	entries := parseFiveMNativeBundleCatalog(name, string(b))
	catalog.bundles[name] = entries
	return entries, nil
}

func readEmbeddedFiveMNativeBundle(name string) ([]byte, error) {
	if name == "" {
		return nil, errors.New("empty FiveM native bundle name")
	}
	if b, err := loadRuntimeFiveMNativeBundle(name); err == nil {
		return b, nil
	}
	return stdlibFS.ReadFile("stdlib/fivem/" + name)
}

func parseFiveMNativeBundleCatalog(bundleName, source string) map[string]fiveMNativeCatalogEntry {
	entries := make(map[string]fiveMNativeCatalogEntry)
	var comments []string
	for _, rawLine := range strings.Split(source, "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "---") {
			comments = append(comments, line)
			continue
		}
		name, params, ok := parseFiveMNativeFunctionLine(line)
		if !ok {
			if line != "" && !strings.HasPrefix(line, "---@") {
				comments = comments[:0]
			}
			continue
		}
		cleaned := cleanLuaCommentBytes(nil, []byte(strings.Join(comments, "\n")))
		luadoc := parseLuaDoc(cleaned, false)
		entry := fiveMNativeCatalogEntry{Name: name, Bundle: bundleName, LuaDoc: luaDocDataFromLuaDoc(luadoc), ParamNames: params}
		entry.Type = structuralFunctionTypeFromLuaDoc(entry.ParamNames, luadoc.Params, luadoc.Returns)
		entries[name] = entry
		comments = comments[:0]
	}
	return entries
}

func parseFiveMNativeFunctionLine(line string) (string, []string, bool) {
	if !strings.HasPrefix(line, "function ") {
		return "", nil, false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "function "))
	open := strings.IndexByte(rest, '(')
	close := strings.LastIndexByte(rest, ')')
	if open <= 0 || close < open {
		return "", nil, false
	}
	name := strings.TrimSpace(rest[:open])
	if idx := strings.LastIndexAny(name, ".:"); idx >= 0 {
		name = name[idx+1:]
	}
	if name == "" {
		return "", nil, false
	}
	var params []string
	for _, part := range strings.Split(rest[open+1:close], ",") {
		param := strings.TrimSpace(part)
		if param != "" {
			params = append(params, param)
		}
	}
	return name, params, true
}

func luaDocDataFromLuaDoc(luadoc LuaDoc) LuaDocData {
	return LuaDocData{Description: luadoc.Description, Params: append([]LuaDocParam(nil), luadoc.Params...), Returns: append([]LuaDocReturn(nil), luadoc.Returns...)}
}

func structuralFunctionTypeFromLuaDoc(paramNames []string, params []LuaDocParam, returns []LuaDocReturn) Type {
	fn := &FunctionType{Params: make([]Type, 0, max(len(paramNames), len(params))), Returns: make([]Type, 0, len(returns))}
	paramCount := max(len(paramNames), len(params))
	for i := 0; i < paramCount; i++ {
		typeName := "any"
		if i < len(params) && params[i].Type != "" {
			typeName = params[i].Type
		}
		fn.Params = append(fn.Params, structuralTypeFromLuaTypeName(typeName))
	}
	for _, ret := range returns {
		if strings.TrimSpace(ret.Type) == "" || strings.TrimSpace(ret.Type) == "void" {
			continue
		}
		fn.Returns = append(fn.Returns, structuralTypeFromLuaTypeName(ret.Type))
	}
	return Type{Primitive: TypeFunction, Structural: &StructuralType{Function: fn}}
}

func structuralTypeFromLuaTypeName(typeName string) Type {
	parsed := ParseTypeString(strings.TrimSpace(typeName))
	primitive := parsed.Basics
	if primitive == TypeUnknown && strings.TrimSpace(typeName) != "" {
		primitive = TypeAny
	}
	return Type{Primitive: primitive}
}

func (idx *GlobalIndex) registerFiveMRuntimeMetadataLocked(scope *ResourceScope, res *FiveMResource) {
	if idx == nil || scope == nil || res == nil {
		return
	}
	idx.clearFiveMGeneratedSymbolsLocked(scope)
	idx.registerFiveMManifestExportsLocked(scope, res)
	idx.registerFiveMNativeCatalogLocked(scope, res, NewFiveMNativeCatalog(nil))
	idx.rebuildHashIndexLocked()
}

func (idx *GlobalIndex) clearFiveMGeneratedSymbolsLocked(scope *ResourceScope) {
	clearGenerated := func(table SymbolTable) {
		for name, entry := range table {
			if entry == nil {
				continue
			}
			if entry.FiveM != nil && entry.FiveM.Kind == fiveMIndexKindNative {
				delete(table, name)
				continue
			}
			if entry.Export != nil && entry.Export.SourceURI == "" {
				delete(table, name)
			}
		}
	}
	clearGenerated(scope.Client)
	clearGenerated(scope.Server)
	clearGenerated(scope.Shared)
}

func (idx *GlobalIndex) registerFiveMNativeCatalogLocked(scope *ResourceScope, res *FiveMResource, catalog *FiveMNativeCatalog) {
	clientSelection := res.NativeSelection(FiveMExecutionProfile{Kind: FiveMProfileClient})
	serverSelection := res.NativeSelection(FiveMExecutionProfile{Kind: FiveMProfileServer})
	clientEntries := loadFiveMNativeCatalogEntries(catalog, clientSelection.Build)
	serverEntries := loadFiveMNativeCatalogEntries(catalog, serverSelection.Build)
	shared := make(map[string]struct{})
	for name := range clientEntries {
		if _, ok := serverEntries[name]; ok {
			shared[name] = struct{}{}
		}
	}
	for name, native := range clientEntries {
		targetScope := GlobalIndexScopeClient
		if _, ok := shared[name]; ok {
			targetScope = GlobalIndexScopeShared
		}
		idx.addFiveMNativeSymbolLocked(scope, targetScope, native, clientSelection)
	}
	for name, native := range serverEntries {
		if _, ok := shared[name]; ok {
			continue
		}
		idx.addFiveMNativeSymbolLocked(scope, GlobalIndexScopeServer, native, serverSelection)
	}
}

func loadFiveMNativeCatalogEntries(catalog *FiveMNativeCatalog, bundle string) map[string]fiveMNativeCatalogEntry {
	if bundle == "" || catalog == nil {
		return nil
	}
	entries, err := catalog.Bundle(bundle)
	if err != nil {
		return nil
	}
	return entries
}

func (idx *GlobalIndex) addFiveMNativeSymbolLocked(scope *ResourceScope, targetScope GlobalIndexScope, native fiveMNativeCatalogEntry, selection FiveMNativeSelection) {
	if native.Name == "" {
		return
	}
	name := SymbolName(native.Name)
	scope.tableForScope(targetScope)[name] = &SymbolEntry{Name: name, Key: GlobalKey{ReceiverHash: 0, PropHash: ast.HashBytes([]byte(native.Name))}, Type: native.Type, LuaDoc: &LuaDocData{Description: native.LuaDoc.Description, Params: append([]LuaDocParam(nil), native.LuaDoc.Params...), Returns: append([]LuaDocReturn(nil), native.LuaDoc.Returns...)}, FiveM: &FiveMData{Kind: fiveMIndexKindNative, Scope: targetScope, Bundle: native.Bundle, Family: selection.Family, UseExperimentalOAL: selection.UseExperimentalOAL}}
}

func (idx *GlobalIndex) registerFiveMManifestExportsLocked(scope *ResourceScope, res *FiveMResource) {
	client := make(map[string]struct{}, len(res.ClientExports))
	for _, name := range res.ClientExports {
		if strings.TrimSpace(name) != "" {
			client[name] = struct{}{}
		}
	}
	server := make(map[string]struct{}, len(res.ServerExports))
	for _, name := range res.ServerExports {
		if strings.TrimSpace(name) != "" {
			server[name] = struct{}{}
		}
	}
	for name := range client {
		targetScope := GlobalIndexScopeClient
		if _, ok := server[name]; ok {
			targetScope = GlobalIndexScopeShared
		}
		idx.addFiveMExportSymbolLocked(scope, targetScope, res, name, Type{})
	}
	for name := range server {
		if _, ok := client[name]; ok {
			continue
		}
		idx.addFiveMExportSymbolLocked(scope, GlobalIndexScopeServer, res, name, Type{})
	}
}

func (idx *GlobalIndex) addFiveMExportSymbolLocked(scope *ResourceScope, targetScope GlobalIndexScope, res *FiveMResource, name string, typ Type) {
	if name == "" {
		return
	}
	if typ.Primitive == TypeUnknown && typ.Structural == nil {
		typ = Type{Primitive: TypeFunction, Structural: &StructuralType{Function: &FunctionType{Variadic: true, Returns: []Type{{Primitive: TypeAny}}}}}
	}
	symName := SymbolName(name)
	scope.tableForScope(targetScope)[symName] = &SymbolEntry{Name: symName, Key: GlobalKey{ReceiverHash: ast.HashBytes([]byte("exports")), PropHash: ast.HashBytes([]byte(name))}, Type: typ, Export: &ExportData{Name: name, Resource: scope.URI, ResourceName: res.Name, Scope: targetScope}}
}

func (idx *GlobalIndex) rebuildHashIndexLocked() {
	if idx.HashIndex == nil {
		idx.HashIndex = make(map[GlobalKey][]*SymbolEntry)
	} else {
		clear(idx.HashIndex)
	}
	for _, resource := range idx.Resources {
		if resource == nil {
			continue
		}
		for _, table := range []SymbolTable{resource.Client, resource.Server, resource.Shared} {
			for _, entry := range table {
				if entry != nil && entry.Key != (GlobalKey{}) {
					idx.HashIndex[entry.Key] = append(idx.HashIndex[entry.Key], entry)
				}
			}
		}
	}
}

func (idx *GlobalIndex) LookupFiveMNative(resource ResourceURI, scope GlobalIndexScope, name SymbolName) *SymbolEntry {
	return idx.lookupVisibleFiveMSymbol(resource, scope, name, func(entry *SymbolEntry) bool {
		return entry != nil && entry.FiveM != nil && entry.FiveM.Kind == fiveMIndexKindNative
	})
}

func (idx *GlobalIndex) LookupFiveMExport(consumer, provider ResourceURI, scope GlobalIndexScope, name SymbolName) *SymbolEntry {
	if idx == nil || provider == "" || name == "" {
		return nil
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	providerScope := idx.findFiveMResourceScopeLocked(provider)
	if providerScope == nil {
		return nil
	}
	if consumer != "" && consumer != providerScope.URI && !idx.fiveMConsumerDependsOnProviderLocked(consumer, providerScope) {
		return nil
	}
	return lookupVisibleFiveMSymbolInScope(providerScope, scope, name, func(entry *SymbolEntry) bool { return entry != nil && entry.Export != nil })
}

func (idx *GlobalIndex) lookupVisibleFiveMSymbol(resource ResourceURI, scope GlobalIndexScope, name SymbolName, match func(*SymbolEntry) bool) *SymbolEntry {
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
	return lookupVisibleFiveMSymbolInScope(res, scope, name, match)
}

func lookupVisibleFiveMSymbolInScope(res *ResourceScope, scope GlobalIndexScope, name SymbolName, match func(*SymbolEntry) bool) *SymbolEntry {
	for _, visibleScope := range fiveMVisibleGlobalIndexScopes(scope) {
		entry := res.tableForScope(visibleScope)[name]
		if match(entry) {
			return entry
		}
	}
	return nil
}

func fiveMVisibleGlobalIndexScopes(scope GlobalIndexScope) []GlobalIndexScope {
	switch scope {
	case GlobalIndexScopeClient:
		return []GlobalIndexScope{GlobalIndexScopeClient, GlobalIndexScopeShared}
	case GlobalIndexScopeServer:
		return []GlobalIndexScope{GlobalIndexScopeServer, GlobalIndexScopeShared}
	default:
		return []GlobalIndexScope{GlobalIndexScopeShared}
	}
}

func (idx *GlobalIndex) findFiveMResourceScopeLocked(resource ResourceURI) *ResourceScope {
	if res := idx.Resources[resource]; res != nil {
		return res
	}
	alias := normalizeFiveMResourceAlias(string(resource))
	for _, res := range idx.Resources {
		if res == nil {
			continue
		}
		for _, table := range []SymbolTable{res.Client, res.Server, res.Shared} {
			for _, entry := range table {
				if entry != nil && entry.Export != nil && normalizeFiveMResourceAlias(entry.Export.ResourceName) == alias {
					return res
				}
			}
		}
	}
	return nil
}

func (idx *GlobalIndex) fiveMConsumerDependsOnProviderLocked(consumer ResourceURI, provider *ResourceScope) bool {
	if provider == nil {
		return false
	}
	deps := idx.DepGraph.DependencyList(consumer)
	providerAliases := map[ResourceURI]struct{}{provider.URI: {}}
	for _, table := range []SymbolTable{provider.Client, provider.Server, provider.Shared} {
		for _, entry := range table {
			if entry != nil && entry.Export != nil && entry.Export.ResourceName != "" {
				providerAliases[ResourceURI(normalizeFiveMResourceAlias(entry.Export.ResourceName))] = struct{}{}
			}
		}
	}
	for _, dep := range deps {
		if _, ok := providerAliases[dep]; ok {
			return true
		}
	}
	return false
}

func (s *Server) syncFiveMDocumentExports(doc *Document) {
	if s == nil || s.GlobalIndex == nil || doc == nil || len(doc.FiveMLuaExports) == 0 {
		return
	}
	profile := s.getDocumentFiveMProfile(doc)
	if profile.ResourceRoot == "" {
		return
	}
	res := s.resolveFiveMResourceByRoot(profile.ResourceRoot)
	if res == nil {
		return
	}
	scope := globalIndexScopeFromFiveMEnv(profile.Env())
	s.GlobalIndex.mu.Lock()
	defer s.GlobalIndex.mu.Unlock()
	resourceScope := s.GlobalIndex.ensureResourceLocked(ResourceURI(profile.ResourceRoot))
	s.GlobalIndex.removeFiveMDocumentExportsLocked(ResourceURI(doc.URI))
	for _, exp := range doc.FiveMLuaExports {
		if exp.Name == "" {
			continue
		}
		typ := s.fiveMExportTypeFromDocument(doc, exp.NodeID)
		s.GlobalIndex.addFiveMExportSymbolLocked(resourceScope, scope, res, exp.Name, typ)
		entry := resourceScope.tableForScope(scope)[SymbolName(exp.Name)]
		if entry != nil && entry.Export != nil {
			entry.Export.SourceURI = ResourceURI(doc.URI)
			entry.Export.NodeID = exp.NodeID
		}
	}
	s.GlobalIndex.rebuildHashIndexLocked()
}

func globalIndexScopeFromFiveMEnv(env FileEnv) GlobalIndexScope {
	switch env {
	case EnvClient:
		return GlobalIndexScopeClient
	case EnvServer:
		return GlobalIndexScopeServer
	default:
		return GlobalIndexScopeShared
	}
}

func (s *Server) fiveMExportTypeFromDocument(doc *Document, nodeID ast.NodeID) Type {
	if doc == nil || nodeID == ast.InvalidNode || int(nodeID) >= len(doc.Tree.Nodes) {
		return Type{}
	}
	if luadoc := doc.GetLuaDoc(nodeID); luadoc != nil {
		paramNames := make([]string, 0, len(luadoc.Params))
		for _, param := range luadoc.Params {
			paramNames = append(paramNames, param.Name)
		}
		return structuralFunctionTypeFromLuaDoc(paramNames, luadoc.Params, luadoc.Returns)
	}
	node := doc.Tree.Nodes[nodeID]
	if node.Kind == ast.KindFunctionExpr {
		return Type{Primitive: TypeFunction, Structural: &StructuralType{Function: &FunctionType{Variadic: true, Returns: []Type{{Primitive: TypeAny}}}}}
	}
	return Type{}
}

func (s *Server) removeFiveMDocumentExports(uri string) {
	if s == nil || s.GlobalIndex == nil || uri == "" {
		return
	}
	s.GlobalIndex.mu.Lock()
	defer s.GlobalIndex.mu.Unlock()
	s.GlobalIndex.removeFiveMDocumentExportsLocked(ResourceURI(uri))
	s.GlobalIndex.rebuildHashIndexLocked()
}

func (idx *GlobalIndex) removeFiveMDocumentExportsLocked(uri ResourceURI) {
	if idx == nil || uri == "" {
		return
	}
	for _, res := range idx.Resources {
		if res == nil {
			continue
		}
		for _, table := range []SymbolTable{res.Client, res.Server, res.Shared} {
			for name, entry := range table {
				if entry != nil && entry.Export != nil && entry.Export.SourceURI == uri {
					delete(table, name)
				}
			}
		}
	}
}
