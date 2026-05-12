## 2026-05-12 — LSP legacy/compat architecture inventory

- `lsp/server.go` still owns both indexes: `LegacyGlobalIndex map[GlobalKey][]GlobalSymbol` and new
  `GlobalIndex *GlobalIndex`; `compactGlobalIndex` only compacts the legacy map.
- Direct `LegacyGlobalIndex` production/removal is centralized in `lsp/symbols.go` (`setGlobalSymbol`,
  `removeDocumentGlobals`, `getGlobalAlias`, `suggestGlobal`) and `lsp/workspace.go` (`finalizeDocumentUpdate` calls
  `setGlobalSymbol` for globals, fields, LuaDoc fields, module exports).
- Runtime consumers still using legacy global lookups: diagnostics undefined/implicit/shadow checks, completion
  global/member suggestions, workspace symbols, definitions/references helpers, safe rename name collision checks, and
  FiveM native lazy symbol checks.
- New architecture exists in `lsp/global_index.go` (`GlobalIndex`, `ResourceScope`, scoped symbol tables, `HashIndex`,
  `AddSymbol`, `LookupByHash`, `LookupByScope`, FiveM resource/dependency graph) and is used by `resolver.go`,
  `export_bridge.go`, `goto_export.go`, `fivem_natives.go`, and tests.
- Migration gap: new `SymbolEntry`/`GlobalIndex` does not yet carry generic source location metadata equivalent to
  `GlobalSymbol.URI`, `GlobalSymbol.NodeID`, `Parent`, `IsRoot`, `IsDeprecated`, `DeprecatedMsg`, nor does it expose
  iteration/filter APIs equivalent to ranging `LegacyGlobalIndex` with document visibility filtering.
- 2026-05-12: Wave 1 GlobalIndex helpers should snapshot symbols under `idx.mu.RLock()` and yield/sort after unlocking
  where possible; this keeps iterator APIs from holding the index lock while consumer code runs.
- 2026-05-12: `RegisterFiveMResource(..., true)` injects native/runtime metadata into resource scopes, so focused
  GlobalIndex unit tests should use `EnsureResource` plus explicit `Dependencies` when testing visibility without native
  noise.
- 2026-05-12: Wave 2A workspace finalization should avoid calling `globalIndexContext` for every parsed document just to
  cache source; doing so can prematurely cache FiveM profiles as plain Lua before manifests are registered. Source
  caching can safely use the document URI while symbol writes derive resource/scope through `setGlobalSymbol`/
  `setGlobalIndexSymbol` only when symbols are actually produced.
- 2026-05-12: Wave 2B `symbols.go` dual-read migration needs priority normalization when consuming
  `GlobalIndex.HashIndex`; FiveM runtime/std symbols can be rebuilt in map iteration order, so `GlobalSymbol` results
  from `SymbolEntry` should be sorted by `globalSymbolPriority` before legacy fallback-sensitive definition resolution.

## 2026-05-12 diagnostics GlobalIndex dual-read

- Updated lsp/diagnostics.go only: undefined global, implicit global, and global shadowing diagnostics now try
  GlobalIndex first and keep LegacyGlobalIndex fallback.
- New-index undefined checks can reuse visibleGlobalSymbolsFromEntries(doc, SymbolsByHash(key), 1) to preserve canSee
  filtering and priority handling.
- Implicit globals must filter SymbolsByHash(key) entries by IsRoot before visibility checks to match root-level legacy
  behavior.
- Shadowing can use globalIndexContext(doc) + VisibleSymbols(resource, scope), then match entry.Key before converting
  via globalSymbolFromEntry for related diagnostic data.
- Verification: lsp_diagnostics clean, go test ./lsp/ -count=1 passed, go build ./lsp/ passed.

## 2026-05-12 final legacy layer removal

- Removed `LegacyGlobalIndex` from `Server`; production paths now read/write/remove via `GlobalIndex` only.
- `handleCompletion` must use `GlobalIndex.AllSymbols()` with `canSeeSymbol` for top-level global completions;
  `VisibleSymbols` alone omits std/FiveM library surfaces that legacy global iteration previously exposed.
- `removeDocumentGlobals` no longer needs the old `Document` parameter once legacy per-document slice pruning is
  removed; `removeGlobalIndexDocumentSymbols(uri)` is the authoritative cleanup path.
- Verification: `go test ./lsp/ -count=1` passed, `go build ./lsp/` passed, PowerShell recursive `LegacyGlobalIndex`
  search returned no matches. The literal `grep -r` command is unavailable in this Windows PowerShell environment.

## 2026-05-12 LSP New-prefix identifier audit

- `lsp/*.go` contains 15 package-level function identifiers starting with `New` and no `type New*` declarations.
- All found `New*` functions match Go constructor naming for a same-named concrete type without the `New` prefix:
  `NewGlobalIndex`, `NewResourceScope`, `NewDependencyGraph`, `NewResolver`, `NewResolverPhaseState`,
  `NewSemanticDataTable`, `NewTypePool`, `NewFiveMResourceGraph`, `NewFiveMNativeCatalog`, `NewCompletionResourceCache`,
  `NewEvictionPolicy`, `NewFormatter`, `NewPrefetchEngine`, `NewServer`, and `NewWarmupOrchestrator`.
- No `NewCompat*`, `NewLegacy*`, or conflict-workaround constructor names were found in `lsp/*.go`; current
  rename-candidate set is empty.
