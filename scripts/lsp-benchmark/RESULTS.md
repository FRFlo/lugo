# LSP benchmark: measured snapshot

This anonymized record summarizes a working performance and regression investigation on a large Lua/FiveM workspace. It is not proof that every branch is covered. Measurements used Go 1.26.1 on Linux amd64, shared machine, and small sample counts. The source workspace and exact source revisions are omitted.

The workspace contained about one thousand Lua files and a large inventory of binary assets. A temporary copy with empty files at the asset paths simulated path existence only, not asset contents. Release-style builds included generated FiveM native stubs; builds without them produced many more undefined-global diagnostics and are not comparable.

| Workload | Reference | Optimized snapshot | Conditions |
| --- | ---: | ---: | --- |
| CI with asset-path inventory | 2.57 s median | 1.75 s median | Three alternating runs each, GOMAXPROCS=2, same 413 diagnostics. |
| CI peak RSS | 348,036 KiB median | 226,528 KiB median | Three alternating processes per version, GOMAXPROCS=2. |
| Cold semantic resolver, prepared ASTs | 111.06 MB/pass | 41.55 MB/pass | Cumulative allocations, not server RSS; allocation count increases. |
| Large-file MCP diagnostics | 354 ms median | 114 ms median | Five calls per binary; includes transport and decoding. |
| MCP events, page of 10 | 21.33 ms median | 2.62 ms median | Five calls per binary; inventory and pagination/filter/error responses matched byte-for-byte after excluding the JSON-RPC ID. |
| General completion | 52.15 ms/op; ~143k allocations | 46.99 ms/op; ~96k allocations | Five repetitions; 8,090 items and byte-identical compared response. |
| Export completion | 9.21 ms median | 3.13 ms median | 15 requests; snapshots differ beyond this change. |
| Debug export, default categories | 4.92 s; ~2.25 GB peak RSS | 0.078 s; ~362 MB peak RSS | Both reject the oversized export; separate isolated probe. |

All 33 registered MCP tools were exercised at least once on a stable snapshot; empty results do not cover every branch. Request timing includes transport and client decoding. On an integrated snapshot with native stubs, `go test -race ./...`, `go vet ./...`, VS Code extension tests and a VSIX package check passed. Concurrent changes require another integrated run.

## Findings and limits

- A synthetic regression test now exercises 46 `shadow-global` diagnostics when scripts are indexed before their manifests. Manifest-first refresh, binding migration and candidate-preserving storage are covered. Independent LSP runs with GOMAXPROCS=1 and 2 produced the same digest over diagnostic publications, initially and after warm reindex. This does not cover every collision or workspace topology.
- Incremental Lua and NUI invalidation was corrected. In an isolated workspace, active diagnostics agreed between warm and cold runs for tested provider edits, JS changes, deletions, recreations and manifest mutations; optional suggestion text differed.
- Semantic tokens use the negotiated position encoding after non-BMP characters. UTF-8, UTF-16 and UTF-32 are covered by targeted tests. Hover, definition, references and rename ranges were correct at tested positions.
- Retaining closed-document source fixes a reproducible crash. Repeated runs found no measurable marginal retention because the global index already held the same buffers; closing freed about 18.5 MiB from caches. Non-library sources totaled about 6 MiB.
- The NUI template callback fix removes a false warning. A split scanner with overlap fallback matched the combined regex on 133 assets and 50,000 generated fragment combinations. Three CI pairs measured 1.98 s median before and 1.88 s after, with identical 412 diagnostics; the snapshots also differed by the closed-document fix, which CI does not exercise.
- NUI scanning covers HTML, JS, TS, JSX and TSX, not Vue sources. Built assets can overlap those sources, so no missing callback count is established. Binary assets and the interactive VS Code UI have not been exhaustively benchmarked.

Reusable harnesses live in `scripts/lsp-benchmark` and `scripts/corpus-benchmark`. Local source and stub fingerprints do not prove what was embedded in a binary built elsewhere; preserve build provenance and raw results for comparisons.

Package-local coverage on the integrated snapshot reported 56.3% statement coverage for `lsp`, 77.0% for `cmd/lugo-mcp`, 84.1% for `parser` and 61.5% for `semantic`. This does not attribute coverage from separate e2e binaries and does not establish branch or feature completeness.

Two alternating LSP benchmark passes using an added newline (with no public-fact change) measured `didChange` plus a document-symbol barrier at 124.8–126.7 ms, versus 124.9–168.7 ms on the preceding snapshot. This limited sample includes the barrier and does not measure edits that change exported facts or establish tail latency. The full race suite and `go vet` passed on the integrated source snapshot with native stubs.
