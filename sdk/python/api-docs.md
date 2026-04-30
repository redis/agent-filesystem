# `redis-afs` Python SDK Reference

Create Redis-backed AFS workspaces, mount them in-process, edit files, search
trees, checkpoint state, and run shell commands from Python.

## Syntax

```python
from redis_afs import AFS

afs = AFS(
    api_key=None,
    base_url=None,
    timeout=30.0,
    headers=None,
)
```

## API Methods

```python
afs.workspace.create(...)       # create a workspace
afs.workspace.list()            # list workspaces
afs.workspace.get(workspace)    # get one workspace
afs.workspace.fork(...)         # fork a workspace
afs.workspace.delete(workspace) # delete a workspace

afs.checkpoint.list(workspace)  # list checkpoints
afs.checkpoint.create(...)      # create a checkpoint
afs.checkpoint.restore(...)     # restore a checkpoint

afs.fs.mount(...)               # create an isolated SDK mount
fs.read_file(path)              # read a text file
fs.write_file(path, content)    # write a text file
fs.list_files(path, depth)      # list a directory
fs.glob(pattern, ...)           # match paths
fs.grep(pattern, **options)     # search file contents
fs.checkpoint(name)             # checkpoint mounted workspaces
fs.bash().exec(command, ...)    # run a shell command against the mount
```

## At A Glance

| Field | Value |
| --- | --- |
| Package | `redis-afs` |
| Import | `from redis_afs import AFS` |
| Runtime | Python 3.10 or newer |
| Transport | MCP over HTTP |
| Default endpoint | `https://afs.cloud/mcp` |
| Auth | `AFS_API_KEY` or `AFS(api_key=...)` |
| Self-managed endpoint | `AFS_API_BASE_URL` or `AFS(base_url=...)` |
| Available since | `redis-afs` `0.1.0` |

## Examples

Create a workspace, write a file, run a command, and clean up the temporary
mount directory.

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

Use the environment for credentials and endpoint selection.

```bash
export AFS_API_KEY="afs_..."
export AFS_API_BASE_URL="https://afs.cloud"
```

## `AFS(...)`

Creates a control-plane client. The constructor validates that an API key is
available and normalizes the endpoint to `/mcp`.

### Syntax

```python
afs = AFS(
    api_key: str | None = None,
    base_url: str | None = None,
    timeout: float = 30.0,
    headers: Mapping[str, str] | None = None,
)
```

### Options

| Option | Default | Description |
| --- | --- | --- |
| `api_key` | `os.environ["AFS_API_KEY"]` | Bearer token for AFS Cloud or a Self-managed control plane. |
| `base_url` | `os.environ["AFS_API_BASE_URL"]` or `https://afs.cloud` | Control-plane base URL or direct `/mcp` endpoint. |
| `timeout` | `30.0` | HTTP request timeout in seconds. |
| `headers` | `{}` | Extra headers sent with each MCP request. |

### Clients

```python
afs.workspace
afs.workspaces
afs.checkpoint
afs.checkpoints
afs.fs
```

Compatibility aliases from the first SDK preview remain available:

```python
afs.repo
afs.repos
```

## Workspace API

Create, list, inspect, fork, and delete workspaces. A workspace is the
Redis-backed file tree agents and tools work against.

### Syntax

```python
afs.workspace.create(
    *,
    name: str,
    description: str | None = None,
    template_slug: str | None = None,
) -> dict[str, Any]
```

```python
afs.workspace.list() -> list[dict[str, Any]]
afs.workspace.get(workspace: str | Mapping[str, Any]) -> dict[str, Any]
afs.workspace.fork(*, source: str, name: str) -> dict[str, Any]
afs.workspace.delete(workspace: str | Mapping[str, Any]) -> dict[str, Any]
```

`get()` and `delete()` also accept the preview keyword alias `repo=...`, but
new code should use `workspace`.

### Return Value

Workspace dictionaries include at least `name`, and may include
server-provided fields:

```python
{
    "id": "...",
    "name": "foobar",
    "description": "...",
    "database_id": "...",
    "database_name": "...",
    "template_slug": "...",
}
```

### Example

```python
workspace = afs.workspace.create(
    name="review-run",
    description="Scratch space for the review agent",
)

fork = afs.workspace.fork(
    source=workspace["name"],
    name="review-run-alt",
)
```

## Checkpoint API

Checkpoints are explicit saved moments in the workspace timeline. File edits
change live state; they do not create checkpoints automatically.

### Syntax

```python
afs.checkpoint.list(workspace: str | Mapping[str, Any]) -> list[dict[str, Any]]
```

```python
afs.checkpoint.create(
    *,
    workspace: str | None = None,
    checkpoint: str | None = None,
) -> dict[str, Any]
```

```python
afs.checkpoint.restore(
    *,
    workspace: str | None = None,
    checkpoint: str,
) -> dict[str, Any]
```

The preview `repo` keyword is still accepted by `create()` and `restore()`, but
new code should use `workspace`.

### Return Value

Checkpoint dictionaries include server fields such as:

```python
{
    "id": "...",
    "name": "before-fix",
    "created_at": "...",
    "file_count": 10,
    "folder_count": 4,
    "total_bytes": 12345,
    "is_head": False,
}
```

