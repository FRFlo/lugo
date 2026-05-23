# LSP Module — Lugo Language Server

**Location**: `lsp/` — 71 Go source files, ~50K lines

The LSP implementation is a monolithic Go package implementing the full Language Server Protocol for Lua 5.4, with deep FiveM framework integration.

---

## File Map

### Core Server
| File | Lines | Role |
|------|-------|------|
| `server.go` | ~800 | Server struct, `Start()` loop, message routing, capability registration |
| `rpc.go` | ~200 | `ReadMessage()` — Content-Length JSON-RPC framing |
| `messages.go` | 729 | All LSP request/response/notification type definitions |
| `utils.go` | ~300 | Position/offset helpers, utility functions |

### Document & Workspace Management
| File | Lines | Role |
|------|-------|------|
| `workspace.go` | 1656 | File indexing, change detection, job scheduling, memory eviction |
| `document.go` | ~800 | Document struct, AST/LuaDoc access, local variable resolution |
| `semantic_data.go` | ~400 | Side-table for semantic annotations outside AST |

### Feature Implementations
| File | Lines | Role |
|------|-------|------|
| `features.go` | 3125 | Hover, completion, signature help, inlay hints, semantic tokens |
| `symbols.go` | 2126 | Go-to-definition, find references, document/workspace symbols, call hierarchy |
| `diagnostics.go` | 2059 | 30+ diagnostic types with pragma suppression |
| `refactor.go` | 3159 | 20+ AST-aware refactoring transformations + code actions |
| `format.go` | ~500 | Built-in Lua formatter (AST-based) |
| `signatures.go` | ~300 | SignatureHelp parameter inference |

### Type System
| File | Lines | Role |
|------|-------|------|
| `infer.go` | 1731 | Type inference (union/intersection, control-flow narrowing, metatables) |
| `types_structural.go` | ~300 | Structural type definitions |
| `type_pool.go` | ~200 | Type caching / arena allocation |
| `luadoc.go` | ~800 | LuaDoc annotation parser (`@param`, `@return`, `@class`, etc.) |
| `resolver.go` | 1276 | Semantic resolver: scope tracking, field resolution, type linking |
| `eval.go` | ~300 | Constant expression evaluation |

### Symbol Index
| File | Lines | Role |
|------|-------|------|
| `global_index.go` | 989 | Cross-document symbol index with resource scoping |
| `goto_export.go` | ~200 | Export bridge resolution |

### FiveM Integration
| File | Lines | Role |
|------|-------|------|
| `fivem.go` | 1800 | Manifest parsing, resource graph, runtime profiles, event tracking |
| `fivem_natives.go` | ~500 | Native function catalog (FiveM GTA5 client/server) |
| `export_bridge.go` | ~300 | Cross-resource export resolution |
| `fivem_*.go` | 25+ files | FiveM-specific LSP features |

### Completion
| File | Lines | Role |
|------|-------|------|
| `completion*.go` | ~5 files | Completion item providers (keywords, members, events, exports, etc.) |

### Embedded Libraries
| Path | Role |
|------|------|
| `stdlib/` | Lua standard library stubs (basic, io, os, string, table, math, etc.) |
| `stdlib/fivem/` | FiveM runtime metadata (client.lua, server.lua, shared.lua, manifest.lua) |

### Testing
| File | Lines | Role |
|------|-------|------|
| `*_test.go` | ~10 files | Unit + integration tests |
| `fivem_fixture_harness_test.go` | 888 | Marker-based LSP test framework |
| `testdata/fivem/` | 16 dirs | Fixture directories for FiveM integration tests |

---

## Feature Inventory

