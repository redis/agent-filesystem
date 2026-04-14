# Benchmark Findings

## What Was Benchmarked

- Tool: [tests/bench_md_workloads/main.go](/Users/rowantrollope/git/agent-filesystem/tests/bench_md_workloads/main.go)
- Corpus: 4,000 markdown files, 24 directories, 31.5 MiB total
- Environment: macOS arm64, Go 1.26.1, Redis 7.2.5, system `grep`
- Output artifacts:
  - [summary.md](/Users/rowantrollope/git/agent-filesystem/tasks/bench-md-20260413/summary.md)
  - [report.json](/Users/rowantrollope/git/agent-filesystem/tasks/bench-md-20260413/report.json)
  - [grep_results.csv](/Users/rowantrollope/git/agent-filesystem/tasks/bench-md-20260413/grep_results.csv)
  - [ops_results.csv](/Users/rowantrollope/git/agent-filesystem/tasks/bench-md-20260413/ops_results.csv)

## Main Result

For simple literal markdown search, the current Redis-backed `afs grep` path was faster than local `grep` on this corpus:

- `grep_literal_rare`: local `371.64 ms`, Redis `182.60 ms`, about `2.0x` faster in Redis
- `grep_literal_common`: local `374.64 ms`, Redis `181.59 ms`, about `2.1x` faster in Redis

For regex search, the Redis-backed path was much slower:

- `grep_regex_escalation`: local `220.72 ms`, Redis `883.25 ms`, about `4.0x` slower in Redis

## RediSearch Status

RediSearch is not currently accelerating workspace grep in this repo.

- The only search-index hook in the current CLI is a stub: [cmd/afs/afs_search_index.go](/Users/rowantrollope/git/agent-filesystem/cmd/afs/afs_search_index.go:9)
- Literal `afs grep` goes through `client.Grep(...)`: [cmd/afs/afs_grep.go](/Users/rowantrollope/git/agent-filesystem/cmd/afs/afs_grep.go:278)
- Regex `afs grep` falls back to `Tree` plus `Cat` over every target file: [cmd/afs/afs_grep.go](/Users/rowantrollope/git/agent-filesystem/cmd/afs/afs_grep.go:381)
- The underlying client grep is a recursive walk over directory children and file contents, not an index lookup: [mount/internal/client/native_walk.go](/Users/rowantrollope/git/agent-filesystem/mount/internal/client/native_walk.go:209)
- The older Redis module `FS.GREP` is also a direct recursive scan: [module/fs.c](/Users/rowantrollope/git/agent-filesystem/module/fs.c:2817)

## Implication

- If the workload is "literal substring search across many markdown files", the current Redis-backed path can outperform local `grep`.
- If the workload needs regex or richer file interactions, local filesystem access is still materially faster.
- Any external claim that current grep speed comes from RediSearch would be incorrect for this worktree.

## Re-run

```bash
go run ./tests/bench_md_workloads --rounds 5 --warmup 1 --output-dir ./tasks/bench-md-$(date +%Y%m%d)
```
