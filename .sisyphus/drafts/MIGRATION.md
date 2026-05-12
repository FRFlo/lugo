# C1 Migration Audit

## Executive Summary

This phase is documentation plus compatibility scaffolding only. The current `lsp/` package stays intact; the new
architecture is introduced behind shims so existing callers keep working while `GlobalIndex v2` and the semantic side
table are prepared.

## Server Struct Fields

### I/O and logging

| Field Name | Current Type    | Classification | Replacement / Notes                     |
|------------|-----------------|----------------|-----------------------------------------|
| Reader     | `*bufio.Reader` | STAY           | Keeps stdio JSON-RPC input.             |
| Writer     | `io.Writer`     | STAY           | Keeps stdio JSON-RPC output.            |
| Log        | `*plain.Plain`  | STAY           | Shared logger remains the runtime sink. |

### Workspace state

| Field Name         | Current Type           | Classification | Replacement / Notes                                          |
|--------------------|------------------------|----------------|--------------------------------------------------------------|
| Documents          | `map[string]*Document` | STAY           | Canonical document registry remains.                         |
| OpenFiles          | `map[string]bool`      | STAY           | Still required for cache eviction rules.                     |
| activeURIs         | `map[string]bool`      | STAY           | Keeps incremental workspace tracking.                        |
| visitedDirs        | `map[string]bool`      | STAY           | Keeps directory traversal dedupe.                            |
| FiveMResourceGraph | `*FiveMResourceGraph`  | STAY           | FiveM resource topology remains the current source of truth. |
| uriCache           | `map[string]string`    | STAY           | URI/path memoization remains a fast-path cache.              |
| symlinkCache       | `map[string]string`    | STAY           | Symlink canonicalization cache remains.                      |

### Global index and symbol resolution

| Field Name        | Current Type                   | Classification | Replacement / Notes                          |
|-------------------|--------------------------------|----------------|----------------------------------------------|
| GlobalIndex       | `map[GlobalKey][]GlobalSymbol` | REPLACE        | Becomes `GlobalIndex v2` via compat wrapper. |
| KnownGlobals      | `map[string]bool`              | STAY           | User-known globals remain a filter input.    |
| KnownGlobalGlobs  | `[]string`                     | STAY           | Wildcard support remains.                    |
| LibraryPaths      | `[]string`                     | STAY           | Library indexing inputs remain.              |
| lowerLibraryPaths | `[]string`                     | STAY           | Normalized path cache remains.               |
| IgnoreGlobs       | `[]string`                     | STAY           | Workspace ignore configuration remains.      |
| compiledIgnores   | `[]IgnorePattern`              | STAY           | Precompiled ignore matcher remains.          |
| BannedSymbols     | `map[string]string`            | STAY           | Diagnostic policy remains.                   |

### Shared parsers and buffers

| Field Name       | Current Type                    | Classification | Replacement / Notes                          |
|------------------|---------------------------------|----------------|----------------------------------------------|
| sharedParser     | `*parser.Parser`                | STAY           | Shared parser object remains.                |
| diagBuf          | `[]Diagnostic`                  | STAY           | Reused diagnostics buffer remains.           |
| semTokensBuf     | `[]SemanticToken`               | STAY           | Semantic token buffer remains.               |
| semDataBuf       | `[]uint32`                      | STAY           | Packed semantic token payload remains.       |
| actualReadsBuf   | `[]int`                         | STAY           | Shared read-tracking buffer remains for now. |
| depCache         | `map[ast.NodeID]DepInfo`        | STAY           | Dependency cache remains.                    |
| seenKeysBuf      | `map[uint64]ast.NodeID`         | STAY           | Dedup cache remains.                         |
| unusedDefsBuf    | `[]bool`                        | STAY           | Reused analysis buffer remains.              |
| deadStoresBuf    | `map[ast.NodeID]*DeadStoreInfo` | STAY           | Reused diagnostic cache remains.             |
| suggestCache     | `map[string]string`             | STAY           | Completion suggestion cache remains.         |
| visibilityCache  | `map[*Document]bool`            | STAY           | Visibility cache remains.                    |
| sharedCommentBuf | `[]byte`                        | STAY           | Shared byte scratch buffer remains.          |
| sharedDepBuf     | `[]byte`                        | STAY           | Shared dependency scratch buffer remains.    |

### Configuration and sizing

