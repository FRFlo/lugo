# FiveM Parser Memory & LSP Optimization

## TL;DR

> **Quick Summary**: Optimize the FiveM LSP-layer by fixing invalidation bugs first, then aggressively reducing memory across duplicated FiveM state, native bundle loading, closed-document retention, and GlobalIndex growth — while preserving all LSP feature correctness.
>
> **Deliverables**:
> - Fixed cache invalidation for FiveM manifests, feature toggles, and file deletes
> - Deduplicated FiveM manifest/resource/graph data (single canonical owner)
> - Lazy native bundle loading (removal from LibraryPaths, rely on existing on-demand path)
> - Source+TypeCache eviction for closed documents with nil-safety and on-demand re-read
> - GlobalIndex compaction after document removal
> - Memory profiling baseline and benchmark improvements
>
> **Estimated Effort**: Large
> **Parallel Execution**: YES — 6 waves
> **Critical Path**: Wave 1 (invalidation) → Wave 2 (dedup) → Wave 3 (lazy natives) → Wave 4 (closed doc) → Wave 5 (GlobalIndex) → Wave FINAL

---

## Context

### Original Request
Optimize the newly introduced FiveM parser. Reduce drastically the memory usage and optimize the LSP.

### Interview Summary
**Key Discussions**:
- Memory target: optimize aggressively across all hotspots, validate with existing perf budget tests
- Native bundles: lazy with warm cache — load on first reference, keep in memory after
- Invalidation scope: fix invalidation bugs first, then optimize memory
- Closed document policy: moderate — drop Source+TypeCache for closed docs, keep AST+Resolver
- Test strategy: tests-after, validate with existing `TestFiveMPerfBudgets` and allocation benchmarks

**Research Findings**:
- FiveM is an LSP-layer feature, not a separate parser — optimization targets are document finalization, global symbol indexing, resource/profile resolution, native-bundle indexing
- Data flow: Source → workspace updateDocument → parser → AST flat arrays → resolver → finalizeDocumentUpdate → global symbol registration + FiveM classification → LSP features
- Hot paths: `finalizeDocumentUpdate`, `refreshWorkspace`, `setGlobalSymbol`, `InferType`, completion, diagnostics publish loop
- Memory hotspots: triple-stored FiveM state (Manifest→Resource→GraphNode), per-document retention of Source+Tree+Resolver+TypeCache+LuaDocCache+ActualReads+MutatedLocals, eager native bundle loading, unbounded GlobalIndex growth

### Metis Review
**Identified Gaps (addressed)**:
- **CRITICAL**: `doc.Source` and `doc.Tree.Source` share the same `[]byte` backing array — niling `doc.Source` alone saves zero bytes. Must nil both or restructure ownership.
- **CRITICAL**: Cross-document Source access at `symbols.go:405`, `infer.go:754`, `infer.go:1136` will crash if `targetDoc.Source` is nil. Must add nil-safety guards or on-demand re-read.
- **HIGH**: GlobalIndex is the largest unbounded memory consumer after Source — needs compaction after document removal.
- **MEDIUM**: FeatureFiveM toggle "bug" may be a no-op by design (setCfg returns early when value unchanged) — verify before fixing.
- **MEDIUM**: Native bundles already have partial lazy loading via `ensureFiveMNativeBundleLoaded` — the optimization is simpler than building new infrastructure, just remove from `LibraryPaths`.
- **MEDIUM**: `FiveMManifestEntry` is value-copied into `FiveMResourceGraphExpansion` — should use pointers.

---

## Work Objectives

### Core Objective
Drastically reduce FiveM LSP memory usage and improve responsiveness by fixing invalidation correctness, deduplicating FiveM state, implementing lazy native loading, evicting closed-document caches, and compacting GlobalIndex — without regressing any LSP feature.

### Concrete Deliverables
- Fixed FiveM cache invalidation for manifest deletes, file deletes, and feature toggles
- Single canonical owner for FiveM manifest/resource state (eliminate triple-copy)
- Lazy native bundle loading (remove from LibraryPaths, rely on `ensureFiveMNativeBundleLoaded`)
- Source+TypeCache eviction for closed documents with nil-safety across all access sites
- GlobalIndex compaction after document removal
- Memory profiling baseline + per-document footprint benchmark

### Definition of Done
- [ ] `go test ./... -race` passes
- [ ] `go test -run TestFiveMPerfBudgets -v ./lsp/` passes
- [ ] All new invalidation tests pass (manifest delete, file delete, toggle)
- [ ] Benchmark regression test shows measurable allocation reduction vs baseline
- [ ] No nil pointer dereferences in cross-document features when Source is evicted

### Must Have
- FiveM invalidation correctness for manifest delete, file delete, and feature toggle
- Deduplicated FiveM state (no triple-copy)
- Lazy native bundles
- Closed-document Source+TypeCache eviction with nil-safety
- GlobalIndex compaction
- Measurable memory reduction in benchmarks

### Must NOT Have (Guardrails)
- No dropping Source without also handling Tree.Source (shared backing array)
- No assuming "keep AST+Resolver" is safe when Source is dropped — Tree.Source must also be handled
- No breaking cross-document features (hover, go-to-def, call hierarchy, type inference) when Source is evicted
- No regressing existing FiveM LSP test suite
- No regressing existing perf budget tests
- No modifying `clearDocument` in-place — new "close but keep some data" path must be a separate function
- No fixing Bug 2 (FeatureFiveM toggle) until verified as a real bug vs no-op-by-design
- No building new lazy-loading infrastructure for native bundles — use existing `ensureFiveMNativeBundleLoaded`
- No AI slop: excessive comments, over-abstraction, generic naming, unnecessary doc comments

---

## Verification Strategy (MANDATORY)

> **ZERO HUMAN INTERVENTION** — ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: YES
- **Automated tests**: Tests-after (add tests after implementation)
- **Framework**: Go `testing` package + `go test -bench -benchmem`

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **LSP features**: Use Go tests — table-driven test cases with fixture workspaces
- **Memory benchmarks**: Use `go test -bench -benchmem` + `runtime.ReadMemStats`
- **Invalidation correctness**: Use Go tests with manifest delete/create/toggle scenarios

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately — invalidation fixes, correctness first):
├── Task 1: Fix manifest delete invalidation [deep]
├── Task 2: Fix file delete invalidation (clearDocument FiveM cleanup) [deep]
└── Task 3: Add invalidation regression tests [quick]

Wave 2 (After Wave 1 — deduplicate FiveM state):
├── Task 4: Restructure FiveMManifestEntry to pointer references [deep]
├── Task 5: Consolidate FiveMResource maps (eliminate redundancy with graph) [deep]
└── Task 6: Add deduplication regression tests [quick]

Wave 3 (After Wave 2 — lazy native bundles):
├── Task 7: Remove native bundles from LibraryPaths, rely on ensureFiveMNativeBundleLoaded [quick]
├── Task 8: Audit all native symbol resolution paths for ensureFiveMNativeBundleLoaded coverage [unspecified-high]
└── Task 9: Add lazy native bundle tests [quick]

Wave 4 (After Wave 3 — closed document eviction):
├── Task 10: Restructure Source ownership (Tree owns, Document borrows) [deep]
├── Task 11: Implement evictClosedDocumentCaches (Source+TypeCache) [deep]
├── Task 12: Add nil-safety guards at cross-document Source access sites [deep]
└── Task 13: Add closed document eviction tests [quick]

Wave 5 (After Wave 4 — GlobalIndex compaction):
├── Task 14: Add GlobalIndex compaction after document removal [deep]
└── Task 15: Add GlobalIndex compaction tests [quick]

Wave 6 (After Waves 1-5 — profiling baselines):
├── Task 16: Add memory profiling baseline and per-document footprint benchmark [quick]
└── Task 17: Update TestFiveMPerfBudgets with new thresholds [quick]

