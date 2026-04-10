# Bench: AFS vs local ~/.claude

Harness noop (overhead): local=1.77ms afs-warm=2.13ms afs-cold=2.17ms


| op | local (ms) | afs warm (ms) | afs cold (ms) | warm/local | cold/local |
|----|-----------:|--------------:|--------------:|-----------:|-----------:|
| stat_root | 5.32 | 5.11 | 5.64 | 1.0x | 1.1x |
| readdir_root | 3.50 | 3.31 | 3.59 | 0.9x | 1.0x |
| tree_walk | 22.64 | 24.86 | 25.02 | 1.1x | 1.1x |
| tree_walk_dirs | 22.38 | 22.12 | 22.68 | 1.0x | 1.0x |
| ls_recursive | 46.67 | 52.25 | 52.50 | 1.1x | 1.1x |
| du | 12.37 | 16.04 | 15.46 | 1.3x | 1.2x |
| grep_text | 42.44 | 40.03 | 42.26 | 0.9x | 1.0x |
| glob_md | 22.69 | 22.38 | 23.23 | 1.0x | 1.0x |
| random_stat | 3.53 | 3.62 | 3.92 | 1.0x | 1.1x |
| random_read | 5.01 | 5.57 | 8.84 | 1.1x | 1.8x |
| head_of_tree | 7.11 | 7.66 | 8.95 | 1.1x | 1.3x |
| write_new_small | 10.00 | 91254.17 | 115828.43 | 9127.2x | 11585.2x |
| write_overwrite | 6.00 | 12700.80 | 17836.21 | 2118.6x | 2975.2x |
| append_jsonl | 6.65 | 4026.46 | 6006.25 | 605.8x | 903.7x |
| _cleanup | 2.79 | 42.85 | 68.77 | 15.4x | 24.7x |