| Field Name            | Current Type | Classification | Replacement / Notes              |
|-----------------------|--------------|----------------|----------------------------------|
| Version               | `string`     | STAY           | Server version string remains.   |
| RootURI               | `string`     | STAY           | Workspace root identity remains. |
| lowerRootPath         | `string`     | STAY           | Lowercased root cache remains.   |
| WorkspaceFolders      | `[]string`   | STAY           | LSP workspace folders remain.    |
| lowerWorkspaceFolders | `[]string`   | STAY           | Normalized folder cache remains. |
| MaxParseErrors        | `int`        | STAY           | Parser threshold remains.        |
| MaxFileSize           | `int64`      | STAY           | File size guard remains.         |

### Diagnostics and features

| Field Name               | Current Type | Classification | Replacement / Notes                 |
|--------------------------|--------------|----------------|-------------------------------------|
| IsIndexing               | `bool`       | STAY           | Reindex lifecycle flag remains.     |
| DiagUndefinedGlobals     | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagImplicitGlobals      | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagUnusedLocal          | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagUnusedFunction       | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagUnusedParameter      | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagUnusedLoopVar        | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagShadowing            | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagUnreachableCode      | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagAmbiguousReturns     | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagDeprecated           | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagDuplicateField       | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagUnbalancedAssignment | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagDuplicateLocal       | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagSelfAssignment       | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagEmptyBlock           | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagFormatString         | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagTypeCheck            | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagRedundantParameter   | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagRedundantValue       | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagRedundantReturn      | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagLoopVarMutation      | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagIncorrectVararg      | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagShadowingLoopVar     | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagConstantCondition    | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagUnreachableElse      | `bool`       | STAY           | Existing diagnostic toggle remains. |
| DiagUsedIgnoredVar       | `bool`       | STAY           | Existing diagnostic toggle remains. |
| InlayParamHints          | `bool`       | STAY           | Existing feature toggle remains.    |
| InlaySuppressMatch       | `bool`       | STAY           | Existing feature toggle remains.    |
| InlayImplicitSelf        | `bool`       | STAY           | Existing feature toggle remains.    |
| FeatureDocHighlight      | `bool`       | STAY           | Existing feature toggle remains.    |
| FeatureHoverEval         | `bool`       | STAY           | Existing feature toggle remains.    |
| FeatureCodeLens          | `bool`       | STAY           | Existing feature toggle remains.    |
| FeatureFormatting        | `bool`       | STAY           | Existing feature toggle remains.    |
| FormatOpinionated        | `bool`       | STAY           | Existing feature toggle remains.    |
| SuggestFunctionParams    | `bool`       | STAY           | Existing feature toggle remains.    |
| FeatureFormatAlerts      | `bool`       | STAY           | Existing feature toggle remains.    |

### FiveM and CI

| Field Name                    | Current Type                        | Classification | Replacement / Notes                                  |
|-------------------------------|-------------------------------------|----------------|------------------------------------------------------|
| FeatureFiveM                  | `bool`                              | STAY           | Must remain the hard gate for every FiveM-only path. |
| DiagFiveMUnaccountedFile      | `bool`                              | STAY           | FiveM diagnostic toggle remains.                     |
| DiagFiveMUnknownExport        | `bool`                              | STAY           | FiveM diagnostic toggle remains.                     |
| DiagFiveMUnknownResource      | `bool`                              | STAY           | FiveM diagnostic toggle remains.                     |
| DiagFiveMEventDirection       | `bool`                              | STAY           | FiveM diagnostic toggle remains.                     |
| DiagFiveMUnregisteredNetEvent | `bool`                              | STAY           | FiveM diagnostic toggle remains.                     |
| DiagFiveMUnknownEvent         | `bool`                              | STAY           | FiveM diagnostic toggle remains.                     |
| fiveMNativeBundleLoader       | `func(name string) ([]byte, error)` | STAY           | Loader shim remains under FeatureFiveM.              |
| IsCI                          | `bool`                              | STAY           | CI mode remains.                                     |
| CIDiagnosticCount             | `int`                               | STAY           | CI accounting remains.                               |
| CIErrorCount                  | `int`                               | STAY           | CI accounting remains.                               |

## Document Struct Fields

### Core identity and ownership

| Field Name | Current Type         | Classification | Replacement / Notes                                |
|------------|----------------------|----------------|----------------------------------------------------|
| Server     | `*Server`            | STAY           | Document remains owned by the current server.      |
| Tree       | `*ast.Tree`          | STAY           | AST stays canonical during migration.              |
| Resolver   | `*semantic.Resolver` | STAY           | Resolver stays until the new resolver is cut over. |

