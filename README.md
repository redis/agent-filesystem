# Redis Agent Filesystem

Redis Agent Filesystem, or RAF, is a Redis-backed workspace system for agents and humans.

Phase 1 uses a hybrid model:

- Redis stores the canonical saved state for workspaces, checkpoints, manifests, blobs, and audit streams.
- Real directories under `~/.raf/workspaces/.../tree` are the live execution surface for shells, editors, compilers, and tests.
- `raf workspace run`, `raf workspace clone`, `raf workspace fork`, `raf checkpoint create`, and `raf checkpoint restore` are the main user-facing workflow.

The repository still contains the original Redis module, mount daemons, and legacy migration flow. RAF is now the main CLI surface.

## What Phase 1 Includes

- `raf` as the main CLI
- Workspace create, list, run, clone, fork, delete, and import
- Checkpoint create, list, and restore
- Local materialization under `~/.raf/workspaces`
- Redis audit streams for import, fork, and restore
- Optimistic concurrency checks on checkpoint restore

## Mental Model

RAF has two user-facing nouns:

- Workspace: one logical codebase or state domain
- Checkpoint: one immutable saved restore point inside a workspace

Phase 1 semantics are intentionally simple:

- Live state is local.
- Saved state is in Redis.
- Checkpoints are immutable.
- `checkpoint restore` rematerializes from Redis instead of replaying inverse operations.
- `workspace clone` copies a workspace into a local directory.
- `workspace fork` is the way to branch into parallel work.

## Build

Build everything:

```bash
make
```

Build just the CLI:

```bash
make cli
```

The CLI build writes `./raf` at the repo root.

Install `raf` onto your `PATH` with a symlink:

```bash
make install
```

By default this links `./raf` into `/usr/local/bin/raf`. Override the destination if needed:

```bash
make install BINDIR="$HOME/.local/bin"
```

Other build targets still exist:

```bash
make module
make mount
make clean
make uninstall
```

## Configure RAF

Run the setup wizard:

```bash
./raf setup
```

On first run, the wizard walks through Redis connection, current workspace selection, and filesystem mount setup. The filesystem mount step asks for a local mount point:

- leave it empty for no mounted filesystem
- enter a path to mount the current workspace there

When you do choose a mount point, RAF automatically picks the platform backend: `nfs` on macOS and `fuse` on Linux.

`raf setup` writes `raf.config.json` next to the `raf` binary and stores runtime state under `~/.raf`.

If you rerun `./raf setup`, it becomes an edit flow and lets you change:

- Redis connection
- current workspace
- filesystem mount

Important notes:

- If you configure RAF against an existing Redis server, RAF commands can run immediately as long as Redis is reachable.
- If you leave the mount point empty, RAF does not require a mountpoint or mount binary.
- Mounted filesystems follow the current workspace; RAF derives the internal mount backing automatically instead of asking for a separate filesystem key.
- If you choose the managed local Redis path in `raf setup`, RAF starts the managed Redis service during setup.
- Setup does not create a workspace automatically when no filesystem is mounted; the next step is usually `raf workspace create <workspace>` or `raf workspace import <workspace> <directory>`.

Legacy runtime controls still work:

```bash
./raf up
./raf down
./raf status
```

Those commands are primarily for the original mount-based workflow. RAF workspace commands do not require the mount, but they do require a reachable Redis.

## Directory Layout

RAF uses this layout on disk:

```text
raf.config.json
~/.raf/
  state.json
  workspaces/
    <workspace>/
      sessions/
        <session>/
          state.json
          tree/
      archive/
```

Key paths:

- `raf.config.json`: saved CLI configuration
- `~/.raf/state.json`: legacy runtime state for `up/down/status`
- `~/.raf/workspaces/<workspace>/sessions/main/tree/`: the current on-disk location of the primary working copy
- `~/.raf/workspaces/<workspace>/archive/`: archived pre-rollback trees

## Quick Start

Create an empty workspace:

```bash
./raf workspace create scratch
./raf workspace run scratch -- /bin/sh
```

Or import a repo into RAF:

```bash
./raf workspace import my-repo ./repo
```

That creates:

- workspace `my-repo`
- checkpoint `initial`

List it and start working in it:

```bash
./raf workspace list
./raf workspace run my-repo -- /bin/sh
./raf checkpoint list my-repo
```

Create a checkpoint before a risky change:

```bash
./raf checkpoint create my-repo before-refactor
```

Clone the workspace into a local folder:

```bash
./raf workspace clone my-repo ./my-repo-copy
```

Fork the workspace if you want a parallel line of work:

```bash
./raf workspace fork my-repo my-repo-experiment
./raf workspace run my-repo-experiment -- /bin/sh
```

Restore a checkpoint when you want to go back:

```bash
./raf checkpoint restore my-repo before-refactor
```

## Command Reference

### `raf workspace create`

```bash
./raf workspace create <workspace>
```

Behavior:

- Creates an empty workspace in Redis
- Initializes checkpoint `initial`
- Creates the default session metadata
- Does not require a mounted filesystem

### `raf workspace import`

