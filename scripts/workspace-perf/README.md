# Workspace indexing and diagnostics harness

Run from the repository root. The workspace is only read; stdout contains the
standard GitHub Actions diagnostics emitted by Lugo's CI path, and stderr
contains indexing progress and elapsed time. Empty settings use Lugo's default
diagnostic options. For modest-machine measurements, pin the run to one
available CPU and set `GOMAXPROCS=1`, `GOFLAGS=-p=1`, and `GOMEMLIMIT=512MiB`.
`GOMEMLIMIT` is a soft Go runtime target, not a hard process memory cap.
The measurement helper defaults to a `512MiB` soft memory target; pass
`--memory-limit 256MiB` for tighter-memory stress measurements.

The harness uses the same default telemetry initialization, bounded journal,
CI flush, and close behavior as the production binary. It does not disable
telemetry or force local-only mode. The default journal lives in the user's
cache; if `LUGO_TRACE_JOURNAL` is set to a path inside the measured workspace,
the harness exits before starting so its output cannot change the measurement.
The bounded journal persists across runs, so it can affect repeatability and
adds filesystem work; use a fresh journal path outside the workspace when you
need isolated repeated measurements, while leaving the default in place for
production-equivalent timing.

This measures the shared workspace indexing and diagnostics path through
`RunCI`. It does not measure JSON-RPC initialization, client-driven LSP
requests, or the MCP server's own protocol and configuration defaults. Those
entry points need their own protocol harnesses for end-to-end measurements.

```sh
go build -o /tmp/lugo-workspace-perf ./scripts/workspace-perf
python3 scripts/workspace-perf/measure.py \
  --stdout /tmp/flashland.diagnostics \
  --stderr /tmp/flashland.run.log \
  --metrics /tmp/flashland.metrics.json -- \
  /tmp/lugo-workspace-perf -workspace /workspace/Flashland
```

The helper picks the lowest CPU in its affinity set, pins the child there, and
records wall time and sampled peak RSS. Use `taskset -c 2` on the helper itself
to reserve a specific available CPU. Add a bounded Go CPU profile with
`-cpuprofile` and `-profile-seconds`:

```sh
python3 scripts/workspace-perf/measure.py \
  --stdout /tmp/flashland.diagnostics \
  --stderr /tmp/flashland.run.log \
  --metrics /tmp/flashland.metrics.json -- \
  /tmp/lugo-workspace-perf -workspace /workspace/Flashland \
  -cpuprofile /tmp/flashland.cpu.pprof -profile-seconds 45
go tool pprof -top /tmp/lugo-workspace-perf /tmp/flashland.cpu.pprof
```

The config uses a workspace folder and empty initialization settings. Use
`-sarif /tmp/flashland.sarif` when a structured report is useful for comparing
diagnostic codes and positions. SARIF generation adds report work to the run;
omit it for timing-only measurements. Keep diagnostic, profile, and SARIF
outputs outside the workspace so the harness never writes into the target.
Compare complete SARIF results as an unordered multiset (including duplicate
diagnostics and every diagnostic field) with:

```sh
python3 scripts/workspace-perf/compare-sarif.py \
  /tmp/flashland-baseline.sarif /tmp/flashland-candidate.sarif
```

The harness measures a cold Lugo process; the operating system may have warm
file caches on later runs. Other processes can still contend for CPU or memory.
`GOMEMLIMIT` is soft, so peak RSS can exceed the target during garbage
collection or runtime overhead.
