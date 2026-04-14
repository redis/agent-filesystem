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
| grep_literal_rare | 358.65 | 1122.05 | 3.13x | identical normalized output |
| grep_literal_common | 363.48 | 1120.40 | 3.08x | identical normalized output |
| grep_regex_escalation | 210.55 | 1055.71 | 5.01x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 3.07 | 35.18 | 11.46x | counts match (4049) |
| find_runbook_names | 3.06 | 38.72 | 12.66x | counts match (1000) |
| read_hot_files | 1.54 | 168.22 | 109.02x | counts match (792005) |
| head_hot_files | 2.05 | 158.77 | 77.30x | counts match (75690) |
| line_window_hot_files | 1.97 | 160.43 | 81.56x | counts match (49587) |
