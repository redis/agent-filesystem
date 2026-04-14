# Markdown Search Benchmark

- Platform: `darwin/arm64`
- Go: `go1.26.1`
- Redis: `Redis server v=7.2.5 sha=00000000:0 malloc=libc bits=64 build=bd81cd1340e80580`
- Grep: `usage: grep [-abcdDEFGHhIiJLlMmnOopqRSsUVvwXxZz] [-A num] [-B num] [-C[num]]`
- Corpus: `4000` markdown files across `24` directories (`31.5 MiB`)

## CLI Grep

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| grep_literal_rare | 371.64 | 182.60 | 0.49x | identical normalized output |
| grep_literal_common | 374.64 | 181.59 | 0.48x | identical normalized output |
| grep_regex_escalation | 220.72 | 883.25 | 4.00x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 2.82 | 10.97 | 3.89x | counts match (4049) |
| find_runbook_names | 2.82 | 11.04 | 3.92x | counts match (1000) |
| read_hot_files | 1.20 | 17.90 | 14.94x | counts match (792005) |
| head_hot_files | 1.56 | 19.04 | 12.19x | counts match (75690) |
| line_window_hot_files | 1.91 | 19.73 | 10.32x | counts match (49587) |
