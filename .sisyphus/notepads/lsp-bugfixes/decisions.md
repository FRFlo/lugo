- Kept GotoExportTarget backward-compatible by making the desired scope variadic, while still routing through
  ExportBridge.LookupExport.
- Removed the now-unused dependencyList helper instead of leaving an unused-symbol warning behind.
- Renamed compatibility-layer `CompatGlobalIndexV2` storage to `CompatPartitionedGlobalIndex`/`Scoped` instead of
  forcing a conflicting `CompatGlobalIndex` rename, preserving the wrapper type while still removing version suffixes.
