## C2 Hybrid Type System + Interning Pool - 2026-05-11

- GitNexus verification could not run for this workspace: `gitnexus_detect_changes({repo:"lugo"})` reported that
  repository `lugo` is not indexed, and the available indexes are `nixos`, `APICube`, `Copy`, and `isen-moyenne`.
- LSP diagnostics report only modernization hints in changed files (`maps.Copy` suggestion in `types_structural.go`,
  range-over-int suggestion in `type_pool_test.go`); no errors were reported.

## C3 SemanticData Side Table - 2026-05-11

- Existing lsp symbols SemanticData and Scope conflicted with the planned API. Both conflicts were resolved with
  low-risk local renames and verified with diagnostics, tests, benchmark, AST diff, and build.

## C4 GlobalIndex v2 - 2026-05-11

- A full `go test ./lsp/ -count=1` run transiently failed in existing
  `TestFiveMGlobalIndexCompaction/NonEmptyKeysPreserved`; the subtest passed in isolation and the full suite passed on
  rerun without code changes.

## E1 Two-Pass Resolver v2 - 2026-05-11

- GitNexus impact lookup for `SemanticDataTable` reported `Target 'SemanticDataTable' not found`, so the index appears
  stale or incomplete for C3 symbols; code-level verification used LSP diagnostics, targeted tests, full tests, and
  build.
- `gitnexus_detect_changes({scope:"all", repo:"lugo"})` reported unrelated pre-existing changes in `AGENTS.md`,
  `lsp/format.go`, and session/plan files, while the new ResolverV2 files are untracked and not represented in that
  indexed change report.
- Post-implementation review found and fixed E1 blockers: local initializer RHS visibility, global assignment RHS type
  lookup, missing table-field `fieldMap`/semantic bindings, and an unsynchronized phase-completion map.
- The first rerun of `go test ./lsp/ -count=1` after review fixes hit the already-known transient
  `TestFiveMGlobalIndexCompaction/NonEmptyKeysPreserved`; subsequent full package/all-package tests passed.
- The workspace contains broader pre-existing/untracked C4/S-task files, so `gitnexus_detect_changes({scope:"all"})`
  reports CRITICAL affected scope outside the ResolverV2 files.

## S1 FiveM Runtime Metadata - 2026-05-11

- `gitnexus_detect_changes(scope=all)` reported CRITICAL because unrelated working-tree changes were already present (
  `AGENTS.md`, `lsp/format.go`, `session-ses_1ec4.md`) alongside the S1 edits; S1 verification still passed targeted
  tests, full `./lsp` tests, and build.
- GitNexus could not resolve some freshly introduced/previously unindexed method names such as `RegisterFiveMResource`
  by name during pre-edit impact lookup, so impact coverage relied on nearby indexed symbols (`Server`, `NewServer`,
  `finalizeDocumentUpdate`, `setGlobalSymbol`, `removeDocumentGlobals`).

## S3 Proactive Data Layer - 2026-05-11

- The continuation request prohibited modifying existing files, so `handleDidOpen`/`handleDidChange` hooks were not
  added in this pass; the proactive layer is implemented as additive files and tests only.
- `gitnexus_detect_changes(scope=all, repo=lugo)` reported low risk with no changed symbols/affected processes for the
  additive S3 files.

## S1-cont Export Bridge - 2026-05-11

- No implementation blockers. lsp_diagnostics on export bridge files, go test ./lsp/ -count=1, go build ./lsp/, and
  gitnexus_detect_changes(scope=all, repo=lugo) completed successfully.

## F1 Plan Compliance Audit - 2026-05-11

- `lsp/prefetch.go` and `lsp/warmup.go` exist, but `lsp/workspace.go:48-73` `handleDidOpen` still only updates/publishes
  diagnostics and never triggers prefetch/warmup; the plan's "dependency-prefetch on didOpen" must-have is not satisfied
  yet.
- `lsp/treediff.go` provides a standalone hash-bucket diff, but `ast.NodeID` in `ast/ast.go:71-87` remains a plain
  tree-slice index and no tree-diff hook was found in workspace/parser flow, so the "stable node IDs + minimal reparse"
  requirement is not yet satisfied.
- The recent-fix note for `infer_v2.go` is not reflected in code: `lsp/infer_v2.go:35` still mutates
  `methodType.Structural.Function.SelfType` in place instead of rebinding through a copy, so the shallow-copy/self-type
  fix is still missing.

## F4 Scope Fidelity Check - 2026-05-11

- Verdict remains REJECT. Several task files exist, tests pass, and diagnostics are clean, but implementation depth is
  not 1:1 with the plan spec.
- S3 is incomplete: `lsp/prefetch.go` is a synchronous queue without the required bounded goroutine pool/on-didOpen
  integration, `lsp/completion_resource.go` builds all scopes together instead of scope-filtered per-resource tables,
  and `lsp/warmup.go` lacks the staged manifest/dependency/type phases and budget enforcement.
- S5 is incomplete: `lsp/treediff.go` is a hash-bucket comparison helper only; it does not provide stable node IDs,
  partial subtree reparse, parser integration, or property-based equivalence to full parse.
- X1 violates source-only eviction scope: `lsp/eviction.go` deletes `GlobalIndexV2.Resources` entries and `Documents`
  entries, which conflicts with the requirement to preserve AST/symbol metadata and evict source text only.
- S4 implementation is much narrower than requested: `completion_chain_test.go`, `signatures.go`, and `goto_export.go`
  cover helper-level lookups, not whole-expression signature resolution or real LSP goto/chain completion integration.
- Scope contamination/unaccounted tracked changes found outside the provided file-to-task mapping: `.gitignore`,
  `lsp/format.go`, `lsp/server.go`, `lsp/symbols.go`, and `lsp/workspace.go`.

- 2026-05-12: GitNexus impact/context queries failed with a LadybugDB lock error (
  `Could not set lock on file .gitnexus/lbug`); `gitnexus_detect_changes` returned low risk but no changed symbols
  despite file changes.
