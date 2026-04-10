# Bench: AFS vs local ~/.claude

Harness noop (overhead): local=1.77ms afs-warm=1.66ms afs-cold=1.55ms


| op | local (ms) | afs warm (ms) | afs cold (ms) | warm/local | cold/local |
|----|-----------:|--------------:|--------------:|-----------:|-----------:|
| stat_root | 5.32 | 5.51 | 4.65 | 1.0x | 0.9x |
| readdir_root | 3.44 | 3.36 | 3.25 | 1.0x | 0.9x |
| tree_walk | 23.35 | 27.79 | 26.13 | 1.2x | 1.1x |
| tree_walk_dirs | 22.96 | 21.65 | 21.17 | 0.9x | 0.9x |
| ls_recursive | 47.83 | 61.45 | 61.97 | 1.3x | 1.3x |
| du | 12.32 | 18.83 | 18.60 | 1.5x | 1.5x |
| grep_text | 42.32 | 43.00 | 44.52 | 1.0x | 1.1x |
| glob_md | 22.88 | 22.27 | 22.17 | 1.0x | 1.0x |
| random_stat | 3.49 | 3.84 | 3.71 | 1.1x | 1.1x |
| random_read | 5.35 | 5.94 | 9.92 | 1.1x | 1.9x |
| head_of_tree | 7.58 | 7.64 | 9.38 | 1.0x | 1.2x |
| write_new_small | 10.47 | 32105.30 | 31640.66 | 3067.3x | 3022.9x |
| write_overwrite | 5.92 | 5296.53 | 5316.29 | 894.8x | 898.2x |
| append_jsonl | 6.97 | 5739.68 | 5627.43 | 823.2x | 807.1x |
| _cleanup | 2.88 | 45.22 | 39.08 | 15.7x | 13.6x |
