# LSP workspace benchmark

Build Lugo and run the benchmark against a local workspace and a representative
symbol in a Lua file:

```sh
go build -o /tmp/lugo-bench .
python3 scripts/lsp-benchmark/benchmark.py \
  --server /tmp/lugo-bench --workspace /path/to/workspace \
  --file /path/to/workspace/file.lua --line 21 --character 10 \
  --query WorkspaceMenu --iterations 5 > /tmp/lugo-lsp-results.json
```

Line and character are zero-based; character is a UTF-16 code-unit offset.
The default JSON output contains per-request samples and median/max wall time,
response size, median Python JSON decoding time, startup, cold index, warm
reindex, open, change, and close times, result counts, Linux peak RSS, and
non-corpus metadata: the server executable SHA-256, Go version, and a
fingerprint of Lugo's generated `lsp/stdlib/natives_*.lua` stubs. It omits
workspace and file paths, corpus revisions and content fingerprints, and raw
server error text. JSON-RPC errors retain only a numeric protocol code (when
available) and a fixed generic message. `metadata.errors` contains fixed
descriptions without exception text. The output measures round trips through
JSON-RPC, not just handler CPU time. The indexing,
open, change, and close measurements include a subsequent request as a barrier.
Full-document and visible-range formatting, semantic tokens, and inlay hints are
measured separately; `--range-lines` sets the latter range (default 60 lines).
Use the same workspace, symbol, settings, binary build, and idle machine for
comparisons. Index time includes the barrier's workspace-symbol search.

The script sends an in-memory change, requests edits, and rejects server apply-edit
requests; it does not write workspace files. It reports resolve and call-hierarchy
operations as skipped when the selected position returns no suitable item. A
null or empty result is a measurement of that path, not evidence that a feature
was exercised on a meaningful symbol. Use additional positions and files to cover
local and global symbols, calls, events, exports, diagnostics, and links. Results
and timings depend on all files actually present in the checkout, including NUI
assets. A sparse checkout can change both diagnostics and performance. JSON-RPC
errors are recorded with their timing rather than treated as successful results.
Result fingerprints are omitted to avoid content-derived corpus identifiers;
compare timings, response sizes, and result counts. The native stub fingerprint
and Go version describe the source checkout and Go executable running this
script, not necessarily the supplied server binary. A binary built elsewhere or
before the current source changes may contain different stubs or code; its
SHA-256 alone does not prove their provenance. Record the build environment
separately for such comparisons.
`--debug-export` additionally measures one full `lugo/debugExport` request. It
may consume substantial memory on a large workspace even if it fails its size
limit, so it is opt-in; its impact is included in the reported peak RSS.
Generate the native stubs using `go run ./cmd/rage-lua-natives -out ./lsp/stdlib`
before building if measuring the FiveM configuration used by release builds.

Example using a local workspace (the benchmark reads the workspace without modifying it):

```sh
python3 scripts/lsp-benchmark/benchmark.py \
  --server /tmp/lugo-bench --workspace /path/to/lua-workspace \
  --file /path/to/lua-workspace/path/to/file.lua \
  --line 21 --character 5 --query symbolName --iterations 1
```
