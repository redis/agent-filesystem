# Bench: AFS vs local ~/.claude

Harness noop (overhead): local=2.05ms afs-warm=2.11ms afs-cold=2.87ms


| op | local (ms) | afs warm (ms) | afs cold (ms) | warm/local | cold/local |
|----|-----------:|--------------:|--------------:|-----------:|-----------:|
| stat_root | 6.14 | 6.52 | 6.66 | 1.1x | 1.1x |
| readdir_root | 4.12 | 3.85 | 3.88 | 0.9x | 0.9x |
| tree_walk | 25.64 | 27.04 | 26.04 | 1.1x | 1.0x |
| tree_walk_dirs | 24.67 | 23.49 | 23.28 | 1.0x | 0.9x |
| ls_recursive | 51.68 | 56.00 | 56.14 | 1.1x | 1.1x |
| du | 14.62 | 17.53 | 17.45 | 1.2x | 1.2x |
| grep_text | 46.01 | 42.70 | 43.67 | 0.9x | 0.9x |
| glob_md | 25.89 | 24.52 | 23.97 | 0.9x | 0.9x |
| random_stat | 4.57 | 4.53 | 4.49 | 1.0x | 1.0x |
| random_read | 6.68 | 6.67 | 9.48 | 1.0x | 1.4x |
| head_of_tree | 9.17 | 9.03 | 10.24 | 1.0x | 1.1x |
