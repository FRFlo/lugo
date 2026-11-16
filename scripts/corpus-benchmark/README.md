# Lua corpus benchmarks

Run the lexer, cold parser construction, and cold semantic resolver across a
local workspace, excluding file I/O and parsing setup from the measured stage:

```sh
LUGO_BENCH_CORPUS=/path/to/lua-workspace go test ./scripts/corpus-benchmark \
  -run '^$' -bench . -benchmem -benchtime=3x

# Compare cold and reused parse trees for every file in the corpus.
LUGO_BENCH_CORPUS=/path/to/lua-workspace go test ./scripts/corpus-benchmark \
  -run '^TestCorpusParserReuseMatchesCold$'
```

Each operation processes the entire corpus, so `B/op` and `allocs/op` are per
corpus, not per file. `MB/s` uses total Lua source bytes. Parsing constructs a
new tree and parser per file; the warm parser benchmark reuses both across all
files and iterations, including initial growth in its allocation totals. The
resolver benchmark uses pre-parsed trees and
constructs a fresh resolver for each file. Files in `.git` and `node_modules`
are excluded; all other `.lua` files are included regardless of Lugo's workspace
ignore settings. The results do not include LSP indexing, diagnostics, file I/O,
or NUI assets. Corpus contents and concurrent machine load must be held fixed
when comparing runs. If the environment variable is absent, benchmarks skip.
