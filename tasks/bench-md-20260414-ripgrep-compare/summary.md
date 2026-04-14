# Markdown Search Benchmark

- Platform: `darwin/arm64`
- Go: `go1.26.1`
- Redis: `Redis server v=8.0.3`
- Redis source: `docker redis:8`
- Grep: `usage: grep [-abcdDEFGHhIiJLlMmnOopqRSsUVvwXxZz] [-A num] [-B num] [-C[num]]`
- Ripgrep: `ripgrep 15.1.0`
- Corpus: `4000` markdown files across `24` directories (`31.5 MiB`)

## CLI Grep

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| grep_literal_rare | 352.69 | 18.43 | 0.05x | identical normalized output |
| grep_literal_common | 358.55 | 48.26 | 0.13x | identical normalized output |
| grep_regex_escalation | 218.49 | 1037.39 | 4.75x | identical normalized output |

## Ripgrep

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| rg_literal_rare | 39.05 | 13.71 | 0.35x | identical normalized output |
| rg_literal_common | 42.44 | 37.35 | 0.88x | identical normalized output |
| rg_regex_escalation | 68.36 | 1043.93 | 15.27x | identical normalized output |

## Nearby Agent Workloads

| operation | local median ms | redis median ms | redis/local | validation |
|---|---:|---:|---:|---|
| tree_walk | 2.83 | 40.53 | 14.34x | counts match (4049) |
| find_runbook_names | 3.21 | 37.86 | 11.80x | counts match (1000) |
| read_hot_files | 1.39 | 182.30 | 131.15x | counts match (792005) |
| head_hot_files | 1.93 | 163.51 | 84.85x | counts match (75690) |
| line_window_hot_files | 1.91 | 164.00 | 85.77x | counts match (49587) |
