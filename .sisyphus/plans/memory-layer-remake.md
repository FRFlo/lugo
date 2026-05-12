# Lugo FiveM Memory Layer Remake

## TL;DR

> **Rebuild the entire memory/caching layer of Lugo LSP for FiveM**: Replace the current flat GlobalIndex, bitmask-only
> TypeSet, single-pass resolver, and full-reparse architecture with structural typing, two-pass resolution, dependency
> graph, dependency-prefetch, and tree-diff incremental parsing. Keep the skeleton (Server, Document, LSP dispatch),
> replace all internals. FiveM natives become first-class. Non-FiveM LSP stays working.
>
> **Deliverables**:
> - New AST with side-table SemanticData (preserves arena zero-allocation)
> - Hybrid type system (bitmask primitives + structural tables/functions + type interning pool)
> - Two-pass resolver (Phase 1: declarations, Phase 2: all type inference including FiveM)
> - Enriched GlobalIndex with hierarchical scope partitioning and integrated dependency graph
> - FiveM export bridge with side propagation
> - Prefetch goroutine pool on didOpen
> - Tree-diff incremental parser
> - LRU eviction (source only, keep symbol metadata) with 256MB configurable budget
> - Progressive migration with feature parity tests
>
> **Estimated Effort**: XL (full subsystem rebuild across 8+ modules)
> **Parallel Execution**: YES - 5 waves + Final verification
> **Optimized Structure**: 11 tasks (merged from 22)
> > **Core Skeleton**: C1 (Audit+Harness) → C2 (Type System+Pool) → C3 (Side Table) → C4 (GlobalIndex v2)
> > **Engine**: E1 (Two-Pass Resolver)
> > **Satellites**: S1 (FiveM Metadata) → S2 (Type Inference) → S3 (Proactive Data) → S4 (LSP Features) → S5 (Tree-Diff)
> > **Endgame**: X1 (Progressive Cutover+Validation)
>
> **Critical Path**: C1 → C3 → C4 → E1 → S1 → S2 → S4 → S3 → X1

---

## Context

### Original Request

Rebuild the entire memory layer of Lugo LSP so it is always one step ahead of the user in FiveM contexts: hover should
be instant, references pre-resolved, diagnostics ready before requested, colon methods fully tracked, dependencies
prefetched on file open.

### Interview Summary

**Key Discussions**:

- P1: GlobalIndex enrichment — carry LuaDoc+TypeSet per symbol
- P2: Static require analysis at index time, FiveM resources only
- P3: Full receiver tracking through method chains (complete `:` resolution)
- P4: Dependency-prefetch goroutine pool on didOpen
- P5: SKIPPED — no diagnostic changes
- P6: FiveM natives via go:embed + logic matching (already partially implemented)
- P7: Typed export graph with side propagation (client/server boundaries)
- P8: Scope-partitioned symbol index (partition by client/server/shared at index time)
- P9: Archive into GlobalIndex enrichment (leveraging P1, no separate archive)
- P10: Absorbed into P2
- P11: Tree-diff incremental reparse
- P12: Pre-computed resource completion tables + incremental invalidation
- P13: Goto through export graph resolution (leveraging P7)
- P14: Whole-expression signature resolution
- P15: SKIPPED — no VSCode client changes
- P16: FiveM pattern-driven assignment type inference
- P17: Record-type inference from table literals
- P18: Method chain completion via return-type tracking + FiveM pattern library
- P19: Staged parallel warm-up (manifests → dependency graph → types in topological order)
- P20: No eviction — metadata is small enough (~5-10KB per file, 3000 files ≈ 15-30MB)
- P21: Progressive transition (old and new in parallel, module by module)
- P22: Modify existing parser for tree-diff support (new AST with stable node IDs)
- P23: SKIPPED — don't care about async/promise inference

**Architecture Decisions**:

- Skeleton stays, internals replaced. Lexer/parser untouched, VSCode client untouched.
- AST: lean (offsets+structure) + separate SemanticData side table
- FiveM: native in core, not separate module — first-class symbols
- Type system: structural typing with field maps + type interning (bitmask for primitives only)
- Resolver: two-pass (Phase 1: declarations, Phase 2: types+FiveM)
- Dependency graph: integrated into GlobalIndex (no separate structure)
- Workspace indexing: Phase 1 manifests → Phase 2 types in topological order
- LRU eviction revised: evict source text only, keep symbol metadata
- Memory: no hard eviction needed — FiveM symbol metadata ~5-10KB/file

### Metis Review

**Architectural Tensions Resolved** (from Metis gap analysis):

- `*SemanticData` vs zero-allocation → Use side table `map[NodeID]*SemanticData`, not pointer on Node
- Bitmask vs structural types → Hybrid: bitmask for primitives, structural for tables/functions
- Tree-diff incompatible with current parser → Stable node ID scheme (not array indices)
- LRU eviction breaks cross-file → Evict source only, keep symbol metadata in GlobalIndex
- Two-pass + FiveM cascading → Phase 1 declarations ONLY, Phase 2 ALL type inference
- Forward references → Phase 1 collects ALL declarations before Phase 2 starts

**Guardrails Applied**:

- G1: Zero-allocation hot path preserved via side table
- G2: FeatureFiveM toggle must work
- G3: Test harness compatibility shim during transition
- G4: Performance budget per phase (not just final)
- G5: Package boundaries defined before implementation
- G6: Max 2 weeks dual-maintenance per module

**Scope Locked Down**:

- SC1: Type system = tables + function signatures ONLY
- SC2: Parser = ONLY add tree-diff support, no new node types
- SC3: FiveM types in core, FiveM-specific logic as separate package
- SC4: Dep graph Phase 1 = FiveM resource deps only
- SC5: Evaluate existing Go tree-diff libraries first

---

## Work Objectives

### Core Objective

Rebuild Lugo's memory layer to make the FiveM LSP always one step ahead: instant hover on any symbol, pre-resolved
cross-file references, complete colon method resolution, prefetched dependencies on file open, and tree-diff incremental
parsing.

### Concrete Deliverables

- `lsp/semantic_data.go` — Side table `map[NodeID]*SemanticData` with modular extensions
- `lsp/types_structural.go` — Hybrid type system (bitmask primitives + structural tables/functions)
- `lsp/type_pool.go` — Type interning pool for deduplication
- `lsp/resolver_v2.go` — Two-pass resolver (declarations → types+FiveM)
- `lsp/global_index_v2.go` — Hierarchical GlobalIndex with scope partitioning + integrated dep graph
- `lsp/export_bridge.go` — Typed export bridge with side propagation
- `lsp/prefetch.go` — Goroutine pool for dependency prefetch on didOpen
- `lsp/treediff.go` — Tree-diff incremental parser (stable node IDs, minimal reparse)
- `lsp/completion_resource.go` — Pre-computed resource completion tables + invalidation
- `lsp/infer_v2.go` — Structural type inference (assignments, table literals, method chains)
- `lsp/fivem_natives.go` — go:embed native catalog with structural type integration
- Progressive migration path with feature parity tests

### Definition of Done

- [x] `go test ./lsp/ -count=1` → ALL PASS (existing + new tests)
- [x] `go test ./lsp/ -bench=BenchmarkFiveM -benchmem` → cold start < 5s, per-request < 20ms
- [x] Hover on any FiveM native → instant LuaDoc + full signature
- [x] Hover on `exports[resource]:method()` → resolved type + docs from exporting resource
- [x] `obj:method():chain()` → complete type inference through chain
- [x] `local x = SomeFunc()` → x has correct structural type
- [x] File open → all transitive FiveM dependencies prefetched within 50ms
- [x] Incremental edit → only changed subtree re-parsed (verified by benchmark)
- [x] FeatureFiveM=false → all non-FiveM features work identically

### Must Have

- Side table for SemanticData (NOT pointer on Node)
- Hybrid bitmask+structural type system
- Two-pass resolver: declarations only in Phase 1, ALL type inference in Phase 2
- Hierarchical GlobalIndex with scope partitioning
- FiveM export bridge with client/server/shared side propagation
- Dependency-prefetch goroutine pool on didOpen
- Tree-diff incremental parsing with stable node IDs
- Progressive module-by-module migration
- Feature parity tests running at every step

### Must NOT Have (Guardrails)

- No `*SemanticData` pointer field on Node — use side table
- No generic types, intersection types, or type guards
- No parser refactoring beyond tree-diff support
- No breaking FeatureFiveM=false path
- No require() dependency tracking in Phase 1 (FiveM resource deps only)
- No VSCode client changes
- No diagnostic changes
- No async/promise inference
- No dual-maintenance period > 2 weeks per module

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** — ALL verification is agent-executed. No exceptions.

### Test Decision

- **Infrastructure exists**: YES (24 test files, 5,070 lines of test code)
- **Automated tests**: TDD for type inference (P3/P16/P17), tests-after for everything else
- **Framework**: Go standard testing + existing `fiveM_fixture_harness_test.go` pattern
- **If TDD**: Write failing test first for type inference tasks → minimal implementation → refactor

### QA Policy

Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **Go code**: Use `go test` commands with specific test names
- **Integration**: Use `go test ./lsp/ -run TestIntegration` patterns
- **Benchmarks**: Use `go test ./lsp/ -bench=BenchmarkX -benchmem`
- **LSP protocol**: Use existing test server harness for LSP message verification

---

## Execution Strategy

### Optimized Task Structure (11 Tasks, Merged from 22)

```
CORE SKELETON (Build the foundation first):
├── C1: Foundation Audit & Migration Harness (T1+T6 merged) [quick]
├── C2: Hybrid Type System + Interning Pool (T2+T4 merged) [deep]
├── C3: SemanticData Side Table (T3) [deep]
└── C4: GlobalIndex v2 — Central Registry (T5, slimmed as coordinator) [deep]

ENGINE (Populates the skeleton):
└── E1: Two-Pass Resolver v2 (T7) [deep]

SATELLITES (Orbit the core — producers and consumers):
├── S1: FiveM Runtime Metadata (T8+T9 merged) — natives + export bridge [unspecified-high]
├── S2: Type Inference Engine (T13+T14 merged) — colon methods + assignment/table inference [deep]
├── S3: Proactive Data Layer (T10+T12+T18 merged) — prefetch + completions + warm-up [unspecified-high]
├── S4: LSP Feature Extensions (T15+T16+T17 merged) — chain completion + signatures + goto [unspecified-high]
└── S5: Tree-Diff Incremental Parser (T11) [deep]

ENDGAME (Deployment + validation):
└── X1: Progressive Cutover & Validation (T19+T20+T21+T22 merged) [deep]

Wave FINAL (After ALL tasks — 4 parallel reviews, then user okay):
├── F1: Plan compliance audit (oracle)
├── F2: Code quality review (unspecified-high)
├── F3: Real manual QA (unspecified-high + playwright if UI)
└── F4: Scope fidelity check (deep)
→ Present results → Get explicit user okay
```

### Parallel Execution Waves

```
Wave 1 (Start Immediately — Core Skeleton, all parallel except C4 depends on C1):
├── C1: Foundation Audit & Migration Harness [quick]
├── C2: Hybrid Type System + Interning Pool [deep]
├── C3: SemanticData Side Table [deep]
└── C4: GlobalIndex v2 (after C1 completes) [deep]

Wave 2 (After Wave 1 — Engine + Producers, MAX PARALLEL):
├── E1: Two-pass resolver v2 (depends: C2, C3, C4) [deep]
├── S1: FiveM Runtime Metadata (depends: C2, C4) [unspecified-high]
├── S3: Proactive Data Layer (depends: C4) [unspecified-high]
└── S5: Tree-diff incremental parser (depends: C3) [deep]

Wave 3 (After Wave 2 — Consumers + Integration):
├── S2: Type Inference Engine (depends: E1, C2) [deep]
└── S4: LSP Feature Extensions (depends: S2, S1, C4) [unspecified-high]

Wave 4 (After Wave 3 — Endgame):
└── X1: Progressive Cutover & Validation (depends: C1, E1, C4, S1-S4) [deep]
```

### Dependency Matrix (11 Tasks)

| Task | Merged From     | Depends On        | Blocks         | Wave |
|------|-----------------|-------------------|----------------|------|
| C1   | T1+T6           | —                 | C4, X1         | 1    |
| C2   | T2+T4           | —                 | E1, S1, S2     | 1    |
| C3   | T3              | —                 | E1, S5         | 1    |
| C4   | T5              | C1                | E1, S1, S3, X1 | 1    |
| E1   | T7              | C2, C3, C4        | S2             | 2    |
| S1   | T8+T9           | C2, C4            | S4             | 2    |
| S2   | T13+T14         | E1, C2            | S4             | 3    |
| S3   | T10+T12+T18     | C4                | X1             | 2    |
| S4   | T15+T16+T17     | S2, S1, C4        | X1             | 3    |
| S5   | T11             | C3                | X1             | 2    |
| X1   | T19+T20+T21+T22 | C1, E1, C4, S1-S4 | F1-F4          | 4    |

### Agent Dispatch Summary

- **Wave 1 (Core Skeleton)**: C1→`quick`, C2→`deep`, C3→`deep`, C4→`deep`
- **Wave 2 (Engine + Producers)**: E1→`deep`, S1→`unspecified-high`, S3→`unspecified-high`, S5→`deep`
- **Wave 3 (Consumers)**: S2→`deep`, S4→`unspecified-high`
- **Wave 4 (Endgame)**: X1→`deep`
- **FINAL**: F1→`oracle`, F2→`unspecified-high`, F3→`unspecified-high`, F4→`deep`

---

## Task Mapping (22 Original → 11 Optimized)

| New Code | New Name                             | Original Tasks        | Rationale                                                               |
|----------|--------------------------------------|-----------------------|-------------------------------------------------------------------------|
| **C1**   | Foundation Audit & Migration Harness | T1 + T6               | Same concern: migration scaffolding (audit + shim + harness)            |
| **C2**   | Hybrid Type System + Interning Pool  | T2 + T4               | Same data model: type definitions + interning are inseparable           |
| **C3**   | SemanticData Side Table              | T3                    | Distinct concern: per-node annotation mechanism                         |
| **C4**   | GlobalIndex v2 — Central Registry    | T5                    | The skeleton: coordinator, not monolith                                 |
| **E1**   | Two-Pass Resolver v2                 | T7                    | The engine: distinct from the store                                     |
| **S1**   | FiveM Runtime Metadata               | T8 + T9               | Same data source: natives + exports both feed FiveM data into registry  |
| **S2**   | Type Inference Engine                | T13 + T14             | Same algorithmic concern: both write to `infer_v2.go`                   |
| **S3**   | Proactive Data Layer                 | T10 + T12 + T18       | Same concern: make data available before user asks                      |
| **S4**   | LSP Feature Extensions               | T15 + T16 + T17       | Same consumer pattern: all read from GlobalIndex to serve LSP requests  |
| **S5**   | Tree-Diff Incremental Parser         | T11                   | Different concern: parsing, not memory layer                            |
| **X1**   | Progressive Cutover & Validation     | T19 + T20 + T21 + T22 | Same lifecycle: end-of-project (eviction + replacement + parity + perf) |

