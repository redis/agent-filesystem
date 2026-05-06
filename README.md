# Agent Filesystem

Agent Filesystem, or AFS, gives agents a filesystem-shaped way to work with data, without being trapped in one machine's local disk.

The name is an explicit nod to the original Andrew File System (AFS): a shared filesystem built for distributed work. Agent Filesystem borrows that shared-filesystem inspiration and adapts it for agents, with Redis as the persistence and coordination layer.

Filesystems are a great interface for agents because they already know how to read files, write files, search trees, run tools, and work in directories. But ordinary local filesystems have a few problems for agent workflows:

- they are tied to one machine
- they are hard to checkpoint, fork, and restore cleanly
- they are awkward to share across agents, shells, and computers
- they do not give you a simple saved source of truth

AFS fixes that by:

- storing workspace state in Redis
- mounting workspaces to real local folders through sync or an optional live mount
- letting you checkpoint and restore workspaces
- letting you fork a workspace for parallel work

If you want the short version, AFS is:

- a workspace system for agents
- backed by Redis
- with real directories for real tools

AFS has three modes of operation:

- Cloud-hosted - now at https://afs.cloud
- Self-managed - run `docker compose up` or `make web-dev`
- Standalone - no control plane, direct to Redis

## Why AFS Instead of the Usual Tools

AFS is not trying to replace every mature filesystem or version-control tool.

If you need one shared live POSIX tree, use NFS, EFS, CephFS, NetApp, or another standard shared filesystem.

If you only need source-code branching and merging, use Git and `git worktree`.

If all work stays on one machine, local disk plus ZFS or Btrfs snapshots is simpler.

AFS exists for a different shape of problem: agent workspaces that should feel like a normal directory, while also being easy to checkpoint, restore, fork, and move between machines.

| If you need... | The usual good answer | Why AFS is different |
| --- | --- | --- |
| one shared live tree | NFS, EFS, CephFS, NetApp, SMB | AFS can expose a shared mount, but the main point is not the mount. The main point is a workspace model with checkpoints and forks. |
| source-code branching | Git branches and `git worktree` | AFS is for whole workspaces, not just committed source. Notes, generated files, prompts, logs, scratch state, and code can all live in one workspace timeline. |
| point-in-time rollback on one machine | ZFS or Btrfs snapshots | AFS gives you similar checkpoint and fork ideas, but with a remote saved source of truth that is not tied to one machine. |
| normal local execution plus remote saved state | a hand-rolled sync or cache layer | AFS makes that model explicit: save state in Redis, sync or mount locally, run real tools, checkpoint when ready. |

Choose something else when:

- you need the strongest possible POSIX fidelity and multi-writer behavior across many clients
- you mostly want a shared NAS for humans or services, not an agent workspace model
- you only need Git-style branching for source code
- you mostly store large binaries, media, or build artifacts

## Why Redis

Redis is not the reason to use AFS. The reason to use AFS is the workspace model above.

Redis is the canonical store because it gives AFS a simple remote source of truth for workspace state, checkpoints, manifests, blobs, and metadata, while still letting agents work through normal local directories and mounts.

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

So AFS is not trying to beat your local SSD, and it is not trying to out-POSIX mature shared filesystems. It is trying to give you a remote, checkpointable, forkable workspace with enough performance that normal tools still feel normal.

## Requirements

AFS requires a Redis instance you provide.

AFS does not start Redis for you, manage a local Redis server, or hide the
connection details behind a separate product flow. The CLI expects a reachable
Redis address in `afs.config.json`, whether that Redis is local, containerized,
hosted, or managed by something else.

## 60-Second Quick Start

Build AFS:

```bash
make
```

Connect the CLI:

```bash
./afs auth login
```

The default path is sync mode: Redis stays the source of truth while AFS keeps
a real local folder up to date.

To start using AFS, create or import a workspace and mount it to a local
folder:

```bash
./afs ws create my-repo
./afs ws mount my-repo ~/my-repo
cd ~/my-repo
ls
echo "# Notes" > notes.md
```

If you want to check or stop local mounts:

```bash
./afs status
./afs ws unmount my-repo
```

If you want to bring an existing folder into AFS:

```bash
./afs ws import my-repo ./repo
./afs ws mount my-repo ./repo
```

If you want to exclude local junk before importing, create a `.afsignore` file in that folder first.

If you want to save a known-good point:

```bash
./afs cp create my-repo before-refactor
```

If you want commands with an optional workspace argument to use `my-repo` by
default:

```bash
./afs ws set-default my-repo
./afs cp list
```

If you want a second line of work:

```bash
./afs ws fork my-repo my-repo-experiment
```

## Config File

AFS stores CLI configuration in `afs.config.json` next to the `afs` binary.
The current config shape is nested and workspace-oriented:

```json
{
  "redis": {
    "addr": "localhost:6379",
    "username": "",
    "password": "",
    "db": 0,
    "tls": false
  },
  "mode": "sync",
  "workspace": {
    "default": "my-repo"
  },
  "runtime": {
    "mount": {
      "backend": "none",
      "allowOther": false,
      "mountBin": "",
      "nfsBin": "",
      "nfsHost": "127.0.0.1",
      "nfsPort": 20490
    },
    "logs": {
      "mount": "/tmp/afs-mount.log",
      "sync": "/tmp/afs-sync.log"
    }
  },
  "sync": {
    "fileSizeCapMB": 2048
  }
}
```

Notable current rules:

- Redis connection is required; there is no `useExistingRedis` or managed-Redis mode
- `redisKey` is not a config setting anymore; workspace keys are derived internally
- `workspace run` is gone; use `afs ws mount` for local work
- `workRoot` is internal state, not a user-facing config field
- `afs.config.json` stores connection defaults and an optional default workspace, not the current local folder

