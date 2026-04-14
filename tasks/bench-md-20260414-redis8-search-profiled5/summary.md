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
| grep_literal_rare | 368.93 | 47.17 | 0.13x | different normalized output (local=9 redis=0) |
| grep_literal_common | 374.39 | 49.52 | 0.13x | different normalized output (local=445 redis=431) |
| grep_regex_escalation | 227.68 | 1086.96 | 4.77x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 2.90 | 44.92 | 15.51x | counts match (4049) |
| find_runbook_names | 2.95 | 38.66 | 13.10x | counts match (1000) |
| read_hot_files | 1.33 | 161.05 | 121.00x | counts match (792005) |
| head_hot_files | 1.89 | 162.20 | 85.91x | counts match (75690) |
| line_window_hot_files | 1.90 | 159.10 | 83.78x | counts match (49587) |