> **Note**: The detailed task descriptions below retain their original T1-T22 numbering for reference stability. Each
> task header now includes its new code (C1, C2, etc.) and indicates if it was merged with another task.

---

## TODOs

- [x] 
  **C1.** Foundation Audit & Migration Harness *(merges T1 + T6)*

  **What to do**:
    - Define package structure for the new architecture: `lsp/` remains the main package (consistent with current
      codebase), but identify logical boundaries within it
    - Audit current `Server` struct fields (`lsp/server.go:27-128`) — classify each field as STAY, REPLACE, or REMOVE
    - Audit `Document` struct fields (`lsp/document.go`) — same classification
    - Audit `GlobalIndex` struct — classify as REPLACE (becoming GlobalIndex v2)
    - Create a `MIGRATION.md` in `.sisyphus/drafts/` listing: (a) which Server fields stay, (b) which get replaced and
      by what, (c) which are removed
    - Define the interface boundary between old and new during migration period (compatibility shim spec)
    - Ensure `FeatureFiveM` toggle (`lsp/server.go`) is preserved and gates all new FiveM-specific code

  **Must NOT do**:
    - Do not create new Go packages — keep everything in `lsp/` for now (consistent with current codebase)
    - Do not refactor any existing code — only audit and document
    - Do not break `FeatureFiveM=false` path

  **Recommended Agent Profile**:
    - **Category**: `quick`
        - Reason: Audit and documentation task, no complex implementation
    - **Skills**: [`svelte-core-bestpractices`] (not needed, but no better option for Go code audit)
    - **Skills Evaluated but Omitted**:
        - `frontend-design`: Not relevant to Go code audit

  **Parallelization**:
    - **Can Run In Parallel**: YES — with T2 and T3 (no shared dependencies)
    - **Parallel Group**: Wave 1 (with T2, T3, T4)
    - **Blocks**: T5, T6, T20
    - **Blocked By**: None (can start immediately)

  **References**:
    - `lsp/server.go:27-128` — Server struct definition with all 128+ fields
    - `lsp/document.go` — Document struct with Tree, Resolver, TypeCache, LuaDocCache, FiveMProfile
    - `lsp/symbols.go (GlobalKey/GlobalSymbol)` — Current GlobalIndex (flat map `GlobalKey→[]GlobalSymbol`)
    - `lsp/server.go:116` — `FeatureFiveM` toggle field on Server struct
    - `lsp/fivem_fixture_harness_test.go:172` — Test harness pattern for compatibility during migration

  **WHY Each Reference Matters**:
    - `server.go:27-128`: Must catalog every field to classify as stay/replace/remove — the foundation for progressive
      migration
    - `document.go`: Document struct will gain new fields (SemanticData side table reference, v2 resolver reference)
      while keeping existing ones during transition
    - `globalindex.go`: Understanding current GlobalIndex shape is required to design the v2 hierarchical replacement
    - `server.go:116`: FeatureFiveM toggle (bool field on Server) — must be preserved and gate all new features
    - ` Harness test pattern`: Must understand to create compatibility shim

  **Acceptance Criteria**:
    - [ ] `.sisyphus/drafts/MIGRATION.md` created with complete field-by-field classification
    - [ ] Server struct fields all classified: STAY / REPLACE / REMOVE
    - [ ] Document struct fields all classified
    - [ ] FeatureFiveM toggle usage audit complete
    - [ ] `go build ./lsp/` → PASS (no changes to source code yet)

  **QA Scenarios**:

  ```
  Scenario: Field audit completeness
    Tool: Bash
    Preconditions: MIGRATION.md exists
    Steps:
      1. Run: grep -c "STAY\|REPLACE\|REMOVE" .sisyphus/drafts/MIGRATION.md
      2. Run: grep -c "Fields:" lsp/server.go (count struct fields)
      3. Assert: STAY+REPLACE+REMOVE count >= Server struct field count
    Expected Result: Every Server struct field is classified
    Failure Indicators: Count mismatch means a field was missed
    Evidence: .sisyphus/evidence/task-1-audit-completeness.txt

  Scenario: FeatureFiveM toggle preserved
    Tool: Bash
    Preconditions: No source code changes made
    Steps:
      1. Run: go build ./lsp/
      2. Run: grep -c "FeatureFiveM" lsp/server.go
      3. Assert: Count > 0 and build passes
    Expected Result: Existing FeatureFiveM toggle untouched, project builds
    Failure Indicators: Build fails or FeatureFiveM references removed
    Evidence: .sisyphus/evidence/task-1-feature-toggle-preserved.txt
  ```

  **Commit**: YES (groups with T6)
    - Message: `docs(migration): add Server/Document field audit and migration plan`
    - Files: `.sisyphus/drafts/MIGRATION.md`
    - Pre-commit: `go build ./lsp/`

- [x] 
  **C2.** Hybrid Type System + Interning Pool *(merges T2 + T4)*

  **What to do**:
    - Create `lsp/types_structural.go` implementing the hybrid type system:
        - `PrimitiveType` using bitmask (like current `BasicType` but clean API): number, string, boolean, nil, table,
          function, thread, userdata, any
        - `StructuralType` for tables: field map `{string→Type}`, optional metatable reference, readonly/const markers
        - `FunctionType` for functions: parameter types (positional + variadic), return types, self-type (for colon
          methods)
        - `Type` union type: either primitive bitmask OR structural (pointer to interned type)
        - Type interning pool in `lsp/type_pool.go`: `Intern(structuralType) → *StructuralType` — structurally identical
          types share the same instance
    - Type equality: `Type.Equal(other)` — O(1) for primitives (bitmask), O(n) for structural (field-by-field), O(1) for
      interned types (pointer comparison)
    - Type union/intersection: `Type.Union(other)`, `Type.Intersect(other)` — bitmask ops for primitives, merge field
      maps for structural
    - Must preserve `FeatureFiveM` gating: when `FeatureFiveM=false`, structural types are minimal (only standard Lua)
    - TDD: write failing tests FIRST for type equality, interning, union, intersection

  **Must NOT do**:
    - No generic types, intersection types, or type guards (SC1)
    - Do not remove `BasicType`/`TypeSet` yet — they coexist during migration
    - Do not modify current infer.go — new type system lives in new files

  **Recommended Agent Profile**:
    - **Category**: `deep`
        - Reason: Core type system design with complex equality/interning logic, needs thorough thinking
    - **Skills**: []
    - **Skills Evaluated but Omitted**:
        - `frontend-design`: Not relevant to Go type system code

  **Parallelization**:
    - **Can Run In Parallel**: YES — with T1, T3 (no shared dependencies)
    - **Parallel Group**: Wave 1 (with T1, T3, T4)
    - **Blocks**: T7, T8, T9, T12, T13, T14
    - **Blocked By**: None (can start immediately)

  **References**:
    - `lsp/infer.go:1-100` — Current `TypeSet` and `BasicType` implementation (bitmask approach to understand and
      preserve for primitives)
    - `lsp/infer.go:TypeSet` — Current type set structure that will COEXIST with new system during migration
    - `ast/ast.go:NodeID` — NodeID type definition (used by side table in T3, new type system must reference)
    - Current FiveM type patterns: `lsp/fivem.go:329-374` — Native type patterns that new system must represent

  **WHY Each Reference Matters**:
    - `infer.go:1-100`: Must understand current `BasicType` bitmask design to replicate for primitives and ensure
      backward compat during migration
    - `infer.go:TypeSet`: Current type set that will coexist — must not break it
    - `ast.go:NodeID`: New type system will reference nodes via NodeID (not pointers), must understand the type
    - `fivem.go:329-374`: FiveM native types must be representable as structural types — these are the primary use case

  **Acceptance Criteria**:
    - [ ] `lsp/types_structural.go` created with `Type`, `StructuralType`, `FunctionType`
    - [ ] `lsp/type_pool.go` created with `Intern()` function
    - [ ] `go test ./lsp/ -run TestStructuralType -count=1` → PASS
    - [ ] `go test ./lsp/ -run TestTypeInterning -count=1` → PASS
    - [ ] `go test ./lsp/ -run TestTypeEqual -count=1` → PASS
    - [ ] `go test ./lsp/ -run TestTypeUnion -count=1` → PASS
    - [ ] Current `TypeSet` still works (coexistence verified)
    - [ ] `go build ./lsp/` → PASS

  **QA Scenarios**:

  ```
  Scenario: Type equality happy path — primitives
    Tool: Bash
    Preconditions: types_structural.go and type_pool.go exist
    Steps:
      1. Run: go test ./lsp/ -run TestTypeEqual -v -count=1
      2. Assert: PASS with output containing "primitive_equality"
    Expected Result: Primitive bitmask types compare equal in O(1)
    Failure Indicators: Test fails or panic
    Evidence: .sisyphus/evidence/task-2-type-equal-primitive.txt

  Scenario: Type equality — structural types with interning
    Tool: Bash
    Preconditions: Type interning pool implemented
    Steps:
      1. Run: go test ./lsp/ -run TestTypeInterning -v -count=1
      2. Assert: Two structurally-identical table types intern to same pointer (O(1) comparison)
    Expected Result: Interned structural types compare via pointer equality
    Failure Indicators: Interning returns different pointers for identical types
    Evidence: .sisyphus/evidence/task-2-type-interning.txt

  Scenario: Type union/intersection edge case — empty intersection
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -run TestTypeUnion -v -count=1
      2. Assert: Union of {x:number} and {y:string} produces {x:number, y:string}
      3. Assert: Intersection of {x:number} and {y:string} produces nil type (empty)
    Expected Result: Union merges, intersection returns nil for no overlapping fields
    Failure Indicators: Union overwrites fields, intersection crashes
    Evidence: .sisyphus/evidence/task-2-type-union-edge.txt
  ```

  **Commit**: YES (standalone)
    - Message: `feat(types): add structural type system with bitmask primitives and type interning`
    - Files: `lsp/types_structural.go`, `lsp/type_pool.go`, `lsp/types_structural_test.go`, `lsp/type_pool_test.go`
    - Pre-commit: `go test ./lsp/ -run "TestStructural|TestTypeInter|TestTypeEqual|TestTypeUnion" -count=1`

- [x] 
  **C3.** SemanticData Side Table *(T3)*

  **What to do**:
    - Create `lsp/semantic_data.go` implementing `SemanticDataTable`:
        - `type SemanticDataTable struct { data map[NodeID]*SemanticData }` — side table, NOT pointer on Node
        - `SemanticData` struct with modular extension pattern:
            - Base fields: `Type Type`, `Scope *Scope`, `Bindings []Binding`
            - Optional extension: `LuaDoc *LuaDocData` (nil when absent — zero allocation)
            - Optional extension: `FiveM *FiveMData` (nil when absent)
            - Optional extension: `Export *ExportData` (nil when absent)
        - `Get(nodeID NodeID) *SemanticData` — lookup, returns nil if not present (zero allocation for nodes without
          semantic data)
        - `Set(nodeID NodeID, data *SemanticData)` — insert/update
        - `Clear()` — reuse map (consistent with current `clear()` pattern in `lsp/document.go`)
        - Arena allocation for `SemanticData` struct: pool of `[]SemanticData` slices, not individual heap allocations
    - Must work with current `Node` struct (`ast/ast.go`) — no changes to Node
    - Must preserve zero-allocation hot path (G1 guardrail): use side table, not pointer on Node

  **Must NOT do**:
    - Do NOT add `*SemanticData` as a pointer field on `Node` — Metis identified this breaks arena model
    - Do NOT modify `ast/ast.go` at all — side table is external
    - Do NOT allocate `SemanticData` per node eagerly — only nodes with semantic data get entries

  **Recommended Agent Profile**:
    - **Category**: `deep`
        - Reason: Core data structure design with performance constraints (zero-allocation hot path)
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: YES — with T1, T2, T4 (no shared dependencies in implementation)
    - **Parallel Group**: Wave 1 (with T1, T2, T4)
    - **Blocks**: T7, T8, T11
    - **Blocked By**: None (can start immediately)

  **References**:
    - `ast/ast.go:Node` — Current Node struct (24 bytes, flat array). Must understand its fields to design side table
      that doesn't modify it
    - `ast/ast.go:NodeID` — NodeID type used as side table key
    - `lsp/document.go:clear()` — Pattern for map reuse that SemanticDataTable.Clear() must follow
    - `lsp/parser.go:Reset()` — Current parser resets entire arena — side table must have matching Reset/Clear

  **WHY Each Reference Matters**:
    - `ast.go:Node`: Must NOT be modified — side table exists BECAUSE we can't add a pointer on Node
    - `ast.go:NodeID`: This is the key type for the side table — must understand its definition
    - `document.go:clear()`: Clear/reuse pattern is established convention — must follow it for consistency
    - `parser.go:Reset()`: Side table must be cleared in sync with parser reset — must understand the lifecycle

  **Acceptance Criteria**:
    - [ ] `lsp/semantic_data.go` created with `SemanticDataTable`, `SemanticData` struct
    - [ ] Modular extensions implemented: LuaDoc, FiveM, Export (all optional, nil when absent)
    - [ ] `go test ./lsp/ -run TestSemanticData -count=1` → PASS
    - [ ] Arena allocation pattern used for SemanticData pool
    - [ ] `ast/ast.go` has ZERO changes (verified by `git diff`)
    - [ ] `go build ./lsp/` → PASS

  **QA Scenarios**:

  ```
  Scenario: Side table lookup for node without semantic data
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -run TestSemanticData/Get_nonexistent -v -count=1
      2. Assert: Returns nil without allocation
    Expected Result: Get returns nil for nodes without semantic data, zero heap allocation
    Failure Indicators: Panic, non-nil result, or allocation detected
    Evidence: .sisyphus/evidence/task-3-side-table-nil.txt

  Scenario: Side table set and get with modular extensions
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -run TestSemanticData/SetGet_extensions -v -count=1
      2. Assert: Set with FiveM extension + LuaDoc extension → Get returns both present
      3. Assert: Set with nil FiveM → Get returns SemanticData with FiveM=nil (not allocated)
    Expected Result: Modular extensions are nil when not set, present when set
    Failure Indicators: Extensions always allocated, nil check panics
    Evidence: .sisyphus/evidence/task-3-side-table-extensions.txt

  Scenario: Zero-allocation hot path verification
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -bench=BenchmarkSemanticDataTable -benchmem -count=1
      2. Assert: Get operation has 0 allocs/op
      3. Assert: Set operation has ≤1 allocs/op (the SemanticData allocation)
    Expected Result: Side table preserves zero-allocation hot path for reads
    Failure Indicators: Get allocates, Set allocates more than 1/op
    Evidence: .sisyphus/evidence/task-3-benchmark-side-table.txt
  ```

  **Commit**: YES (standalone)
    - Message: `feat(ast): add SemanticData side table for modular semantic annotations`
    - Files: `lsp/semantic_data.go`, `lsp/semantic_data_test.go`
    - Pre-commit: `go test ./lsp/ -run TestSemanticData -count=1`

