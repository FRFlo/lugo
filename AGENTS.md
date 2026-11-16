# Lugo — Lua 5.4 Parser & LSP

**Module**: `github.com/FRFlo/lugo` · **Go** 1.26.1 · **License**: MIT

A ridiculously fast, zero-allocation Lua 5.4 parser and Language Server (LSP) written in Go. Designed for massive codebases (game servers, modding frameworks) where traditional LSPs struggle with RAM and indexing speed.

---

## Architecture Overview

```
main.go                      # Binary entry point (creates LSP server, handles --ci flag)
│
├── ast/              (2)    # Flat-array arena AST — all nodes in []Node slice
├── lexer/            (2)    # Zero-allocation tokenizer ([]byte offsets, no heap strings)
├── token/            (1)    # Token type definitions + TokenSet bitmask utility
├── parser/           (2)    # Pratt parser with precedence climbing + panic-mode error recovery
├── semantic/         (2)    # Resolver: links variable references to definitions
├── lsp/             (71)    # Full LSP implementation (hover, completion, diagnostics, etc.)
│   ├── server.go            # JSON-RPC handler, lifecycle, capability registration
│   ├── rpc.go               # Content-Length framed message parsing
│   ├── messages.go          # All LSP type definitions (729 lines)
│   ├── features.go          # Hover, completion, signature help, inlay hints, semantic tokens
│   ├── symbols.go           # Go-to-definition, references, document/workspace symbols
│   ├── diagnostics.go       # 30+ diagnostic types with pragma suppression
│   ├── refactor.go          # 20+ AST-aware refactoring transformations
│   ├── infer.go             # Type inference engine (union types, control-flow narrowing)
│   ├── workspace.go         # File indexing, change detection, job scheduling
│   ├── resolver.go          # Semantic resolver (scope tracking, field resolution)
│   ├── global_index.go      # Cross-document symbol index with resource scoping
│   ├── format.go            # Built-in Lua formatter
│   ├── luadoc.go            # LuaDoc annotation parser (@param, @return, @class, etc.)
│   ├── signatures.go        # Signature help parameter inference
│   ├── fivem.go             # FiveM: manifests, resources, events, natives
│   ├── fivem_natives.go     # FiveM native function catalog
│   ├── export_bridge.go     # Cross-resource export resolution
│   ├── stdlib/              # Embedded Lua + FiveM standard library stubs
│   ├── completion*.go       # Completion item providers
│   └── fivem_*.go           # FiveM-specific LSP features (25+ files)
├── vscode/            (10)  # VS Code extension (extension.js, package.json)
├── fivem-specs/             # FiveM reference documentation
└── scripts/                 # Build utilities
```

**Numbers**: 233 source files, ~31K lines Go, 82 Go files, 93 Lua files, 20 files >500 lines.

---

## Key Conventions

### Go Code
- **gofmt** formatting (tabs, standard layout) — no external formatter config
- **No type error suppression** (`as any`, `@ts-ignore`) in any language
- **Table-driven tests** with `t.Run()` subtests
- **Benchmarks** use `b.ReportAllocs()` and `b.Loop()` (Go 1.24+)
- **Minimal dependencies** — only `github.com/coalaura/plain` (logging)

### Zero-Allocation Design (pervasive)
- Byte offsets (`uint32`), never heap strings
- Flat `[]Node` arena for AST (no per-node pointers/allocations)
- `TokenSet [2]uint64` bitmask for O(1) token lookups
- Pre-computed character property table (`[256]uint8`)
- Compiler-optimized `switch string([]byte)` for keyword matching (zero-alloc)
- `Reset()` methods reuse slice capacities across parses

### Lua (in stdlib + fixtures)
- **LuaCATS-style annotations**: `---@class`, `---@param`, `---@return`, `---@field`, `---@alias`, `---@type`, `---@generic`, `---@overload`, `---@deprecated`, `---@see`
- **Diagnostic suppression**: `---@diagnostic disable-next-line code` or `---@diagnostic disable-file code`
- **Ignored variables**: Prefix with `_` to mark as intentionally unused
- **Special comment tags**: `NOTE:`, `TODO:`, `FIXME:`, `WARNING:` auto-formatted in hovers

### FiveM
- Resources identified by `fxmanifest.lua` or `__resource.lua`
- Execution profiles: `client`, `server`, `shared`
- Export bridge: `exports.resourceName:method()` syntax

---

