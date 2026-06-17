# `redis-afs`

Python SDK for creating AFS workspaces, mounting them in-process, reading and
writing files, checkpointing work, and running shell commands against an
isolated AFS-backed workspace.

## Install

```bash
pip install redis-afs
```

For the async client, install the optional `async` extra (installs `httpx`):

```bash
pip install 'redis-afs[async]'
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

## Async usage

The async client mirrors the sync surface, but every network call is a
coroutine. Install the `async` extra (`pip install 'redis-afs[async]'`) and
import `AsyncAFS`.

```python
import os
from redis_afs import AsyncAFS

async def main():
    async with AsyncAFS(api_key=os.environ["AFS_API_KEY"]) as afs:
        workspace = await afs.workspace.create(name="foobar")
        async with await afs.fs.mount(
            workspaces=[{"name": workspace["name"]}],
            mode="rw",
        ) as fs:
            await fs.write_file("/src/README.md", "hello world")
            result = await fs.bash().exec("cat /foobar/src/README.md")
            print(result.stdout)
```

Note that `afs.fs.mount(...)` is itself a coroutine, so the mounted filesystem
is entered with `async with await afs.fs.mount(...)`.

The async client matters most inside an async server. A blocking HTTP call in
an `async` handler stalls the event loop and blocks every other request; the
async client `await`s its network I/O instead, so the loop stays free.

```python
from fastapi import FastAPI
from redis_afs import AsyncAFS

app = FastAPI()

@app.get("/readme/{workspace}")
async def read_readme(workspace: str):
    async with AsyncAFS() as afs:
        async with await afs.fs.mount(workspaces=[{"name": workspace}], mode="ro") as fs:
            return {"content": await fs.read_file(f"/{workspace}/README.md")}
```

Prefer `async with` (or call `await afs.aclose()` and `await fs.aclose()`) so
the underlying `httpx` connection pools are closed.

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
