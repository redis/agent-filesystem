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
| grep_literal_rare | 368.07 | 42.02 | 0.11x | different normalized output (local=9 redis=0) |
| grep_literal_common | 370.20 | 23.76 | 0.06x | different normalized output (local=445 redis=0) |
| grep_regex_escalation | 224.96 | 1144.30 | 5.09x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 2.90 | 41.02 | 14.12x | counts match (4049) |
| find_runbook_names | 2.88 | 38.19 | 13.26x | counts match (1000) |
| read_hot_files | 1.32 | 163.39 | 123.78x | counts match (792005) |
| head_hot_files | 1.80 | 174.94 | 97.19x | counts match (75690) |
| line_window_hot_files | 1.96 | 174.19 | 89.10x | counts match (49587) |
