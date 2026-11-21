# Flashland workspace performance investigation

The measurements below describe one checkout of `/workspace/Flashland`, not a
general performance guarantee. The corpus was read without modifying it. It
contained 1,112 indexed documents (including generated native stubs) and 1,094
workspace files receiving diagnostics. All compared SARIF reports contained the
same 10,227 complete results, compared as an unordered multiset.
The CI SARIF exporter records codes, levels, messages and start positions, but
does not include every LSP diagnostic field such as end positions or tags.
An additional LSP stdio capture compared all `publishDiagnostics` objects as
unordered per-URI multisets: both binaries published 1,107 notifications for
1,107 URIs, containing 10,227 diagnostics and 462 empty notifications. There
were no added or removed objects, including their end positions, tags and data.
That capture used local-only telemetry and identical initial journal contents;
it compared correctness rather than timing.

The reference was Lugo commit `79682659cbcd7f5cd8167721ceb3e391028b390b`;
the candidate is the associated optimization change. Both used Go 1.26.1 on
Linux amd64, an Intel Xeon E5-1620 v2, one pinned CPU, `GOMAXPROCS=1`, and a
soft Go memory target of 512 MiB. Both used empty initialization settings, which
select Lugo's defaults, and identical generated FiveM native stubs. Filesystem
caches were warm and the host was shared with other processes. Memory figures
are sampled peak resident set sizes, not hard limits.

The SHA-256 hashes of the generated `natives_client.lua`, `natives_server.lua`,
and `natives_shared.lua` were respectively `f0ded4481262ee63108e4fae89f6dd6e87da48ef53c18232e222b786e1100596`,
`2dc86dc5bf6f353246dcda2775885c036f13a42251062b8927819a6687fcd172`, and
`c64fc4223f0b38d957250b812fcdc2dea797ecde366b4d4d83db2389f800177a`.
The generator uses live upstream metadata, so future regeneration can change
the indexed corpus and results.

| Run | Index | Diagnostics | Total | Peak RSS |
| --- | ---: | ---: | ---: | ---: |
| Reference binary | 8.20 s | 503.09 s | 511.29 s | 369 MiB |
| Inference optimization only | 8.93 s | 30.16 s | 39.10 s | 346 MiB |
| Inference and batched invalidation | 5.33 s | 39.86 s | 45.20 s | 361 MiB |
| Combined candidate, journal rewrite before buffering | 4.00 s | 45.36 s | 49.37 s | 340 MiB |
| Combined candidate, buffered journal | 3.07 s | 16.41 s | 19.49 s | 335 MiB |
| Retained candidate, visibility experiment removed | 2.93 s | 16.14 s | 19.07 s | 337 MiB |

The latter two runs started with separate identical copies of a filled local
telemetry journal. A second pair gave 44.20 s and 18.51 s respectively. The
reference used the normal persistent journal, whose initial contents were not
recorded; its total is consequently not a controlled measurement of journal
overhead. Varying document iteration order, machine load, telemetry history,
and operating-system caches also affect run-to-run timings. Index and diagnostic
durations above are Lugo's own logged durations; RSS is sampled by the helper.
The combined measurements included a visibility-loop experiment that was
subsequently removed: three paired runs showed its 1.2% median difference was
smaller than observed run-to-run variation and its median RSS was higher.

With a 256 MiB soft Go target and a filled journal, the candidate took 19.06 s
and reached a sampled peak RSS of 220 MiB; this does not establish behavior
under a hard 256 MiB process limit. A real LSP protocol run on a selected RageUI
symbol with the retained binary measured a 16.40 s cold index and 11.06 s warm
reindex; hover, completion, and definition round trips were 0.28 ms, 1.31 ms,
and 0.11 ms respectively. The sampled peak RSS was 343 MiB.
Those single-position results do not represent every editor operation or MCP
workload.

A publication-scoped visibility cache prototype reached its 65,536-entry cap
and used 22–31 MB more sampled RSS. Two runs took 19.09/19.70 s against
18.73/18.54 s without the cache, so it was not retained. Reordering include
resource resolution before glob matching was also slower on this corpus.

For reproduction and SARIF comparison see
[`scripts/workspace-perf/README.md`](../scripts/workspace-perf/README.md). For
release-like FiveM coverage, generate native stubs once and use identical copies
for both builds. The profiling harness initializes telemetry like the executable;
choose separate identical journal copies through `LUGO_TRACE_JOURNAL` when
isolating code changes. The CPU profile itself adds overhead. The telemetry
journal remains non-atomic across crashes or simultaneous processes, as before.