| Feature | LSP Method | Implementation |
|---------|-----------|----------------|
| **Completion** | `textDocument/completion` | Member access, locals, globals, keywords, snippets, FiveM exports/events |
| **Hover** | `textDocument/hover` | Type display, LuaDoc rendering, constant eval, deprecation warnings |
| **Go to Definition** | `textDocument/definition` | Local/global resolution, cross-file, exports, events |
| **Find References** | `textDocument/references` | Within-file + workspace-wide, exports, events |
| **Document Symbols** | `textDocument/documentSymbol` | Hierarchical outline (functions, classes, variables) |
| **Workspace Symbols** | `workspace/symbol` | Fuzzy search across all indexed files |
| **Diagnostics** | `textDocument/publishDiagnostics` | 30+ checks: undefined globals, unused vars, shadowing, type errors, FiveM-specific |
| **Signature Help** | `textDocument/signatureHelp` | Parameter display, active parameter, implicit self |
| **Inlay Hints** | `textDocument/inlayHint` | Parameter name hints with smart suppression |
| **Semantic Tokens** | `textDocument/semanticTokens/full` | 10 token types + 4 modifiers |
| **Code Actions** | `textDocument/codeAction` | Quick fixes + bulk safe fixes (refactor.rewrite) |
| **Code Lens** | `textDocument/codeLens` | Reference counters |
| **Rename** | `textDocument/rename` | Cross-file with prepare support |
| **Linked Editing** | `textDocument/linkedEditingRange` | Multi-cursor local renaming |
| **Formatting** | `textDocument/formatting` | AST-based Lua formatter |
| **Folding Range** | `textDocument/foldingRange` | Functions, tables, blocks, comments |
| **Selection Range** | `textDocument/selectionRange` | Semantic expansion (ident → expr → stmt → block → func) |
| **Call Hierarchy** | `textDocument/prepareCallHierarchy` | Incoming/outgoing calls |
| **Document Highlight** | `textDocument/documentHighlight` | Read/write usages of current symbol |

---

## Architectural Patterns

### 1. Document Lifecycle
```
textDocument/didOpen   → Parse + resolve + index + publish diagnostics
textDocument/didChange → Re-parse + re-resolve + re-publish diagnostics
textDocument/didClose  → Drop source cache (keep AST/resolver for warm restart)
```

### 2. Incremental Indexing
- Files hashed on first index; unchanged files skip re-parsing
- Global index uses `clear()` to reuse map memory
- Closed documents evicted from hot cache but retain AST for potential reopen

### 3. Symbol Resolution Flow
```
position → offset → NodeAt(offset) → resolveSymbolAt()
  → Local scope: resolver.ReferenceAt(node) → definition
  → Global scope: GlobalIndex.VisibleSymbols() → fuzzy match
  → FiveM: resolveFiveMResource() → export_bridge → target doc
```

### 4. Diagnostic Pipeline
```
Parse errors (immediate) → Semantic diagnostics (async) → FiveM checks (cross-file)
  → Suppression filter (---@diagnostic disable) → Publish
```

### 5. Type Inference (`infer.go`)
- Lazy evaluation with caching per expression
- Control-flow narrowing (`type(x) == "string"` narrows union)
- Metatable resolution (`setmetatable` + `__index` inheritance)
- Union types via `TypeSet` bitmask
- `require` module alias tracking

### 6. Refactoring Engine (`refactor.go`)
- Each refactoring: AST pattern match → safety check → workspace edit
- Bulk "Safe Fixes" (command: `lugo.applySafeFixes`) apply across file/workspace
- Code actions auto-register via `codeAction` handler + metadata in `refactor.go`

---

## FiveM Integration

FiveM is a significant extension of the LSP:

- **Manifests**: Parses `fxmanifest.lua` and `__resource.lua` for directives, dependencies, and file lists
- **Resource Graph**: Cross-resource dependency tracking for exports and includes
- **Execution Profiles**: Files classified as `client`, `server`, or `shared`; profile-scoped globals
- **Native Functions**: FiveM GTA5 client/server native catalog generated into Go source by `go generate ./lsp`; scripted builds run generation first, while plain `go build` uses the checked-in generated structs
- **Events**: Cross-references `AddEventHandler`, `TriggerServerEvent`, `TriggerClientEvent`, `RegisterNetEvent` with direction validation
- **Exports**: `exports.resourceName:method()` resolution across resource boundaries

---

## Testing Patterns

The LSP module uses a custom **marker-based fixture harness** for integration testing:

```lua
-- In test fixture Lua files:
return UnknownReceiver:missingMethod() --[[@code_action_unresolved_method]]
```

- Markers (`--[[@markerName]]`) are embedded in fixture Lua source
- Markers map to exact byte positions in the source
- Tests call LSP methods at marker positions and assert results
- 16 fixture directories under `testdata/fivem/` cover different workspace layouts

External test packages use `package lsp` (not `package lsp_test`) to access unexported internals.
