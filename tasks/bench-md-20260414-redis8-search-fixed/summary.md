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
| grep_literal_rare | 355.11 | 21.72 | 0.06x | identical normalized output |
| grep_literal_common | 361.32 | 45.75 | 0.13x | identical normalized output |
| grep_regex_escalation | 210.22 | 1070.92 | 5.09x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 2.96 | 38.27 | 12.92x | counts match (4049) |
| find_runbook_names | 2.88 | 38.13 | 13.25x | counts match (1000) |
| read_hot_files | 1.34 | 171.68 | 128.31x | counts match (792005) |
| head_hot_files | 1.89 | 162.96 | 86.40x | counts match (75690) |
| line_window_hot_files | 1.90 | 162.16 | 85.30x | counts match (49587) |