Wave FINAL (After ALL tasks — 4 parallel reviews, then user okay):
├── F1: Plan compliance audit (oracle)
├── F2: Code quality review (unspecified-high)
├── F3: Full test suite + benchmark verification (unspecified-high)
└── F4: Scope fidelity check (deep)
→ Present results → Get explicit user okay
```

### Dependency Matrix

| Task | Depends On | Blocks |
|------|-----------|--------|
| 1 | — | 3, 4, 5 |
| 2 | — | 3, 4, 5 |
| 3 | 1, 2 | 4, 5 |
| 4 | 3 | 6, 7 |
| 5 | 3 | 6, 7 |
| 6 | 4, 5 | 7 |
| 7 | 6 | 8, 9 |
| 8 | 7 | 9 |
| 9 | 7, 8 | 10 |
| 10 | 9 | 11, 12 |
| 11 | 10 | 12, 13 |
| 12 | 10 | 13 |
| 13 | 11, 12 | 14 |
| 14 | 13 | 15 |
| 15 | 14 | 16, 17 |
| 16 | 15 | 17 |
| 17 | 16 | FINAL |
| F1 | 17 | — |
| F2 | 17 | — |
| F3 | 17 | — |
| F4 | 17 | — |

### Agent Dispatch Summary

| Wave | Count | Dispatch |
|------|-------|----------|
| 1 | 3 | T1 → `deep`, T2 → `deep`, T3 → `quick` |
| 2 | 3 | T4 → `deep`, T5 → `deep`, T6 → `quick` |
| 3 | 3 | T7 → `quick`, T8 → `unspecified-high`, T9 → `quick` |
| 4 | 4 | T10 → `deep`, T11 → `deep`, T12 → `deep`, T13 → `quick` |
| 5 | 2 | T14 → `deep`, T15 → `quick` |
| 6 | 2 | T16 → `quick`, T17 → `quick` |
| FINAL | 4 | F1 → `oracle`, F2 → `unspecified-high`, F3 → `unspecified-high`, F4 → `deep` |

---

## TODOs

- [x] 1. Fix manifest delete invalidation

  **What to do**:
  - In `lsp/workspace.go` `handleDidChangeWatchedFiles`: when a `fxmanifest.lua` or `__resource.lua` file is deleted, before calling `clearDocument`, also:
    1. Look up the `FiveMResource` by document URI
    2. Remove it from `s.FiveMResources` and `s.FiveMResourceByName`
    3. Call `s.FiveMResourceGraph.removeNode()` to clean the graph
    4. Invalidate `FiveMProfileCached` on all documents under the deleted manifest's resource root
  - In `lsp/fivem.go`: add a public function `removeFiveMResource(server *Server, uri URI)` that performs steps 1-4 above
  - Call this function from the appropriate delete path in `handleDidChangeWatchedFiles`

  **Must NOT do**:
  - Modify `clearDocument` in-place — the new "remove FiveM resource" function is separate
  - Fix FeatureFiveM toggle invalidation (that's verified separately in Task 2 scope)
  - Add new data structures — reuse existing `FiveMResourceByName` and `FiveMResourceGraph`

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Requires understanding cross-module invalidation logic between workspace file watching and FiveM resource graph
  - **Skills**: []
    - No specialized skills needed — this is Go code modification within existing patterns

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 2)
  - **Parallel Group**: Wave 1 (with Tasks 2, 3)
  - **Blocks**: Tasks 3, 4, 5
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `lsp/workspace.go:handleDidChangeWatchedFiles` — File delete handling path; currently calls `clearDocument` without FiveM cleanup
  - `lsp/workspace.go:clearDocument` — Canonical document removal function (do NOT modify in-place)
  - `lsp/fivem.go:registerFiveMManifestResource` — Inverse operation (registration); follow the same fields for removal

  **API/Type References**:
  - `lsp/server.go:Server.FiveMResources` — Map keyed by URI to remove from
  - `lsp/server.go:Server.FiveMResourceByName` — Map keyed by resource name to remove from
  - `lsp/fivem.go:FiveMResourceGraph` — Has `ByRoot`, `ByName`, `ByProvide` maps and `removeNode` method
  - `lsp/document.go:Document.FiveMProfileCached` — Bool field to invalidate on affected docs

  **External References**: None

  **WHY Each Reference Matters**:
  - `handleDidChangeWatchedFiles`: The entry point where manifest deletes are currently handled incorrectly (missing FiveM cleanup)
  - `registerFiveMManifestResource`: Shows exactly which data structures are populated during registration; removal must clean the same set
  - `FiveMResourceGraph.removeNode`: Already exists but is NOT called on file delete — this is the gap

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Manifest file delete cleans up FiveM state
    Tool: Bash (go test)
    Preconditions: Workspace with a FiveM resource containing fxmanifest.lua
    Steps:
      1. Create a FiveM workspace fixture with `fxmanifest.lua` and at least one Lua file under the resource root
      2. Index the workspace, verify `FiveMResources` contains the resource entry keyed by manifest URI
      3. Delete the manifest file via the file watcher event
      4. Assert `FiveMResources` no longer contains the key
      5. Assert `FiveMResourceByName` no longer contains the resource name
      6. Assert `FiveMResourceGraph.ByRoot` no longer has the manifest root
      7. Assert all documents under the deleted manifest root have `FiveMProfileCached == false`
    Expected Result: All FiveM state for the deleted manifest is fully removed; affected documents have invalidated profiles
    Failure Indicators: Any FiveM map still contains the deleted manifest key; any doc still has FiveMProfileCached == true
    Evidence: .sisyphus/evidence/task-1-manifest-delete-cleanup.txt

  Scenario: Manifest delete does not affect unrelated resources
    Tool: Bash (go test)
    Preconditions: Workspace with two independent FiveM resources
    Steps:
      1. Create fixture with two separate resource directories, each with its own fxmanifest.lua
      2. Index the workspace
      3. Delete one manifest file
      4. Assert the OTHER resource's FiveM state is fully intact
      5. Assert the OTHER resource's documents still have valid FiveMProfileCached
    Expected Result: Unrelated resource is completely unaffected by the delete
    Failure Indicators: Other resource's state was partially or fully cleaned up
    Evidence: .sisyphus/evidence/task-1-unrelated-resource-intact.txt
  ```

  **Commit**: YES (groups with Task 2, 3)
  - Message: `fix(lsp): FiveM invalidation correctness for manifest and file deletes`
  - Files: `lsp/fivem.go`, `lsp/workspace.go`, `lsp/fivem_invalid*_test.go`
  - Pre-commit: `go test -run TestFiveMInvalid ./lsp/`

