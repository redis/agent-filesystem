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
| grep_literal_rare | 371.09 | 54.17 | 0.15x | different normalized output (local=9 redis=0) |
| grep_literal_common | 367.30 | 48.03 | 0.13x | different normalized output (local=445 redis=424) |
| grep_regex_escalation | 227.15 | 1065.99 | 4.69x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 3.03 | 38.82 | 12.81x | counts match (4049) |
| find_runbook_names | 3.05 | 36.16 | 11.86x | counts match (1000) |
| read_hot_files | 1.33 | 164.52 | 123.61x | counts match (792005) |
| head_hot_files | 1.89 | 165.33 | 87.62x | counts match (75690) |
| line_window_hot_files | 1.85 | 162.47 | 87.92x | counts match (49587) |
