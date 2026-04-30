# `redis-afs`

Python SDK for creating AFS workspaces, mounting them in-process, reading and
writing files, checkpointing work, and running shell commands against an
isolated AFS-backed workspace.

## Install

```bash
pip install redis-afs
```

## Quick Start

```python
import os
from redis_afs import AFS

afs = AFS(api_key=os.environ["AFS_API_KEY"])
workspace = afs.workspace.create(name="foobar")

fs = afs.fs.mount(
    workspaces=[{"name": workspace["name"]}],
    mode="rw",
)

try:
    fs.write_file("/src/README.md", "hello world")
    result = fs.bash().exec("cat /foobar/src/README.md")
    print(result.stdout)
finally:
    fs.close()
```

`MountedFS` also works as a context manager:

```python
with afs.fs.mount(workspaces=[{"name": "foobar"}], mode="rw") as fs:
    fs.write_file("/README.md", "hello")
```

## Authentication

```bash
export AFS_API_KEY="afs_..."
```

Set `AFS_API_BASE_URL` to target a local or Self-managed control plane. If not
provided, the SDK defaults to `https://afs.cloud`.

## API Reference

See [api-docs.md](api-docs.md) for the full Python API surface, including
workspace management, checkpoints, mount semantics, file operations, shell
execution, low-level MCP access, and current limitations.

## Test

From `sdk/python`:

```bash
PYTHONPATH=src python3 -m unittest discover -s tests
```

From the project root:

```bash
PYTHONPATH=sdk/python/src python3 -m unittest discover -s sdk/python/tests
```
