- AddSymbol must remove the prior same-name/scope/resource entry from HashIndex before re-appending the new symbol to
  avoid stale hash duplicates.
- Export lookup should delegate to ExportBridge.LookupExport so dependency/dependent traversal and scope-aware fallback
  stay consistent.
- InferColonMethod needs to bind SelfType on the underlying FunctionType used by callers/tests, not just a detached
  copy.
- Manual QA rerun on 2026-05-11 passed full `go test ./lsp/`, `TestFiveM`, and targeted integration/edge suites covering
  InferColonMethod self-binding, ExportBridge/goto_export resolution, ChainComplete ordering/filtering, circular
  dependency diagnostics, shared-scope partitioning, and warmup cancellation.
- Removing the LSP package `V2` suffixes requires renaming the legacy `Server.GlobalIndex` map to
  `Server.LegacyGlobalIndex` first so the enriched `*GlobalIndex` field can take the canonical `GlobalIndex` name
  without collisions.
- The FiveM `IsCfxV2` manifest/runtime flags are product terminology, not versioned LSP implementation names, so they
  should stay untouched during package-wide V2 cleanup.