## Build & Test

```bash
# Regenerate FiveM native structs from live upstream metadata
go generate ./lsp

# Test (race detector enabled)
go test -v -race ./...

# Build LSP binary
go build -o lugo .

# VS Code extension (from vscode/ dir)
cd vscode && npm install && npm run build

# CI mode (headless diagnostics)
./lugo --ci example.ci.json
```

`build.sh`, `build.cmd`, and CI workflows run `go generate ./lsp` before testing/building so scripted builds fetch current FiveM native metadata. A plain `go build` uses the checked-in generated Go structs and does not contact upstream by itself.

**Release pipeline** (`.github/workflows/release.yml`): Zig 0.15.2 cross-compilation for 6 platforms (linux/windows/darwin × amd64/arm64), static musl builds, dual artifact strategy (CLI binaries + VS Code extension binaries).

---

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| **Flat-array AST** ([]Node, not pointers) | Cache locality, zero GC pressure, compact (48 bytes/node), reusable arena |
| **Byte offsets instead of strings** | No heap allocation during lexing/parsing; deferred string conversion |
| **Three-token lookahead** | Efficient Pratt parsing without backtracking |
| **Panic-mode error recovery** | Parser syncs on statement boundaries, continues after errors |
| **Incremental indexing** | File hashing skips unchanged files; map `clear()` reuses memory |
| **Monolithic `lsp/` package** | Avoids import cycles in LSP handler code; all internal state shared |
| **LSP-as-linter** (`--ci` flag) | Server doubles as CI linter via `--ci` flag, outputs GitHub Actions annotations |
| **Zig cross-compilation** | Enables CGO static builds across 6 platforms without native toolchains |

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **lugo** (5611 symbols, 25530 relationships, 479 execution flows).

> Index stale? Run `node .gitnexus/run.cjs analyze --index-only` from the project root — it auto-selects an available runner. No `.gitnexus/run.cjs` yet? Bootstrap with `npx`, `bunx`, or `pnpm dlx` — e.g. `bunx gitnexus@latest analyze` (npm 11 npx crash; #1939).

## Always Do

- **MUST run impact before editing.** Use `impact({target: "symbolName", direction: "upstream"})` or `node .gitnexus/run.cjs impact "symbolName" --direction upstream --repo .`; report callers, processes, and risk. Never substitute grep for graph analysis.
- **MUST analyze graph changes before committing.** Use `detect_changes({scope: "all"})` (MCP) or `node .gitnexus/run.cjs detect-changes --scope all --repo .` (CLI fallback). `partial: true` or `truncated: true` is not a clean check — a zero means unseen, not unaffected; re-run it. For regression review: `detect_changes({scope: "compare", base_ref: "master"})` or `node .gitnexus/run.cjs detect-changes --scope compare --base-ref "master" --repo .`.
- MUST warn on HIGH/CRITICAL `risk` pre-edit; never use `riskSharedAxes` to waive a HIGH/CRITICAL `risk` warning. Compare File/symbol: MCP File omits axes; Graph-RAG expands File.
- **MUST treat `risk: UNKNOWN` as unresolved, not as low.** An empty caller set is not evidence the symbol is unused — it can also mean the callers are not resolvable by the index (plain-object property access, dynamic dispatch, cross-language calls). `impact` pairs `UNKNOWN` with a `riskNote` saying so. Confirm with a text search before treating the symbol as safe to change or delete; do not proceed on the strength of a zero.
- **MUST use `query({search_query: "concept"})` for concepts/flows, `context({name: "symbolName"})` for a named symbol, or `impact` for blast radius, on read-only callers, dependencies, imports, or execution flow.** Graph first; text search only for empty/`UNKNOWN`/literals.
- For security review, `explain({target: "fileOrSymbol"})` lists taint findings (source→sink flows; needs `analyze --pdg`).

## Never Do

- NEVER edit a function, class, or method before MCP/CLI impact analysis.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis, and never read `UNKNOWN` as an all-clear — it means the walk could not answer, which is the one verdict that requires confirming by other means.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit before MCP/CLI graph change analysis.

## Resources

| Resource | Use for |
| --- | --- |
| `gitnexus://repo/lugo/context` | Codebase overview, check index freshness |
| `gitnexus://repo/lugo/clusters` | All functional areas |
| `gitnexus://repo/lugo/processes` | All execution flows |
| `gitnexus://repo/lugo/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
| --- | --- |
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