- [x] 
  **C2-cont.** Type Interning Pool *(absorbed into C2 — see C2 header above)*

  **What to do**:
    - Create `lsp/type_pool.go` implementing type interning (or extend from T2 if T2 already includes it):
        - `TypePool` struct: `sync.Pool`-like or simple `map[TypeHash]*StructuralType` for deduplication
        - `Intern(st StructuralType) *StructuralType` — returns canonical pointer for structurally identical types
        - `TypeHash` function: hash structural type fields for map key (field names sorted, types hashed recursively)
        - Must handle recursive types (tables with `__index` chains that reference themselves)
        - Thread-safe for concurrent access during workspace indexing
        - Must handle cross-document type identity: same field structure in different files → same interned pointer
    - TDD: write tests for interning, hash collisions, recursive types, concurrent access

  **Must NOT do**:
    - Do not implement generics or intersection types in the pool (SC1)
    - Do not make interning mandatory for Type creation — allow un-interned types for temporary use

  **Recommended Agent Profile**:
    - **Category**: `unspecified-high`
        - Reason: Requires careful hash function design and concurrency handling, but is a focused utility
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: YES — with T1, T3 (depends on T2 types but can be developed in parallel with test stubs)
    - **Parallel Group**: Wave 1 (with T1, T2, T3)
    - **Blocks**: T14
    - **Blocked By**: T2 (needs `StructuralType` definition)

  **References**:
    - `lsp/types_structural.go` (from T2) — `StructuralType` definition that will be interned
    - `lsp/infer.go:TypeSet` — Current type comparison patterns to understand for hash function design
    - `lsp/symbols.go (GlobalKey/GlobalSymbol):GlobalKey` — Current key hashing approach (may provide patterns)

  **WHY Each Reference Matters**:
    - `types_structural.go`: The type definition being interned — hash function must match its structure
    - `infer.go:TypeSet`: Current type comparison logic — reveals what "type equality" means in practice
    - `globalindex.go:GlobalKey`: Key hashing patterns already in use — follow same conventions

  **Acceptance Criteria**:
    - [ ] `lsp/type_pool.go` created with `TypePool`, `Intern`, `TypeHash`
    - [ ] Thread-safe: concurrent goroutines can Intern without data races
    - [ ] Recursive types: tables with `__index` chains don't cause infinite loops in hashing
    - [ ] `go test ./lsp/ -run TestTypePool -count=1` → PASS
    - [ ] `go test ./lsp/ -bench=BenchmarkTypeInterning -benchmem` → interning faster than deep structural comparison
    - [ ] `go build ./lsp/` → PASS

  **QA Scenarios**:

  ```
  Scenario: Interning identical types returns same pointer
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -run TestTypePool/Intern_identical -v -count=1
      2. Assert: Intern({x:number, y:string}) called twice returns same pointer
    Expected Result: Pointer equality for structurally identical types
    Failure Indicators: Different pointers returned for identical types
    Evidence: .sisyphus/evidence/task-4-interning-identity.txt

  Scenario: Recursive type hashing doesn't loop
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -run TestTypePool/Intern_recursive -v -count=1
      2. Assert: Table type with __index pointing to itself interns without infinite loop
    Expected Result: Recursive types hash correctly with cycle detection
    Failure Indicators: Timeout, stack overflow, infinite loop
    Evidence: .sisyphus/evidence/task-4-recursive-hashing.txt

  Scenario: Concurrent interning without data races
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -run TestTypePool/Concurrent -race -count=1
      2. Assert: No data races detected
    Expected Result: TypePool is thread-safe
    Failure Indicators: Race condition detected by Go race detector
    Evidence: .sisyphus/evidence/task-4-concurrent-interning.txt
  ```

  **Commit**: YES (groups with T2)
    - Message: `feat(types): add type interning pool for structural type deduplication`
    - Files: `lsp/type_pool.go`, `lsp/type_pool_test.go`
    - Pre-commit: `go test ./lsp/ -run "TestTypePool" -count=1`

- [x] 
  **C4.** GlobalIndex v2 — Hierarchical Scope Partition + Integrated Dependency Graph *(T5)*

  **What to do**:
    - Create `lsp/global_index_v2.go` implementing the enriched GlobalIndex:
        - **Hierarchical structure**: `GlobalIndexV2` with `Resources map[ResourceURI]*ResourceScope`
        - **ResourceScope**: Contains `Client *SymbolTable`, `Server *SymbolTable`, `Shared *SymbolTable` — partitioned
          by manifest scope
        - **SymbolTable**: `map[SymbolName]*SymbolEntry` where `SymbolEntry` carries `Type Type` (from T2),
          `LuaDoc *LuaDocData` (from T3), `FiveM *FiveMData` (from T3), `Export *ExportData` (from T3)
        - **Integrated dependency graph**: Each resource has `Dependencies []ResourceURI` and `Dependents []ResourceURI`
        - **FiveM resource dependency extraction**: From `fxmanifest.lua` / `__resource.lua` — `dependency` directives
          parsed during workspace indexing
        - **Topological sort**: Method to return resources in dependency order for Phase 2 type inference
        - **Circular dependency detection**: Flag circular deps, produce diagnostic warning, break cycle for indexing
        - **Dual lookup API**: `LookupByHash(key GlobalKey) []SymbolEntry` (backward compat) +
          `LookupByScope(resource, scope, name) *SymbolEntry` (new)
        - **LRU eviction**: `EvictSource(uri ResourceURI)` — evicts source text and AST but KEEPS symbol metadata (
          leveraging enrichment)
        - **Memory budget**: Track total memory, expose `MemoryUsage() uint64`, configurable `MaxMemory uint64` (default
          256MB)
    - Must preserve existing `GlobalKey` lookup for backward compatibility during migration
    - Must handle EC2: shared scripts appearing in multiple scopes (client_scripts + server_scripts)
    - Must handle EC1: circular FiveM resource dependencies (detect, warn, break cycle)

  **Must NOT do**:
    - Do not remove `GlobalIndex` (old) yet — both coexist during migration
    - Do not add `require()` dependency tracking — Phase 1 is FiveM resource deps only (SC4)
    - Do not break `FeatureFiveM=false` path — GlobalIndexV2 must work without FiveM scope partitioning

  **Recommended Agent Profile**:
    - **Category**: `deep`
        - Reason: Core data structure with complex scope partitioning, dependency cycles, and dual lookup API
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: NO — depends on T1 (field audit) and T2 (Type system)
    - **Parallel Group**: Wave 1 (depends on T1 completing first)
    - **Blocks**: T7, T9, T10, T12, T17, T18, T19, T20
    - **Blocked By**: T1 (needs field audit to understand what stays), T2 (needs Type for SymbolEntry)

  **References**:
    - `lsp/symbols.go (GlobalKey/GlobalSymbol)` — Current `GlobalIndex` implementation (flat
      `map[GlobalKey][]GlobalSymbol`) — must understand to design dual lookup API
    - `lsp/symbols.go (GlobalKey/GlobalSymbol):GlobalKey` — Current key structure (ReceiverHash + PropHash) — must
      preserve for backward compat
    - `lsp/symbols.go (GlobalKey/GlobalSymbol):GlobalSymbol` — Current symbol structure — see what metadata fields exist
      to carry forward
    - `lsp/fivem.go:329-374` — FiveM manifest parsing logic that extracts dependencies — must integrate
    - `lsp/fivem.go:FiveMResourceGraph` — Current dependency graph structure — will be replaced by integrated dep graph
    - `.sisyphus/drafts/MIGRATION.md` (from T1) — Field audit results

  **WHY Each Reference Matters**:
    - `globalindex.go`: Current implementation that must be preserved during migration — dual API must wrap it
    - `GlobalKey`/`GlobalSymbol`: Must understand current key/symbol structure to ensure backward-compatible lookup
    - `fivem.go:329-374`: FiveM manifest parsing is the source of dependency data for the integrated dep graph
    - `FiveMResourceGraph`: Current dependency graph that gets replaced — must understand its shape and usage
    - `MIGRATION.md`: Field audit results showing what stays in GlobalIndex

  **Acceptance Criteria**:
    - [ ] `lsp/global_index_v2.go` created with `GlobalIndexV2`, `ResourceScope`, `SymbolTable`, `SymbolEntry`
    - [ ] Dual lookup API works: `LookupByHash` and `LookupByScope`
    - [ ] FiveM dependency extraction from manifest files integrated
    - [ ] Topological sort returns resources in correct order
    - [ ] Circular dependency detection works (diagnostic warning produced, cycle broken)
    - [ ] Shared scripts in multiple scopes handled correctly (EC2)
    - [ ] `go test ./lsp/ -run TestGlobalIndexV2 -count=1` → PASS
    - [ ] `go test ./lsp/ -run TestDependencyCycle -count=1` → PASS
    - [ ] `go build ./lsp/` → PASS

  **QA Scenarios**:

  ```
  Scenario: Hierarchical scope partitioning — client/server/shared
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -run TestGlobalIndexV2/ScopePartitioning -v -count=1
      2. Assert: Resource with client_scripts, server_scripts, shared_scripts has 3 symbol tables
      3. Assert: Symbol in client_scripts ONLY appears in Client SymbolTable
      4. Assert: Symbol in shared_scripts appears in Shared SymbolTable (EC2 handling)
    Expected Result: Symbols partitioned by manifest scope, shared scripts in Shared scope
    Failure Indicators: Server symbol in Client table, shared script missing from Shared
    Evidence: .sisyphus/evidence/task-5-scope-partitioning.txt

  Scenario: Circular dependency detection and handling
    Tool: Bash
    Steps:
      1. Create test fixtures: resource A depends on B, B depends on A
      2. Run: go test ./lsp/ -run TestDependencyCycle -v -count=1
      3. Assert: Cycle detected, diagnostic warning produced, indexing still completes
    Expected Result: Circular deps don't crash indexing, produce diagnostic
    Failure Indicators: Infinite loop, crash, no diagnostic
    Evidence: .sisyphus/evidence/task-5-circular-deps.txt

  Scenario: Topological sort produces correct order
    Tool: Bash
    Steps:
      1. Create test: A→B→C dependency chain
      2. Run: go test ./lsp/ -run TestGlobalIndexV2/TopologicalSort -v -count=1
      3. Assert: Sort result is [C, B, A] (dependencies before dependents)
    Expected Result: Resources ordered so dependencies come before dependents
    Failure Indicators: Wrong order, missing resources, cycle error for DAG
    Evidence: .sisyphus/evidence/task-5-topo-sort.txt

  Scenario: Memory tracking and source eviction
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -run TestGlobalIndexV2/Eviction -v -count=1
      2. Assert: EvictSource removes source+AST but keeps SymbolEntry in SymbolTable
      3. Assert: MemoryUsage() decreases after eviction
      4. Assert: LookupByScope still works for evicted resource
    Expected Result: Source eviction preserves symbol metadata (G4: LRU evicts source only)
    Failure Indicators: LookupByScope fails for evicted resource, memory doesn't decrease
    Evidence: .sisyphus/evidence/task-5-source-eviction.txt
  ```

  **Commit**: YES (standalone)
    - Message: `feat(index): add GlobalIndex v2 with hierarchical scope partition and dependency graph`
    - Files: `lsp/global_index_v2.go`, `lsp/global_index_v2_test.go`
    - Pre-commit: `go test ./lsp/ -run "TestGlobalIndexV2|TestDependencyCycle" -count=1`

- [x] 
  **C1-cont.** Feature Parity Test Harness + Compatibility Shim *(absorbed into C1 — see C1 header above)*

  **What to do**:
    - Create a compatibility shim that lets old and new modules coexist during migration:
        - `lsp/compat.go`: Type aliases and wrapper functions that delegate from old API to new API
        - Example: `CompatGlobalIndex` wraps both old `GlobalIndex` and new `GlobalIndexV2`, delegating based on feature
          flags
    - Create a test harness that validates feature parity between old and new:
        - Based on existing `fiveM_fixture_harness_test.go` pattern (lsp/fivem_fixture_harness_test.go:172)
        - `lsp/parity_test.go`: For each LSP feature (hover, completion, goto-def, find-refs, diagnostics), run both old
          and new codepaths and assert identical results
        - `lsp/parity_bench_test.go`: Benchmarks comparing old vs new performance per feature
    - Must not modify existing test files — only add new ones
    - Must work with `FeatureFiveM=true` AND `FeatureFiveM=false`

  **Must NOT do**:
    - Do not modify existing `fiveM_fixture_harness_test.go` — only add new test files
    - Do not remove or break any existing tests
    - Do not change `FeatureFiveM` toggle behavior

  **Recommended Agent Profile**:
    - **Category**: `unspecified-high`
        - Reason: Test harness design with existing patterns, needs understanding of FiveM fixture patterns
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: YES — depends only on T1 (field audit) for knowing which fields to shim
    - **Parallel Group**: Wave 1 (with T2, T3, T4)
    - **Blocks**: T20 (progressive module replacement needs the shim)
    - **Blocked By**: T1 (needs field audit to design compatibility shim)

  **References**:
    - `lsp/fivem_fixture_harness_test.go:172` — Existing test harness pattern that must be replicated/extended
    - `lsp/server.go:27-128` — Server struct with all fields that need shimming
    - `.sisyphus/drafts/MIGRATION.md` (from T1) — Field audit showing what stays vs replaces
    - `lsp/symbols.go (GlobalKey/GlobalSymbol)` — Current GlobalIndex API that compatibility shim must wrap

  **WHY Each Reference Matters**:
    - `fivem_fixture_harness_test.go:172`: Established pattern for FiveM tests — must follow for consistency
    - `server.go:27-128`: Server fields that need compat wrappers — shim must delegate correctly
    - `MIGRATION.md`: Classification of each field (STAY/REPLACE/REMOVE) — defines what needs shimming
    - `globalindex.go`: Current API surface that `CompatGlobalIndex` must wrap

  **Acceptance Criteria**:
    - [ ] `lsp/compat.go` created with type aliases and wrapper functions
    - [ ] `lsp/parity_test.go` created with feature parity tests for hover, completion, goto-def, find-refs
    - [ ] `lsp/parity_bench_test.go` created with performance comparison benchmarks
    - [ ] All existing tests still pass: `go test ./lsp/ -count=1` → PASS
    - [ ] Feature parity tests build: `go test ./lsp/ -run TestParity -count=1` (may skip if new modules not ready yet)
    - [ ] `go build ./lsp/` → PASS

  **QA Scenarios**:

  ```
  Scenario: Compatibility shim delegates correctly
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -run TestCompat/Delegation -v -count=1
      2. Assert: CompatGlobalIndex.LookupByHash produces same results as old GlobalIndex.Lookup
    Expected Result: Old and new API produce identical results through compat shim
    Failure Indicators: Different results, panic, or missing method
    Evidence: .sisyphus/evidence/task-6-compat-delegation.txt

  Scenario: Existing tests still pass with compat shim
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -count=1
      2. Assert: ALL existing tests pass (no regressions)
    Expected Result: Full test suite green with new compat code
    Failure Indicators: Any test failure
    Evidence: .sisyphus/evidence/task-6-existing-tests.txt

  Scenario: FeatureFiveM=false path works
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -run TestCompat/FeatureFiveM_off -v -count=1
      2. Assert: With FeatureFiveM=false, compat shim uses old codepaths only
    Expected Result: Non-FiveM path works identically through shim
    Failure Indicators: FiveM code invoked, panic, crash
    Evidence: .sisyphus/evidence/task-6-feature-toggle.txt
  ```

  **Commit**: YES (groups with T1)
    - Message: `test(parity): add feature parity test harness and compatibility shim`
    - Files: `lsp/compat.go`, `lsp/parity_test.go`, `lsp/parity_bench_test.go`
    - Pre-commit: `go test ./lsp/ -count=1` (MANDATORY — after ALL implementation tasks)

