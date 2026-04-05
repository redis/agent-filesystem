# Redis Agent Filesystem

Redis Agent Filesystem, or `raf`, gives agents a filesystem-shaped way to work with data, without being trapped in one machine's local disk.

Filesystems are a great interface for agents because they already know how to read files, write files, search trees, run tools, and work in directories. But ordinary local filesystems have a few problems for agent workflows:

- they are tied to one machine
- they are hard to checkpoint, fork, and restore cleanly
- they are awkward to share across agents, shells, and computers
- they do not give you a simple saved source of truth

RAF fixes that by:

- storing workspace state in Redis
- exposing the current workspace as a normal local filesystem
- materializing a real local working copy when you run commands
- letting you checkpoint and restore workspaces
- letting you fork a workspace for parallel work

If you want the short version, RAF is:

- a workspace system for agents
- backed by Redis
- with real directories for real tools

## 60-Second Quick Start

Build the CLI:

```bash
make cli
```

Run setup:

```bash
./raf setup
```

The default path is to mount a workspace and use it like a normal folder.

During setup:

- choose your Redis connection
- choose a workspace name
- choose a local mountpoint like `~/raf`

When setup finishes, RAF mounts that workspace for you. Then you can just use the folder:

```bash
cd ~/raf
ls
echo "# Notes" > notes.md
```

If you want to remount it later:

```bash
./raf up
./raf status
./raf down
```

If you want to bring an existing folder into RAF:

```bash
./raf workspace import my-repo ./repo
./raf workspace use my-repo
./raf up
```

If you want to save a known-good point:

```bash
./raf checkpoint create my-repo before-refactor
```

If you want a second line of work:

```bash
./raf workspace fork my-repo my-repo-experiment
```

## The Basic Model

RAF has two main concepts:

- `workspace`: a codebase or state tree
- `checkpoint`: a saved restore point inside that workspace

Typical flow:

1. Put a workspace into RAF with `workspace create` or `workspace import`
2. Mount it and use it like a normal directory
3. Save stable moments with `checkpoint create`
4. Fork it when you want a second line of work
5. Restore a checkpoint if you want to go back

## Mounted Filesystem

The simplest way to think about RAF is:

- Redis stores the workspace state
- RAF mounts the current workspace into a local folder
- your editor, shell, and tools use that folder like any other directory

The main mount commands are:

```bash
./raf up
./raf status
./raf down
```

On macOS RAF uses NFS. On Linux RAF uses FUSE.

If no workspace exists yet, setup will ask for one and create it before mounting.

## Most Useful Commands

```bash
./raf setup
./raf workspace create <workspace>
./raf workspace import <workspace> <directory>
./raf workspace list
./raf workspace use <workspace>
./raf workspace run <workspace> -- <command...>
./raf workspace clone <workspace> <directory>
./raf workspace fork <workspace> <new-workspace>
./raf checkpoint create <workspace> <name>
./raf checkpoint list <workspace>
./raf checkpoint restore <workspace> <name>
./raf up
./raf status
./raf down
```

For command help:

```bash
./raf --help
./raf workspace --help
./raf checkpoint --help
```

## What Gets Stored Where

- Redis stores the saved workspace state
- your chosen mountpoint is the live local folder in mounted filesystem mode
- RAF creates local working copies under `~/.raf/workspaces` only for `workspace run`
- `raf.config.json` stores local CLI configuration next to the `raf` binary

You can think of Redis as the saved source of truth.

In mounted filesystem mode:

- you work in your chosen mountpoint
- you can mostly ignore `~/.raf/workspaces`

In `workspace run` mode:

- RAF materializes a local working copy under `~/.raf/workspaces`
- your command runs there
- RAF saves changes back to Redis when the command exits

## Build

Build everything:

```bash
make
```

Build just the CLI:

```bash
make cli
```

Install `raf` onto your `PATH`:

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

If you do not want a mounted filesystem, RAF also has a command-oriented mode:

```bash
./raf workspace run my-repo -- bash
```

This is useful when:

- you want to run one command and save the results back automatically
- you are driving RAF from another agent or script
- you do not want to keep a mounted directory around
- you want a more explicit “open, run, save” workflow

`workspace run`:

- makes sure the workspace exists locally
- runs your command with that workspace as the current working directory
- captures any changes and saves them back to Redis when the command exits

For example:

```bash
./raf workspace run my-repo -- go test ./...
```

## Repo Contents

This repo includes:

- the `raf` CLI
- the Redis module
- mount daemons for local filesystem access
- Python and MCP integrations

But if you are brand new here, start with:

```bash
make cli
./raf setup
cd <your-mountpoint>
```
