# Redis-FS

Redis-FS is a Redis module that provides a complete POSIX-like filesystem as a native data type, plus a FUSE mount that exposes it as a regular local directory. One Redis key holds one filesystem volume — directories, files, symlinks, permissions, and all metadata.

**Why Redis-FS:**
- **Persistence** — data survives process restarts via RDB/AOF
- **Multi-client access** — any Redis client can read/write the same volume concurrently
- **Atomic operations** — every FS.* command is atomic, no partial writes
- **No local disk dependency** — agents can run statelessly; storage lives in Redis
- **Instant cleanup** — `DEL vol` removes an entire filesystem in one command

**Key concept:** one Redis key = one filesystem volume. The key name is the volume name. All paths within a volume are absolute (start with `/`).

## Build

```bash
cd /home/ubuntu/git/redis-fs
make          # builds module/fs.so + mount/redis-fs-mount + rfs
make module   # builds only the Redis module (module/fs.so)
make mount    # builds only the FUSE mount binary
make cli      # builds only the rfs CLI
```

## CLI Commands

The `rfs` binary is at the repo root after building.

| Command | Description |
|---------|-------------|
| `rfs setup` | Interactive first-time wizard — saves config, starts services |
| `rfs up` | Start services from saved config |
| `rfs down` | Stop all services and unmount |
| `rfs status` | Show current status |
| `rfs migrate <dir>` | Import a directory into Redis, honoring `.rfsignore` if present |

Use `--config <path>` before any command to override the config file location:
```bash
./raf --config /path/to/custom.json up
```

## Programmatic Setup (for agents)

Skip the interactive wizard by writing the config file directly.

### 1. Build

```bash
cd /home/ubuntu/git/redis-fs && make
```

### 2. Write config

```bash
cat > /home/ubuntu/git/redis-fs/raf.config.json << 'EOF'
{
  "useExistingRedis": false,
  "redisAddr": "localhost:6379",
  "redisPassword": "",
  "redisDB": 0,
  "currentWorkspace": "my-workspace",
  "mountpoint": "/home/ubuntu/redis-fs",
  "readOnly": false,
  "allowOther": false,
  "redisServerBin": "",
  "modulePath": "",
  "mountBin": "",
  "redisLog": "/tmp/rfs-redis.log",
  "mountLog": "/tmp/rfs-mount.log"
}
EOF
```

Empty strings for `redisServerBin`, `modulePath`, and `mountBin` are auto-detected at runtime.

### 3. Start

```bash
./raf up
```

### 4. Verify

```bash
./raf status
ls ~/redis-fs
```

## Configuration Reference

File: `raf.config.json` (next to the `raf` binary in the repo root)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `useExistingRedis` | bool | `false` | `true` = connect to running Redis; `false` = start a managed instance |
| `redisAddr` | string | `"localhost:6379"` | Redis server address (host:port) |
| `redisPassword` | string | `""` | Redis password (empty = no auth) |
| `redisDB` | int | `0` | Redis database number (0–15) |
| `currentWorkspace` | string | `""` | Workspace mounted locally when a mountpoint is configured |
| `mountpoint` | string | | Local directory where the filesystem is mounted |
| `readOnly` | bool | `false` | Mount as read-only |
| `allowOther` | bool | `false` | Allow other system users to access the mount |
| `redisServerBin` | string | auto | Path to `redis-server` (only used when `useExistingRedis` is `false`) |
| `modulePath` | string | auto | Path to `fs.so` module |
| `mountBin` | string | auto | Path to `redis-fs-mount` binary |
| `redisLog` | string | `"/tmp/rfs-redis.log"` | Log file for managed Redis |
| `mountLog` | string | `"/tmp/rfs-mount.log"` | Log file for mount daemon |

### Common config patterns

**Managed Redis (simplest):**
```json
{
  "useExistingRedis": false,
  "currentWorkspace": "my-workspace",
  "mountpoint": "/home/ubuntu/data"
}
```

**Connect to existing Redis:**
```json
{
  "useExistingRedis": true,
  "redisAddr": "redis.local:6379",
  "redisPassword": "secret",
  "currentWorkspace": "my-workspace",
  "mountpoint": "/home/ubuntu/data"
}
```

## Runtime State

- Config: `rfs.config.json` (repo root, next to the binary)
- State: `~/.rfs/state.json` (runtime PIDs, created/removed automatically)

## FS.* Command Reference

All commands use the pattern: `FS.<CMD> <key> <path> [args...]`

Replace `vol` below with your chosen key name.

### Quick reference

