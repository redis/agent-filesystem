# Redis Agent Filesystem

Redis Agent Filesystem, or `raf`, gives agents a filesystem-shaped way to work with data, without being trapped in one machine's local disk.

Filesystems are a great interface for agents because they already know how to read files, write files, search trees, run tools, and work in directories. But ordinary local filesystems have a few problems for agent workflows:

- they are tied to one machine
- they are hard to checkpoint, fork, and restore cleanly
- they are awkward to share across agents, shells, and computers
- they do not give you a simple saved source of truth

RAF fixes that by:

- storing workspace state in Redis
- materializing a real local working copy when you run commands
- letting you checkpoint and restore workspaces
- letting you fork a workspace for parallel work
- optionally mounting the current workspace as a normal local filesystem

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

Then do one of these:

Create an empty workspace:

```bash
./raf workspace create scratch
./raf workspace run scratch -- bash
```

Import an existing folder:

```bash
./raf workspace import my-repo ./repo
./raf workspace run my-repo -- bash
```

Create a checkpoint before a risky change:

```bash
./raf checkpoint create my-repo before-refactor
```

Restore that checkpoint later:

```bash
./raf checkpoint restore my-repo before-refactor
```

Fork a workspace for parallel work:

```bash
./raf workspace fork my-repo my-repo-experiment
./raf workspace run my-repo-experiment -- bash
```

Clone a workspace into a normal local directory:

```bash
./raf workspace clone my-repo ./my-repo-copy
```

## The Basic Model

RAF has two main concepts:

- `workspace`: a codebase or state tree
- `checkpoint`: a saved restore point inside that workspace

Typical flow:

1. Put a workspace into RAF with `workspace create` or `workspace import`
2. Work inside it with `workspace run`
3. Save stable moments with `checkpoint create`
4. Fork it when you want a second line of work
5. Restore a checkpoint if you want to go back

## How `workspace run` Works

`raf workspace run` is the heart of the tool.

It:

- makes sure the workspace exists locally
- runs your command with that workspace as the current working directory
- captures any changes and saves them back to Redis when the command exits

So this:

```bash
./raf workspace run my-repo -- go test ./...
```

means:

- open the workspace locally
- run `go test ./...` inside it
- save any file changes back to the workspace state

## Optional Filesystem Mount

Some tools want a stable mounted directory instead of a one-shot command run.

RAF can do that too.

During `./raf setup`, you can:

- leave the mountpoint empty and just use workspace commands
- choose a mountpoint and RAF will mount the current workspace there

If no workspace is selected yet, setup will ask for one and create it before mounting.

Then you can use:

```bash
./raf up
./raf status
./raf down
```

On macOS RAF uses NFS. On Linux RAF uses FUSE.

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
- RAF creates local working copies under `~/.raf/workspaces`
- `raf.config.json` stores local CLI configuration next to the `raf` binary

You can think of Redis as the saved source of truth, and the local working copy as the place where real tools run.

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
./raf workspace import my-repo ./repo
./raf workspace run my-repo -- bash
```
