# Bench: AFS vs local ~/.claude

Harness noop (overhead): local=2.09ms afs-warm=1.91ms afs-cold=2.89ms


| op | local (ms) | afs warm (ms) | afs cold (ms) | warm/local | cold/local |
|----|-----------:|--------------:|--------------:|-----------:|-----------:|
| stat_root | 5.83 | 6.14 | 6.63 | 1.1x | 1.1x |
| readdir_root | 3.96 | 3.90 | 3.78 | 1.0x | 1.0x |
| tree_walk | 25.79 | 25.88 | 26.17 | 1.0x | 1.0x |
| tree_walk_dirs | 24.32 | 23.50 | 23.60 | 1.0x | 1.0x |
| ls_recursive | 51.00 | 54.97 | 56.32 | 1.1x | 1.1x |
| du | 14.01 | 17.71 | 16.61 | 1.3x | 1.2x |
| grep_text | 47.27 | 43.70 | 42.25 | 0.9x | 0.9x |
| glob_md | 25.66 | 23.37 | 24.12 | 0.9x | 0.9x |
| random_stat | 365.19 | 378.15 | 365.78 | 1.0x | 1.0x |
| random_read | 303.23 | 311.82 | 308.25 | 1.0x | 1.0x |
| head_of_tree | 8.63 | 8.33 | 9.73 | 1.0x | 1.1x |