- [x] 
  **E1.** Two-Pass Resolver v2 — Phase 1 Declarations + Phase 2 Types/FiveM *(T7)*

  **What to do**:
    - Create `lsp/resolver_v2.go` implementing a two-pass semantic resolver:
        - **Phase 1 (Declarations)**: Collect ALL declarations (functions, locals, globals, exports, FiveM resources)
          without type inference. Build scope tree, bind references to declarations, handle forward references (Lua
          allows calling a function before its definition).
        - **Phase 2 (Type Inference + FiveM)**: Resolve ALL types including FiveM context (natives, exports, resource
          scope). This phase only runs after Phase 1 has collected every declaration in the file and its dependencies (
          from dependency graph in T5).
    - Must handle FiveM cascading: Phase 2 must not run until dependency files have completed their Phase 1, allowing
      cross-file type resolution.
    - Must handle forward references: Phase 1 collects ALL declarations first, then Phase 2 resolves using complete
      declaration information.
    - Must preserve existing resolver behavior for `FeatureFiveM=false` path.

  **Must NOT do**:
    - Do not remove the existing `resolver.go` — both must coexist during migration
    - Do not perform ANY type inference in Phase 1 (declarations only)
    - Do not implement `require()` dependency tracking in Phase 1 (FiveM resource deps only)
    - Do not break the `FeatureFiveM=false` path

  **Recommended Agent Profile**:
    - **Category**: `deep`
        - Reason: Complex two-phase architecture with FiveM integration, forward reference handling
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: NO — depends on T2 (structural types), T3 (SemanticData), T5 (GlobalIndex v2)
    - **Parallel Group**: Wave 2 (first task in this wave, can start once Wave 1 completes)
    - **Blocks**: T13, T14, T16
    - **Blocked By**: T2 (needs structural type definitions), T3 (needs SemanticData side table), T5 (needs GlobalIndex
      v2 for dependency resolution)

  **References**:
    - `semantic/resolver.go` — Existing single-pass resolver to understand current scope handling, reference resolution,
      and PendingFields
    - `semantic/resolver.go:scopeStack` — How scopes are currently managed (stack-based)
    - `semantic/resolver.go:References` — How references are currently bound to declarations
    - `semantic/resolver.go:PendingFields` — How forward references are currently handled (deferred resolution)
    - `lsp/fivem.go:1-1478` — FiveM integration that Phase 2 must incorporate
    - `lsp/infer.go` — Current type inference that Phase 2 replaces/enhances

  **WHY Each Reference Matters**:
    - `resolver.go`: Current single-pass implementation — must understand what Phase 1 and Phase 2 each replace
    - `scopeStack`: Must be replicated in Phase 1 with proper scope nesting
    - `References`: Must be bound during Phase 1, used during Phase 2
    - `PendingFields`: Current forward reference mechanism — Phase 1 must handle ALL forward refs natively
    - `fivem.go`: FiveM integration currently bolted on — Phase 2 must make it first-class

  **Acceptance Criteria**:
    - [ ] `lsp/resolver_v2.go` created with two-pass implementation
    - [ ] Phase 1 collects ALL declarations without type inference (verified by test)
    - [ ] Phase 2 resolves all types including FiveM context
    - [ ] Forward references work correctly (test: function called before defined resolves)
    - [ ] `FeatureFiveM=false` path produces identical results to existing resolver
    - [ ] `go test ./lsp/ -run TestResolverV2 -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Two-pass resolves forward references correctly
    Tool: Bash
    Steps:
      1. Create test file with: `local x = foo(); function foo() return 1 end`
      2. Run: go test ./lsp/ -run TestResolverV2/ForwardRef -v -count=1
      3. Assert: `x` has correct type (number) and `foo` is resolved
    Expected Result: Forward reference resolved in Phase 2 using Phase 1 declarations
    Failure Indicators: x type is "unknown" or foo shows "unresolved reference"
    Evidence: .sisyphus/evidence/task-7-forward-ref.txt

  Scenario: Phase 1 does NOT perform type inference
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -run TestResolverV2/Phase1NoInfer -v -count=1
      2. Assert: After Phase 1, all declarations are collected but NO types are resolved
      3. Assert: Type fields on SemanticData are nil/zero after Phase 1
    Expected Result: Phase 1 output has declarations only, no type information
    Failure Indicators: Any type information populated after Phase 1
    Evidence: .sisyphus/evidence/task-7-phase1-no-infer.txt

  Scenario: FeatureFiveM=false produces identical results
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -run TestParity/Resolver -v -count=1
      2. Assert: ResolverV2 with FeatureFiveM=false produces same output as old resolver
    Expected Result: Identical hover, completion, goto-def results
    Failure Indicators: Any difference in outputs between old and new resolver
    Evidence: .sisyphus/evidence/task-7-parity.txt
  ```

  **Commit**: YES (groups with Wave 2)
    - Message: `feat(resolver): implement two-pass resolver v2`
    - Files: `lsp/resolver_v2.go`, `lsp/resolver_v2_test.go`
    - Pre-commit: `go test ./lsp/ -run TestResolverV2 -count=1`

- [x] 
  **S1.** FiveM Runtime Metadata — Native Catalog + Export Bridge *(merges T8 + T9)*

  **What to do**:
    - Modify the existing FiveM native catalog (`lsp/fivem.go` native handling + go:embed data) to:
        - Make natives first-class symbols in the enriched GlobalIndex (from T5) with full structural types
        - Each native gets a `StructuralType` (from T2) representing its function signature (params + return types)
        - Natives are categorized by scope (client/server/shared) matching T5's scope partitioning
        - Match natives through logic (not memory cache) — leverage existing go:embed pattern
    - Handle edge cases from Metis: no manifest, cerulean bundle, NY bundle, `use_experimental_fxv2_oal`
    - Must work with `FeatureFiveM=false` path (skip native catalog entirely)

  **Must NOT do**:
    - Do not remove existing go:embed native handling — extend it
    - Do not load all natives into memory at once if not needed (lazy per-native is acceptable)
    - Do not break `FeatureFiveM=false` path

  **Recommended Agent Profile**:
    - **Category**: `unspecified-high`
        - Reason: Requires understanding existing FiveM native system + new structural type integration
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: YES (with T9, T10, T11, T12)
    - **Parallel Group**: Wave 2
    - **Blocks**: T13 (indirect, via resolver)
    - **Blocked By**: T2 (needs structural type definitions), T3 (needs SemanticData side table)

  **References**:
    - `lsp/fivem.go:329-374` — Native bundle selection logic (fx_version, game, resource_manifest_version branching)
    - `lsp/fivem.go:1-100` — go:embed native data loading pattern
    - `lsp/fivem.go` (entire file) — Current FiveM module that needs integration with core types
    - `lsp/types_structural.go` (from T2) — Structural type system that natives must use
    - `lsp/global_index_v2.go` (from T5) — Enriched GlobalIndex where natives become first-class symbols

  **WHY Each Reference Matters**:
    - `fivem.go:329-374`: Complex bundle selection logic with multiple edge cases — must preserve all branches
    - `fivem.go:1-100`: go:embed pattern to follow for native data loading
    - `types_structural.go`: New type system — natives must create StructuralType instances, not bitmask TypeSet
    - `global_index_v2.go`: Where natives are registered as first-class symbols

  **Acceptance Criteria**:
    - [ ] FiveM natives registered in GlobalIndex v2 with full structural types
    - [ ] `GetHashKey` hover shows full LuaDoc + param types + return type
    - [ ] `Citizen.Wait` hover shows correct signature
    - [ ] Natives scoped by client/server/shared (no server-only natives in client files)
    - [ ] Edge cases handled: no manifest, cerulean bundle, NY bundle
    - [ ] FeatureFiveM=false: no native catalog loaded, no performance cost
    - [ ] `go test ./lsp/ -run TestNativeIntegration -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Hover on FiveM native shows full LuaDoc
    Tool: Bash
    Steps:
      1. Open a FiveM client script file
      2. Hover on `GetHashKey`
      3. Run: go test ./lsp/ -run TestNativeIntegration/HoverGetHashKey -v -count=1
      4. Assert: Hover shows full function signature with parameters and return type
    Expected Result: LuaDoc with param names, types, return type, and description
    Failure Indicators: Missing LuaDoc, wrong types, or "unknown" type
    Evidence: .sisyphus/evidence/task-8-native-hover.txt

  Scenario: Server-only natives not available in client scripts
    Tool: Bash
    Steps:
      1. Open a FiveM client script (client_scripts section of manifest)
      2. Attempt completion to see if server-only natives appear
      3. Run: go test ./lsp/ -run TestNativeIntegration/ScopeFiltering -v -count=1
      4. Assert: Server-only natives do NOT appear in completions for client files
    Expected Result: Only client-appropriate natives shown
    Failure Indicators: Server-only natives visible in client context
    Evidence: .sisyphus/evidence/task-8-native-scope.txt

  Scenario: Edge case - resource with no manifest
    Tool: Bash
    Steps:
      1. Index a plain Lua file (not in a FiveM resource, no manifest)
      2. Run: go test ./lsp/ -run TestNativeIntegration/NoManifest -v -count=1
      3. Assert: No crash, no FiveM natives loaded, plain Lua works
    Expected Result: Graceful handling, no panic, plain Lua completions work
    Failure Indicators: Panic, crash, or FiveM natives appearing in non-FiveM context
    Evidence: .sisyphus/evidence/task-8-no-manifest.txt
  ```

  **Commit**: YES (groups with Wave 2)
    - Message: `feat(fivem): integrate native catalog with structural types`
    - Files: `lsp/fivem_natives.go`, `lsp/fivem_natives_test.go`
    - Pre-commit: `go test ./lsp/ -run TestNativeIntegration -count=1`

- [x] 
  **S1-cont.** FiveM Export Bridge with Side Propagation *(absorbed into S1 — see S1 header above)*

  **What to do**:
    - Create `lsp/export_bridge.go` implementing typed export bridge resolution:
        - Build a `map[ResourceName][]ExportMethod` during workspace indexing, where each `ExportMethod` has: structural
          type (from T2), side (client/server/shared), and LuaDoc
        - Handle `exports[resource]:method()` resolution through typed export graph with side propagation:
            - Client-side exports resolve to client-side implementations only
            - Server-side exports resolve to server-side implementations only
            - Shared exports are visible from both sides
        - Handle edge cases from Metis: dynamic export names (warn about runtime-computed names), circular export
          references
    - Export methods registered in GlobalIndex v2 (from T5) as part of scope-partitioned symbols
    - Integration with T7 resolver v2: Phase 2 resolves export calls through this bridge

  **Must NOT do**:
    - Do not implement dynamic export name resolution (runtime-computed strings) — warn only
    - Do not implement require() tracking in this task (separate concern)
    - Do not break FeatureFiveM=false path

  **Recommended Agent Profile**:
    - **Category**: `deep`
        - Reason: Complex cross-resource type tracking with client/server boundary logic
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: YES (with T8, T10, T11, T12)
    - **Parallel Group**: Wave 2
    - **Blocks**: T17 (goto via export bridge)
    - **Blocked By**: T2 (needs structural types), T5 (needs GlobalIndex v2 for scope partitioning)

  **References**:
    - `lsp/fivem.go:1374-1478` — Current export handling (RegisterExport, CallExport patterns)
    - `lsp/fivem.go:1100-1373` — Manifest parsing that determines client/server/shared scope
    - `lsp/global_index_v2.go` (from T5) — Where export bridge integrates into scope-partitioned index
    - `lsp/types_structural.go` (from T2) — Structural type system for export method signatures

  **WHY Each Reference Matters**:
    - `fivem.go:1374-1478`: Current export handling — must understand what's being replaced
    - `fivem.go:1100-1373`: Manifest parsing — needed to determine side (client/server/shared) of each export
    - `global_index_v2.go`: New GlobalIndex where exports are registered
    - `types_structural.go`: Export methods must have structural types, not bitmask

  **Acceptance Criteria**:
    - [ ] `lsp/export_bridge.go` created with typed export bridge
    - [ ] `exports[resource]:method()` resolved to correct function signature with LuaDoc
    - [ ] Client-side exports not visible from server scripts (and vice versa)
    - [ ] Shared exports visible from both sides
    - [ ] Dynamic export names produce warning (not crash)
    - [ ] Circular export references handled gracefully (no infinite loop)
    - [ ] FeatureFiveM=false: export bridge inactive, no performance cost
    - [ ] `go test ./lsp/ -run TestExportBridge -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Export bridge resolves cross-resource call
    Tool: Bash
    Steps:
      1. Create two FiveM resources: resource_a exports `getPlayerName`, resource_b calls `exports[resource_a]:getPlayerName()`
      2. Run: go test ./lsp/ -run TestExportBridge/CrossResource -v -count=1
      3. Assert: Hover on `getPlayerName` shows full signature + LuaDoc from resource_a
    Expected Result: Full function signature with parameter types and return type
    Failure Indicators: "unknown" type, missing LuaDoc, or unresolved reference
    Evidence: .sisyphus/evidence/task-9-export-cross-resource.txt

  Scenario: Server-only export not visible from client
    Tool: Bash
    Steps:
      1. Create resource with server_scripts exporting `getBankBalance`
      2. Create client script attempting to call `exports[resource]:getBankBalance()`
      3. Run: go test ./lsp/ -run TestExportBridge/SidePropagation -v -count=1
      4. Assert: Server export produces diagnostic "cannot access server export from client"
    Expected Result: Side-aware error diagnostic
    Failure Indicators: Server export visible in client context without warning
    Evidence: .sisyphus/evidence/task-9-side-propagation.txt

  Scenario: Dynamic export name produces warning
    Tool: Bash
    Steps:
      1. Create code: `local name = 'method'; exports[resource]:name()`
      2. Run: go test ./lsp/ -run TestExportBridge/DynamicWarning -v -count=1
      3. Assert: Warning diagnostic produced, no crash
    Expected Result: Warning about dynamic export name, graceful handling
    Failure Indicators: Crash, panic, or silent failure
    Evidence: .sisyphus/evidence/task-9-dynamic-export.txt
  ```

  **Commit**: YES (groups with Wave 2)
    - Message: `feat(fivem): add export bridge with side propagation`
    - Files: `lsp/export_bridge.go`, `lsp/export_bridge_test.go`
    - Pre-commit: `go test ./lsp/ -run TestExportBridge -count=1`

