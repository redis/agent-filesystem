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
| grep_literal_rare | 365.28 | 45.83 | 0.13x | different normalized output (local=9 redis=0) |
| grep_literal_common | 361.01 | 47.20 | 0.13x | identical normalized output |
| grep_regex_escalation | 211.32 | 1043.09 | 4.94x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 2.82 | 34.94 | 12.39x | counts match (4049) |
| find_runbook_names | 2.85 | 37.48 | 13.13x | counts match (1000) |
| read_hot_files | 1.45 | 159.05 | 109.62x | counts match (792005) |
| head_hot_files | 1.96 | 175.27 | 89.65x | counts match (75690) |
| line_window_hot_files | 1.85 | 187.76 | 101.22x | counts match (49587) |
