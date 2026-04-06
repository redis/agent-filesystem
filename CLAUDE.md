# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
make                # build module/fs.so + mount/agent-filesystem-mount + mount/agent-filesystem-nfs + afs
make module         # build module/fs.so only
make mount          # build mount/agent-filesystem-mount + mount/agent-filesystem-nfs
make cli            # build afs only
make clean          # remove compiled artifacts

# CLI lifecycle helper:
./afs setup
./afs up
./afs down
./afs status
./afs workspace import <workspace> <directory>
./afs workspace run <workspace> -- /bin/sh
./afs checkpoint list <workspace>
```

Load into Redis for manual testing:
```bash
redis-server --loadmodule ./module/fs.so
# or at runtime:
redis-cli MODULE LOAD $(pwd)/module/fs.so
```

CLI coverage exists under `cli/`; run `cd cli && go test ./...` for automated checks. Module testing is still manual via `redis-cli`.

## Architecture

Agent Filesystem is a native Redis module (C, `-std=c11`) that registers a custom data type (`fsObject`) and an `FS.*` command family. **One Redis key = one complete filesystem.**

### Data Model

- **fsObject**: Top-level container holding a `RedisModuleDict` mapping absolute path strings to inodes, plus aggregate counters (file_count, dir_count, symlink_count, total_data_size).
- **fsInode**: Union-typed node — file (inline content + 256-byte trigram bloom filter), directory (children basename array), or symlink (target string). Every inode carries POSIX metadata (mode, uid, gid, ctime/mtime/atime).
- **Storage is flat**: paths like `/etc/nginx/nginx.conf` are keys in a single dict, not a tree. Directories track only child basenames. This gives O(1) path lookups.

### Source Files

| File | Purpose |
|------|---------|
| `module/fs.c` | All command handlers, RDB persistence, type registration, bloom filter logic |
| `module/fs.h` | Inode/object struct definitions, type constants |
| `module/path.c` | Path normalization, parent/basename/join, full glob matching (`*`, `?`, `[a-z]`, `[!x]`, `\` escaping) |
| `module/path.h` | Path utility declarations |
| `module/redismodule.h` | Redis module API (vendored header) |

### Key Internals

- **Auto-create / auto-delete**: First write to a key creates it with root `/`; deleting everything removes the key.
- **Symlink resolution** (`fsResolvePath`): follows up to 40 levels, supports absolute and relative targets.
- **Bloom filters for GREP**: each file inode has a 256-byte bloom filter built from lowercased trigrams. `FS.GREP` skips files whose bloom filter proves the literal portion of the pattern cannot match.
- **Binary detection in GREP**: files with NUL bytes in the first 8KB report "Binary file matches" instead of content.
- **Parent auto-creation** (`fsEnsureParents`): write commands like `FS.ECHO` and `FS.MKDIR PARENTS` recursively create missing ancestor directories.
- **FS.ECHO APPEND flag**: `FS.ECHO key path content APPEND` appends instead of overwriting, matching the shell `echo >>` pattern. `FS.APPEND` is retained as a backward-compatible alias.
- **RDB format version 0**: serializes all inodes with path/type/metadata/payload; bloom filters are rebuilt on load, not persisted.

### Command Pattern

Every `FS.*` command handler follows the same pattern:
1. Call `fsGetObject()` to open the key and get/create the `fsObject`
2. Normalize the path with `fsNormalizePath()`
3. Look up the inode in the dict
4. Perform the operation, replicate if write (`RedisModule_ReplicateVerbatim()`)
5. For writes, call `fsMaybeDeleteKey()` if content was removed