### Side-table candidates

| Field Name         | Current Type       | Classification | Replacement / Notes                                       |
|--------------------|--------------------|----------------|-----------------------------------------------------------|
| TypeCache          | `[]TypeSet`        | REPLACE        | Moves to semantic side table.                             |
| Inferring          | `[]bool`           | REPLACE        | Moves to semantic side table.                             |
| LuaDocCache        | `[]*LuaDoc`        | REPLACE        | Moves to semantic side table.                             |
| ActualReads        | `[]uint16`         | REPLACE        | Moves to semantic side table.                             |
| MutatedLocals      | `[]bool`           | REPLACE        | Moves to semantic side table.                             |
| ExportedGlobalDefs | `[]ExportedSymbol` | REPLACE        | Moves to semantic side table with export graph ownership. |

### File identity and module metadata

| Field Name   | Current Type          | Classification | Replacement / Notes                            |
|--------------|-----------------------|----------------|------------------------------------------------|
| Errors       | `[]parser.ParseError` | STAY           | Parse errors remain part of the document view. |
| URI          | `string`              | STAY           | Document identity remains.                     |
| Path         | `string`              | STAY           | Filesystem identity remains.                   |
| LowerPath    | `string`              | STAY           | Lowercased path cache remains.                 |
| Dir          | `string`              | STAY           | Directory identity remains.                    |
| ModuleName   | `string`              | STAY           | Module naming remains.                         |
| ExportedNode | `ast.NodeID`          | STAY           | Export anchor remains.                         |

### FiveM metadata

| Field Name         | Current Type            | Classification | Replacement / Notes                  |
|--------------------|-------------------------|----------------|--------------------------------------|
| FiveMProfile       | `FiveMExecutionProfile` | STAY           | Profile metadata remains.            |
| IsMeta             | `bool`                  | STAY           | Manifest/meta file identity remains. |
| IsLibrary          | `bool`                  | STAY           | Library classification remains.      |
| IsWorkspace        | `bool`                  | STAY           | Workspace classification remains.    |
| IsFiveMManifest    | `bool`                  | STAY           | Manifest detection remains.          |
| FiveMLuaExports    | `[]FiveMLuaExport`      | STAY           | Export metadata remains.             |
| FiveMEvents        | `[]FiveMEventInfo`      | STAY           | Event metadata remains.              |
| FiveMProfileCached | `bool`                  | STAY           | Cache flag remains.                  |

### Misc

| Field Name  | Current Type  | Classification | Replacement / Notes                 |
|-------------|---------------|----------------|-------------------------------------|
| ModTime     | `time.Time`   | STAY           | File timestamp remains.             |
| DiagPragmas | `DiagPragmas` | STAY           | Per-file suppression state remains. |

## GlobalIndex Migration Plan

Current shape: `map[GlobalKey][]GlobalSymbol`.

Target shape: `GlobalIndex v2` with hierarchical partitions plus dependency edges.

Plan:

1. Keep the legacy flat map alive behind `CompatGlobalIndex`.
2. Mirror writes into the v2 structure during the migration window.
3. Prefer v2 lookups first; fall back to legacy data for parity.
4. Keep `FeatureFiveM` as the gate for any FiveM-specific partitioning, invalidation, or export lookups.
5. Remove the legacy map only after parity tests cover hover, completion, definition, references, and diagnostics.

## Compatibility Shim Specification

### `CompatGlobalIndex`

- Wraps the legacy map and the v2 structure.
- Provides dual-read lookup helpers.
- Supports write-through mirroring while callers are migrated.

### `CompatDocument`

- Wraps the current `Document` plus the new semantic side table.
- Reads semantic caches from the new table first, then falls back to legacy document fields.
- Sync helpers keep legacy fields populated until the cutover.

### Boundary rules

- Legacy callers keep using current package symbols.
- New code reads through the shim only.
- No FiveM-only path may bypass `FeatureFiveM`.
- `FeatureFiveM=false` must remain a no-op for all FiveM-specific caches and lookups.

## Migration Timeline

1. Audit and freeze field ownership.
2. Add shims and parity tests.
3. Introduce v2 side tables behind compatibility helpers.
4. Route new code through v2 while keeping legacy reads alive.
5. Remove legacy storage after parity and build checks are stable.

## Removed Fields

None in C1.