- [x] 2. Fix file delete invalidation (clearDocument FiveM cleanup)

  **What to do**:
  - In `lsp/fivem.go`: verify whether the FeatureFiveM toggle actually has an invalidation bug or is a no-op-by-design
    1. Read `lsp/server.go:setCfg` — it returns early when the value hasn't changed
    2. Test: set FeatureFiveM=true when already true → should be no-op
    3. Test: toggle FeatureFiveM false→true or true→false → should trigger full reindex with cleanup
    4. If toggle works correctly for actual value changes, DO NOT add a fix — this is not a bug
  - For actual file deletes (non-manifest files under a FiveM resource root):
    1. In `handleDidChangeWatchedFiles`, when a `.lua` file is deleted from within a FiveM resource root
    2. Invalidate `FiveMProfileCached` on the deleted document and any siblings that reference its exports
    3. Re-classify affected documents via `classifyDocumentEnv` if the file was listed in the manifest

  **Must NOT do**:
  - Fix FeatureFiveM toggle if it's already working correctly (no-op for same value, reindex for value change)
  - Add new events or notifications — just cleanup existing invalidation paths

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Requires careful verification of feature toggle semantics and cross-document invalidation
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 1)
  - **Parallel Group**: Wave 1 (with Tasks 1, 3)
  - **Blocks**: Tasks 3, 4, 5
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `lsp/server.go:setCfg` — Helper that sets config and triggers reindex; returns early if value unchanged
  - `lsp/workspace.go:handleDidChangeWatchedFiles` — File delete event handler
  - `lsp/fivem.go:getDocumentFiveMProfile` — Returns cached profile; check where FiveMProfileCached is read vs invalidated

  **API/Type References**:
  - `lsp/server.go:Server.FeatureFiveM` — Feature flag field
  - `lsp/document.go:Document.FiveMProfileCached` — Cache invalidation target

  **External References**: None

  **WHY Each Reference Matters**:
  - `setCfg`: Must verify whether the toggle bug is real or a no-op by design
  - `handleDidChangeWatchedFiles`: The path where file deletes currently don't clean FiveM profile caches
  - `getDocumentFiveMProfile`: Shows how profiles are cached and when they're recomputed

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: FeatureFiveM toggle from false to true triggers reindex
    Tool: Bash (go test)
    Preconditions: Server initialized with FeatureFiveM=false
    Steps:
      1. Create a FiveM workspace fixture
      2. Set FeatureFiveM=true via didChangeConfiguration
      3. Assert all documents under FiveM resources get FiveMProfileCached computed
      4. Assert FiveMResources and FiveMResourceByName are populated
    Expected Result: Full reindex with FiveM state computed from scratch
    Failure Indicators: Documents still have no FiveM profiles; resources not populated
    Evidence: .sisyphus/evidence/task-2-toggle-on.txt

  Scenario: FeatureFiveM toggle from true to false cleans up FiveM state
    Tool: Bash (go test)
    Preconditions: Server with FiveM workspace fully indexed and FeatureFiveM=true
    Steps:
      1. Set FeatureFiveM=false via didChangeConfiguration
      2. Assert FiveMResources is empty
      3. Assert FiveMResourceGraph is empty
      4. Assert all documents have FiveMProfileCached=false
    Expected Result: All FiveM state cleaned up
    Failure Indicators: Resources or graph still populated after toggle off
    Evidence: .sisyphus/evidence/task-2-toggle-off.txt

  Scenario: FeatureFiveM set to same value is no-op
    Tool: Bash (go test)
    Preconditions: Server with FeatureFiveM=true
    Steps:
      1. Set FeatureFiveM=true again (same value)
      2. Assert no reindex is triggered (document count unchanged, no diagnostics republished)
    Expected Result: No-op, no reindex
    Failure Indicators: Unnecessary reindex triggered
    Evidence: .sisyphus/evidence/task-2-toggle-noop.txt

  Scenario: Non-manifest file delete invalidates FiveM profile on siblings
    Tool: Bash (go test)
    Preconditions: FiveM resource with manifest listing client_scripts = {"a.lua", "b.lua"}
    Steps:
      1. Delete "a.lua" via file watcher event
      2. Assert "b.lua" still has a valid FiveM profile (recomputed if needed)
      3. Assert the resource's client_scripts is updated (a.lua removed)
    Expected Result: Sibling document profiles are still valid; manifest entries updated
    Failure Indicators: Sibling profiles are stale or missing; manifest still references deleted file
    Evidence: .sisyphus/evidence/task-2-sibling-profile-invalidated.txt
  ```

  **Commit**: YES (groups with Task 1, 3)
  - Message: `fix(lsp): FiveM invalidation correctness for manifest and file deletes`
  - Files: `lsp/fivem.go`, `lsp/workspace.go`, `lsp/fivem_invalid*_test.go`
  - Pre-commit: `go test -run TestFiveMInvalid ./lsp/`

- [x] 3. Add invalidation regression tests

  **What to do**:
  - Create `lsp/fivem_invalidation_test.go` with table-driven tests covering:
    1. Manifest file delete → FiveM state fully removed
    2. Manifest file delete → unrelated resources unaffected
    3. FeatureFiveM toggle off → FiveM state fully cleaned up
    4. FeatureFiveM toggle on → FiveM state computed from scratch
    5. FeatureFiveM set to same value → no-op (no reindex)
    6. Non-manifest file delete under FiveM resource → sibling profiles invalidated
  - Use the existing fixture harness pattern from `lsp/fivem_fixture_harness_test.go`
  - Each test must verify the specific cleanup assertions from Tasks 1 and 2

  **Must NOT do**:
  - Test FeatureFiveM toggle as a bug if it's verified to work correctly — only test actual value changes
  - Create new test infrastructure — reuse existing fixture harness

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Writing tests following existing fixture patterns is well-defined and low-risk
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 1 (sequential after Tasks 1 and 2)
  - **Blocks**: Tasks 4, 5
  - **Blocked By**: Tasks 1, 2

  **References**:

  **Pattern References**:
  - `lsp/fivem_fixture_harness_test.go` — Main integration harness for FiveM/LSP test scenarios
  - `lsp/fivem_manifest_authoring_test.go` — Existing manifest LSP test structure
  - `lsp/fivem_exports_test.go` — Export resolution test structure
  - `lsp/fivem_perf_test.go` — Existing perf test patterns with `runtime.ReadMemStats`

  **API/Type References**:
  - `lsp/fivem.go:removeFiveMResource` — New function from Task 1
  - `lsp/server.go:Server.FiveMResources`, `FiveMResourceByName`, `FiveMResourceGraph` — State to verify cleanup

  **External References**: None

  **WHY Each Reference Matters**:
  - `fivem_fixture_harness_test.go`: The canonical pattern for setting up FiveM test workspaces
  - `fivem_manifest_authoring_test.go`: Shows how to create manifest-based test scenarios
  - `fivem_perf_test.go`: Shows how to use `runtime.ReadMemStats` for verification

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: All invalidation regression tests pass
    Tool: Bash (go test)
    Preconditions: Tasks 1 and 2 are implemented
    Steps:
      1. Run `go test -run TestFiveMInvalidation -v ./lsp/`
      2. Assert all 6 test cases pass (manifest delete, unrelated resource, toggle off, toggle on, toggle no-op, sibling profile)
    Expected Result: All tests pass with no panics or failures
    Failure Indicators: Any test fails or panics
    Evidence: .sisyphus/evidence/task-3-invalidation-tests.txt
  ```

  **Commit**: YES (groups with Tasks 1, 2)
  - Message: `fix(lsp): FiveM invalidation correctness for manifest and file deletes`
  - Files: `lsp/fivem_invalid*_test.go`
  - Pre-commit: `go test -run TestFiveMInvalid ./lsp/`

