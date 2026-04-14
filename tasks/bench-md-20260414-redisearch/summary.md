# Markdown Search Benchmark

- Platform: `darwin/arm64`
- Go: `go1.26.1`
- Redis: `Redis server v=7.2.5 sha=00000000:0 malloc=libc bits=64 build=bd81cd1340e80580`
- Grep: `usage: grep [-abcdDEFGHhIiJLlMmnOopqRSsUVvwXxZz] [-A num] [-B num] [-C[num]]`
- Corpus: `4000` markdown files across `24` directories (`31.5 MiB`)

## CLI Grep

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| grep_literal_rare | 355.65 | 185.75 | 0.52x | identical normalized output |
| grep_literal_common | 360.56 | 184.84 | 0.51x | identical normalized output |
| grep_regex_escalation | 214.75 | 940.49 | 4.38x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 2.75 | 11.57 | 4.21x | counts match (4049) |
| find_runbook_names | 2.92 | 11.42 | 3.91x | counts match (1000) |
| read_hot_files | 1.31 | 20.52 | 15.66x | counts match (792005) |
| head_hot_files | 1.88 | 21.27 | 11.34x | counts match (75690) |
| line_window_hot_files | 1.74 | 20.89 | 12.00x | counts match (49587) |
