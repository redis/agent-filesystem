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
| grep_literal_rare | 367.05 | 51.24 | 0.14x | different normalized output (local=9 redis=0) |
| grep_literal_common | 366.99 | 48.92 | 0.13x | different normalized output (local=445 redis=418) |
| grep_regex_escalation | 227.98 | 1155.18 | 5.07x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 2.96 | 45.44 | 15.35x | counts match (4049) |
| find_runbook_names | 2.90 | 41.70 | 14.35x | counts match (1000) |
| read_hot_files | 1.26 | 169.05 | 133.85x | counts match (792005) |
| head_hot_files | 1.93 | 169.76 | 88.09x | counts match (75690) |
| line_window_hot_files | 1.79 | 161.00 | 89.89x | counts match (49587) |