| Unix command | Redis-FS equivalent | Notes |
|---|---|---|
| `echo "text" > file` | `FS.ECHO vol /file "text"` | Creates parents automatically |
| `echo "text" >> file` | `FS.ECHO vol /file "text" APPEND` | Also `FS.APPEND` |
| `cat file` | `FS.CAT vol /file` | Follows symlinks |
| `touch file` | `FS.TOUCH vol /file` | Creates empty file or updates mtime |
| `mkdir dir` | `FS.MKDIR vol /dir` | Parent must exist |
| `mkdir -p a/b/c` | `FS.MKDIR vol /a/b/c PARENTS` | Creates intermediates |
| `ls dir` | `FS.LS vol /dir` | Returns child names |
| `ls -l dir` | `FS.LS vol /dir LONG` | Includes type, mode, size, mtime |
| `rm file` | `FS.RM vol /file` | Works on files, dirs, symlinks |
| `rm -r dir` | `FS.RM vol /dir RECURSIVE` | Deletes entire subtree |
| `cp src dst` | `FS.CP vol /src /dst` | Files only without RECURSIVE |
| `cp -r src dst` | `FS.CP vol /src /dst RECURSIVE` | Deep copy with metadata |
| `mv src dst` | `FS.MV vol /src /dst` | Moves entire subtrees atomically |
| `find dir -name "*.txt"` | `FS.FIND vol /dir "*.txt"` | Glob: `*`, `?`, `[a-z]`, `[!x]`, `\` |
| `find dir -name "*.txt" -type f` | `FS.FIND vol /dir "*.txt" TYPE file` | Filter: file, dir, symlink |
| `grep -r "pattern" dir` | `FS.GREP vol /dir "*pattern*"` | Glob on each line |
| `grep -ri "pattern" dir` | `FS.GREP vol /dir "*pattern*" NOCASE` | Case-insensitive |
| `stat file` | `FS.STAT vol /file` | type, mode, uid, gid, size, times |
| `test -e file` | `FS.TEST vol /file` | Returns 1 or 0 |
| `chmod 0755 file` | `FS.CHMOD vol /file 0755` | Octal mode string |
| `chown uid:gid file` | `FS.CHOWN vol /file uid gid` | Separate uid and gid args |
| `ln -s target link` | `FS.LN vol /target /link` | Relative or absolute target |
| `readlink link` | `FS.READLINK vol /link` | Returns raw target string |
| `tree dir` | `FS.TREE vol /dir` | Nested array structure |
| `tree -L 2 dir` | `FS.TREE vol /dir DEPTH 2` | Limits recursion depth |
| `du -sh` / `df` | `FS.INFO vol` | File/dir/symlink counts + total bytes |

### Usage examples

**Write and read:**
```bash
redis-cli FS.ECHO vol /hello.txt "Hello, World!"
redis-cli FS.CAT vol /hello.txt
```

**Append:**
```bash
redis-cli FS.ECHO vol /log.txt "first line" APPEND
redis-cli FS.ECHO vol /log.txt "second line" APPEND
```

**Directories:**
```bash
redis-cli FS.MKDIR vol /data/projects PARENTS
redis-cli FS.LS vol /data LONG
```

**Search by filename:**
```bash
redis-cli FS.FIND vol / "*.md"
redis-cli FS.FIND vol /src "*.go" TYPE file
```

**Search file contents:**
```bash
redis-cli FS.GREP vol / "*TODO*"
redis-cli FS.GREP vol /src "*error*" NOCASE
```

**BM25-ranked search (RedisSearch via redis-qmd):**
```bash
redis-qmd --key vol index create
redis-qmd --key vol search "TODO auth"
redis-qmd --key vol query "\"disk full\" AND retry path:/logs/"
redis-qmd --key vol grep "TODO"
```

**Copy, move, delete:**
```bash
redis-cli FS.CP vol /config.json /config.json.bak
redis-cli FS.CP vol /src /src-backup RECURSIVE
redis-cli FS.MV vol /draft.txt /final.txt
redis-cli FS.RM vol /temp RECURSIVE
```

**Symlinks:**
```bash
redis-cli FS.LN vol /config.json /current-config
redis-cli FS.READLINK vol /current-config
```

**Filesystem overview:**
```bash
redis-cli FS.INFO vol
redis-cli FS.TREE vol / DEPTH 2
```

## Bulk Import via redis-cli

If you need to import files directly via `redis-cli` without the `rfs migrate` command:

```bash
cd /path/to/local/dir && find . -type f | while read -r f; do
  redis-cli FS.ECHO vol "/${f#./}" "$(cat "$f")"
done
```

Optional — preserve permissions:
```bash
cd /path/to/local/dir && find . -type f | while read -r f; do
  path="/${f#./}"
  redis-cli FS.CHMOD vol "$path" "0$(stat -c '%a' "$f")"
  redis-cli FS.CHOWN vol "$path" "$(stat -c '%u' "$f")" "$(stat -c '%g' "$f")"
done
```

Or use `rfs migrate <dir>` which handles import, archiving, and mounting in one step. If the source directory contains a `.rfsignore`, matching files and directories are skipped using `.gitignore`-style patterns.

## Volume Management

```bash
redis-cli DEL vol                     # delete entire filesystem
redis-cli EXPIRE vol 3600             # auto-expire after 1 hour
redis-cli SCAN 0 TYPE redis-fs0       # list all filesystem volumes
redis-cli RENAME staging production   # rename a volume
```

## Key Differences and Gotchas

1. **All paths must be absolute** — start with `/`. There is no working directory.
2. **No `cd` or `pwd`** — every command takes the full path explicitly.
3. **Grep uses glob patterns, not regex** — use `*error*` not `.*error.*`.
4. **FS.GREP returns triples** — each match is `[filepath, line_number, line]`.
5. **FS.FIND matches basename only** — `FS.FIND vol / "*.md"` matches `/docs/README.md`.
6. **FS.ECHO auto-creates parents** — no need to `mkdir -p` before writing.
7. **Symlinks resolve at read time** — max 40 levels; cycles produce an error.
8. **Large recursive operations block Redis** — partition across multiple keys for millions of files.
9. **No streaming reads** — `FS.CAT` returns the entire file at once.
10. **Permission bits are metadata only** — `FS.CHMOD`/`FS.CHOWN` store values but Redis does not enforce them.

## Troubleshooting

| Problem | Solution |
|---------|----------|
| `no configuration found` | Run `rfs setup` or create `rfs.config.json` next to the binary |
| `module not loaded` | Load with: `redis-cli MODULE LOAD /path/to/module/fs.so` |
| `cannot find redis-server` | Install Redis, or set `useExistingRedis: true` |
| `cannot find redis-fs-mount` | Run `make mount` in the repo root |
| `mount did not become ready` | Check `mountLog` path for errors |
| `redis-fs is already running` | Run `rfs down` first |
