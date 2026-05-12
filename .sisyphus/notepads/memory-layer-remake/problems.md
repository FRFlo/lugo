## C2 Hybrid Type System + Interning Pool - 2026-05-11

- No unresolved implementation problems remain for C2. Follow-up migration tasks still need to decide where the new
  `Type` is integrated into resolver/global-index flows.

## E1 Two-Pass Resolver v2 - 2026-05-11

- No unresolved E1 implementation blockers remain. Later migration tasks still need to wire `ResolverV2` into workspace
  indexing/LSP feature paths once S1-S4 are ready to consume populated v2 semantic data.
- Full FiveM native/export runtime metadata remains represented by a minimal v2 hook shim in `lsp/fivem_natives.go`;
  later FiveM migration tasks should replace this shim with complete generated/runtime metadata registration.