- [x] 
  **S3.** Proactive Data Layer — Prefetch + Completion Tables + Warm-up *(merges T10 + T12 + T18)*

  **What to do**:
    - Create `lsp/prefetch.go` implementing a goroutine pool for dependency prefetching:
        - On `didOpen`, look up the file's dependencies in GlobalIndex v2 (from T5)
        - Kick off goroutines to load + resolve each FiveM dependency (using enriched GlobalIndex metadata, not full
          reparse)
        - Use a bounded goroutine pool (configurable, default 8) to prevent thundering herd
        - Prefetched data feeds into SemanticData side table (from T3) and type resolution
    - Integration with T11 (tree-diff): when prefetch completes, tree-diff ensures only changed nodes are recomputed
    - Must handle edge cases from Metis: workspace editing during indexing, circular deps

  **Must NOT do**:
    - Do not prefetch full AST trees — only symbol metadata from GlobalIndex enrichment (P1/P9)
    - Do not implement client-side changes (P15 was skipped)
    - Do not prefetch on every keystroke — only on didOpen

  **Recommended Agent Profile**:
    - **Category**: `unspecified-high`
        - Reason: Concurrent Go patterns (goroutine pool, bounded concurrency) with LSP integration
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: YES (with T7, T8, T9, T11, T12)
    - **Parallel Group**: Wave 2
    - **Blocks**: T18 (workspace warm-up uses this pool)
    - **Blocked By**: T5 (needs GlobalIndex v2 with dependency graph)

  **References**:
    - `lsp/server.go:didOpen` — Current didOpen handler that needs prefetch hook
    - `lsp/global_index_v2.go` (from T5) — Dependency graph to look up transitive deps
    - `lsp/server.go:27-128` — Server struct where prefetch pool runs

  **WHY Each Reference Matters**:
    - `server.go:didOpen`: Entry point where prefetch is triggered — must understand current didOpen flow
    - `global_index_v2.go`: Dependency graph that determines what to prefetch
    - `server.go:27-128`: Server struct where the prefetch pool is stored and managed

  **Acceptance Criteria**:
    - [ ] `lsp/prefetch.go` created with bounded goroutine pool
    - [ ] On didOpen, transitive dependencies are prefetch-loaded within 50ms
    - [ ] Pool is bounded (max 8 concurrent goroutines by default)
    - [ ] No thundering herd: 10 files opening simultaneously doesn't spike to 80 goroutines
    - [ ] Workspace editing during indexing doesn't corrupt state (test: edit while prefetch running)
    - [ ] `go test ./lsp/ -run TestPrefetch -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Prefetch loads dependencies on file open
    Tool: Bash
    Steps:
      1. Index a FiveM workspace with resource A depending on resource B
      2. Simulate didOpen for a file in resource A
      3. Run: go test ./lsp/ -run TestPrefetch/OnDidOpen -v -count=1
      4. Assert: Resource B's metadata is loaded into GlobalIndex within 50ms
    Expected Result: Dependencies prefetch-loaded, hover on resource B symbols works instantly
    Failure Indicators: Resource B symbols not available, or delayed loading
    Evidence: .sisyphus/evidence/task-10-prefetch-didopen.txt

  Scenario: Bounded pool prevents thundering herd
    Tool: Bash
    Steps:
      1. Configure pool size = 4
      2. Simulate didOpen for 10 files simultaneously
      3. Run: go test ./lsp/ -run TestPrefetch/BoundedPool -v -count=1
      4. Assert: Max concurrent goroutines never exceeds 4
    Expected Result: Pool bounds respected, no more than 4 concurrent prefetch operations
    Failure Indicators: Goroutine count exceeds pool size
    Evidence: .sisyphus/evidence/task-10-bounded-pool.txt

  Scenario: Edit during prefetch doesn't corrupt state
    Tool: Bash
    Steps:
      1. Start prefetch for resource A's dependencies
      2. Mid-prefetch, simulate didChange for a file in resource A
      3. Assert: No panic, no data corruption, prefetch completes or restarts cleanly
    Expected Result: Clean concurrent edit handling, no races
    Failure Indicators: Panic, data race, or stale metadata
    Evidence: .sisyphus/evidence/task-10-concurrent-edit.txt
  ```

  **Commit**: YES (groups with Wave 2)
    - Message: `feat(server): add dependency-prefetch goroutine pool`
    - Files: `lsp/prefetch.go`, `lsp/prefetch_test.go`
    - Pre-commit: `go test ./lsp/ -run TestPrefetch -count=1`