- [x] 4. Restructure FiveMManifestEntry to pointer references

  **What to do**:
  - In `lsp/fivem.go`: change `FiveMResourceGraphExpansion.Entry` from `FiveMManifestEntry` (value) to `*FiveMManifestEntry` (pointer), making all expansions reference the canonical entry stored in `FiveMManifest.Entries`
  - In `lsp/fivem.go`: change `FiveMResource.ClientGlobs`, `ServerGlobs`, `SharedGlobs` and corresponding export slices to reference entries by index or pointer rather than copying full `FiveMManifestEntry` values
  - In `lsp/fivem.go`: update `deriveFromManifest` to assign pointers/references instead of copies
  - In `lsp/fivem.go`: update `newFiveMResourceGraphNode` to assign pointers instead of copying entries
  - Verify all consumers of `FiveMResourceGraphExpansion.Entry` and `FiveMResource` export slices handle pointer indirection correctly
  - Run all FiveM tests to verify no regressions

  **Must NOT do**:
  - Change the `FiveMManifest` struct itself — it remains the single canonical owner of entries
  - Break any existing FiveM LSP feature (completion, hover, diagnostics, definition)
  - Add new data structures — only change value types to pointer types in existing structures

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Careful pointer indirection refactoring across interconnected data structures; high risk of subtle bugs
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 5)
  - **Parallel Group**: Wave 2 (with Tasks 5, 6)
  - **Blocks**: Task 6
  - **Blocked By**: Task 3

  **References**:

  **Pattern References**:
  - `lsp/fivem.go:FiveMResourceGraphExpansion` — Contains `Entry FiveMManifestEntry` by value; change to `*FiveMManifestEntry`
  - `lsp/fivem.go:FiveMManifest.Entries` — Single canonical owner of manifest entries
  - `lsp/fivem.go:deriveFromManifest` — Copies entries into `FiveMResource` slices; change to reference by pointer
  - `lsp/fivem.go:newFiveMResourceGraphNode` — Copies entries into graph expansions; change to reference by pointer
  - `lsp/fivem.go:registerFiveMManifestResource` — Already rebuilds `FiveMResources` and `FiveMResourceByName` from graph; canonical single-source-of-truth pattern

  **API/Type References**:
  - `lsp/fivem.go:FiveMManifestEntry` — 12 fields including 5 strings and 2 Range structs
  - `lsp/fivem.go:FiveMResource` — Holds client/server/shared export slices
  - `lsp/fivem.go:FiveMResourceGraphNode` — Stores expansion entries

  **External References**: None

  **WHY Each Reference Matters**:
  - `FiveMResourceGraphExpansion.Entry`: The main duplication site — each entry is value-copied, creating N copies of the same data for every manifest
  - `FiveMManifest.Entries`: The canonical single owner that all other references should point to
  - `deriveFromManifest` and `newFiveMResourceGraphNode`: The two copy sites that need to become pointer assignments

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Manifest entries are referenced by pointer, not copied
    Tool: Bash (go test)
    Preconditions: Workspace with a FiveM resource containing fxmanifest.lua with 10+ entries
    Steps:
      1. Index the workspace
      2. Assert that `FiveMResourceGraphExpansion.Entry` pointers point into the canonical `FiveMManifest.Entries` slice
      3. Assert no `FiveMManifestEntry` value copies exist in FiveMResource export slices
      4. Run `go test -run TestFiveM ./lsp/`
    Expected Result: All FiveM tests pass; entry data is shared by pointer, not duplicated
    Failure Indicators: Any FiveM test fails; any entry is a value copy
    Evidence: .sisyphus/evidence/task-4-pointer-entries.txt

  Scenario: Completion and hover still work after pointer refactor
    Tool: Bash (go test)
    Preconditions: Workspace with cross-resource exports
    Steps:
      1. Index a workspace with two FiveM resources where one references exports from another
      2. Trigger completion for `exports.otherResource:` — assert completions appear
      3. Trigger hover on an export function — assert signature help appears
    Expected Result: All LSP features work correctly with pointer-based entries
    Failure Indicators: Nil pointer dereference; completions missing; hover returns empty
    Evidence: .sisyphus/evidence/task-4-lsp-features-pointer.txt
  ```

  **Commit**: YES (groups with Tasks 5, 6)
  - Message: `refactor(lsp): deduplicate FiveM manifest/resource state`
  - Files: `lsp/fivem.go`, `lsp/server.go`, `lsp/fivem_dedup*_test.go`
  - Pre-commit: `go test -run TestFiveMDedup ./lsp/`

- [x] 5. Consolidate FiveMResource maps (eliminate redundancy with graph)

  **What to do**:
  - Analyze whether `Server.FiveMResources` and `Server.FiveMResourceByName` are redundant with `FiveMResourceGraph.ByRoot` and `FiveMResourceGraph.ByName`
  - If redundant, consolidate: make `FiveMResourceGraph` the single source of truth, remove `FiveMResources` and `FiveMResourceByName` from `Server` struct
  - Update all consumers that previously accessed `s.FiveMResources[uri]` or `s.FiveMResourceByName[name]` to use `s.FiveMResourceGraph.ByRoot[uri]` or `s.FiveMResourceGraph.ByName[name]`
  - If NOT redundant (consumers need different access patterns), document why both are needed and keep both but add a comment

  **Must NOT do**:
  - Remove maps without verifying all consumers — the graph may not support all query patterns
  - Break existing LSP features that rely on these maps
  - Add new data structures — only consolidate existing ones

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Need to trace all consumers and verify access pattern compatibility
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 4)
  - **Parallel Group**: Wave 2 (with Tasks 4, 6)
  - **Blocks**: Task 6
  - **Blocked By**: Task 3

  **References**:

  **Pattern References**:
  - `lsp/server.go:Server.FiveMResources` — Map keyed by URI to `FiveMResource`
  - `lsp/server.go:Server.FiveMResourceByName` — Map keyed by resource name to `FiveMResource`
  - `lsp/fivem.go:FiveMResourceGraph.ByRoot` — Map keyed by root URI to graph node
  - `lsp/fivem.go:FiveMResourceGraph.ByName` — Map keyed by resource name to graph node
  - `lsp/fivem.go:registerFiveMManifestResource` — Already rebuilds `FiveMResources` and `FiveMResourceByName` from the graph

  **API/Type References**:
  - `lsp/fivem.go:FiveMResource` — Resource struct with client/server/shared/export data
  - `lsp/fivem.go:FiveMResourceGraphNode` — Graph node containing resource + expansion data

  **External References**: None

  **WHY Each Reference Matters**:
  - `registerFiveMManifestResource`: Shows that both maps are already populated FROM the graph, strongly suggesting they're redundant
  - `FiveMResourceGraph.ByRoot/ByName`: The graph already provides URI and name lookups; the separate maps may duplicate this

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: FiveM lookup by URI and name works correctly after consolidation
    Tool: Bash (go test)
    Preconditions: Workspace with multiple FiveM resources
    Steps:
      1. Index a workspace with 3+ FiveM resources
      2. Resolve a FiveM resource by URI (via graph)
      3. Resolve a FiveM resource by name (via graph)
      4. Assert both lookups return correct resource data
      5. Run full FiveM test suite: `go test -run TestFiveM ./lsp/`
    Expected Result: All lookups return correct data; no tests regress
    Failure Indicators: Any lookup returns nil; any FiveM test fails
    Evidence: .sisyphus/evidence/task-5-graph-consolidation.txt
  ```

  **Commit**: YES (groups with Tasks 4, 6)
  - Message: `refactor(lsp): deduplicate FiveM manifest/resource state`
  - Files: `lsp/fivem.go`, `lsp/server.go`, `lsp/fivem_dedup*_test.go`
  - Pre-commit: `go test -run TestFiveMDedup ./lsp/`

- [x] 6. Add deduplication regression tests

  **What to do**:
  - Create `lsp/fivem_dedup_test.go` with tests covering:
    1. Manifest entries are referenced by pointer, not value-copied (verify same address)
    2. Changing a manifest entry in `FiveMManifest.Entries` is reflected through all pointer references
    3. FiveM resource lookup by URI works via graph (no separate map)
    4. FiveM resource lookup by name works via graph (no separate map)
    5. Full FiveM LSP feature suite (completion, hover, definition, diagnostics) passes after dedup
  - Use existing fixture harness pattern from `lsp/fivem_fixture_harness_test.go`

  **Must NOT do**:
  - Test implementation details that may change — test behavior, not struct layout
  - Create new test infrastructure — reuse existing fixture harness

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Well-defined test cases following existing patterns
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 2 (sequential after Tasks 4 and 5)
  - **Blocks**: Task 7
  - **Blocked By**: Tasks 4, 5

  **References**:

  **Pattern References**:
  - `lsp/fivem_fixture_harness_test.go` — Main integration harness
  - `lsp/fivem_manifest_authoring_test.go` — Manifest test structure
  - `lsp/fivem_exports_test.go` — Export resolution tests

  **API/Type References**:
  - `lsp/fivem.go:FiveMManifest.Entries` — Canonical entry slice
  - `lsp/fivem.go:FiveMResourceGraphExpansion.Entry` — Should now be `*FiveMManifestEntry`
  - `lsp/fivem.go:FiveMResourceGraph.ByRoot`, `.ByName` — Primary lookup paths

  **External References**: None

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: All deduplication regression tests pass
    Tool: Bash (go test)
    Preconditions: Tasks 4 and 5 are implemented
    Steps:
      1. Run `go test -run TestFiveMDedup -v ./lsp/`
      2. Assert all 5 test cases pass
    Expected Result: All tests pass
    Failure Indicators: Any test fails or panics
    Evidence: .sisyphus/evidence/task-6-dedup-tests.txt
  ```

  **Commit**: YES (groups with Tasks 4, 5)
  - Message: `refactor(lsp): deduplicate FiveM manifest/resource state`
  - Files: `lsp/fivem_dedup_test.go`
  - Pre-commit: `go test -run TestFiveMDedup ./lsp/`

- [x] 7. Remove native bundles from LibraryPaths, rely on ensureFiveMNativeBundleLoaded

  **What to do**:
  - In `lsp/server.go:buildConfiguredLibraryPaths`: remove the code that appends the runtime native cache directory to `LibraryPaths` when `FeatureFiveM` is enabled
  - Verify that `ensureFiveMNativeBundleLoaded` (in `lsp/fivem_native_catalog.go`) is already called on-demand during symbol resolution — this is the existing lazy path
  - The change: bundles are NO LONGER pre-indexed during `refreshWorkspace` via `indexWorkspace`; they are loaded lazily when a native symbol is first referenced
  - Update `lsp/fivem_perf_test.go`: adjust the "cold index" benchmark to reflect that native bundles are no longer eagerly indexed; the allocation budget for cold index should decrease

  **Must NOT do**:
  - Build new lazy-loading infrastructure — `ensureFiveMNativeBundleLoaded` already provides this
  - Remove `ensureFiveMNativeBundleLoaded` — it's still needed, just becomes the ONLY path
  - Change how native bundles are generated or stored on disk — only change when they're indexed

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Small, well-scoped change — removing an eager path and relying on an existing lazy path
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 8)
  - **Parallel Group**: Wave 3 (with Tasks 8, 9)
  - **Blocks**: Tasks 8, 9
  - **Blocked By**: Task 6

  **References**:

  **Pattern References**:
  - `lsp/server.go:buildConfiguredLibraryPaths` — Currently appends native cache dir to `LibraryPaths` when `FeatureFiveM` is true
  - `lsp/fivem_native_catalog.go:ensureFiveMNativeBundleLoaded` — Existing on-demand lazy loading function
  - `lsp/fivem_native_runtime.go` — Generates/loads native bundle cache on disk

  **API/Type References**:
  - `lsp/server.go:Server.LibraryPaths` — Slice of paths indexed during workspace refresh
  - `lsp/fivem_native_catalog.go:FiveMNativeBundle` — Bundle type loaded on demand

  **External References**: None

  **WHY Each Reference Matters**:
  - `buildConfiguredLibraryPaths`: The exact code to modify — remove native cache dir from library paths
  - `ensureFiveMNativeBundleLoaded`: The existing lazy path that will now be the only loading mechanism
  - `fivem_perf_test.go`: Budget test expectations need updating for lower cold-index allocations

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Native bundles NOT indexed during workspace refresh
    Tool: Bash (go test)
    Preconditions: FiveM workspace with FeatureFiveM=true
    Steps:
      1. Index a FiveM workspace from scratch
      2. Measure memory after initial index (before any native symbol lookup)
      3. Assert native bundle Documents are NOT in s.Documents
      4. Trigger native symbol completion (e.g., `Citizen.` in a client file)
      5. Assert native bundles ARE now loaded and in s.Documents
      6. Assert completions for native symbols work correctly
    Expected Result: Native bundles load lazily on first reference, not during initial index
    Failure Indicators: Native bundles appear in Documents before first reference; completions fail
    Evidence: .sisyphus/evidence/task-7-lazy-natives.txt

  Scenario: Cold index memory is measurably lower without eager native loading
    Tool: Bash (go test -bench)
    Preconditions: Existing benchmark baseline
    Steps:
      1. Run `go test -bench=BenchmarkFiveMColdIndex -benchmem ./lsp/`
      2. Compare `TotalAlloc` against baseline
      3. Assert allocation is lower than baseline (no native bundles in cold path)
    Expected Result: Cold index allocates measurably less memory than eager baseline
    Failure Indicators: Cold index allocation same or higher than baseline
    Evidence: .sisyphus/evidence/task-7-cold-index-bench.txt
  ```

  **Commit**: YES (groups with Tasks 8, 9)
  - Message: `perf(lsp): lazy native bundle loading`
  - Files: `lsp/server.go`, `lsp/fivem_native_catalog.go`, `lsp/fivem_lazy*_test.go`
  - Pre-commit: `go test -run TestFiveMLazyNatives ./lsp/`

