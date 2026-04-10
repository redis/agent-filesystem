# Bench: AFS vs local ~/.claude

Harness noop (overhead): local=1.70ms afs-warm=2.45ms afs-cold=2.40ms


| op | local (ms) | afs warm (ms) | afs cold (ms) | warm/local | cold/local |
|----|-----------:|--------------:|--------------:|-----------:|-----------:|
| stat_root | 5.26 | 5.49 | 5.29 | 1.0x | 1.0x |
| readdir_root | 3.27 | 3.36 | 3.27 | 1.0x | 1.0x |
| tree_walk | 23.25 | 23.98 | 23.62 | 1.0x | 1.0x |
| tree_walk_dirs | 22.83 | 21.73 | 21.36 | 1.0x | 0.9x |
| ls_recursive | 49.13 | 50.97 | 51.51 | 1.0x | 1.0x |
| du | 13.17 | 15.06 | 14.22 | 1.1x | 1.1x |
| grep_text | 41.16 | 39.33 | 40.25 | 1.0x | 1.0x |
| glob_md | 22.30 | 21.13 | 21.68 | 0.9x | 1.0x |
| random_stat | 3.48 | 3.62 | 3.60 | 1.0x | 1.0x |
| random_read | 5.08 | 5.06 | 7.84 | 1.0x | 1.5x |
| head_of_tree | 7.01 | 7.02 | 8.23 | 1.0x | 1.2x |
| write_new_small | 9.99 | 58530.71 | 58346.35 | 5860.7x | 5842.2x |
| write_overwrite | 5.68 | 3760.16 | 3602.21 | 662.4x | 634.5x |
| append_jsonl | 6.58 | 3815.82 | 3730.46 | 580.1x | 567.1x |
| _cleanup | 2.77 | 45.58 | 37.45 | 16.5x | 13.5x |
