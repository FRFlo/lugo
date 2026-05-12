## C2 Hybrid Type System + Interning Pool - 2026-05-11

- Kept the new type system isolated in `lsp/types_structural.go` and `lsp/type_pool.go`; `lsp/infer.go` and the existing
  `BasicType`/`TypeSet` path were not modified.
- Used `type PrimitiveType = BasicType` to satisfy migration coexistence while preserving the existing bitmask constants
  and avoiding package-name conflicts.
- Implemented `TypePool` as a thread-safe hash bucket map (`map[TypeHash][]*StructuralType`) guarded by `sync.RWMutex`;
  hash collisions are resolved by structural equality checks.
- Empty structural intersections return `Type{Primitive: TypeNil}` for the explicit C2 QA edge case where
  `{x:number} ∩ {y:string}` should produce nil.

## C3 SemanticData Side Table - 2026-05-11

- Renamed the formatter-local Scope type to FormatScope so the semantic package-level Scope can match the planned API (
  Parent, Symbols).
- Renamed the transitional compat cache shape from SemanticData to CompatSemanticData because the new SemanticData name
  is reserved for per-node annotations.

## C4 GlobalIndex v2 - 2026-05-11

- Implemented `GlobalIndexV2` as a coordinator around `ResourceScope` partitions, a hash side index, and an integrated
  `DependencyGraph` rather than folding all behavior into one monolithic map.
- Kept the legacy `GlobalIndex` untouched; the old compat shim remains available through `CompatGlobalIndexV2`, while
  new C4 APIs use `LookupByHash` and `LookupByScope`.
- Cycle handling returns warning diagnostics with code `fivem-circular-dependency` and still returns a deterministic
  resource order by appending cyclic leftovers after DAG-ordered resources.
- Memory budget enforcement uses LRU source eviction only, preserving `SymbolEntry` metadata for completion/goto/hover
  consumers after source text and AST are evicted.

## E1 Two-Pass Resolver v2 - 2026-05-11

- Kept `ResolverV2` in the `lsp` package so it can directly use C2 `Type`, C3 `SemanticDataTable`, and C4
  `GlobalIndexV2` without changing the existing `semantic` package API.
- Used an internal resolver scope wrapper around the public C3 `Scope` because binding needs declaration order and child
  scopes, while the public side-table type intentionally remains small (`Parent`, `Symbols`).
- Implemented FiveM cascading as a Phase 2 guard using `GlobalIndexV2` resource dependencies plus a caller-provided
  `Phase1Complete` map; this avoids adding migration-only state fields to `ResourceScope`.
- Preserved the `FeatureFiveM=false` path by keeping FiveM globals/index lookups behind the feature flag and adding a
  parity subtest against `semantic.Resolver` on a plain-Lua sample.

## S1 FiveM Runtime Metadata - 2026-05-11

- Added `Server.GlobalIndexV2` as an initialized companion to the legacy `GlobalIndex` so S1 metadata can be produced
  without replacing existing resolver/hover behavior.
- Kept runtime native registration behind `GlobalIndexV2.RegisterFiveMResource(..., featureFiveM)` instead of indexing
  all native bundles globally; this preserves the FeatureFiveM gate and limits work to active FiveM resources.
- Modeled native signatures as `Type{Primitive: TypeFunction, Structural: &StructuralType{Function: ...}}`, with LuaDoc
  param/return metadata copied into `LuaDocData` for consumers that need names/descriptions.
- Modeled manifest/document exports as scoped `SymbolEntry.Export` records and enforced dependency visibility in
  `LookupFiveMExport`, so cross-resource bridge resolution does not expose exports to unrelated resources.
- Replaced the raw phase-completion map with `ResolverV2PhaseState` guarded by `sync.RWMutex`; phase status can be
  shared by parallel indexing without concurrent map reads/writes.
- Published ResolverV2 field definitions to `GlobalIndexV2` using receiver/property `GlobalKey`s so Phase 2 member
  lookup has a path from collected field declarations into C4 metadata.

## C5 TreeDiff - 2026-05-11

- Kept the diff implementation self-contained in `lsp/treediff.go` and avoided touching `ast/` or `parser/` as
  requested.
- Used stdlib hashing (`hash/fnv`) plus a map bucket pass instead of a tree walk with edit-distance logic to preserve
  the O(N) budget.
- Chose deterministic slice sorting for the public result fields so tests can assert exact NodeID lists.
- 2026-05-12: Added source-location metadata directly to `SymbolEntry` and populated resolver-created entries from
  `ResourceURI`, AST node ID, receiver parent, root-level detection, and adjacent `@deprecated` comments so the new
  index can carry legacy `GlobalSymbol` data without removing legacy consumers yet.