- [x] 
  **S5.** Tree-Diff Incremental Parser *(T11)*

  **What to do**:
    - Create `lsp/treediff.go` implementing incremental parsing with stable node IDs:
        - Implement stable node ID scheme (not array indices) — use a sequential counter that persists across edits
        - Implement tree-diff algorithm that identifies unchanged subtrees between old and new AST
        - On edit: diff old tree → new source, only reparse changed nodes and their dependents
        - Use existing parser (`parser/parser.go`) but add ability to parse partial subtrees
        - Modified parser produces new AST with stable IDs mapped from old tree
    - Must evaluate existing Go tree-diff libraries before building custom (Metis guardrail SC5)
    - Must handle: multi-cursor edits, large insertions/deletions, encoding changes
    - Property-based test: tree-diff produces identical AST to full reparse for all edits

  **Must NOT do** (guardrails):
    - Do not refactor parser beyond tree-diff support — no new node types, no precedence changes
    - Do not add `*SemanticData` pointer to Node — use side table from T3
    - Do not break existing parser behavior when tree-diff is disabled

  **Recommended Agent Profile**:
    - **Category**: `deep`
        - Reason: Complex algorithmic work (tree diffing, stable IDs, partial reparse), core infrastructure
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: YES (with T7, T8, T9, T10, T12)
    - **Parallel Group**: Wave 2
    - **Blocks**: T20 (progressive replacement needs tree-diff working)
    - **Blocked By**: T3 (needs SemanticData side table for stable node ID mapping)

  **References**:
    - `parser/parser.go` — Entire parser package that needs tree-diff support added
    - `ast/ast.go` — Current AST tree structure with `Reset()` that clears arena
    - `ast/ast.go:Node` — Current Node struct (24 bytes, flat arena, byte offsets)
    - `lsp/semantic_data.go` (from T3) — Side table that maps NodeID→SemanticData

  **WHY Each Reference Matters**:
    - `parser/`: Must understand parser architecture to add partial reparse capability
    - `tree.go`: Current `Reset()` pattern must be replaced with incremental update
    - `Node` struct: Must stay 24 bytes — stable IDs go in side table, not in Node
    - `semantic_data.go`: Side table is where NodeID→SemanticData mapping lives, also where stable IDs are managed

  **Acceptance Criteria**:
    - [ ] `lsp/treediff.go` created with stable node ID scheme and tree-diff algorithm
    - [ ] Incremental edit produces identical AST to full reparse (property-based test)
    - [ ] Stable node IDs persist across edits (test: edit node, verify ID unchanged)
    - [ ] Multi-cursor edits handled correctly (test: simultaneous edits don't corrupt tree)
    - [ ] Benchmark: incremental reparse < 2ms for typical edit (vs current full reparse)
    - [ ] Existing parser behavior preserved when tree-diff disabled
    - [ ] `go test ./lsp/ -run TestTreeDiff -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Incremental reparse produces identical AST to full reparse
    Tool: Bash
    Steps:
      1. Create property-based test: generate random edits, compare tree-diff result vs full reparse
      2. Run: go test ./lsp/ -run TestTreeDiff/PropertyBased -v -count=1
      3. Assert: 100+ random edits all produce identical AST
    Expected Result: Tree-diff and full reparse produce byte-identical results
    Failure Indicators: Any structural difference between incremental and full AST
    Evidence: .sisyphus/evidence/task-11-property-based.txt

  Scenario: Stable node IDs survive edits
    Tool: Bash
    Steps:
      1. Parse file, record node IDs for nodes A, B, C
      2. Edit file (insert text between A and B)
      3. Re-run incremental parse
      4. Assert: Node A and C retain same IDs, new nodes get new IDs
    Expected Result: Unchanged nodes keep IDs, only new/changed nodes get new IDs
    Failure Indicators: All node IDs shift after edit
    Evidence: .sisyphus/evidence/task-11-stable-ids.txt

  Scenario: Performance - incremental faster than full reparse
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -bench=BenchmarkIncrementalEdit -benchmem -count=5
      2. Assert: Incremental edit time < 2ms for typical edit (single line change)
      3. Assert: Incremental time < 10% of full reparse time
    Expected Result: Significant speedup over full reparse
    Failure Indicators: Incremental not faster than full reparse
    Evidence: .sisyphus/evidence/task-11-benchmark.txt
  ```

  **Commit**: YES (groups with Wave 2)
    - Message: `feat(parser): add tree-diff incremental parser`
    - Files: `lsp/treediff.go`, `lsp/treediff_test.go`
    - Pre-commit: `go test ./lsp/ -run TestTreeDiff -count=1`

- [x] 
  **S3-cont.** Pre-Computed Resource Completion Tables + Incremental Invalidation *(absorbed into S3 — see S3 header
  above)*

  **What to do**:
    - Create `lsp/completion_resource.go` implementing pre-computed completion tables per resource:
        - During workspace indexing, for each FiveM resource, extract all globals/functions/tables from its scripts (
          shared, client, server)
        - Store in resource-scoped completion tables keyed by manifest scope (client/server/shared)
        - Always ready, no per-request computation cost
        - On file change, invalidate ONLY the affected resource's completion table and recompute
        - Integration with T5 (GlobalIndex v2 scope partitioning) and T7 (resolver v2 Phase 2 types)
    - Must respect manifest scope: client-only symbols don't appear in server scripts

  **Must NOT do**:
    - Do not compute completions per-request (pre-compute at index time)
    - Do not invalidate all tables on single file change (incremental only)
    - Do not break FeatureFiveM=false path

  **Recommended Agent Profile**:
    - **Category**: `unspecified-high`
        - Reason: Completion table design with scope filtering and incremental invalidation
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: YES (with T7, T8, T9, T10, T11)
    - **Parallel Group**: Wave 2
    - **Blocks**: T15 (method chain completion extends these tables)
    - **Blocked By**: T2 (structural type definitions), T5 (scope partitioned GlobalIndex)

  **References**:
    - `lsp/features.go` — Current completion logic to understand what symbols are offered
    - `lsp/fivem.go:1100-1373` — Manifest parsing that determines script scope
    - `lsp/global_index_v2.go` (from T5) — Scope-partitioned symbol index that completion tables build from

  **WHY Each Reference Matters**:
    - `completion.go`: Current completion implementation — must understand what data completions provide
    - `fivem.go:1100-1373`: Manifest scope detection — completion tables must respect this
    - `global_index_v2.go`: Source of scope-partitioned symbols for completion tables

  **Acceptance Criteria**:
    - [ ] `lsp/completion_resource.go` created with pre-computed completion tables
    - [ ] Completion tables built during workspace indexing for each resource
    - [ ] Scope filtering: client-only symbols NOT in server script completions (and vice versa)
    - [ ] Single file change invalidates ONLY the affected resource's table
    - [ ] Full workspace re-index time < 2x current time (pre-computation overhead)
    - [ ] `go test ./lsp/ -run TestResourceCompletion -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Resource completion table shows scope-appropriate symbols
    Tool: Bash
    Steps:
      1. Index FiveM workspace with `client_scripts` and `server_scripts`
      2. Request completions in a client script file
      3. Assert: Server-only functions NOT in completion list
      4. Request completions in a server script file
      5. Assert: Client-only functions NOT in completion list
    Expected Result: Scope-appropriate completions, no cross-contamination
    Failure Indicators: Server functions visible from client or vice versa
    Evidence: .sisyphus/evidence/task-12-scope-completion.txt

  Scenario: Incremental invalidation on file change
    Tool: Bash
    Steps:
      1. Index workspace, record completion table for resource A and resource B
      2. Edit a file in resource A
      3. Assert: Resource A table recomputed, Resource B table unchanged
      4. Run: go test ./lsp/ -run TestResourceCompletion/IncrementalInvalidation -v -count=1
    Expected Result: Only affected resource's table invalidated
    Failure Indicators: All resources recomputed, or resource B table changed
    Evidence: .sisyphus/evidence/task-12-incremental.txt

  Scenario: FeatureFiveM=false still works
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -run TestResourceCompletion/FeatureToggle -v -count=1
      2. Assert: With FeatureFiveM=false, standard completion works without resource tables
    Expected Result: Non-FiveM path unaffected
    Failure Indicators: FiveM code invoked when toggle is off
    Evidence: .sisyphus/evidence/task-12-feature-toggle.txt
  ```

  **Commit**: YES (groups with Wave 2)
    - Message: `feat(complete): add pre-computed resource completion tables`
    - Files: `lsp/completion_resource.go`, `lsp/completion_resource_test.go`
    - Pre-commit: `go test ./lsp/ -run TestResourceCompletion -count=1`

- [x] 
  **S2.** Type Inference Engine — Colon Methods + Assignment/Table Inference *(merges T13 + T14)*

  **What to do**:
    - Implement full receiver tracking through method call chains in `lsp/infer_v2.go`:
        - When resolving `obj:method()`, track the receiver object (`obj`) through the method call
        - Propagate `self` type: `obj:method()` binds `self = obj` in method body
        - Handle chains: `obj:method1():method2():method3()` — each step's return type becomes the next step's receiver
        - Distinguish `.` calls (function call) from `:` calls (method call with implicit self)
        - FiveM pattern integration: `MySQL:Await:Execute`, `Citizen:CreateThread:Wait` patterns
    - TDD approach: write failing tests first, then implement until tests pass

  **Must NOT do** (guardrails):
    - Do not implement generics, intersection types, or type guards (SC1)
    - Do not modify parser syntax — colon method resolution is a type inference concern
    - Do not break `.' call resolution

  **Recommended Agent Profile**:
    - **Category**: `deep`
        - Reason: Core type inference logic, tracking receiver through call chains requires careful design
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: NO — depends on T7 (resolver v2) and T2 (structural types)
    - **Parallel Group**: Wave 3 (first task in wave)
    - **Blocks**: T15, T16
    - **Blocked By**: T7 (needs resolver v2 Phase 2 for type resolution), T2 (needs structural type system)

  **References**:
    - `lsp/infer.go` — Current type inference (1,356 lines) that needs receiver tracking added
    - `lsp/infer.go:inferMemberExpr` — Current member expression inference (4-step lookup)
    - `lsp/infer.go:BasicType` — Current bitmask type system to be replaced by structural types from T2
    - `lsp/resolver_v2.go` (from T7) — Phase 2 type inference that calls into this
    - `lsp/types_structural.go` (from T2) — Structural type definitions (StructType, MethodType) that method resolution
      uses

  **WHY Each Reference Matters**:
    - `infer.go`: Current type inference — understanding existing approach informs what to replace
    - `inferMemberExpr`: 4-step lookup (doc declarations, reassignments, metatables, global classes) — self tracking
      must be added here
    - `BasicType`: Bitmask being replaced — colon resolution uses structural MethodType
    - `resolver_v2.go`: Phase 2 calls type inference — method resolution must integrate
    - `types_structural.go`: MethodType and StructType definitions — the type language for receiver tracking

  **Acceptance Criteria**:
    - [ ] `obj:method()` correctly infers `self` as the receiver type
    - [ ] `obj:method1():method2():method3()` resolves each step's return type as next receiver
    - [ ] `obj.method()` (dot call) does NOT bind `self`
    - [ ] FiveM patterns: `MySQL:Await:Execute` chain resolves correctly
    - [ ] TDD: failing test → implementation → green → refactor cycle visible in commits
    - [ ] `go test ./lsp/ -run TestColonMethod -count=1` → ALL PASS

  **QA Scenarios**:

  ```
  Scenario: Simple colon method resolves self type
    Tool: Bash
    Steps:
      1. Create test: `local obj = MyClass(); obj:greet()` where MyClass defines `greet` as method
      2. Run: go test ./lsp/ -run TestColonMethod/SelfType -v -count=1
      3. Assert: `self` inside `greet` has type MyClass
    Expected Result: self bound to receiver type
    Failure Indicators: self is "unknown" or wrong type
    Evidence: .sisyphus/evidence/task-13-self-type.txt

  Scenario: Method chain resolves each step
    Tool: Bash
    Steps:
      1. Create test: `obj:transform():scale():position()` where each returns a different type
      2. Run: go test ./lsp/ -run TestColonMethod/Chain -v -count=1
      3. Assert: Each step resolves return type, next step's receiver is previous return type
    Expected Result: Full chain type inference works end-to-end
    Failure Indicators: Chain breaks at step 2 or later, "unknown" return type
    Evidence: .sisyphus/evidence/task-13-chain.txt

  Scenario: Dot call does NOT bind self
    Tool: Bash
    Steps:
      1. Create test: `obj.method()` (dot call, no colon)
      2. Run: go test ./lsp/ -run TestColonMethod/DotCall -v -count=1
      3. Assert: `self` is NOT bound, function called with explicit args only
    Expected Result: Dot call treated as regular function call, no implicit self
    Failure Indicators: self bound even on dot call
    Evidence: .sisyphus/evidence/task-13-dot-call.txt

  Scenario: FiveM pattern chain resolves
    Tool: Bash
    Steps:
      1. Create FiveM test: `MySQL:Await:Execute(query)` pattern
      2. Run: go test ./lsp/ -run TestColonMethod/FiveMChain -v -count=1
      3. Assert: Each colon step resolves with correct return type
    Expected Result: Full FiveM method chain inference
    Failure Indicators: FiveM chain breaks or returns "unknown"
    Evidence: .sisyphus/evidence/task-13-fivem-chain.txt
  ```

  **Commit**: YES
    - Message: `feat(infer): implement colon method resolution with receiver tracking`
    - Files: `lsp/infer_v2.go`, `lsp/infer_v2_test.go`
    - Pre-commit: `go test ./lsp/ -run TestColonMethod -count=1`

- [x] 
  **S2-cont.** Assignment + Table Literal Type Inference *(absorbed into S2 — see S2 header above)*

  **What to do**:
    - Implement assignment type inference in `lsp/infer_v2.go`:
        - When a local is assigned `local x = SomeFunc()`, propagate SomeFunc's return type to x
        - Handle multiple returns: `local x, y = SomeFunc()` — infer each variable from return position
        - Handle method call returns: `local result = obj:method()` — uses T13 receiver tracking
    - Implement table literal type inference:
        - When a table literal `{ key = value, key2 = value2 }` is assigned, infer a StructType with named fields
        - Record: `t = { name = 'test', value = 5 }` → `StructType{fields: {name: string, value: number}}`
        - Handle nested tables and array-like tables
    - TDD approach: write failing tests first for each inference rule
    - Integration with T4 type interning pool: deduplicate identical structural types

  **Must NOT do** (guardrails):
    - Do not implement generics, intersection types, or type guards (SC1)
    - Do not implement full flow analysis (if/else branches) — assignment inference only
    - Do not validate against LuaDoc annotations (no annotation-checked inference)

  **Recommended Agent Profile**:
    - **Category**: `deep`
        - Reason: Core type inference with structural type construction, requires careful TDD
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: NO — depends on T7, T2, T4
    - **Parallel Group**: Wave 3
    - **Blocks**: T21 (parity validation)
    - **Blocked By**: T7 (needs resolver v2), T2 (structural type system), T4 (type interning pool)

  **References**:
    - `lsp/infer.go` — Current type inference showing assignment handling
    - `lsp/infer.go:inferLocal` — Current local variable inference
    - `lsp/types_structural.go` (from T2) — StructType, FunctionType definitions
    - `lsp/type_pool.go` (from T4) — Type interning pool for deduplication

  **WHY Each Reference Matters**:
    - `infer.go`: Understanding existing inference flows to know what to extend
    - `inferLocal`: Local variable inference is what assignment type inference builds on
    - `types_structural.go`: StructType is the result type for table literals, FunctionType for call returns
    - `type_pool.go`: Deduplication ensures identical table types share the same instance

  **Acceptance Criteria**:
    - [ ] `local x = SomeFunc()` → x has correct return type
    - [ ] `local x, y = SomeFunc()` → x and y have correct types from multi-return
    - [ ] `local result = obj:method()` → result has method's return type (via T13)
    - [ ] `t = { name = 'test', value = 5 }` → t has StructType{ name: string, value: number }
    - [ ] Identical table types deduplicated via type pool
    - [ ] TDD: failing test → implementation → green cycle visible in commits
    - [ ] `go test ./lsp/ -run TestAssignmentInference -count=1` → PASS
    - [ ] `go test ./lsp/ -run TestTableLiteralInference -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Assignment infers return type
    Tool: Bash
    Steps:
      1. Create test: `local x = tonumber("42")` where tonumber returns number
      2. Run: go test ./lsp/ -run TestAssignmentInference/ReturnType -v -count=1
      3. Assert: x has type number
    Expected Result: Assignment correctly propagates return type
    Failure Indicators: x has type "unknown" or wrong type
    Evidence: .sisyphus/evidence/task-14-assign-return.txt

  Scenario: Multi-return assignment
    Tool: Bash
    Steps:
      1. Create test: `local x, y = someFunc()` where func returns (number, string)
      2. Run: go test ./lsp/ -run TestAssignmentInference/MultiReturn -v -count=1
      3. Assert: x is number, y is string
    Expected Result: Each variable gets correct type from corresponding return position
    Failure Indicators: Wrong types or "unknown" for any variable
    Evidence: .sisyphus/evidence/task-14-multi-return.txt

  Scenario: Table literal infers struct type
    Tool: Bash
    Steps:
      1. Create test: `local t = { name = 'test', value = 5 }`
      2. Run: go test ./lsp/ -run TestTableLiteralInference/StructType -v -count=1
      3. Assert: t has StructType with fields {name: string, value: number}
    Expected Result: Table literal produces correct structural type
    Failure Indicators: t has type "table" (generic) without field information
    Evidence: .sisyphus/evidence/task-14-table-literal.txt

  Scenario: Type deduplication via pool
    Tool: Bash
    Steps:
      1. Create test: two identical table literals `local a = {x=1}; local b = {x=1}`
      2. Run: go test ./lsp/ -run TestTableLiteralInference/Dedup -v -count=1
      3. Assert: a and b's types are pointer-equal (same pool entry)
    Expected Result: Identical structural types share the same pool entry
    Failure Indicators: Two separate allocations for identical types
    Evidence: .sisyphus/evidence/task-14-dedup.txt
  ```

  **Commit**: YES
    - Message: `feat(infer): add assignment and table literal type inference`
    - Files: `lsp/infer_v2.go`, `lsp/infer_v2_test.go`
    - Pre-commit: `go test ./lsp/ -run "TestAssignmentInference|TestTableLiteralInference" -count=1`

- [x] 
  **S4.** LSP Feature Extensions — Chain Completion + Signatures + Goto *(merges T15 + T16 + T17)*

  **What to do**:
    - Add method chain completion to `lsp/completion_resource.go` (extends T12):
        - After each `:method()` call in a chain, use T13's receiver tracking to determine the return type
        - When user types `.` or `:` after a method call, complete based on the return type's fields/methods
        - Include FiveM-specific chain pattern library for common patterns:
            - `MySQL:Await:Execute` → complete Await methods, then Execute methods
            - `Citizen:CreateThread:Wait` → complete thread methods, then wait return
            - `Player:getId:getName` → complete ID methods, then name methods
        - Fallback: if type inference can't determine chain type, use FiveM pattern library

  **Must NOT do**:
    - Do not implement pattern matching for every possible FiveM chain — common patterns only
    - Do not break completion for non-chain contexts

  **Recommended Agent Profile**:
    - **Category**: `unspecified-high`
        - Reason: Builds on T13 receiver tracking and T12 completion tables, integration work
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: YES (with T14, T16, T17, T18)
    - **Parallel Group**: Wave 3
    - **Blocks**: T21 (parity validation)
    - **Blocked By**: T13 (method resolution), T5 (scope-partitioned completions)

  **References**:
    - `lsp/features.go` — Current completion logic
    - `lsp/completion_resource.go` (from T12) — Resource completion tables to extend
    - `lsp/infer_v2.go` (from T13) — Receiver tracking results for chain resolution

  **WHY Each Reference Matters**:
    - `completion.go`: Current completion implementation — must understand how completions are triggered and returned
    - `completion_resource.go`: Pre-computed tables that chain completion queries
    - `infer_v2.go`: Receiver tracking that determines what methods are available after a chain step

  **Acceptance Criteria**:
    - [ ] `obj:method():` completes with methods available on method's return type
    - [ ] `obj:method():chain():` completes with next chain's return type methods
    - [ ] FiveM patterns (MySQL, Citizen, Player) have specialized completions
    - [ ] Fallback for unknown types doesn't crash — returns empty completions gracefully
    - [ ] `go test ./lsp/ -run TestMethodChainCompletion -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Method chain completion shows correct methods
    Tool: Bash
    Steps:
      1. Create FiveM file: `local result = MySQL:Await:` and request completion
      2. Run: go test ./lsp/ -run TestMethodChainCompletion/ChainStep -v -count=1
      3. Assert: Completion shows methods available on Await's return type
    Expected Result: Correct method list for chain step
    Failure Indicators: Empty completion or wrong method list
    Evidence: .sisyphus/evidence/task-15-chain-completion.txt

  Scenario: FiveM pattern library provides fallback
    Tool: Bash
    Steps:
      1. Create FiveM file where type inference can't determine return type
      2. Request completion after `:`
      3. Assert: FiveM pattern library provides known completions as fallback
    Expected Result: Pattern-matched completions when type inference fails
    Failure Indicators: No completions at all, or crash
    Evidence: .sisyphus/evidence/task-15-fallback.txt
  ```

  **Commit**: YES
    - Message: `feat(complete): add method chain completion`
    - Files: `lsp/completion_resource.go` (extend T12)
    - Pre-commit: `go test ./lsp/ -run TestMethodChainCompletion -count=1`

- [x] 
  **S4-cont.** Whole-Expression Signature Resolution *(absorbed into S4 — see S4 header above)*

  **What to do**:
    - Implement whole-expression signature help in `lsp/signatures.go`:
        - When cursor enters a function call, resolve signatures for ALL nested calls in the expression
        - `MySQL:Await:Execute(query)` → resolve signatures for Await (self: MySQL), then Execute (self: Await return)
        - Each nested call gets full LuaDoc + parameter names + types
        - Handles FiveM-native function calls, export calls, and method chains
    - Integration with T13 (receiver tracking) and T7 (resolver v2)

  **Must NOT do**:
    - Do not implement per-keystroke signature resolution — only when `(` is typed
    - Do not preload all signatures on file open (too wasteful)

  **Recommended Agent Profile**:
    - **Category**: `unspecified-high`
        - Reason: Expression-level analysis with multiple call sites, moderate complexity
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: YES (with T14, T15, T17, T18)
    - **Parallel Group**: Wave 3
    - **Blocks**: T21 (parity validation)
    - **Blocked By**: T13 (needs receiver tracking for chained calls), T7 (needs resolver v2)

  **References**:
    - `lsp/features.go` — Current signature help implementation (if exists) or LSP signature help handler
    - `lsp/infer_v2.go` (from T13) — Receiver tracking for chain resolution
    - `lsp/types_structural.go` (from T2) — FunctionType with parameter and return type info

  **WHY Each Reference Matters**:
    - `signature_help.go`: Current impl to extend or replace
    - `infer_v2.go`: Chain receiver tracking to determine signature context for each call
    - `types_structural.go`: FunctionType definition — what signature data looks like

  **Acceptance Criteria**:
    - [ ] Cursor in `MySQL:Await(` shows full Await signature including self type
    - [ ] Cursor in nested call `MySQL:Await:Execute(query)` shows Execute signature (not Await)
    - [ ] Signatures include LuaDoc, parameter names, parameter types, return type
    - [ ] Export call signatures resolved through T9 export bridge
    - [ ] `go test ./lsp/ -run TestSignatureResolution -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Whole-expression signature resolves chained calls
    Tool: Bash
    Steps:
      1. Open FiveM file with: `MySQL:Await:Execute(query)`
      2. Place cursor inside `Execute(`
      3. Run: go test ./lsp/ -run TestSignatureResolution/Chain -v -count=1
      4. Assert: Shows Execute signature, NOT Await signature
    Expected Result: Correct signature for the call at cursor position
    Failure Indicators: Shows outer call signature, or "unknown" parameters
    Evidence: .sisyphus/evidence/task-16-chain-signature.txt

  Scenario: Export call signature resolved
    Tool: Bash
    Steps:
      1. Open file with: `exports[resource]:methodName(`
      2. Request signature help
      3. Assert: Shows methodName signature from export bridge (T9)
    Expected Result: Full signature with LuaDoc from exporting resource
    Failure Indicators: "unknown" or missing LuaDoc
    Evidence: .sisyphus/evidence/task-16-export-signature.txt
  ```

  **Commit**: YES
    - Message: `feat(lsp): add whole-expression signature resolution`
    - Files: `lsp/signatures.go`, `lsp/signatures_test.go`
    - Pre-commit: `go test ./lsp/ -run TestSignatureResolution -count=1`

- [x] 
  **S4-cont.** Goto Definition via Export Bridge *(absorbed into S4 — see S4 header above)*

  **What to do**:
    - Implement cross-resource goto definition in `lsp/goto_export.go`:
        - When goto-definition targets `exports[resource]:method()`, resolve through T9 export bridge
        - Navigate directly to the target method in the exporting resource's file
        - Handle side propagation: client-side goto only resolves to client resources
        - Handle edge cases: circular export references, dynamic export names (warn, don't crash)
    - For require'd modules: resolve to the target file using GlobalIndex v2 dependency graph (T5)

  **Must NOT do**:
    - Do not lazy-load entire target files on goto (use GlobalIndex metadata)
    - Do not break existing same-file goto-definition behavior

  **Recommended Agent Profile**:
    - **Category**: `unspecified-high`
        - Reason: Cross-resource navigation with scope checking, moderate complexity
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: YES (with T14, T15, T16, T18)
    - **Parallel Group**: Wave 3
    - **Blocks**: T21 (parity validation)
    - **Blocked By**: T9 (needs export bridge for cross-resource resolution), T5 (needs GlobalIndex v2)

  **References**:
    - `lsp/features.go` — Current goto-definition implementation
    - `lsp/export_bridge.go` (from T9) — Export bridge for resolving `exports[resource]:method()`
    - `lsp/global_index_v2.go` (from T5) — Workspace index for require'd module resolution

  **WHY Each Reference Matters**:
    - `definition.go`: Current single-file goto definition — must understand what's being extended
    - `export_bridge.go`: Typed export graph — the resolution engine for cross-resource goto
    - `global_index_v2.go`: Dependency graph for finding require'd files

  **Acceptance Criteria**:
    - [ ] `exports[resource]:method()` → goto navigates to method definition in exporting resource
    - [ ] Side propagation: client-side goto only resolves client-visible exports
    - [ ] `require('module')` → goto navigates to module file
    - [ ] Circular exports: handled gracefully (no infinite loop)
    - [ ] Dynamic export names: warning diagnostic, no crash
    - [ ] `go test ./lsp/ -run TestGotoExport -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Goto definition on export call
    Tool: Bash
    Steps:
      1. Open file with `exports[my_resource]:myMethod()`
      2. Place cursor on `myMethod`
      3. Trigger goto definition
      4. Assert: Navigates to myMethod in my_resource's file
    Expected Result: Correct file and line number of method definition
    Failure Indicators: "No definition found" or wrong location
    Evidence: .sisyphus/evidence/task-17-goto-export.txt

  Scenario: Side-aware goto (client can't goto server-only export)
    Tool: Bash
    Steps:
      1. Open a client script file
      2. Try goto on a server-only export
      3. Assert: Returns "no definition found" or diagnostic
    Expected Result: Correct scope restriction
    Failure Indicators: Navigates to server-side definition from client context
    Evidence: .sisyphus/evidence/task-17-side-goto.txt
  ```

  **Commit**: YES
    - Message: `feat(lsp): add goto definition via export bridge`
    - Files: `lsp/goto_export.go`, `lsp/goto_export_test.go`
    - Pre-commit: `go test ./lsp/ -run TestGotoExport -count=1`

- [x] 
  **S3-cont.** Staged Parallel Workspace Warm-up *(absorbed into S3 — see S3 header above)*

  **What to do**:
    - Create `lsp/warmup.go` implementing staged parallel workspace warm-up:
        - **Phase 1**: Scan all FiveM manifests (fx_version, game, scripts lists) — fast, O(n) file reads
        - **Phase 2**: Build dependency graph in topological order (using T5 GlobalIndex v2)
        - **Phase 3**: Resolve types in topological order — each resource's types resolved after its dependencies
        - All phases use bounded parallelism (goroutine pool from T10)
        - Handle edge cases: circular dependencies (skip and warn), indexing during active editing (queue changes)
    - Target: ~5s cold start for 3000-file workspace (acceptable per user decision)

  **Must NOT do**:
    - Do not resolve types in Phase 1 (declarations only — per Metis guardrail)
    - Do not index require() chains in Phase 1 (FiveM resource deps only — per SC4)
    - Do not block the LSP server during warm-up (background process)

  **Recommended Agent Profile**:
    - **Category**: `unspecified-high`
        - Reason: Parallel scheduling with dependency ordering, complex edge case handling
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: YES (with T14, T15, T16, T17)
    - **Parallel Group**: Wave 3
    - **Blocks**: T21 (parity validation)
    - **Blocked By**: T5 (needs GlobalIndex v2 for dependency graph), T10 (uses prefetch goroutine pool)

  **References**:
    - `lsp/workspace.go:743-824` — Current workspace scanning patterns (byte comparison, no string allocation)
    - `lsp/fivem.go:1100-1373` — Manifest parsing that Phase 1 uses
    - `lsp/global_index_v2.go` (from T5) — Dependency graph for Phase 2
    - `lsp/prefetch.go` (from T10) — Goroutine pool for parallel execution

  **WHY Each Reference Matters**:
    - `workspace.go:743-824`: Established patterns for fast workspace scanning — must follow (byte comparison)
    - `fivem.go:1100-1373`: Manifest parsing logic — Phase 1 must use this
    - `global_index_v2.go`: Dependency graph determines topological order for Phase 2-3
    - `prefetch.go`: Bounded goroutine pool for parallel warm-up execution

  **Acceptance Criteria**:
    - [ ] Phase 1 (manifests) completes in < 500ms for 3000-file workspace
    - [ ] Phase 2 (dependency graph) completes in < 1s for 3000-file workspace
    - [ ] Phase 3 (type resolution) completes in < 3.5s for 3000-file workspace
    - [ ] Total cold start < 5s for 3000-file workspace
    - [ ] Circular dependencies detected and warned (no infinite loop)
    - [ ] Edits during indexing are queued and applied after warm-up completes
    - [ ] `go test ./lsp/ -run TestWarmup -count=1` → PASS
    - [ ] `go test ./lsp/ -bench=BenchmarkWarmup -benchmem` → < 5s for 3000 files

  **QA Scenarios**:

  ```
  Scenario: Cold start completes within budget
    Tool: Bash
    Steps:
      1. Prepare 3000-file FiveM workspace fixture
      2. Run: go test ./lsp/ -bench=BenchmarkWarmup -benchmem -benchtime=1x -count=1
      3. Assert: Total cold start time < 5s
    Expected Result: Workspace fully indexed and ready within 5 seconds
    Failure Indicators: Cold start > 5 seconds
    Evidence: .sisyphus/evidence/task-18-benchmark.txt

  Scenario: Circular dependencies handled gracefully
    Tool: Bash
    Steps:
      1. Create workspace with A depends on B, B depends on A
      2. Run: go test ./lsp/ -run TestWarmup/CircularDeps -v -count=1
      3. Assert: No infinite loop, circular dep detected, warning issued
    Expected Result: Graceful handling with diagnostic warning
    Failure Indicators: Infinite loop, hang, or crash
    Evidence: .sisyphus/evidence/task-18-circular.txt

  Scenario: Edits during indexing queued correctly
    Tool: Bash
    Steps:
      1. Start workspace warm-up
      2. Before warm-up completes, simulate didChange for a file
      3. Assert: Change queued and applied after warm-up without data loss
    Expected Result: Edit applied correctly after warm-up, no stale data
    Failure Indicators: Lost edits, corrupted state, or crash
    Evidence: .sisyphus/evidence/task-18-concurrent-edit.txt
  ```

  **Commit**: YES
    - Message: `feat(server): implement staged parallel workspace warm-up`
    - Files: `lsp/warmup.go`, `lsp/warmup_test.go`
    - Pre-commit: `go test ./lsp/ -run TestWarmup -count=1`

- [x] 
  **X1.** Progressive Cutover & Validation *(merges T19 + T20 + T21 + T22)*

  **What to do**:
    - Create `lsp/eviction.go` implementing LRU eviction:
        - Evict source text strings from closed files when memory exceeds configurable 256MB limit
        - Keep ALL symbol metadata in GlobalIndex v2 (P1/P9 enrichment) — LuaDoc, types, signatures
        - Measurement: count AST nodes + type metadata + source strings toward limit (NOT just source)
        - When a file is evicted (source removed), its symbol metadata REMAINS in GlobalIndex
        - If a request needs the evicted file's source (e.g., goto-definition), lazily reload it
        - Handle edge case from Metis: eviction during active request — check if file is needed before evicting
    - Configurable limit via `memoryLimitMB` setting (default 256)

  **Must NOT do**:
    - Do not evict AST or symbol metadata (only source strings)
    - Do not evict currently-open files
    - Do not break cross-file features when a file's source is evicted (metadata stays)

  **Recommended Agent Profile**:
    - **Category**: `quick`
        - Reason: LRU eviction is a well-understood pattern, moderate complexity on top of existing GlobalIndex
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: NO — depends on T5 (GlobalIndex v2)
    - **Parallel Group**: Wave 4
    - **Blocks**: T20 (progressive replacement needs eviction policy)
    - **Blocked By**: T5 (needs GlobalIndex v2 with enriched metadata)

  **References**:
    - `lsp/server.go:Document management` — Current document open/close handling
    - `lsp/global_index_v2.go` (from T5) — Enriched GlobalIndex where symbol metadata is preserved
    - `lsp/document.go` — Current Document struct that holds Source, Tree, Resolver

  **WHY Each Reference Matters**:
    - `server.go`: Where documents are tracked, opened, and closed — eviction hook goes here
    - `global_index_v2.go`: Enriched metadata that MUST NOT be evicted
    - `document.go`: Current Document fields — source is what gets evicted, Tree/Resolver stay

  **Acceptance Criteria**:
    - [ ] Source text evicted when memory limit exceeded
    - [ ] Symbol metadata (LuaDoc, types, signatures) preserved after eviction
    - [ ] Hover/goto-definition on evicted file works via lazy reload
    - [ ] Open files never evicted
    - [ ] Configurable memory limit (default 256MB)
    - [ ] Eviction during active request handled gracefully (check before evicting)
    - [ ] `go test ./lsp/ -run TestLRUEviction -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Source evicted, metadata preserved
    Tool: Bash
    Steps:
      1. Index workspace, hover on function in file B
      2. Close file B, trigger eviction (simulate memory pressure)
      3. Assert: Source for B evicted, but symbol metadata for B still in GlobalIndex
      4. Hover on same function in file A → still works
    Expected Result: Cross-file hover works even after source eviction
    Failure Indicators: "Unknown type" or missing LuaDoc after eviction
    Evidence: .sisyphus/evidence/task-19-metadata-preserved.txt

  Scenario: Lazy reload on access
    Tool: Bash
    Steps:
      1. Evict file B's source
      2. Request goto-definition targeting file B
      3. Assert: File B's source lazily reloaded, goto works
    Expected Result: Transparent reload, correct goto target
    Failure Indicators: "File not found" or stale/partial data
    Evidence: .sisyphus/evidence/task-19-lazy-reload.txt

  Scenario: Open files never evicted
    Tool: Bash
    Steps:
      1. Open file A, simulate memory pressure beyond limit
      2. Assert: File A's source NOT evicted
      3. Only closed files' sources evicted
    Expected Result: Open files immune to eviction
    Failure Indicators: Currently-open file's source evicted
    Evidence: .sisyphus/evidence/task-19-open-files.txt
  ```

  **Commit**: YES
    - Message: `feat(memory): add LRU eviction (source only)`
    - Files: `lsp/eviction.go`, `lsp/eviction_test.go`
    - Pre-commit: `go test ./lsp/ -run TestLRUEviction -count=1`

- [x] 
  **X1-cont.** Progressive Module Replacement in Server *(absorbed into X1 — see X1 header above)*

  **What to do**:
    - Replace old modules with new ones in `lsp/server.go` one at a time:
        - Use the compatibility shim from T6 to delegate from old API to new API
        - Order of replacement: GlobalIndex → TypeSystem → Resolver → FiveM → Inference
        - Each replacement is a single PR with a clear cutover
        - After each replacement, run full parity test suite (T6) to verify no regression
        - Remove old code only after ALL parity tests pass
    - Each module replacement must be self-contained and testable independently
    - Max 2 weeks dual-maintenance per module (guardrail G6)

  **Must NOT do**:
    - Do not replace all modules at once (progressive, one at a time)
    - Do not remove old code before parity tests pass
    - Do not extend dual-maintenance beyond 2 weeks per module

  **Recommended Agent Profile**:
    - **Category**: `deep`
        - Reason: Critical integration work, each replacement affects multiple dependent modules
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: NO — depends on T6, T7, T5
    - **Parallel Group**: Wave 4
    - **Blocks**: T21 (parity validation), T22 (perf validation)
    - **Blocked By**: T6 (needs compatibility shim), T7 (needs resolver v2), T5 (needs GlobalIndex v2)

  **References**:
    - `lsp/server.go:27-128` — Server struct fields to be replaced progressively
    - `lsp/compat.go` (from T6) — Compatibility shim for delegation
    - `.sisyphus/drafts/MIGRATION.md` (from T1) — Field audit showing what stays vs replaces
    - `lsp/parity_test.go` (from T6) — Feature parity tests to run after each replacement

  **WHY Each Reference Matters**:
    - `server.go:27-128`: Each field that gets replaced must be handled via compat shim
    - `compat.go`: Wrappers that delegate old→new during transition
    - `MIGRATION.md`: The roadmap showing which fields stay and which get replaced
    - `parity_test.go`: Tests that MUST pass after each module replacement

  **Acceptance Criteria**:
    - [ ] GlobalIndex replaced: old `map[GlobalKey][]GlobalSymbol` → new `GlobalIndexV2`
    - [ ] TypeSystem replaced: old `TypeSet` bitmask → new hybrid bitmask+structural
    - [ ] Resolver replaced: old single-pass → new two-pass v2
    - [ ] FiveM replaced: old bolt-on → new first-class integration
    - [ ] Inference replaced: old `infer.go` → new `infer_v2.go`
    - [ ] Parity tests pass after EACH replacement: `go test ./lsp/ -run TestParity -count=1`
    - [ ] FeatureFiveM=false path works after EACH replacement
    - [ ] `go build ./lsp/` → PASS after each replacement

  **QA Scenarios**:

  ```
  Scenario: GlobalIndex replacement parity
    Tool: Bash
    Steps:
      1. Replace GlobalIndex with GlobalIndexV2 via compat shim
      2. Run: go test ./lsp/ -run TestParity/GlobalIndex -v -count=1
      3. Assert: Identical results for hover, completion, goto-def, find-refs
    Expected Result: New GlobalIndex produces same results as old
    Failure Indicators: Any difference in LSP feature output
    Evidence: .sisyphus/evidence/task-20-globalindex-parity.txt

  Scenario: FeatureFiveM=false path works after each replacement
    Tool: Bash
    Steps:
      1. Set FeatureFiveM = false
      2. Run: go test ./lsp/ -run TestParity/FeatureToggle -v -count=1
      3. Assert: All non-FiveM features work identically
    Expected Result: Non-FiveM path unaffected by module replacement
    Failure Indicators: FiveM code invoked, or missing features
    Evidence: .sisyphus/evidence/task-20-feature-toggle.txt

  Scenario: Full build after all replacements
    Tool: Bash
    Steps:
      1. Replace all modules: GlobalIndex, TypeSystem, Resolver, FiveM, Inference
      2. Run: go build ./lsp/ && go test ./lsp/ -count=1
      3. Assert: Clean build, all tests pass
    Expected Result: Clean build and all tests passing
    Failure Indicators: Build errors or test failures
    Evidence: .sisyphus/evidence/task-20-full-build.txt
  ```

  **Commit**: YES (multiple commits, one per module replacement)
    - Message: `feat(server): progressive module replacement — GlobalIndex`
    - Files: `lsp/server.go`, `lsp/compat.go`
    - Pre-commit: `go test ./lsp/ -run TestParity -count=1`

- [x] 
  **X1-cont.** Feature Parity Validation Suite *(absorbed into X1 — see X1 header above)*

  **What to do**:
    - Create comprehensive feature parity validation suite in `lsp/parity_v2_test.go`:
        - For EACH LSP feature (hover, completion, goto-def, find-refs, signature-help, code-lens):
            - Test with FiveM=true: all new features work (natives, exports, colon methods, chains)
            - Test with FiveM=false: identical output to old system
        - Cross-module integration tests:
            - Type inference + export bridge + goto (hover on chained export call, goto on method)
            - Method chains + completion (complete after chain, with signature help)
            - Warm-up + prefetch + hover (file open → prefetch → instant hover)
        - Edge case tests from Metis:
            - Circular FiveM resource dependencies
            - Shared scripts in multiple scopes
            - Native bundle selection edge cases
            - Workspace indexing during active editing
            - Dynamic export names

  **Must NOT do**:
    - Do not modify existing test files — add new ones only
    - Do not skip edge cases — Metis identified important ones

  **Recommended Agent Profile**:
    - **Category**: `deep`
        - Reason: Comprehensive test suite design covering all integration paths and edge cases
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: NO — depends on ALL Wave 3 tasks completing
    - **Parallel Group**: Wave 4
    - **Blocks**: F1-F4 (final verification reviews)
    - **Blocked By**: T13, T14, T15, T16, T17, T18 (all Wave 3 tasks)

  **References**:
    - `lsp/fivem_fixture_harness_test.go:172` — Test harness pattern to follow
    - `lsp/parity_test.go` (from T6) — Parity test infrastructure to extend
    - All Wave 2 and Wave 3 task deliverables — Features being tested

  **Why Each Reference Matters**:
    - `fivem_fixture_harness_test.go`: Established test pattern — must follow for consistency
    - `parity_test.go`: Existing parity framework to extend with integration and edge case tests

  **Acceptance Criteria**:
    - [ ] `lsp/parity_v2_test.go` created with integration tests
    - [ ] Hover+export+goto integration test passes
    - [ ] Chain+completion+signature integration test passes
    - [ ] Warm-up+prefetch+hover integration test passes
    - [ ] Circular dep edge case test passes
    - [ ] Shared scripts in multiple scopes edge case passes
    - [ ] Native bundle edge case passes
    - [ ] `go test ./lsp/ -run TestParityV2 -count=1` → ALL PASS

  **QA Scenarios**:

  ```
  Scenario: Full integration test — hover on chained export
    Tool: Bash
    Steps:
      1. Create workspace with resource A exporting `method1`, resource B calling `exports[A]:method1():method2()`
      2. Hover on `method2` in resource B
      3. Assert: Full type + LuaDoc from resource A, resolved through export bridge
    Expected Result: Correct hover with type info from cross-resource export chain
    Failure Indicators: Missing type, "unknown", or incomplete LuaDoc
    Evidence: .sisyphus/evidence/task-21-integration-hover.txt

  Scenario: Circular resource dependency edge case
    Tool: Bash
    Steps:
      1. Create workspace where resource A depends on B, B depends on A
      2. Index workspace (warm-up should handle this)
      3. Assert: No crash, circular dep detected, symbolic metadata still available
    Expected Result: Graceful handling, no infinite loop
    Failure Indicators: Crash, hang, or corrupted metadata
    Evidence: .sisyphus/evidence/task-21-circular-dep.txt

  Scenario: Feature parity with FeatureFiveM=false
    Tool: Bash
    Steps:
      1. Run full parity suite with FeatureFiveM=false
      2. Assert: All non-FiveM features produce identical output to old system
    Expected Result: 100% feature parity for non-FiveM path
    Failure Indicators: Any difference in non-FiveM LSP features
    Evidence: .sisyphus/evidence/task-21-parity-fivem-off.txt
  ```

  **Commit**: YES
    - Message: `test(parity): add comprehensive feature parity validation suite`
    - Files: `lsp/parity_v2_test.go`
    - Pre-commit: `go test ./lsp/ -run TestParityV2 -count=1`

- [x] 
  **X1-cont.** Performance Budget Validation Per Phase *(absorbed into X1 — see X1 header above)*

  **What to do**:
    - Create performance validation benchmarks in `lsp/perf_budget_test.go`:
        - Per-phase budget:
            - GlobalIndex v2: < 200ms overhead for 3000 files
            - Type system: < 50ms per type inference (hybrid bitmask+structural)
            - Resolver v2: < 100ms for two-pass resolution
            - Tree-diff incremental: < 2ms per edit (vs current full reparse)
            - Full workspace warm-up: < 5s for 3000 files
        - Per-request budget:
            - Hover: < 20ms after warm-up
            - Completion: < 20ms after warm-up
            - Goto-definition: < 20ms after warm-up
        - Memory budget:
            - Total < 256MB for 3000 files (with LRU eviction active)
        - Run benchmarks against both old and new systems for comparison

  **Must NOT do**:
    - Do not set budgets that can't be measured (e.g., "fast enough")
    - Do not skip memory measurement

  **Recommended Agent Profile**:
    - **Category**: `unspecified-high`
        - Reason: Performance benchmark design with Go testing framework
    - **Skills**: []

  **Parallelization**:
    - **Can Run In Parallel**: YES (with T19, T20, T21)
    - **Parallel Group**: Wave 4
    - **Blocks**: F1-F4 (final verification reviews)
    - **Blocked By**: T20 (needs progressive replacement complete for fair benchmarking)

  **References**:
    - `lsp/parity_bench_test.go` (from T6) — Performance comparison benchmarks
    - Current benchmark: 746ms/3042 files cold — this is the baseline
    - User-accepted target: ~5s cold start, <20ms per-request

  **Why Each Reference Matters**:
    - `parity_bench_test.go`: Existing benchmark infrastructure to extend
    - Current baseline: Performance must not regress beyond accepted targets

  **Acceptance Criteria**:
    - [ ] Per-phase benchmarks all within budget
    - [ ] Per-request benchmarks all < 20ms
    - [ ] Total memory < 256MB for 3000-file workspace
    - [ ] Comparison benchmarks show improvement over baseline where applicable
    - [ ] `go test ./lsp/ -bench=BenchmarkPerfBudget -benchmem` → all within budget

  **QA Scenarios**:

  ```
  Scenario: Per-request latency within budget
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -bench=BenchmarkHoverLatency -benchmem -count=5
      2. Run: go test ./lsp/ -bench=BenchmarkCompletionLatency -benchmem -count=5
      3. Assert: Average latency < 20ms for each
    Expected Result: All per-request operations under 20ms
    Failure Indicators: Any operation consistently > 20ms
    Evidence: .sisyphus/evidence/task-22-per-request.txt

  Scenario: Memory within budget
    Tool: Bash
    Steps:
      1. Index 3000-file FiveM workspace
      2. Measure total memory via runtime.ReadMemStats
      3. Assert: Total alloc < 256MB
    Expected Result: Memory usage within configurable limit
    Failure Indicators: Total allocation exceeding 256MB
    Evidence: .sisyphus/evidence/task-22-memory.txt

  Scenario: Cold start within budget
    Tool: Bash
    Steps:
      1. Run: go test ./lsp/ -bench=BenchmarkWarmup -benchmem -benchtime=1x
      2. Assert: Total warm-up time < 5s for 3000 files
    Expected Result: Workspace fully indexed and ready within 5 seconds
    Failure Indicators: Cold start exceeding 5 seconds
    Evidence: .sisyphus/evidence/task-22-cold-start.txt
  ```

  **Commit**: YES
    - Message: `test(perf): add performance budget validation`
    - Files: `lsp/perf_budget_test.go`
    - Pre-commit: `go test ./lsp/ -bench=BenchmarkPerfBudget -benchmem`

---

- [x] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, run command). For each "Must
  NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in
  .sisyphus/evidence/. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — `unspecified-high`
  Run `go vet ./lsp/` + `go test ./lsp/ -count=1` + `go test ./lsp/ -bench=. -benchmem`. Review all changed files for:
  `as any`/type assertions without ok check, empty catches, fmt.Println in prod, commented-out code, unused imports.
  Check AI slop: excessive comments, over-abstraction, generic names (data/result/item/temp).
  Output: `Build [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [x] F3. **Real Manual QA** — `unspecified-high`
  Start from clean state. Execute EVERY QA scenario from EVERY task — follow exact steps, capture evidence. Test
  cross-task integration (type inference + export bridge + goto, method chains + completion, etc.). Test edge cases:
  circular deps, shared scripts in multiple scopes, workspace editing during indexing. Save to
  `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [x] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff (git log/diff). Verify 1:1 — everything in spec was built (no
  missing), nothing beyond spec was built (no creep). Check "Must NOT do" compliance. Detect cross-task contamination:
  Task N touching Task M's files. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy (Grouped by 11 Optimized Tasks)

**Wave 1 (Core Skeleton):**

- **C1**: `docs(migration): add Server/Document field audit and migration plan` — MIGRATION.md + compat.go +
  parity_test.go
- **C2**: `feat(types): add hybrid type system with bitmask primitives and type interning` — types_structural.go +
  type_pool.go + tests
- **C3**: `feat(ast): add SemanticData side table for modular semantic annotations` — semantic_data.go + tests
- **C4**: `feat(index): add GlobalIndex v2 with hierarchical scope partition and dependency graph` —
  global_index_v2.go + tests

**Wave 2 (Engine + Producers):**

- **E1**: `feat(resolver): implement two-pass resolver v2` — resolver_v2.go + tests
- **S1**: `feat(fivem): integrate native catalog and export bridge with structural types` — fivem_natives.go +
  export_bridge.go + tests
- **S3**: `feat(server): add proactive data layer — prefetch, completion tables, warm-up` — prefetch.go +
  completion_resource.go + warmup.go + tests
- **S5**: `feat(parser): add tree-diff incremental parser` — treediff.go + tests

**Wave 3 (Consumers):**

- **S2**: `feat(infer): add type inference engine — colon methods, assignments, table literals` — infer_v2.go + tests
- **S4**: `feat(lsp): add LSP feature extensions — chain completion, signatures, goto exports` —
  completion_resource.go (extend) + signatures.go + goto_export.go + tests

**Wave 4 (Endgame):**

- **X1**: `feat(server): progressive cutover — eviction, module replacement, parity + perf validation` — eviction.go +
  server.go migration + parity_v2_test.go + perf_budget_test.go

---

## Success Criteria

### Verification Commands

```bash
go test ./lsp/ -count=1                                    # ALL tests pass
go test ./lsp/ -run TestFiveM -count=1                     # FiveM-specific tests pass
go test ./lsp/ -bench=BenchmarkFiveMIndex -benchmem        # Cold start < 5s
go test ./lsp/ -bench=BenchmarkHoverLatency -benchmem      # Per-request < 20ms
go test ./lsp/ -bench=BenchmarkIncrementalEdit -benchmem   # Incremental < full reparse
go vet ./lsp/                                               # No vet issues
```

### Final Checklist

- [x] All "Must Have" present and working
- [x] All "Must NOT Have" absent from codebase
- [x] All existing tests pass (FeatureFiveM=true AND FeatureFiveM=false)
- [x] Feature parity: hover, completion, goto-def, find-refs produce same results
- [x] Cold start < 5s for 3000-file FiveM workspace
- [x] Per-request < 20ms for hover/completion/goto
- [x] Memory budget within 256MB for 3000-file workspace
- [x] Progressive migration: old and new coexist for ≤2 weeks per module
- [x] Tree-diff produces identical AST to full reparse (property-based test)