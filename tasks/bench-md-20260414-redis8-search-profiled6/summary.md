# Markdown Search Benchmark

- Platform: `darwin/arm64`
- Go: `go1.26.1`
- Redis: `Redis server v=8.0.3`
- Redis source: `docker redis:8`
- Grep: `usage: grep [-abcdDEFGHhIiJLlMmnOopqRSsUVvwXxZz] [-A num] [-B num] [-C[num]]`
- Corpus: `4000` markdown files across `24` directories (`31.5 MiB`)

## CLI Grep

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| grep_literal_rare | 367.44 | 50.75 | 0.14x | different normalized output (local=9 redis=0) |
| grep_literal_common | 369.50 | 54.65 | 0.15x | different normalized output (local=445 redis=419) |
| grep_regex_escalation | 227.17 | 1104.16 | 4.86x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 2.97 | 37.58 | 12.66x | counts match (4049) |
| find_runbook_names | 2.93 | 35.41 | 12.09x | counts match (1000) |
| read_hot_files | 1.38 | 159.90 | 115.54x | counts match (792005) |
| head_hot_files | 1.92 | 163.04 | 84.87x | counts match (75690) |
| line_window_hot_files | 1.71 | 159.52 | 93.40x | counts match (49587) |
