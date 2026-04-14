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
| grep_literal_rare | 363.31 | 20.64 | 0.06x | identical normalized output |
| grep_literal_common | 362.07 | 51.79 | 0.14x | identical normalized output |
| grep_regex_escalation | 214.11 | 1083.17 | 5.06x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 3.13 | 34.61 | 11.06x | counts match (4049) |
| find_runbook_names | 3.75 | 37.16 | 9.92x | counts match (1000) |
| read_hot_files | 1.39 | 166.11 | 119.94x | counts match (792005) |
| head_hot_files | 1.89 | 168.32 | 89.29x | counts match (75690) |
| line_window_hot_files | 1.89 | 159.43 | 84.31x | counts match (49587) |
