# Lugo — Lua 5.4 Parser & LSP

**Module**: `github.com/coalaura/lugo` · **Go** 1.26.1 · **License**: MIT

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
└── scripts/                 # Build utilities (fivem_native_catalog generator)
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
# Test (race detector enabled)
go test -v -race ./...

# Build LSP binary
go build -o lugo .

# VS Code extension (from vscode/ dir)
cd vscode && npm install && npm run build

# CI mode (headless diagnostics)
./lugo --ci example.ci.json
```

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

This project is indexed by GitNexus as **lugo** (2364 symbols, 8164 relationships, 197 execution flows). Use the
GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run
  `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected
  processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and
  execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping.
  It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use
  `gitnexus_context({name: "symbolName"})`.

## When Debugging

1. `gitnexus_query({query: "<error or symptom>"})` — find execution flows related to the issue
2. `gitnexus_context({name: "<suspect function>"})` — see all callers, callees, and process participation
3. `READ gitnexus://repo/lugo/process/{processName}` — trace the full execution flow step by step
4. For regressions: `gitnexus_detect_changes({scope: "compare", base_ref: "main"})` — see what your branch changed

## When Refactoring

- **Renaming**: MUST use `gitnexus_rename({symbol_name: "old", new_name: "new", dry_run: true})` first. Review the
  preview — graph edits are safe, text_search edits need manual review. Then run with `dry_run: false`.
- **Extracting/Splitting**: MUST run `gitnexus_context({name: "target"})` to see all incoming/outgoing refs, then
  `gitnexus_impact({target: "target", direction: "upstream"})` to find all external callers before moving code.
- After any refactor: run `gitnexus_detect_changes({scope: "all"})` to verify only expected files changed.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Tools Quick Reference

| Tool             | When to use                   | Command                                                                 |
|------------------|-------------------------------|-------------------------------------------------------------------------|
| `query`          | Find code by concept          | `gitnexus_query({query: "auth validation"})`                            |
| `context`        | 360-degree view of one symbol | `gitnexus_context({name: "validateUser"})`                              |
| `impact`         | Blast radius before editing   | `gitnexus_impact({target: "X", direction: "upstream"})`                 |
| `detect_changes` | Pre-commit scope check        | `gitnexus_detect_changes({scope: "staged"})`                            |
| `rename`         | Safe multi-file rename        | `gitnexus_rename({symbol_name: "old", new_name: "new", dry_run: true})` |
| `cypher`         | Custom graph queries          | `gitnexus_cypher({query: "MATCH ..."})`                                 |

## Impact Risk Levels

| Depth | Meaning                               | Action                |
|-------|---------------------------------------|-----------------------|
| d=1   | WILL BREAK — direct callers/importers | MUST update these     |
| d=2   | LIKELY AFFECTED — indirect deps       | Should test           |
| d=3   | MAY NEED TESTING — transitive         | Test if critical path |

## Resources

| Resource                              | Use for                                  |
|---------------------------------------|------------------------------------------|
| `gitnexus://repo/lugo/context`        | Codebase overview, check index freshness |
| `gitnexus://repo/lugo/clusters`       | All functional areas                     |
| `gitnexus://repo/lugo/processes`      | All execution flows                      |
| `gitnexus://repo/lugo/process/{name}` | Step-by-step execution trace             |

## Self-Check Before Finishing

Before completing any code modification task, verify:

1. `gitnexus_impact` was run for all modified symbols
2. No HIGH/CRITICAL risk warnings were ignored
3. `gitnexus_detect_changes()` confirms changes match expected scope
4. All d=1 (WILL BREAK) dependents were updated

## Keeping the Index Fresh

After committing code changes, the GitNexus index becomes stale. Re-run analyze to update it:

```bash
npx gitnexus analyze
```

If the index previously included embeddings, preserve them by adding `--embeddings`:

```bash
npx gitnexus analyze --embeddings
```

To check whether embeddings exist, inspect `.gitnexus/meta.json` — the `stats.embeddings` field shows the count (0 means
no embeddings). **Running analyze without `--embeddings` will delete any previously generated embeddings.**

> Claude Code users: A PostToolUse hook handles this automatically after `git commit` and `git merge`.

## CLI

| Task                                         | Read this skill file                                        |
|----------------------------------------------|-------------------------------------------------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md`       |
| Blast radius / "What breaks if I change X?"  | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?"             | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md`       |
| Rename / extract / split / refactor          | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md`     |
| Tools, resources, schema reference           | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md`           |
| Index, status, clean, wiki CLI commands      | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md`             |

<!-- gitnexus:end -->
