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
| grep_literal_rare | 358.30 | 1073.82 | 3.00x | identical normalized output |
| grep_literal_common | 371.39 | 1104.69 | 2.97x | identical normalized output |
| grep_regex_escalation | 216.27 | 7740.64 | 35.79x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 3.04 | 34.12 | 11.22x | counts match (4049) |
| find_runbook_names | 3.06 | 34.39 | 11.24x | counts match (1000) |
| read_hot_files | 1.41 | 181.69 | 128.77x | counts match (792005) |
| head_hot_files | 1.90 | 182.85 | 96.29x | counts match (75690) |
| line_window_hot_files | 1.99 | 201.05 | 101.28x | counts match (49587) |