- [x] 8. Audit all native symbol resolution paths for ensureFiveMNativeBundleLoaded coverage

  **What to do**:
  - Search all code paths that access native symbols (completions, hover, diagnostics, definition, signature help, references)
  - Verify `ensureFiveMNativeBundleLoaded` is called in EVERY path that needs native symbols, not just the two currently calling it (`diagnostics.go:266`, `symbols.go:803`)
  - Add `ensureFiveMNativeBundleLoaded` calls wherever they're missing
  - Ensure the lazy loading path properly handles the case where bundles aren't loaded yet (nil checks, lazy init)

  **Must NOT do**:
  - Load bundles eagerly "just to be safe" — the point is lazy loading
  - Skip paths because they're "rarely used" — every feature path must be covered

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Requires exhaustive code path audit; missing a path means a nil panic in production
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 7)
  - **Parallel Group**: Wave 3 (with Tasks 7, 9)
  - **Blocks**: Task 9
  - **Blocked By**: Task 7

  **References**:

  **Pattern References**:
  - `lsp/fivem_native_catalog.go:9-45` — `ensureFiveMNativeBundleLoaded` existing calls and lazy loading pattern
  - `lsp/diagnostics.go:266` — Currently calls `ensureFiveMNativeBundleLoaded`
  - `lsp/symbols.go:803` — Currently calls `ensureFiveMNativeBundleLoaded`

  **API/Type References**:
  - `lsp/features.go` — Completion, hover, signature help — may need lazy native loading
  - `lsp/infer.go` — Type inference — may need lazy native loading for native type resolution
  - `lsp/symbols.go` — Global symbol access — already has one call, may need more

  **External References**: None

  **WHY Each Reference Matters**:
  - `features.go`: Completion/hover/signature are the most user-visible paths — they MUST have lazy loading
  - `infer.go`: Type inference for native types will fail if bundles aren't loaded
  - `diagnostics.go` and `symbols.go`: Already call `ensureFiveMNativeBundleLoaded` — use these as the pattern

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: All FiveM LSP features work without eager native loading
    Tool: Bash (go test)
    Preconditions: FiveM workspace, FeatureFiveM=true, no native bundles pre-indexed
    Steps:
      1. Index workspace (without native bundles in LibraryPaths)
      2. Trigger completion for Citizen. → assert native completions appear
      3. Trigger hover on a native function → assert hover renders correctly
      4. Trigger go-to-definition on a native → assert definition found
      5. Trigger signature help on a native call → assert signature appears
      6. Assert diagnostics for unknown exports still work
    Expected Result: All FiveM LSP features work correctly with lazy native loading
    Failure Indicators: Nil pointer dereference; completions/hover/definition empty; signatures missing
    Evidence: .sisyphus/evidence/task-8-native-audit.txt
  ```

  **Commit**: YES (groups with Tasks 7, 9)
  - Message: `perf(lsp): lazy native bundle loading`
  - Files: `lsp/fivem_native_catalog.go`, `lsp/features.go`, `lsp/infer.go`, `lsp/symbols.go`, `lsp/diagnostics.go`
  - Pre-commit: `go test -run TestFiveM ./lsp/`

- [x] 9. Add lazy native bundle tests

  **What to do**:
  - Create `lsp/fivem_lazy_test.go` with tests covering:
    1. Native bundles are NOT in Documents after initial workspace index (before first reference)
    2. Native bundles ARE loaded after first native symbol lookup
    3. Subsequent native symbol lookups use cached bundles (warm cache)
    4. All FiveM LSP features work correctly with lazy loading (completion, hover, definition, diagnostics, signature help)
    5. Feature toggle off → native bundles remain loaded but not used; toggle on → same lazy behavior
  - Use existing fixture harness pattern

  **Must NOT do**:
  - Test implementation details of `ensureFiveMNativeBundleLoaded` — test behavior

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Following existing fixture harness patterns
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 3 (sequential after Tasks 7 and 8)
  - **Blocks**: Task 10
  - **Blocked By**: Tasks 7, 8

  **References**:

  **Pattern References**:
  - `lsp/fivem_fixture_harness_test.go` — Main integration harness
  - `lsp/fivem_perf_test.go` — Performance test patterns with `runtime.ReadMemStats`

  **API/Type References**:
  - `lsp/fivem_native_catalog.go:ensureFiveMNativeBundleLoaded` — Function to verify lazy behavior

  **External References**: None

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: All lazy native bundle tests pass
    Tool: Bash (go test)
    Preconditions: Tasks 7 and 8 are implemented
    Steps:
      1. Run `go test -run TestFiveMLazyNatives -v ./lsp/`
      2. Assert all 5 test cases pass
    Expected Result: All tests pass
    Failure Indicators: Any test fails
    Evidence: .sisyphus/evidence/task-9-lazy-natives-tests.txt
  ```

  **Commit**: YES (groups with Tasks 7, 8)
  - Message: `perf(lsp): lazy native bundle loading`
  - Files: `lsp/fivem_lazy_test.go`
  - Pre-commit: `go test -run TestFiveMLazyNatives ./lsp/`