## MCP Server

AFS includes a workspace-first MCP server directly in the Go CLI:

```bash
./afs mcp
```

That command serves MCP over stdio and is meant to be launched by an MCP
client on demand. A minimal config looks like:

```json
{
  "mcpServers": {
    "afs": {
      "command": "/absolute/path/to/afs",
      "args": ["mcp"]
    }
  }
}
```

The MCP surface is workspace-oriented: list/create/fork workspaces,
read and edit files, grep a workspace, and create or restore checkpoints.
File-edit tools update the live workspace state and leave the workspace dirty
until `checkpoint_create` is called explicitly.

## The Basic Model

AFS has two main concepts:

- `workspace`: a codebase or state tree
- `checkpoint`: a saved restore point inside that workspace

Typical flow:

1. Put a workspace into AFS with `ws create` or `ws import`
2. Mount it to a normal local directory
3. Save stable moments with `cp create`
4. Fork it when you want a second line of work
5. Restore a checkpoint if you want to go back

## Local Surfaces

The simplest way to think about AFS is:

- Redis stores the workspace state
- AFS exposes that workspace locally either through sync mode or live mount mode
- your editor, shell, and tools use the local folder like any other directory

### Sync Mode

Sync mode is the default and recommended local surface.

- `afs ws mount <workspace> <directory>` starts a background sync daemon
- local edits are reconciled with the live Redis-backed workspace root
- `afs ws unmount <workspace-or-directory>` stops the sync daemon and preserves local files

### Live Mount Mode

Live mount mode exposes the workspace directly as a Redis-backed filesystem.

- `afs ws mount <workspace> <directory>` owns the local mountpoint
- edits go straight into the live Redis-backed workspace root
- `afs ws unmount <workspace-or-directory>` unmounts the filesystem and stops the mount daemon

On macOS AFS uses NFS. On Linux AFS uses FUSE.

Before using live mount mode, create or import a workspace and mount it with
`afs ws mount <workspace> <directory>`.

Current macOS/NFS caveats:

- overwriting the same path in place is not atomic under contention; a tight truncate-then-rewrite cycle can briefly expose an empty file to concurrent readers. If you need atomic replacement semantics, write a temp file and rename it over the destination.
- macOS may create `._*` AppleDouble sidecar files on the NFS mount to preserve Finder metadata, resource-fork data, and some extended attributes. These files can show up in listings, tree walks, and grep results.

## Most Useful Commands

```bash
./afs auth login
./afs config show --json
./afs ws create <workspace>
./afs ws import --mount-at-source <workspace> <directory>
./afs ws list
./afs ws set-default <workspace>
./afs ws mount <workspace> <directory>
./afs ws unmount <workspace-or-directory>
./afs ws fork <workspace> <new-workspace>
./afs cp create <workspace> <name>
./afs cp list <workspace>
./afs cp restore <workspace> <name>
./afs status
```

For command help:

```bash
./afs --help
./afs ws --help
./afs cp --help
make help
```

Note: `make --help` is handled by GNU `make` itself, so it shows make's built-in flags rather than this repo's targets. Use `make help` for the project target list.

## `.afsignore`

`afs ws import` respects a `.afsignore` file in the source directory.

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

## What Gets Stored Where

- Redis stores both the live workspace working copy and the immutable checkpoint history
- mounted folders are tracked as runtime state
- `~/.afs` is reserved for AFS runtime state and internal local metadata
- `afs.config.json` stores local CLI configuration next to the `afs` binary

You can think of Redis as the source of truth for both:

- the live mutable workspace root
- the explicit checkpoint history

In sync mode:

- you work in the directory passed to `afs ws mount`
- AFS keeps that directory reconciled with the live Redis-backed workspace root
- `afs ws unmount` stops the sync daemon but does not discard workspace state

In mounted filesystem mode:

- you work in the mountpoint
- edits go straight into the live Redis-backed workspace root
- `afs ws unmount` just unmounts; it does not create or sync a separate draft

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
make help
make mount
make commands
make test
make web-install
make web-server
make web-ui
make web-dev
make clean
make uninstall
```

Browse and run the repo's tests and benchmarks interactively:

```bash
./scripts/test_harness.py
./scripts/test_harness.py --list
```

## Web UI Dev

To run the local control plane and web UI together:

```bash
make web-dev
```

That target:

- builds `afs-control-plane`
- installs UI dependencies if `ui/node_modules` is missing
- starts `afs-control-plane` on `http://127.0.0.1:8091`
- starts the Vite dev server on `http://127.0.0.1:5173`
- wires the UI to the control plane with `VITE_AFS_API_BASE_URL`

If you want to run the pieces separately:

```bash
make web-server
make web-ui
```

You can override the defaults at invocation time:

```bash
make web-dev AFS_WEB_SERVER_ADDR=127.0.0.1:9001 AFS_WEB_API_BASE_URL=http://127.0.0.1:9001 AFS_WEB_UI_PORT=4173
```

## Mount a Workspace

Mount a workspace when you want a normal local directory that stays connected
to the Redis-backed workspace:

```bash
./afs ws mount my-repo ~/src/my-repo
```

## Repo Contents

This repo includes:

- the `afs` CLI
- mount daemons for local filesystem access
- the workspace-first MCP server built into `afs`
- sandbox and web tooling
- benchmark/helpers under `tests/`

But if you are brand new here, start with:

```bash
make
./afs auth login
./afs ws create demo
./afs ws mount demo ~/demo
cd ~/demo
```
