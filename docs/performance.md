# Performance Notes

Last reviewed: 2026-04-24.

This file is the durable replacement for old one-off benchmark output
directories. Keep raw benchmark runs out of the repo; rerun them into `/tmp` or
another artifact directory, then copy only stable findings here.

## Current Storage Baseline

- File content now lives in external Redis string keys (`content_ref = "ext"`)
  rather than inline inode hash fields.
- Byte-range reads and writes use `GETRANGE` and `SETRANGE`.
- Sync uses chunked delta transfer for files above 1 MB, with 256 KB chunks and
  16 chunks per Redis pipeline batch.
- The default sync file-size cap is 2 GB.
- The NFS write path includes the HSETNX create fast path and batched `SetAttrs`
  path.

## Search Baseline

`afs grep` has two paths:

- Simple literal searches try the RediSearch-backed trigram index when Redis
  search commands are available and the index is ready.
- Regex, glob, and advanced grep options fall back to collecting candidate files
  and verifying content through the AFS client path.

Historical benchmark context from the removed task artifacts:

- On a 4,000-file markdown corpus (31.5 MiB), indexed literal searches were in
  the low tens of milliseconds once Redis 8 search was available.
- Regex-style escalation remained much slower than local `ripgrep` because it
  still needed content verification over the AFS client path.

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
