# Performance Notes

Last reviewed: 2026-09-10 (sync save profiling).

This file is the durable replacement for old one-off benchmark output
directories. Keep raw benchmark runs out of the repo; rerun them into `/tmp` or
another artifact directory, then copy only stable findings here.

## Current Storage Baseline

- File content now lives in external Redis content keys rather than inline
  inode hash fields.
- On Redis servers without Array support, files use external string keys
  (`content_ref = "ext"`).
- On Redis servers with Array support, new and rewritten files prefer chunked
  Redis Array content keys (`content_ref = "array"`), while existing `ext`
  files remain readable.
- Byte-range reads and writes use `GETRANGE` / `SETRANGE` for `ext` files and
  chunked Array reads / writes for `array` files.
- Sync uses chunked delta transfer for files above 1 MB, with 256 KB chunks and
  16 chunks per Redis pipeline batch.
- The default sync file-size cap is 2 GB.
- The NFS write path includes the HSETNX create fast path and batched `SetAttrs`
  path.

## Search Baseline

`afs fs grep` has two paths:

- Simple literal searches try the RediSearch-backed trigram index when Redis
  search commands are available and the index is ready.
- When RediSearch is unavailable and a file is Array-backed, the client can use
  `ARGREP` as a conservative prefilter before loading full content for exact
  verification.
- Regex, glob, and advanced grep options fall back to collecting candidate files
  and verifying content through the AFS client path.

Historical benchmark context from the removed task artifacts:

- On a 4,000-file markdown corpus (31.5 MiB), indexed literal searches were in
  the low tens of milliseconds once Redis 8 search was available.
- Regex-style escalation remained much slower than local `ripgrep` because it
  still needed content verification over the AFS client path.

Latest local rerun on macOS/arm64, 4,000 markdown files, 31.5 MiB, 5 measured
rounds:

- With Docker `redis:8` and RediSearch available, indexed `afs fs grep` took
  17.35 ms for a rare literal and 42.56 ms for a common literal. Local BSD
  `grep` took 371.74 ms and 381.71 ms for the same searches; `ripgrep` took
  37.99 ms and 41.10 ms.
- Regex escalation still used the advanced non-indexed path: `afs fs grep` took
  1078.74 ms versus 213.16 ms for BSD `grep` and 67.53 ms for `ripgrep`.
- On the existing local control plane at `http://127.0.0.1:8091`, the backing
  `localhost:6379` Redis did not expose RediSearch commands. The same corpus
  imported through the control plane used `fast_backend_grep` with
  `search_unavailable`: literal `afs fs grep` was about 187 ms, and regex
  escalation was about 196 ms. Treat those as non-indexed local-database
  numbers, not the indexed Redis 8 baseline.

## NFS Hot Path Findings

The old NFS perf notes produced two changes that are now part of the codebase:

- `createFileIfMissing` uses an HSETNX name claim instead of the older
  WATCH/MULTI flow.
- NFS `SETATTR` dispatches through a batched `SetAttrs` fast path instead of
  separate chmod/chown/utimens calls when the filesystem supports it.

The remaining high-value benchmark target is not another raw output directory;
it is a repeatable comparison after storage or sync behavior changes.

## Sync Save Profiling

On 2026-09-10, a temporary instrumented build of save implementation `202ac685`
ran in an ARM64 AWS AgentCore microVM with Redis 8.10.1 on a t4g.large host
(8 GiB RAM). The workload installed a Python environment, then immediately
saved 7,081 entries, including 6,444 regular files and 148,888,880 bytes.
The CLI completed in 266.699 seconds with a ten minute timeout.

| Phase | Wall time, seconds |
| --- | ---: |
| Apply pending changes | 169.309 |
| Read back and verify Redis | 93.827 |
| Remote preflight | 2.236 |
| Four local tree scans combined | 0.935 |

Applying changes included 6,280 `EchoCreate` calls totaling 149.598 seconds.
Final readback included 6,444 `Cat` calls totaling 45.942 seconds and 6,444
`ChunkMeta` calls totaling 43.547 seconds. These method times are included in
the phase totals. They measure filesystem client calls, each of which may
issue multiple Redis commands; they do not isolate network or Redis CPU time.
The current engine performs these file operations sequentially.

The writer daemon was paused before an independent audit verified all bytes
and metadata. A fresh reader restored the complete tree and passed all 4,142
package RECORD hashes and the analytics workload. The instrumentation remained
outside the production source. This was one profiling run on a larger host
than the earlier 228.206 second run, so the totals do not establish a speedup.
Removing the redundant local scan primarily simplifies the save operation;
local scanning accounts for little of this workload's elapsed time.

## Rerun Commands

Markdown/search workload:

```bash
go run ./tests/bench_md_workloads --markdown-files 4000 --rounds 5 --output-dir /tmp/afs-bench-md-$(date +%Y%m%d)
```

Mounted NFS comparison:

```bash
scripts/bench_compare.sh 5 /tmp/afs-perf-run-$(date +%Y%m%d-%H%M%S)
```

After a meaningful rerun, summarize the stable result here rather than
committing the generated CSV/JSON output.


## Atomic publication receipts (2026-09-19)

A local Redis 7.4.2 String-storage microbenchmark on an Apple M4 Pro compared
`origin/main` at `1ff1fa0` with the atomic sync publication changes. Each size ran
three one-second samples, using `BenchmarkAtomicFilePublication` in the mount
client package. Median direct publication latency was 281 → 293 microseconds
for 1 KiB (+4.3%) and 552 → 529 microseconds for 1 MiB (-4.2%). The latter should
be treated as local measurement variation, not an established speedup. The
receipt adds roughly 1–3 KiB of Go allocation per operation in these samples.

This benchmark measures direct file publication, not end-to-end sync. Stable
sync snapshot verification adds Redis reads; private staging temporarily needs
both the old and new content, and seven-day receipts consume server memory in
proportion to write rate. Remote-network and large-workspace latency need
workload-specific measurement.

Rerun from `mount/` with a String-capable Redis server on PATH:

```bash
go test ./internal/client -run '^$' -bench '^BenchmarkAtomicFilePublication$' -benchtime=1s -count=3
```