```bash
./raf workspace import [--force] [--clone-at-source] <workspace> <directory>
```

Behavior:

- Scans the source directory
- Builds a whole-manifest snapshot
- Uploads large-file blobs to Redis
- Creates checkpoint `initial`
- Does not create a local working copy

Notes:

- `raf workspace import` honors `.rafignore` using `.gitignore`-style patterns.
- Legacy `.rfsignore` files are still accepted, but `.rafignore` is the preferred name.
- Interactive imports show a scan summary and rough time estimate before starting.
- `--force` deletes the existing RAF workspace state for that workspace before reimporting it.
- `--clone-at-source` archives the source directory to `<dir>.pre-raf` and materializes the imported workspace back at the original path.

### `raf workspace run`

```bash
./raf workspace run [workspace] [--readonly] -- <command...>
```

Behavior:

- Ensures the local working copy exists, creating it on demand if needed
- Sets the child process working directory to that tree
- Runs the command with real stdio and real OS semantics
- Emits audit events for process start and exit
- Auto-checkpoints working-copy changes back to Redis after the command exits
- With `--readonly`, skips the auto-checkpoint and leaves any local changes unsaved in the working copy

Examples:

```bash
./raf workspace run my-repo -- bash
./raf workspace run my-repo -- go test ./...
./raf workspace run -- /bin/sh -c 'make test'
```

### `raf workspace clone`

```bash
./raf workspace clone <workspace> <directory>
```

Behavior:

- Copies the workspace's current saved head into a local directory
- Refuses to overwrite a non-empty destination directory

### `raf workspace fork`

```bash
./raf workspace fork <workspace> <new-workspace>
```

Behavior:

- Creates a new workspace from the source workspace's current saved head
- Keeps parallel work at the workspace level instead of exposing branches

### `raf workspace list` and `raf workspace delete`

```bash
./raf workspace list
./raf workspace delete <workspace>...
```

Behavior:

- `list` shows known RAF workspaces from Redis
- `delete` removes workspace state from Redis and local working-copy directories

### `raf checkpoint`

```bash
./raf checkpoint list <workspace>
./raf checkpoint create <workspace> [checkpoint]
./raf checkpoint restore <workspace> <checkpoint>
```

Behavior:

- `list` shows the available checkpoints for a workspace
- `create` records the current materialized state as an immutable checkpoint
- `restore` rematerializes the workspace from the selected checkpoint

## Example Workflow

Start with one repo:

```bash
./raf workspace import my-repo ./repo
```

Start a shell in that workspace:

```bash
./raf workspace run my-repo -- bash
```

Back outside:

```bash
./raf workspace clone my-repo ./my-repo-copy
./raf checkpoint create my-repo before-refactor
./raf workspace fork my-repo my-repo-experiment
./raf workspace run my-repo-experiment -- bash
./raf checkpoint restore my-repo before-refactor
```

## Redis Storage Model

Phase 1 uses a `raf:{workspace}:...` keyspace.

Main keys:

```text
raf:{ws}:workspace:meta
raf:{ws}:workspace:sessions
raf:{ws}:workspace:savepoints
raf:{ws}:workspace:audit

raf:{ws}:session:<sid>:meta
raf:{ws}:session:<sid>:audit

raf:{ws}:savepoint:<sp>:meta
raf:{ws}:savepoint:<sp>:manifest

raf:{ws}:blob:<sha256>
raf:{ws}:blobref:<sha256>
```

Data model:

- Workspace metadata is JSON
- Session metadata is JSON
- Savepoint metadata is JSON
- Manifests are stored whole as JSON
- Small files are inlined in manifests
- Larger files are stored as content-addressed blobs by SHA-256
- Audit logs are Redis Streams

## Development and Testing

CLI tests:

```bash
cd cli
go test ./...
```

The RAF CLI tests cover:

- key helpers
- manifest hashing stability
- metadata and blob store round-trips
- local materialization
- import
- workspace inspection helpers
- workspace run using the materialized working copy
- checkpoint create and restore
- local conflict detection
- clone and archive preservation

Redis module manual testing still works separately:

```bash
make module
redis-server --loadmodule ./module/fs.so
```

There is no automated end-to-end test suite yet for the native Redis module side. That part is still tested manually with `redis-cli`.

## Limitations in Phase 1

- RAF commands require a reachable Redis server.
- There is no public diff command yet.
- `raf apply` is not implemented yet.
- Forking uses the current saved head, not unsaved local state.
- Savepoints are immutable and there is no garbage collection for blobs or old savepoints yet.
- There is no shared live multi-writer overlay mode yet.
- The new RAF workflow does not use the FUSE or NFS mount path for live workspace execution.

## Legacy Components Still in the Repo

The repository still contains:

- `module/fs.c`: Redis module implementing the original `FS.*` command family
- `mount/redis-fs-mount`: FUSE daemon
- `mount/redis-fs-nfs`: NFS daemon
- `raf up/down/status/migrate`: the original runtime and migration flow

Those pieces remain useful, but RAF is now the main workspace-oriented interface for branching, savepoints, rollback, and shell-first execution.
