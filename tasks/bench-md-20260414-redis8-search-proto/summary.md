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
| grep_literal_rare | 386.70 | 1085.03 | 2.81x | identical normalized output |
| grep_literal_common | 372.71 | 1050.70 | 2.82x | identical normalized output |
| grep_regex_escalation | 215.46 | 1075.60 | 4.99x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 2.90 | 41.68 | 14.37x | counts match (4049) |
| find_runbook_names | 3.05 | 39.58 | 12.97x | counts match (1000) |
| read_hot_files | 1.42 | 165.62 | 116.31x | counts match (792005) |
| head_hot_files | 1.94 | 163.29 | 84.21x | counts match (75690) |
| line_window_hot_files | 1.74 | 166.18 | 95.78x | counts match (49587) |