### Example

```python
afs.checkpoint.create(
    workspace="review-run",
    checkpoint="before-fix",
)

afs.checkpoint.restore(
    workspace="review-run",
    checkpoint="before-fix",
)
```

## Mount API

`afs.fs.mount()` issues workspace-scoped MCP tokens and returns an isolated SDK
mount. This is not a kernel FUSE or NFS mount.

### Syntax

```python
fs = afs.fs.mount(
    workspaces=[{"name": "foobar"}],
    mode="rw",
    token_name="optional token label",
)
```

```python
workspaces: Sequence[Mapping[str, Any]]
mode: "ro" | "rw" | "rw-checkpoint"
token_name: str | None
```

The preview `repos` mount option still works, but new code should use
`workspaces`.

### Modes

| Mode | Token profile | Use |
| --- | --- | --- |
| `ro` | `workspace-ro` | Read-only workspace access. |
| `rw` | `workspace-rw` | Read and write workspace access. |
| `rw-checkpoint` | `workspace-rw-checkpoint` | Read/write access with checkpoint permission. |

### Path Rules

- With one mounted workspace, `/src/file.py` is workspace-relative.
- With multiple mounted workspaces, paths must start with the workspace name,
  such as `/api/app.py` or `/web/package.json`.
- Paths are normalized as POSIX paths and cannot contain `..`.

## MountedFS API

Use `MountedFS` for workspace file operations, search, local materialization,
and shell execution.

### Properties

```python
fs.workspace_names -> list[str]
fs.local_root -> str | None
fs.mode -> str
```

The preview `repo_names` property still works.

### File Methods

```python
fs.read_file(path: str) -> str
fs.write_file(path: str, content: str | bytes) -> None
fs.list_files(path: str = "/", depth: int = 1) -> list[dict[str, Any]]
```

`read_file()` returns text. It raises if the path is a directory or if the
control plane marks the file as binary. `write_file()` sends text content
through the workspace MCP token.

### Search Methods

```python
fs.glob(
    pattern: str,
    *,
    path: str = "/",
    kind: str | None = None,
    limit: int | None = None,
) -> dict[str, Any]
```

```python
fs.grep(pattern: str, **options: Any) -> dict[str, Any]
```

`glob()` matches workspace paths. `grep()` searches file contents and forwards
extra options to the hosted `file_grep` MCP tool.

### Checkpoint Method

```python
fs.checkpoint(name: str | None = None) -> list[dict[str, Any]]
```

Creates a checkpoint for each mounted workspace.

### Local Materialization

```python
fs.sync_from_remote() -> str
fs.sync_to_remote() -> None
fs.map_absolute_workspace_paths(command: str) -> str
fs.close() -> None
```

`sync_from_remote()` creates a temporary local directory and downloads mounted
workspaces into it. `sync_to_remote()` writes created and modified local text
files back through MCP. `close()` removes the temporary local directory.

The preview `map_absolute_repo_paths()` helper still works, but new code should
use `map_absolute_workspace_paths()`.

## Bash API

Run shell commands against the mounted workspace. The command executes with
`/bin/bash` after the workspace has been materialized into a temporary local
directory.

### Syntax

```python
result = fs.bash().exec(
    command: str,
    *,
    cwd: str | None = None,
    env: Mapping[str, str | None] | None = None,
    timeout: float | None = None,
    check: bool = False,
)
```

### Return Value

```python
@dataclass(frozen=True)
class BashResult:
    stdout: str
    stderr: str
    exit_code: int
    command: str
    mapped_command: str
```

### Behavior

`bash().exec()` materializes workspaces, rewrites absolute workspace paths such
as `/foobar/src/README.md` to the isolated local directory, runs the command,
then syncs created and modified text files back to AFS.

Nonzero exit codes are returned in `exit_code`. Pass `check=True` to raise
`AFSError` on nonzero exit.

### Example

```python
result = fs.bash().exec(
    "python -m pytest",
    cwd="foobar",
    timeout=120.0,
)

if result.exit_code != 0:
    print(result.stderr)
```

## Low-Level MCP Client

Use `MCPHttpClient` when you need direct MCP access. Most applications should
prefer `AFS`.

### Syntax

```python
from redis_afs import MCPHttpClient

client = MCPHttpClient(api_key="afs_...")
client.call_tool("workspace_list")
client.request("tools/call", {
    "name": "workspace_list",
    "arguments": {},
})
```

## Errors

The SDK raises `AFSError` for API, validation, binary-read, and command wrapper
errors.

```python
error.status  # HTTP status, when available
error.code    # JSON-RPC error code, when available
error.payload # raw server or command payload, when available
```

## Compatibility

The first SDK preview used `repo` language. These aliases remain available, but
new code should use `workspace` language:

```python
afs.repo
afs.repos
fs.repo_names
fs.map_absolute_repo_paths(command)
```

## Current Limits

- `read_file()` returns text only. Binary file reads raise.
- `write_file()` treats `bytes` content as UTF-8 text.
- `sync_to_remote()` writes created and modified text files, but does not
  currently propagate local file deletion.
- `MountedFS` does not yet expose `mkdir`, `rename`, `delete`, `stat`, streams,
  or binary file helpers.
