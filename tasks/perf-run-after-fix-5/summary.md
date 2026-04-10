# Bench: AFS vs local ~/.claude

Harness noop (overhead): local=1.68ms afs-warm=1.81ms afs-cold=1.73ms


| op | local (ms) | afs warm (ms) | afs cold (ms) | warm/local | cold/local |
|----|-----------:|--------------:|--------------:|-----------:|-----------:|
| stat_root | 4.95 | 5.34 | 5.74 | 1.1x | 1.2x |
| readdir_root | 3.29 | 3.69 | 3.39 | 1.1x | 1.0x |
| tree_walk | 23.52 | 28.27 | 28.76 | 1.2x | 1.2x |
| tree_walk_dirs | 22.69 | 22.02 | 23.34 | 1.0x | 1.0x |
| ls_recursive | 47.55 | 61.42 | 62.25 | 1.3x | 1.3x |
| du | 12.96 | 17.68 | 18.86 | 1.4x | 1.5x |
| grep_text | 42.76 | 42.20 | 47.94 | 1.0x | 1.1x |
| glob_md | 22.77 | 21.61 | 20.99 | 0.9x | 0.9x |
| random_stat | 3.53 | 3.86 | 3.54 | 1.1x | 1.0x |
| random_read | 5.12 | 6.07 | 8.37 | 1.2x | 1.6x |
| head_of_tree | 6.99 | 7.71 | 8.06 | 1.1x | 1.2x |
| write_new_small | 10.13 | 26046.62 | 27378.12 | 2570.5x | 2701.9x |
| write_overwrite | 6.03 | 3895.00 | 5245.74 | 645.5x | 869.4x |
| append_jsonl | 6.74 | 4661.19 | 5371.32 | 691.2x | 796.5x |
| _cleanup | 2.95 | 46.12 | 50.63 | 15.6x | 17.2x |
