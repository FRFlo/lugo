![banner](banner.jpg)

[![Tests](https://github.com/FRFlo/lugo/actions/workflows/test.yml/badge.svg)](https://github.com/FRFlo/lugo/actions/workflows/test.yml)

A ridiculously fast, zero-allocation Lua 5.4 parser and Language Server (LSP) written in Go.

Lugo is built from the ground up for maximum performance. By iterating over source code using a flat-array/arena architecture (`[]Node`) and storing only byte offsets, it heavily eliminates pointer allocations, heap strings and garbage collection pressure.

[**Install from the VS Code Marketplace**](https://marketplace.visualstudio.com/items?itemName=FRFlo.lugo-vscode-fivem-enhanced)

## Why Lugo?

Most Lua language servers struggle when dropped into massive codebases (like game server environments or large modding frameworks). They consume gigabytes of RAM, take minutes to index and lag while typing.

**Lugo is different:**
* **Blistering Performance:** In real-world benchmarks on modern hardware, Lugo completely cold-indexes massive workspaces (including full AST generation, symbol resolution and publishing workspace-wide diagnostics) in a matter of seconds.
  ```text
  Starting workspace re-index...
  Indexing external library: /opt/lua-fivem-sdk
  Indexing workspace folder: file:///workspace/server/resources
  Indexing workspace folder: file:///workspace/server/framework-assets
  Indexing workspace folder: file:///workspace/server/legacy-assets
  Re-indexed workspace in 746.5286ms (indexed=3042, unchanged=0, failed=0)
  Published diagnostics for 3018 files in 720.1679ms
  Total time taken for 29668087 bytes: 1.4666965s
  ```
* **Incremental Warm Starts:** Lugo hashes your workspace files. If you trigger a re-index, it skips parsing unchanged files and reuses map memory pools (`clear()`), dropping warm re-indexes to a fraction of a second.
* **Zero-Allocation Architecture:** The parser, lexer and symbol resolver are designed to never allocate heap strings during normal typing. Tight loops execute inside CPU registers, leveraging SIMD-accelerated byte scanning to maximize cache locality.
* **Microscopic Memory Footprint:** Lugo only stores flat arrays of integers. Only actively open files keep their source strings in memory, meaning Lugo can index thousands of files while consuming a fraction of the RAM used by traditional LSPs.
* **Dynamic by Design:** Instead of forcing strict typing on a dynamic language, Lugo embraces Lua. If you do `MySQL = this` in a local file, Lugo dynamically resolves all deep table fields (e.g., `MySQL.Await.Execute`) across your entire workspace in real-time.
* **Standalone Binary:** No NodeJS, no Java, no Lua runtimes. Just a single, blazingly fast compiled Go binary.

## Features & Capabilities

Lugo implements a comprehensive suite of modern Language Server Protocol features:

* **Intelligent Autocomplete:** Resilient, context-aware member access (`table.|`), locals, globals and keywords. Works even when the surrounding syntax tree is temporarily broken.
* **Semantic Tokens (Rich Highlighting):** Compiler-accurate syntax highlighting. Visually distinguishes locals from globals, properties from methods and identifies modifiers like `readonly` (`<const>`), `deprecated` and `defaultLibrary`.
* **Document Highlights:** Click or move your cursor over any variable or function to instantly highlight all read/write usages within the current file.
* **Smart Selection (Selection Range):** Press `Shift+Alt+RightArrow` to semantically expand your text selection based on the AST (Identifier -> Call Expression -> Statement -> Block -> Function -> File).
* **Go to Definition & Hover:** Instant cross-file jumps. Fully parses LuaDoc (`@param`, `@return`, `@field`, `@class`, `@alias`, `@type`, `@generic`, `@overload`, `@see`, `@deprecated`) and renders beautifully formatted function signatures.
* **Hover Evaluation:** Lugo statically evaluates constant expressions (math, bitwise operations, string concatenation and logic) in real-time, displaying the computed result directly in the hover tooltip.
* **Advanced Type Inference:** Lazily evaluates and caches types. Supports control-flow type narrowing (e.g., `type(x) == "string"`), loop variable unpacking (`ipairs`/`pairs`), `require` module aliasing/exports and deep metatable resolution (understands `setmetatable` and `__index` inheritance).
* **Find References & Code Lens:** Find all usages of a symbol across your workspace. Automatically embeds clickable Code Lens reference counters directly above function definitions.
* **Format Alerts:** Automatically formats special comment tags (e.g., `NOTE:`, `TODO:`, `FIXME:`, `WARNING:`) with emojis and bold text in hover tooltips for better visibility.
* **Rename & Linked Editing Ranges:** Instantly rename symbols across your workspace. Supports Linked Editing for simultaneous, multi-cursor renaming of local variables as you type.
* **Call Hierarchy:** Visually explore a tree of incoming and outgoing function calls.
* **Document & Workspace Symbols:** Instant workspace-wide search (`Ctrl+T`) for fully qualified names (e.g., `OP.Math.Round`) and full VS Code "Outline" tree generation.
* **Signature Help & Inlay Hints:** Real-time active-parameter tooltips and inline parameter name hints with smart implicit `self` offset calculation. Automatically suppresses hints when the argument matches the parameter name to reduce visual noise.
* **Code Actions (Quick Fixes & Refactoring):** Fast automated fixes for common diagnostics (prefixing unused variables, adding `local`, fixing typos). Includes powerful **AST-aware refactorings**: invert conditions, recursively convert `if` chains to early returns, optimize `table.insert` to `t[#t+1]`, convert between dot/colon method signatures, merge nested `if` statements, split multiple assignments, swap `if`/`else` branches, remove redundant parentheses, convert `for i=1, #t` to `ipairs` and toggle between dot/bracket table indexing. Includes **bulk Safe Fixes** (via command palette) to automatically clean up unused variables, parameters and assignments across the current file or your entire workspace securely without breaking side-effects.
* **Diagnostic Suppression:** Disable specific diagnostics per-line or per-file using standard `---@diagnostic disable-line code` comments (with built-in Code Actions to instantly generate them).
* **Full Lua 5.4 Support:** Native parsing, type-inference and semantic highlighting for `<const>` and `<close>` attributes, `goto` statements and `::labels::`.
* **FiveM Profiles, Manifests & Runtime Metadata:** Native support for `fxmanifest.lua` and `__resource.lua`, profile-scoped runtime globals, export/resource validation, callable bridge signatures and manifest-aware diagnostics.
* **FiveM Event Intelligence:** Indexes `RegisterNetEvent`, `AddEventHandler`, `TriggerEvent`, `TriggerServerEvent` and `TriggerClientEvent` across resources. Events support completion, hover, go-to-definition, references, workspace symbols and Code Lens, with diagnostics for unknown events, invalid client/server direction, missing registrations and payload arity mismatches.
* **Cross-Resource Contracts:** Tracks resource dependencies, exports, convars, state bags and NUI callback/message names. It validates declarations and consumers across Lua and JavaScript assets while preserving resource/profile boundaries.
* **Native and Framework Metadata:** Generated FiveM native catalogs, runtime libraries, JSON/msgpack/GLM types, OAL-aware signatures, and optional ESX, QBCore and ox framework metadata are indexed without requiring a local FiveM installation.
* **File Watching:** Automatically synchronizes with workspace file creations, deletions and external changes in real-time.
* **Built-in Formatter:** A blazingly fast, AST-aware Lua formatter. Elegantly fixes whitespace, enforces indentation rules, strips trailing semicolons, expands minified code and optionally applies opinionated stylistic tweaks (like separating unrelated statements with blank lines).
* **Folding Ranges:** Accurately fold functions, tables, control flow blocks and multi-line strings/comments.
* **Embedded Standard Library:** Indexes Lua 5.4 standard library stubs from the embedded filesystem so runtime globals, functions and types are available for hover, completion and diagnostics without requiring workspace files.
* **Fast-Path Smart Ignores:** Automatically inherits VS Code's native `files.exclude` and `search.exclude` settings. Lugo pre-compiles these into high-speed prefix/suffix byte matchers, instantly skipping ignored directories without the overhead of regex.

### Advanced Diagnostics
Lugo performs workspace-wide analysis to catch bugs before runtime:
* **Undefined Globals:** Detects typos with wildcard ignore support (e.g., `N_0x*`) and provides quick-fixes to the closest known global.
* **Implicit Globals:** Warns when you forget the `local` keyword inside a function and provides a quick-fix to inject it.
* **Unused Variables:** Granular detection for unused locals, functions, parameters and loop variables.
* **Shadowing:** Warns when a local or loop variable shadows an outer scope or global, providing a clickable link to the shadowed definition.
* **Unreachable Code:** Detects dead code after `return`, `break` or `goto`, as well as statically unreachable `elseif` or `else` branches.
* **Constant Conditions:** Warns when a condition is statically known to be always true or always false.
* **Ambiguous Returns:** Catches Lua's infamous newline evaluation trap where expressions on the next line are accidentally returned.
* **Redundant Code:** Warns about empty blocks (`do end`), self-assignments, redundant parameters, redundant assignment values and redundant returns (with quick-fixes to remove them).
* **Sanity Checks:** Detects duplicate fields in table literals, unbalanced assignments, loop variable mutations and incorrect vararg (`...`) usage.
* **Type Checking:** Optionally catches strictly invalid operations like attempting to call a number or index a non-table.
* **Format String Validation:** Warns when `string.format` is called with an incorrect number of arguments.
* **Used Ignored Variables:** Warns when a variable conventionally marked as ignored (prefixed with `_`) is actually used in the code, offering a quick-fix to safely rename it.
* **Deprecation:** Warns when using symbols marked with `@deprecated`.
* **Banned Symbols:** Warns when using customized banned functions or properties (e.g., banning `print` to enforce a custom logger).
* **FiveM Safety and Performance:** Detects untrusted event data reaching sensitive sinks, synchronous SQL and other likely game-thread hotspots, source access after yields, invalid SQL placeholders/schema references, unsafe resource contracts and unused NUI handlers.

## FiveM Support

This fork is dedicated to FiveM support. Lugo activates FiveM metadata for files that belong to a detected
`fxmanifest.lua` or `__resource.lua` resource.

* **Manifest authoring:** `fxmanifest.lua` and `__resource.lua` get directive completion, hover and definition support from the embedded manifest reference. Manifest files do not expose runtime globals such as `Citizen`, `Wait`, `exports` or `source`.
* **Runtime profiles:** Files matched by the manifest are classified as `client`, `server` or `shared`. Client files see client + shared runtime metadata, server files see server + shared metadata, and shared or dual-listed files keep the shared intersection only. Plain Lua files outside a matched resource stay plain Lua.
* **Resource accounting:** Lugo warns when a Lua file sits inside a detected resource root but is not referenced by the active manifest. Unaccounted files stay isolated from the resource runtime surface until the manifest includes them.
* **Export/resource validation:** Lugo validates `exports.resourceName:methodName()` and `exports.resourceName.methodName` lookups against the addressed resource, warning for unknown resources and unknown exports.
* **Callable proxies & bridge metadata:** Imported exports and bridge callback values stay table-shaped, but Lugo still provides hover and signature help for their callable proxy surface.
* **Native helpers:** Client and server native helper docs are selected automatically from manifest metadata such as `fx_version`, `game`, `resource_manifest_version` and `use_experimental_fxv2_oal`. There is no separate FiveM setting for native bundle selection.
* **Event intelligence:** Event registrations and triggers are indexed globally, including built-in FiveM events such as `playerConnecting` and `playerDropped`. Completion, hover, definition, references, symbols and Code Lens respect client/server/shared profile visibility.
* **Resource graph:** Manifest includes, dependencies, scripts, files and unaccounted assets are tracked incrementally. Cross-resource `@resource/path.lua` references and exports are resolved through the graph.
* **Commands and convars:** Manifest `command`, `convar` and `convar_category` declarations are checked against literal uses, including scope, type and default conflicts.
* **NUI contracts:** Literal `RegisterNUICallback` and `SendNUIMessage` names are matched against local JavaScript handlers. Missing and unused handlers are reported with precise asset ranges.
* **Trust-boundary checks:** Event parameters are treated as untrusted until validated before reaching sensitive operations such as exports, SQL, HTTP, command execution or event forwarding.
* **Performance and SQL checks:** Low-confidence diagnostics flag tight `Wait(0)` loops, synchronous SQL, excessive network handlers, placeholder mismatches and optional annotated-table/column mismatches. Built-in adapter metadata covers `oxmysql` and `mysql-async`, and custom adapters can be configured.

## Repository quality gates

Run the same checks used by CI with the quality-gate script:

```bash
bash ./scripts/quality-gate.sh format     # gofmt and whitespace/diff checks
bash ./scripts/quality-gate.sh coverage   # package tests with coverage
bash ./scripts/quality-gate.sh benchmark  # lexer/parser benchmark + zero allocations
bash ./scripts/quality-gate.sh race       # race-enabled test suite (requires CGO and a C compiler)
bash ./scripts/quality-gate.sh all
```

The lexer and parser benchmarks must continue to report `0 allocs/op`.

## MCP server

`lugo-mcp` exposes the indexed workspace over MCP (stdio), using one process per workspace:

```bash
lugo-mcp /path/to/project
```

The server registers read-only LSP tools such as `lugo_hover`, `lugo_completion`, `lugo_definition`, `lugo_references`, `lugo_document_symbols`, `lugo_workspace_symbols`, `lugo_format`, `lugo_range_format`, `lugo_diagnostics`, and `lugo_semantic_tokens`. Higher-level tools include `lugo_workspace`, `lugo_workspace_status`, `lugo_symbol_context`, `lugo_fivem_resources`, `lugo_fivem_events`, `lugo_fivem_exports`, `lugo_fivem_contracts`, and `lugo_reindex`. `lugo_validate_workspace_edit` and `lugo_preview_workspace_edit` validate or preview edits only; they never write files.

`lugo_lsp_request_advanced` is restricted to the supported read-only methods: `textDocument/hover`, `textDocument/completion`, `textDocument/signatureHelp`, `textDocument/definition`, `textDocument/typeDefinition`, `textDocument/implementation`, `textDocument/references`, `textDocument/documentSymbol`, `workspace/symbol`, `textDocument/inlayHint`, `textDocument/semanticTokens/full`, `textDocument/foldingRange`, `textDocument/selectionRange`, `textDocument/codeLens`, `textDocument/documentLink`, and `textDocument/prepareCallHierarchy`.

Resources are available at `lugo://workspace/summary`, with templates `lugo://workspace/document/{+path}` (plain Lua source) and `lugo://workspace/resource/{name}` (FiveM metadata). Tool paths are always workspace-relative and edit tools are preview-only. For example:

```json
{"name":"lugo_diagnostics","arguments":{"path":"client/main.lua"}}
```

A client can read `lugo://workspace/document/client/main.lua` before reviewing it. The `lugo_fivem_review` prompt takes a required workspace-relative `path` and directs clients to `lugo_diagnostics`, `lugo_hover`, `lugo_definition`, `lugo_references`, and `lugo_workspace`.

MCP requests are constrained to the workspace root and reject traversal, unsafe methods and invalid paths. Structured tools return stable JSON suitable for agents and automation. Workspace edits are exposed as validated or previewed `WorkspaceEdit` results; Lugo never writes files through MCP. The server also exposes FiveM contract summaries and deterministic workspace freshness/status information.

## Installation

### VS Code
Simply install the extension from the [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=FRFlo.lugo-vscode-fivem-enhanced). The extension automatically detects your OS and architecture and runs the correct bundled Go binary. No external dependencies are required.

### Other Editors (Neovim, Helix, etc.)
Lugo is entirely editor-agnostic and communicates using standard JSON-RPC over `stdio`. You can download the standalone LSP binaries for Windows, Linux and macOS from the [GitHub Releases](https://github.com/FRFlo/lugo/releases) page.

Because Lugo does not rely on a generic wrapper, you must pass your settings directly into `initializationOptions` when setting up the client.

#### Neovim (`nvim-lspconfig`)
You can easily add Lugo as a custom server in your Neovim environment. Since Lugo is standalone, you will need to pass the initialization options directly.

See [**`example.init.lua`**](example.init.lua) for a complete setup snippet.

## Optional FiveM runtime integration tier

Static Go/Lua tests do not require FXServer and remain the default test tier. An optional smoke tier exercises a real resource manifest, server event, export, and (when available) NUI contract:

```bash
# Use an already running FXServer (the resource must be started there)
FIVEM_RUNTIME_ENDPOINT=http://127.0.0.1:30120 \\
  bash ./scripts/fivem-runtime-smoke.sh

# Or start a configured server command. {resource} and {port} are replaced.
FIVEM_SERVER_COMMAND='FXServer +exec server.cfg' \\
  bash ./scripts/fivem-runtime-smoke.sh
```

With no configuration, or when the endpoint is unavailable, the script prints an explicit `SKIP` reason and exits successfully. It is never part of static `go test ./...`. Set `FIVEM_RUNTIME_REQUIRED=1` to make an unavailable configured runtime fail. `FIVEM_RUNTIME_RESOURCE`, `FIVEM_RUNTIME_PORT`, and `FIVEM_RUNTIME_SMOKE_URL` customize the deployment. The fixture is in `scripts/fixtures/fivem-runtime-smoke/`.

## CI/CD Pipeline Integration

Lugo can be run directly in your CI/CD pipelines (like GitHub Actions) to enforce the exact same strictness and diagnostics as your local editor. By passing the `--ci` flag along with a configuration JSON file, Lugo bypasses the standard JSON-RPC loop, indexes your workspace and outputs diagnostics in standard GitHub Actions format (`::warning`, `::error`).

This means line-specific annotations will automatically appear in your Pull Request diffs!

Check out the examples:
* [**`example.ci.json`**](example.ci.json) - An example CI configuration file (maps exactly to the LSP `initializationOptions`).
* [**`example.ci.yml`**](example.ci.yml) - A sample GitHub Actions workflow demonstrating how to download and execute Lugo.

CI policy can filter diagnostic codes, set the failure severity and enforce diagnostic budgets. It can also emit a SARIF report for code-scanning integrations:

```json
{
  "workspaceFolders": ["."],
  "settings": {},
  "ciPolicy": {
    "failOnSeverity": "warning",
    "excludeCodes": ["style"],
    "maxErrors": 0,
    "maxWarnings": 25,
    "sarifPath": "lugo.sarif"
  }
}
```

## Configuration

You can configure Lugo via your VS Code `settings.json` (also available via the settings UI under **Extensions -> Lugo LSP**):

**Workspace & Environment**
* `lugo.workspace.libraryPaths`: An array of absolute paths to external Lua libraries to index.
* `lugo.workspace.ignoreGlobs`: Additional glob patterns to ignore during indexing. Inherits VS Code's `files.exclude` automatically.
* `lugo.environment.knownGlobals`: Global variables to ignore when reporting undefined globals. Supports wildcards (e.g., `N_0x*`).
* `lugo.workspace.maxFileSizeMB`: Maximum file size in megabytes to index (default: `4`). Files larger than this are ignored to prevent out-of-memory crashes.
* `lugo.telemetry.enabled`: Enable or disable anonymous crash reporting and telemetry (default: `true`).

**Parser & Diagnostics**
* `lugo.diagnostics.bannedSymbols`: Map of banned global functions/symbols to a custom warning message (e.g., `{"print": "Use customLogger instead"}`).
* `lugo.parser.maxErrors`: Maximum number of syntax errors to report per file (default: `50`). Reduces cascade noise on heavily broken files. Set to `0` for unlimited.
* `lugo.diagnostics.undefinedGlobals`: Toggle undefined global warnings.
* `lugo.diagnostics.implicitGlobals`: Toggle warnings for forgetting the `local` keyword.
* `lugo.diagnostics.unused.local`: Toggle unused local variable detection.
* `lugo.diagnostics.unused.function`: Toggle unused local function detection.
* `lugo.diagnostics.unused.parameter`: Toggle unused parameter detection.
* `lugo.diagnostics.unused.loopVar`: Toggle unused loop variable detection.
* `lugo.diagnostics.shadowing`: Toggle warnings when a local shadows an outer scope or global.
* `lugo.diagnostics.unreachableCode`: Toggle graying out unreachable code.
* `lugo.diagnostics.ambiguousReturns`: Toggle warnings for expressions accidentally returned due to newlines.
* `lugo.diagnostics.duplicateField`: Toggle warnings for duplicate fields inside table literals.
* `lugo.diagnostics.unbalancedAssignment`: Toggle warnings when assigning fewer or more values than variables.
* `lugo.diagnostics.duplicateLocal`: Toggle warnings when a local variable is defined twice in the exact same scope.
* `lugo.diagnostics.selfAssignment`: Toggle warnings when assigning a variable to itself.
* `lugo.diagnostics.emptyBlock`: Toggle hints for empty blocks (e.g., `do end`).
* `lugo.diagnostics.formatString`: Toggle diagnostics for `string.format` argument counts.
* `lugo.diagnostics.typeCheck`: Toggle strict type checking for operations like calling numbers or indexing non-tables.
* `lugo.diagnostics.redundantParameter`: Toggle diagnostics for passing more arguments to a function than it accepts.
* `lugo.diagnostics.redundantValue`: Toggle diagnostics for assigning more values than there are variables.
* `lugo.diagnostics.redundantReturn`: Toggle diagnostics for empty return statements at the very end of a function.
* `lugo.diagnostics.loopVarMutation`: Toggle diagnostics for mutating a loop variable inside the loop body.
* `lugo.diagnostics.incorrectVararg`: Toggle diagnostics for using the vararg `...` expression outside of a vararg function.
* `lugo.diagnostics.shadowingLoopVar`: Toggle diagnostics when a loop variable shadows an outer local or global variable.
* `lugo.diagnostics.constantCondition`: Toggle diagnostics for conditions that are statically known to be always true or always false.
* `lugo.diagnostics.unreachableElse`: Toggle diagnostics for unreachable `elseif` or `else` branches.
* `lugo.diagnostics.usedIgnoredVariable`: Toggle diagnostics for variables that are used but their name starts with `_`.
* `lugo.diagnostics.deprecated`: Toggle warnings for usage of `@deprecated` symbols.

**Editor Features**
* `lugo.completion.suggestFunctionParams`: Automatically insert function parameters as snippets when autocompleting a function call.
* `lugo.inlayHints.parameterNames`: Enable inline parameter name hints for function and method calls.
* `lugo.inlayHints.suppressWhenArgumentMatchesName`: Suppress parameter name hints when the argument name exactly matches the parameter name (e.g., avoiding `pSource: pSource`).
* `lugo.inlayHints.implicitSelf`: Enable inline `self` hints for method definitions using the colon syntax.
* `lugo.features.documentHighlight`: Enable document highlights for variables and function/method calls.
* `lugo.features.hoverEvaluation`: Evaluate and display the result of constant expressions on hover (e.g., `1 + 2` -> `3`).
* `lugo.features.codeLens`: Enable CodeLens annotations (e.g., reference counts) above function definitions.
* `lugo.features.formatAlerts`: Format special comment tags (e.g., `NOTE:`, `TODO:`) with emojis and bold text in hovers.
* `lugo.features.formatting`: Enable the built-in Lua formatter for inline format fixing and document formatting.
* `lugo.features.formatOpinionated`: Apply opinionated formatting tweaks (e.g., forcing blank lines between unrelated statements).

**FiveM Support**

VS Code uses the `lugo.fivem.*` names below. Standalone clients and CI use the matching `initializationOptions` keys shown in parentheses.

* `lugo.fivem.diagnostics.eventDirection` (`diagFiveMEventDirection`): Toggle client/server event direction validation.
* `lugo.fivem.diagnostics.eventPayload` (`diagFiveMEventPayload`): Toggle validation of network event arguments against known handler payloads.
* `lugo.fivem.diagnostics.unregisteredNetEvent` (`diagFiveMUnregisteredNetEvent`): Toggle checks for registered-but-never-triggered and triggered-but-never-registered network events.
* `lugo.fivem.diagnostics.unknownEvent` (`diagFiveMUnknownEvent`): Toggle warnings for event names unknown to the workspace.
* `lugo.fivem.diagnostics.unaccountedFile` (`diagFiveMUnaccountedFile`): Toggle warnings for Lua files inside a detected resource root that are not matched by the active manifest. Unaccounted files remain plain Lua.
* `lugo.fivem.diagnostics.unknownExport` (`diagFiveMUnknownExport`): Toggle warnings for missing exports on known FiveM resources addressed through the `exports` bridge.
* `lugo.fivem.diagnostics.unknownResource` (`diagFiveMUnknownResource`): Toggle warnings for unknown resource names addressed through the `exports` bridge.
* `lugo.fivem.diagnostics.trustBoundary` (`diagFiveMTrustBoundary`): Toggle diagnostics for untrusted event data flowing to sensitive FiveM APIs.
* `lugo.fivem.diagnostics.performance` (`diagFiveMPerformance`): Toggle FiveM game-thread performance diagnostics.
* `lugo.fivem.diagnostics.sql` (`diagFiveMSQL`): Toggle SQL diagnostics for synchronous calls, placeholder counts, and annotated schemas.
* `lugo.fivem.frameworkAdapters` (`frameworkAdapters`): Optional metadata packs for `esx`, `qbcore`, and `ox`. Each item accepts `name`, optional `version`, and optional `enabled`, for example `[{"name":"qbcore","version":"1"}]`.
* `lugo.fivem.sqlAdapters` (`sqlAdapters`): Additional SQL adapter metadata. Each item requires `name` and may set `calls` and `syncCalls` arrays. Built-in `oxmysql` and `mysql-async` call shapes are always available.

Manifest/runtime/native selection remains derived from the active resource manifest. The FiveM Resources view discovers manifests asynchronously and incrementally; diagnostic changes refresh it after a short debounce.

## Commands

Available via the VS Code Command Palette (`Ctrl+Shift+P`):
* **Lugo: Re-index Workspace:** Manually trigger a full workspace re-index.
* **Lugo: Apply Safe Fixes (Current File):** Automatically clean up unused variables, parameters and assignments in the active file without breaking side-effects.
* **Lugo: Apply Safe Fixes (Workspace):** Apply all safe fixes across the entire workspace.
* **Lugo: Ignore Diagnostic:** Instantly adds a `---@diagnostic disable-next-line` (or `disable-file`) comment for the selected rule (Triggered via Quick Fix Code Actions).
* **Lugo: Export Debug Data...:** Export selected workspace/index/resource data for troubleshooting without exposing source files by default.
* **Lugo: Refresh FiveM Resources:** Refresh the asynchronous FiveM Resources view in the Explorer.

The Explorer also provides folder actions for adding a directory to `libraryPaths` or `ignoreGlobs`. The FiveM Resources view discovers manifests asynchronously and refreshes after relevant diagnostics or workspace changes.