- [x] 10. Restructure Source ownership (Tree owns, Document borrows)

  **What to do**:
  - Restructure `Document.Source` so that `Tree.Source` is the single owner of the source byte slice, and `Document.Source` becomes a borrowed reference (or is removed entirely)
  - Currently `workspace.go:654` sets `doc.Source = source` and `workspace.go:630` calls `tree.Reset(source)` — both hold the same `[]byte`. The restructure must ensure there is ONE owner (Tree) so that niling Tree.Source frees memory
  - Update all references to `doc.Source` to use `doc.Tree.Source` instead (or a helper method like `doc.SourceBytes() []byte`)
  - Add a helper `func (d *Document) SourceBytes() []byte` that returns `d.Tree.Source` for ergonomic access
  - Verify that `workspace.go:ExistingSource` byte-equality skip still works (it should, since it compares against `doc.Tree.Source`)

  **Must NOT do**:
  - Nil `doc.Source` without also handling `doc.Tree.Source` — they share the same backing array
  - Break the `ExistingSource` optimization in `refreshWorkspace`
  - Change the parser or AST — only change the ownership pattern in the LSP layer

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Cross-cutting refactoring affecting Document struct, workspace, and all Source access sites
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 4 (sequential, first task)
  - **Blocks**: Tasks 11, 12
  - **Blocked By**: Task 9

  **References**:

  **Pattern References**:
  - `lsp/workspace.go:630` — `tree.Reset(source)` — Tree receives the source bytes
  - `lsp/workspace.go:654` — `doc.Source = source` — Document also stores the same source bytes (DUPLICATE)
  - `lsp/workspace.go:1228-1245` — `clearDocument` — Canonical document removal path
  - `lsp/document.go:Document.Source` — The field to consolidate

  **API/Type References**:
  - `ast/ast.go:Tree.Source` — The Source field on AST Tree (will become the single owner)
  - `lsp/document.go:Document` — Document struct with Source field

  **External References**: None

  **WHY Each Reference Matters**:
  - `workspace.go:630` and `workspace.go:654`: The two sites where the same `[]byte` is assigned to both Tree and Document — the root cause of duplication
  - `clearDocument`: The removal path that must properly nil the single owner
  - `Document.Source`: The field that will be replaced by `Tree.Source` accessor

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Source ownership consolidated to Tree.Source
    Tool: Bash (go test)
    Preconditions: Source ownership restructure complete
    Steps:
      1. Run full test suite: `go test -race ./...`
      2. Assert all tests pass
      3. Grep for direct `doc.Source` accesses that should now use `doc.Tree.Source` or `doc.SourceBytes()`
      4. Assert no remaining `doc.Source` reads that bypass the accessor
    Expected Result: All tests pass; Source is owned by Tree only
    Failure Indicators: Any test fails; direct `doc.Source` reads still exist
    Evidence: .sisyphus/evidence/task-10-source-ownership.txt
  ```

  **Commit**: YES (groups with Tasks 11-13)
  - Message: `perf(lsp): evict Source+TypeCache for closed documents`
  - Files: `lsp/document.go`, `lsp/workspace.go`, `ast/ast.go`
  - Pre-commit: `go test -race ./...`

- [x] 11. Implement evictClosedDocumentCaches (Source+TypeCache)

  **What to do**:
  - Create a new function `evictClosedDocumentCaches(doc *Document)` in `lsp/document.go` (NOT modifying `clearDocument`)
  - This function should:
    1. Nil `doc.Tree.Source` (the single owner after Task 10) — this frees the source byte slice
    2. Nil `doc.TypeCache` — frees type inference cache (`len(tree.Nodes)` sized)
    3. Nil `doc.Inferring` — frees the concurrent type inference tracking map
    4. Nil `doc.LuaDocCache` — frees LuaDoc cache
    5. Nil `doc.ActualReads` — frees read tracking
    6. Nil `doc.MutatedLocals` — frees mutation tracking
    7. Preserve: `doc.Tree` (AST nodes remain — integer offsets only, no string data), `doc.Resolver` (nameArena-based data survives without Source), `doc.FiveMLuaExports` (FiveM metadata)
  - Determine when to call `evictClosedDocumentCaches` — a document is "closed" when:
    1. It's NOT the currently open document in the editor (i.e., `s.Documents[uri]` where uri != active document)
    2. It's a library/native bundle document (never directly edited)
  - Add a `Closed` field or use existing visibility tracking to determine eviction eligibility
  - Call `evictClosedDocumentCaches` from `workspace.go` after document finalization for non-active documents
  - When a closed document is re-opened, re-read from disk (`indexWorkspace` handles this during `refreshWorkspace`)

  **Must NOT do**:
  - Modify `clearDocument` — the eviction path is separate from full document removal
  - Drop `doc.Tree` (AST nodes) — they're integer offsets and very compact
  - Drop `doc.Resolver` — ReceiverName slices point into nameArena (separate allocation), not Source
  - Drop `doc.FiveMLuaExports` — needed for export resolution
  - Evict Source for documents referenced by other open documents (zombie doc problem)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Careful memory management with cross-document dependency awareness
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 4 (sequential after Task 10)
  - **Blocks**: Tasks 12, 13
  - **Blocked By**: Task 10

  **References**:

  **Pattern References**:
  - `lsp/workspace.go:1228-1245` — `clearDocument` — Full document removal; do NOT modify, use as reference for what to keep vs remove
  - `lsp/workspace.go:650+` — `finalizeDocumentUpdate` — Post-parse cache filling; site where eviction should happen after finalization

  **API/Type References**:
  - `lsp/document.go:Document` — Fields to nil: `TypeCache`, `Inferring`, `LuaDocCache`, `ActualReads`, `MutatedLocals`; Fields to keep: `Tree` (nodes only), `Resolver`, `FiveMLuaExports`
  - `ast/ast.go:Tree.Source` — The single owner of source bytes after Task 10; niling this frees the source memory

  **External References**: None

  **WHY Each Reference Matters**:
  - `clearDocument`: Shows exactly what fields are cleared during full removal — eviction should be a subset of this
  - `finalizeDocumentUpdate`: The correct place to call eviction — after all caches are filled, before returning to the caller

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Closed document caches are evicted, memory is freed
    Tool: Bash (go test -bench)
    Preconditions: Workspace with 50+ files indexed
    Steps:
      1. Index all workspace files
      2. Measure total memory via runtime.ReadMemStats
      3. Open one file (making all others "closed")
      4. Trigger eviction on all closed documents
      5. Force GC and measure memory again
      6. Assert memory decreased by at least the size of evicted caches
      7. Verify closed docs have nil Tree.Source, nil TypeCache, nil LuaDocCache
      8. Verify closed docs still have non-nil Tree (nodes), non-nil Resolver
    Expected Result: Measurable memory reduction; correct fields evicted; correct fields preserved
    Failure Indicators: Memory not reduced; Tree niled; Resolver niled; Source still held
    Evidence: .sisyphus/evidence/task-11-evict-caches.txt

  Scenario: Re-opening an evicted document reloads correctly
    Tool: Bash (go test)
    Preconditions: Document has been evicted (Source niled)
    Steps:
      1. Open an evicted document by URI
      2. Assert the document is re-read from disk
      3. Assert Tree.Source is non-nil
      4. Assert TypeCache and LuaDocCache are re-populated
      5. Assert all LSP features (hover, definition, completion) work correctly
    Expected Result: Re-opened document fully functional with rebuilt caches
    Failure Indicators: Nil pointer dereference; features return empty results
    Evidence: .sisyphus/evidence/task-11-reopen-evicted.txt
  ```

  **Commit**: YES (groups with Tasks 10, 12, 13)
  - Message: `perf(lsp): evict Source+TypeCache for closed documents`
  - Files: `lsp/document.go`, `lsp/workspace.go`
  - Pre-commit: `go test -race ./...`

