# Performance Notes

Last reviewed: 2026-05-05.

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
