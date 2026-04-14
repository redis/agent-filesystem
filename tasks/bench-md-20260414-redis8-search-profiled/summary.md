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
| grep_literal_rare | 366.24 | 1091.42 | 2.98x | identical normalized output |
| grep_literal_common | 359.30 | 1099.38 | 3.06x | identical normalized output |
| grep_regex_escalation | 209.28 | 1081.52 | 5.17x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 2.95 | 40.22 | 13.63x | counts match (4049) |
| find_runbook_names | 2.96 | 34.99 | 11.83x | counts match (1000) |
| read_hot_files | 1.34 | 164.85 | 122.75x | counts match (792005) |
| head_hot_files | 1.94 | 163.68 | 84.29x | counts match (75690) |
| line_window_hot_files | 1.91 | 162.45 | 85.19x | counts match (49587) |
