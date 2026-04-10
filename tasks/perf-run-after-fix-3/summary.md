# Bench: AFS vs local ~/.claude

Harness noop (overhead): local=1.75ms afs-warm=2.58ms afs-cold=2.39ms


| op | local (ms) | afs warm (ms) | afs cold (ms) | warm/local | cold/local |
|----|-----------:|--------------:|--------------:|-----------:|-----------:|
| stat_root | 5.75 | 6.00 | 5.68 | 1.0x | 1.0x |
| readdir_root | 3.49 | 3.62 | 3.52 | 1.0x | 1.0x |
| tree_walk | 25.62 | 25.70 | 25.75 | 1.0x | 1.0x |
| tree_walk_dirs | 24.73 | 23.46 | 23.57 | 0.9x | 1.0x |
| ls_recursive | 50.61 | 53.70 | 57.13 | 1.1x | 1.1x |
| du | 13.91 | 16.10 | 16.48 | 1.2x | 1.2x |
| grep_text | 44.93 | 42.01 | 43.16 | 0.9x | 1.0x |
| glob_md | 24.03 | 23.98 | 23.97 | 1.0x | 1.0x |
| random_stat | 3.83 | 3.81 | 4.20 | 1.0x | 1.1x |
| random_read | 5.86 | 5.43 | 9.23 | 0.9x | 1.6x |
| head_of_tree | 7.71 | 7.77 | 10.67 | 1.0x | 1.4x |
| write_new_small | 10.86 | 67277.43 | 69808.63 | 6192.7x | 6425.7x |
| write_overwrite | 6.40 | 4143.34 | 5253.28 | 647.5x | 821.0x |
| append_jsonl | 7.33 | 5693.86 | 5583.83 | 777.1x | 762.1x |
| _cleanup | 2.98 | 79.51 | 54.64 | 26.7x | 18.4x |