- [x] 12. Add nil-safety guards at cross-document Source access sites

  **What to do**:
  - Find all sites that access `targetDoc.Source` or `doc.Source` where `targetDoc` / `doc` is a different document than the one being actively processed
  - Key sites identified by Metis:
    1. `symbols.go:405-406`: `ast.String(targetDoc.Source[pNode.Start:pNode.End])` — call hierarchy parameter names
    2. `infer.go:754-756`: `targetDoc = doc.Server.Documents[leftType.DeclURI]` — cross-doc type inference
    3. `infer.go:1136-1140`: `targetDoc = doc.Server.Documents[t.DeclURI]` — type resolution across docs
    4. `workspace.go:956-968`: `fd.ReceiverName` used for global/module alias expansion
    5. `fivem.go:1008-1019`: `doc.Source[node.Start:node.End]` for manifest value extraction
  - For each site, add a nil check: if `targetDoc.Tree.Source == nil`, either:
    - Re-read the source from disk (preferred for correctness)
    - Return a degraded result (empty hover, skip call hierarchy param names)
    - Use ReceiverName from nameArena (safe — doesn't depend on Source)
  - Add a helper method `func (d *Document) EnsureSourceLoaded() error` that re-reads from disk if `Tree.Source == nil`

  **Must NOT do**:
  - Crash on nil Source — every cross-document access must handle the evicted case
  - Silently return wrong results — if Source can't be re-read, return an explicit "unavailable" response
  - Add source re-reading for every access — only re-read when needed (on-demand)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Requires finding and carefully handling every cross-document Source access site
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 4 (sequential after Task 10, alongside Task 11)
  - **Blocks**: Task 13
  - **Blocked By**: Task 10

  **References**:

  **Pattern References**:
  - `lsp/symbols.go:405-406` — `ast.String(targetDoc.Source[pNode.Start:pNode.End])` — Needs nil guard for evicted Source
  - `lsp/infer.go:754-756` — Cross-doc type inference accessing targetDoc — Needs on-demand re-read
  - `lsp/infer.go:1136-1140` — Type resolution across docs — Needs on-demand re-read
  - `lsp/workspace.go:956-968` — ReceiverName for global/module alias — Safe (nameArena-based), but verify
  - `lsp/fivem.go:1008-1019` — Manifest value extraction using Source — Needs nil guard

  **API/Type References**:
  - `lsp/document.go:Document.EnsureSourceLoaded()` — New helper method to re-read Source from disk
  - `lsp/document.go:Document.SourceBytes()` — New helper from Task 10

  **External References**: None

  **WHY Each Reference Matters**:
  - `symbols.go:405`: Will nil panic if targetDoc.Source is evicted — must add nil guard
  - `infer.go:754,1136`: Cross-doc type inference is a core LSP feature — must handle evicted source gracefully
  - `fivem.go:1008`: Manifest value extraction must handle evicted source for closed manifests

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Cross-document features work when target doc has evicted Source
    Tool: Bash (go test)
    Preconditions: Two documents, one with Source evicted
    Steps:
      1. Create workspace with document A referencing symbol defined in document B
      2. Evict document B's Source
      3. Trigger go-to-definition from A to B — assert it works (Source re-read on demand)
      4. Trigger hover on B's symbol from A — assert hover content appears
      5. Trigger call hierarchy for B's function — assert parameter names appear
      6. Assert no nil pointer dereferences in any code path
    Expected Result: All cross-document features work correctly, with on-demand Source re-read
    Failure Indicators: Nil pointer dereference; empty hover; definition not found; panic
    Evidence: .sisyphus/evidence/task-12-nil-safety.txt

  Scenario: Manifest value extraction works with evicted Source
    Tool: Bash (go test)
    Preconditions: FiveM workspace with manifest Source evicted
    Steps:
      1. Index FiveM workspace
      2. Evict manifest document's Source
      3. Trigger manifest diagnostics — assert they still work (Source re-loaded)
      4. Trigger manifest completion — assert directive completions appear
    Expected Result: Manifest features work correctly with on-demand Source re-read
    Failure Indicators: Manifest diagnostics fail; completions empty; panic
    Evidence: .sisyphus/evidence/task-12-manifest-nil-safety.txt
  ```

  **Commit**: YES (groups with Tasks 10, 11, 13)
  - Message: `perf(lsp): evict Source+TypeCache for closed documents`
  - Files: `lsp/symbols.go`, `lsp/infer.go`, `lsp/workspace.go`, `lsp/fivem.go`, `lsp/document.go`
  - Pre-commit: `go test -race ./...`

- [x] 13. Add closed document eviction tests

  **What to do**:
  - Create `lsp/evict_test.go` with tests covering:
    1. Source and TypeCache are nil after eviction
    2. Tree and Resolver are preserved after eviction
    3. Cross-document features (hover, definition, completion) still work when target doc is evicted
    4. Re-opening an evicted document reloads Source correctly
    5. FiveM features work when manifest Source is evicted (on-demand re-read)
    6. No nil pointer dereferences in any code path with evicted docs
  - Use existing fixture harness pattern

  **Must NOT do**:
  - Test implementation details — test behavior (features work, memory reduced)
  - Create new test infrastructure — reuse existing harness

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Well-defined test cases following existing patterns
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 4 (sequential after Tasks 11 and 12)
  - **Blocks**: Task 14
  - **Blocked By**: Tasks 11, 12

  **References**:

  **Pattern References**:
  - `lsp/fivem_fixture_harness_test.go` — Integration harness
  - `lsp/fivem_perf_test.go` — Memory measurement patterns

  **API/Type References**:
  - `lsp/document.go:evictClosedDocumentCaches` — New function from Task 11
  - `lsp/document.go:Document.EnsureSourceLoaded` — New helper from Task 12

  **External References**: None

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: All eviction tests pass
    Tool: Bash (go test)
    Preconditions: Tasks 10-12 are implemented
    Steps:
      1. Run `go test -run TestEvictClosedDoc -v ./lsp/`
      2. Assert all 6 test cases pass
    Expected Result: All tests pass with no panics
    Failure Indicators: Any test fails or panics
    Evidence: .sisyphus/evidence/task-13-eviction-tests.txt
  ```

  **Commit**: YES (groups with Tasks 10-12)
  - Message: `perf(lsp): evict Source+TypeCache for closed documents`
  - Files: `lsp/evict_test.go`
  - Pre-commit: `go test -run TestEvictClosedDoc ./lsp/`

- [x] 14. Add GlobalIndex compaction after document removal

  **What to do**:
  - In `lsp/symbols.go`: add a function `compactGlobalIndex(s *Server)` that:
    1. Iterates over `s.GlobalIndex`
    2. For each `GlobalKey`, if the associated slice of `GlobalSymbol` has empty slots (from removed documents), compact the slice
    3. If a `GlobalKey` maps to an empty slice, delete the key from the map
  - Call `compactGlobalIndex` after `removeDocumentGlobals` in `clearDocument` and in the new `removeFiveMResource` function (from Wave 1)
  - Consider adding a threshold — only compact if the total removed symbols exceed a certain percentage (e.g., >20% of total)
  - Measure GlobalIndex memory before and after compaction in the existing perf test

  **Must NOT do**:
  - Compact on every document change — only after batch removals or periodically
  - Change the GlobalIndex map type — just compact the slices within it
  - Break symbol lookup performance — compaction must preserve lookup correctness

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: GlobalIndex is a core data structure; compaction must preserve correctness
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 5 (after Wave 4)
  - **Blocks**: Task 15
  - **Blocked By**: Task 13

  **References**:

  **Pattern References**:
  - `lsp/symbols.go:1475+` — `setGlobalSymbol` — How symbols are added to GlobalIndex
  - `lsp/symbols.go` — `removeDocumentGlobals` — How symbols are removed (leaves empty slots)
  - `lsp/workspace.go:1228-1245` — `clearDocument` — Where compaction should be triggered

  **API/Type References**:
  - `lsp/server.go:Server.GlobalIndex` — `map[GlobalKey][]GlobalSymbol`
  - `lsp/symbols.go:GlobalSymbol` — Symbol struct with document URI reference

  **External References**: None

  **WHY Each Reference Matters**:
  - `setGlobalSymbol`: Shows how GlobalIndex grows — append-only, no compaction
  - `removeDocumentGlobals`: Shows how symbols are removed — likely nils entries or removes individual items but doesn't compact the slice
  - `clearDocument`: The natural trigger point for compaction after removal

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: GlobalIndex compacts after document removal
    Tool: Bash (go test)
    Preconditions: Workspace with 100+ files indexed
    Steps:
      1. Index workspace, measure GlobalIndex size (number of keys + total symbol count)
      2. Remove 50% of documents
      3. Trigger compaction
      4. Measure GlobalIndex size again
      5. Assert GlobalIndex size decreased proportionally
      6. Assert symbol lookups still return correct results for remaining documents
    Expected Result: GlobalIndex size decreased; lookups still correct
    Failure Indicators: GlobalIndex same size; lookups return stale/missing results
    Evidence: .sisyphus/evidence/task-14-globalindex-compaction.txt

  Scenario: Symbol lookups work correctly after compaction
    Tool: Bash (go test)
    Preconditions: Workspace with cross-document symbol references
    Steps:
      1. Index workspace with document A defining function X and document B referencing X
      2. Remove document C (unrelated to A or B)
      3. Trigger compaction
      4. Resolve definition of X from B — assert it still points to A
      5. Complete global symbols — assert X appears in completions
    Expected Result: All symbol lookups work correctly after compaction
    Failure Indicators: Definition not found; completion missing; wrong definition target
    Evidence: .sisyphus/evidence/task-14-compaction-correctness.txt
  ```

  **Commit**: YES (groups with Task 15)
  - Message: `perf(lsp): GlobalIndex compaction after document removal`
  - Files: `lsp/symbols.go`, `lsp/workspace.go`
  - Pre-commit: `go test -run TestGlobalIndexCompaction ./lsp/`

- [ ] 15. Add GlobalIndex compaction tests

  **What to do**:
  - Create `lsp/globalindex_test.go` with tests covering:
    1. Compaction reduces GlobalIndex size after document removal
    2. Symbol lookups still work after compaction
    3. Compaction threshold: only compacts when removal exceeds threshold
    4. Empty keys are removed from the map
    5. Full test suite passes after compaction is enabled
  - Use existing fixture harness pattern

  **Must NOT do**:
  - Test implementation details — test behavior

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Well-defined test cases
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 5 (sequential after Task 14)
  - **Blocks**: Tasks 16, 17
  - **Blocked By**: Task 14

  **References**:

  **Pattern References**:
  - `lsp/fivem_fixture_harness_test.go` — Integration harness
  - `lsp/fivem_perf_test.go` — Memory measurement patterns

  **API/Type References**:
  - `lsp/symbols.go:compactGlobalIndex` — New function from Task 14
  - `lsp/server.go:Server.GlobalIndex` — The data structure under test

  **External References**: None

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: All GlobalIndex compaction tests pass
    Tool: Bash (go test)
    Preconditions: Task 14 is implemented
    Steps:
      1. Run `go test -run TestGlobalIndexCompaction -v ./lsp/`
      2. Assert all 5 test cases pass
    Expected Result: All tests pass
    Failure Indicators: Any test fails
    Evidence: .sisyphus/evidence/task-15-compaction-tests.txt
  ```

  **Commit**: YES (groups with Task 14)
  - Message: `perf(lsp): GlobalIndex compaction after document removal`
  - Files: `lsp/globalindex_test.go`
  - Pre-commit: `go test -run TestGlobalIndexCompaction ./lsp/`

- [ ] 16. Add memory profiling baseline and per-document footprint benchmark

  **What to do**:
  - Create `lsp/mem_baseline_test.go` with:
    1. A benchmark `BenchmarkMemoryBaseline` that measures `runtime.ReadMemStats` before and after indexing a FiveM workspace
    2. A benchmark `BenchmarkPerDocumentFootprint` that measures per-document memory (`TotalAlloc / numDocuments`)
    3. A benchmark `BenchmarkClosedDocumentSavings` that measures memory before and after evicting closed document caches
    4. A test `TestMemoryBaselineSnapshot` that records current memory metrics for future comparison
  - Use `runtime.GC()` before each measurement to get stable numbers
  - Include both FiveM-enabled and plain-Lua variants to measure FiveM overhead

  **Must NOT do**:
  - Change any production code — this is measurement-only
  - Add pprof endpoints (out of scope for this task; can be added later)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Adding benchmarks following existing patterns
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 17)
  - **Parallel Group**: Wave 6 (with Task 17)
  - **Blocks**: FINAL
  - **Blocked By**: Task 15

  **References**:

  **Pattern References**:
  - `lsp/fivem_perf_test.go` — Existing perf/memory-budget tests with `runtime.ReadMemStats`
  - `parser/parser_test.go` — Parser allocation benchmarks
  - `semantic/resolver_test.go` — Resolver allocation benchmarks

  **API/Type References**:
  - `runtime.ReadMemStats` — Go memory measurement
  - `runtime.GC()` — Force GC before measurement

  **External References**: None

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Memory profiling benchmarks run and produce results
    Tool: Bash (go test -bench)
    Preconditions: Workspace is buildable
    Steps:
      1. Run `go test -bench=BenchmarkMemoryBaseline -benchmem ./lsp/`
      2. Run `go test -bench=BenchmarkPerDocumentFootprint -benchmem ./lsp/`
      3. Run `go test -bench=BenchmarkClosedDocumentSavings -benchmem ./lsp/`
      4. Assert all three benchmarks produce measurable results
      5. Run `go test -run TestMemoryBaselineSnapshot -v ./lsp/`
      6. Assert snapshot test passes and prints memory metrics
    Expected Result: All benchmarks run; measurable allocation numbers produced
    Failure Indicators: Benchmarks fail; zero allocations reported; panic
    Evidence: .sisyphus/evidence/task-16-memory-profiling.txt
  ```

  **Commit**: YES (groups with Task 17)
  - Message: `test(lsp): memory profiling baseline and updated perf budgets`
  - Files: `lsp/mem_baseline_test.go`
  - Pre-commit: `go test -bench=BenchmarkMemory ./lsp/`

- [ ] 17. Update TestFiveMPerfBudgets with new thresholds

  **What to do**:
  - Update `lsp/fivem_perf_test.go` to reflect the new allocation expectations:
    1. Lower the "cold index" budget since native bundles are no longer eagerly loaded
    2. Add a budget for "cold index without native bundles" vs "after native bundle lazy load"
    3. Update the "warm reindex" budget to reflect closed document cache eviction
    4. Add a new budget test for "memory after eviction" — assert that closing documents and evicting caches reduces total memory
  - Document the baseline numbers in comments so future changes can track regressions

  **Must NOT do**:
  - Loosen budgets indiscriminately — only adjust budgets based on actual measured changes
  - Remove existing budget tests — add new ones, don't replace

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Updating test thresholds based on measured results
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 16)
  - **Parallel Group**: Wave 6 (with Task 16)
  - **Blocks**: FINAL
  - **Blocked By**: Task 16

  **References**:

  **Pattern References**:
  - `lsp/fivem_perf_test.go` — Existing perf budget tests with `runtime.ReadMemStats`
  - `lsp/mem_baseline_test.go` — New benchmark results from Task 16

  **API/Type References**:
  - `runtime.ReadMemStats` — Memory measurement
  - `testing.AllocsPerRun` — Allocation measurement

  **External References**: None

  **Acceptance Criteria**:

  **QA Scenarios (MANDATORY)**:

  ```
  Scenario: Updated perf budgets pass
    Tool: Bash (go test)
    Preconditions: All optimization tasks (1-15) complete
    Steps:
      1. Run `go test -run TestFiveMPerfBudgets -v ./lsp/`
      2. Assert all budget tests pass with new thresholds
    Expected Result: All perf budget tests pass
    Failure Indicators: Any budget test fails
    Evidence: .sisyphus/evidence/task-17-perf-budgets.txt
  ```

  **Commit**: YES (groups with Task 16)
  - Message: `test(lsp): memory profiling baseline and updated perf budgets`
  - Files: `lsp/fivem_perf_test.go`, `lsp/mem_baseline_test.go`
  - Pre-commit: `go test -bench=BenchmarkFiveM -benchmem ./lsp/`

---

## Final Verification Wave (MANDATORY — after ALL implementation tasks)

> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.

- [ ] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, run command). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in .sisyphus/evidence/. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [ ] F2. **Code Quality Review** — `unspecified-high`
  Run `go test -race ./...` + `go vet ./...`. Review all changed files for: `any` type assertions without comma-ok, empty catches, `fmt.Print` in prod, commented-out code, unused imports. Check AI slop: excessive comments, over-abstraction, generic names (data/result/item/temp). Verify no new allocations in hot paths without benchmark justification.
  Output: `Build [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [ ] F3. **Full Test Suite + Benchmark Verification** — `unspecified-high`
  Start from clean state. Run `go test -race ./...`. Run `go test -bench=BenchmarkFiveM -benchmem ./lsp/`. Run `go test -run TestFiveMPerfBudgets -v ./lsp/`. Verify no regressions. Compare allocation benchmarks against baseline. Save output to `.sisyphus/evidence/final-qa/`.
  Output: `Tests [N/N pass] | Benchmarks [N/N improved] | PerfBudgets [PASS/FAIL] | VERDICT`

- [ ] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff (git log/diff). Verify 1:1 — everything in spec was built (no missing), nothing beyond spec was built (no creep). Check "Must NOT do" compliance. Detect cross-task contamination: Task N touching Task M's files. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

| Tasks | Message | Files | Pre-commit |
|-------|---------|-------|------------|
| 1-3 | `fix(lsp): FiveM invalidation correctness for manifest and file deletes` | `lsp/fivem.go`, `lsp/workspace.go`, `lsp/fivem_invalid*_test.go` | `go test -run TestFiveMInvalid ./lsp/` |
| 4-6 | `refactor(lsp): deduplicate FiveM manifest/resource state` | `lsp/fivem.go`, `lsp/server.go`, `lsp/fivem_dedup*_test.go` | `go test -run TestFiveMDedup ./lsp/` |
| 7-9 | `perf(lsp): lazy native bundle loading` | `lsp/server.go`, `lsp/fivem_native_catalog.go`, `lsp/fivem_lazy*_test.go` | `go test -run TestFiveMLazyNatives ./lsp/` |
| 10-13 | `perf(lsp): evict Source+TypeCache for closed documents` | `lsp/document.go`, `lsp/workspace.go`, `ast/ast.go`, `lsp/symbols.go`, `lsp/infer.go`, `lsp/fivem*.go`, `lsp/evict*_test.go` | `go test -run TestEvictClosedDoc ./lsp/` |
| 14-15 | `perf(lsp): GlobalIndex compaction after document removal` | `lsp/symbols.go`, `lsp/workspace.go`, `lsp/globalindex*_test.go` | `go test -run TestGlobalIndexCompaction ./lsp/` |
| 16-17 | `test(lsp): memory profiling baseline and updated perf budgets` | `lsp/fivem_perf_test.go`, `lsp/mem_baseline_test.go` | `go test -bench=BenchmarkFiveM -benchmem ./lsp/` |

---

## Success Criteria

### Verification Commands
```bash
go test -race ./...                                          # Expected: PASS (all packages)
go test -run TestFiveMPerfBudgets -v ./lsp/                   # Expected: PASS (allocation budgets met)
go test -run TestFiveM -v ./lsp/                              # Expected: PASS (all FiveM tests)
go test -bench=BenchmarkFiveM -benchmem ./lsp/                 # Expected: measurably lower allocations vs baseline
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] All tests pass (race detector enabled)
- [ ] Benchmark allocation reduction measurable vs baseline