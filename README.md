# Redis Agent Filesystem

Redis Agent Filesystem, or AFS, gives agents a filesystem-shaped way to work with data, without being trapped in one machine's local disk.

The name is an explicit nod to the original Andrew File System (AFS): a shared filesystem built for distributed work. Redis Agent Filesystem borrows that shared-filesystem inspiration and adapts it for agents, with Redis as the persistence and coordination layer.

Filesystems are a great interface for agents because they already know how to read files, write files, search trees, run tools, and work in directories. But ordinary local filesystems have a few problems for agent workflows:

- they are tied to one machine
- they are hard to checkpoint, fork, and restore cleanly
- they are awkward to share across agents, shells, and computers
- they do not give you a simple saved source of truth

AFS fixes that by:

- storing workspace state in Redis
- exposing the current workspace as a normal local filesystem
- materializing a real local working copy when you run commands
- letting you checkpoint and restore workspaces
- letting you fork a workspace for parallel work

If you want the short version, AFS is:

- a workspace system for agents
- backed by Redis
- with real directories for real tools

## Why Redis

Local filesystems are already fast. AFS uses Redis so you can keep a remote, checkpointable, forkable source of truth without paying a huge performance penalty when you mount it locally.

On this machine, using AFS over NFS on macOS against a 33-file corpus with 7 benchmark rounds, the median timings looked like this:

| Operation | Local median | AFS median |
| --- | ---: | ---: |
| read medium source file | 0.01 ms | 0.01 ms |
| grep literal across corpus | 0.95 ms | 1.26 ms |
| grep ignore-case across corpus | 1.69 ms | 3.07 ms |
| walk tree | 0.15 ms | 0.11 ms |
| overwrite a 2 KB file | 0.07 ms | 0.91 ms |
| mkdir + rmdir | 0.06 ms | 2.34 ms |

The important pattern is:

- reads, `ls`, and tree walks stay very close to local filesystem speed
- search stays in the same low-millisecond range
- writes and renames are slower than local disk, but still stay in low single-digit milliseconds

So AFS is not trying to beat your local SSD. It is trying to give you a remote filesystem with enough performance that normal tools still feel normal.

## 60-Second Quick Start

Build AFS:

```bash
make
```

Run setup:

```bash
./afs setup
```

The default path is to mount a workspace and use it like a normal folder.

During setup:

- choose your Redis connection
- choose a workspace name
- choose a local mountpoint like `~/afs`

When setup finishes, AFS mounts that workspace for you. Then you can just use the folder:

```bash
cd ~/afs
ls
echo "# Notes" > notes.md
```

If you want to remount it later:

```bash
./afs up
./afs status
./afs down
```

If you want to bring an existing folder into AFS:

```bash
./afs workspace import my-repo ./repo
./afs workspace use my-repo
./afs up
```

If you want to exclude local junk before importing, create a `.afsignore` file in that folder first.

If you want to save a known-good point:

```bash
./afs checkpoint create my-repo before-refactor
```

If you want a second line of work:

```bash
./afs workspace fork my-repo my-repo-experiment
```

## The Basic Model

AFS has two main concepts:

- `workspace`: a codebase or state tree
- `checkpoint`: a saved restore point inside that workspace

Typical flow:

1. Put a workspace into AFS with `workspace create` or `workspace import`
2. Mount it and use it like a normal directory
3. Save stable moments with `checkpoint create`
4. Fork it when you want a second line of work
5. Restore a checkpoint if you want to go back

## Mounted Filesystem

The simplest way to think about AFS is:

- Redis stores the workspace state
- AFS mounts the current workspace into a local folder
- your editor, shell, and tools use that folder like any other directory

The main mount commands are:

```bash
./afs up
./afs status
./afs down
```

On macOS AFS uses NFS. On Linux AFS uses FUSE.

If no workspace exists yet, setup will ask for one and create it before mounting.

## Most Useful Commands

```bash
./afs setup
./afs workspace create <workspace>
./afs workspace import <workspace> <directory>
./afs workspace list
./afs workspace use <workspace>
./afs workspace run <workspace> -- <command...>
./afs workspace clone <workspace> <directory>
./afs workspace fork <workspace> <new-workspace>
./afs checkpoint create <workspace> <name>
./afs checkpoint list <workspace>
./afs checkpoint restore <workspace> <name>
./afs up
./afs status
./afs down
```

For command help:

```bash
./afs --help
./afs workspace --help
./afs checkpoint --help
```

## `.afsignore`

`afs workspace import` respects a `.afsignore` file in the source directory.

Use it to skip things you do not want stored in AFS, like:

- build output
- caches
- logs
- machine-local settings
- large temporary files

`.afsignore` uses `.gitignore`-style patterns. For example:

```gitignore
node_modules/
.venv/
dist/
*.log
.DS_Store
tmp/
```

You can also re-include something with `!`:

```gitignore
*.log
!important.log
```

Notes:

- `.afsignore` is only used when importing a directory into AFS
- the `.afsignore` file itself is imported too, so the workspace keeps its own import rules
- legacy `.rafignore` and `.rfsignore` files are still accepted, but `.afsignore` is the preferred name

## What Gets Stored Where

- Redis stores the saved workspace state
- your chosen mountpoint is the live local folder in mounted filesystem mode
- AFS creates local working copies under `~/.afs/workspaces` only for `workspace run`
- `afs.config.json` stores local CLI configuration next to the `afs` binary

You can think of Redis as the saved source of truth.

In mounted filesystem mode:

- you work in your chosen mountpoint
- you can mostly ignore `~/.afs/workspaces`

In `workspace run` mode:

- AFS materializes a local working copy under `~/.afs/workspaces`
- your command runs there
- AFS saves changes back to Redis when the command exits

## Build

Build everything:

```bash
make
```

Install `afs` onto your `PATH`:

```bash
make install
```

Other build targets:

```bash
make module
make mount
make clean
make uninstall
```

## Alternative: `workspace run`

If you do not want a mounted filesystem, AFS also has a command-oriented mode:

```bash
./afs workspace run my-repo -- bash
```

This is useful when:

- you want to run one command and save the results back automatically
- you are driving AFS from another agent or script
- you do not want to keep a mounted directory around
- you want a more explicit “open, run, save” workflow

`workspace run`:

- makes sure the workspace exists locally
- runs your command with that workspace as the current working directory
- captures any changes and saves them back to Redis when the command exits

For example:

```bash
./afs workspace run my-repo -- go test ./...
```

## Repo Contents

This repo includes:

- the `afs` CLI
- the Redis module
- mount daemons for local filesystem access
- Python and MCP integrations

But if you are brand new here, start with:

```bash
make
./afs setup
cd <your-mountpoint>
```
