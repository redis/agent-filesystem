# `redis-afs` TypeScript SDK Reference

Create Redis-backed AFS workspaces, mount them in-process, edit files, search
trees, checkpoint state, and run shell commands from Node.js.

## Syntax

```ts
import { AFS } from "redis-afs";

const afs = new AFS(options);
```

## API Methods

```ts
afs.workspace.create(input)        // create a workspace
afs.workspace.list()               // list workspaces
afs.workspace.get(workspace)       // get one workspace
afs.workspace.fork(input)          // fork a workspace
afs.workspace.delete(workspace)    // delete a workspace

afs.checkpoint.list(workspace)     // list checkpoints
afs.checkpoint.create(input)       // create a checkpoint
afs.checkpoint.restore(input)      // restore a checkpoint

afs.fs.mount(input)                // create an isolated SDK mount
fs.readFile(path)                  // read a text file
fs.writeFile(path, content)        // write a text file
fs.listFiles(path, depth)          // list a directory
fs.glob(pattern, options)          // match paths
fs.grep(pattern, options)          // search file contents
fs.checkpoint(name)                // checkpoint mounted workspaces
fs.bash().exec(command, options)   // run a shell command against the mount
```

## At A Glance

| Field | Value |
| --- | --- |
| Package | `redis-afs` |
| Import | `import { AFS } from "redis-afs"` |
| Runtime | Node.js 18 or newer |
| Transport | MCP over HTTP |
| Default endpoint | `https://afs.cloud/mcp` |
| Auth | `AFS_API_KEY` or `new AFS({ apiKey })` |
| Self-managed endpoint | `AFS_API_BASE_URL` or `new AFS({ baseUrl })` |
| Available since | `redis-afs` `0.1.0` |

## Examples

Create a workspace, write a file, run a command, and clean up the temporary
mount directory.

```ts
import { AFS } from "redis-afs";

const afs = new AFS({ apiKey: process.env.AFS_API_KEY });
const workspace = await afs.workspace.create({ name: "foobar" });

const fs = await afs.fs.mount({
  workspaces: [{ name: workspace.name }],
  mode: "rw",
});

try {
  await fs.writeFile("/src/README.md", "hello world");

  const result = await fs.bash().exec("cat /foobar/src/README.md");
  console.log(result.stdout);
} finally {
  await fs.close();
}
```

Use the environment for credentials and endpoint selection.

```bash
export AFS_API_KEY="afs_..."
export AFS_API_BASE_URL="https://afs.cloud"
```

## `new AFS(options)`

Creates a control-plane client. The constructor validates that an API key is
available and normalizes the endpoint to `/mcp`.

### Syntax

```ts
const afs = new AFS({
  apiKey?: string;
  baseUrl?: string;
  fetch?: FetchLike;
  timeoutMs?: number;
  headers?: Record<string, string>;
});
```

### Options

| Option | Default | Description |
| --- | --- | --- |
| `apiKey` | `process.env.AFS_API_KEY` | Bearer token for AFS Cloud or a Self-managed control plane. |
| `baseUrl` | `process.env.AFS_API_BASE_URL` or `https://afs.cloud` | Control-plane base URL or direct `/mcp` endpoint. |
| `fetch` | `globalThis.fetch` | Fetch implementation. Required when the runtime does not provide global fetch. |
| `timeoutMs` | `30000` | HTTP request timeout in milliseconds. |
| `headers` | `{}` | Extra headers sent with each MCP request. |

### Clients

```ts
afs.workspace
afs.workspaces
afs.checkpoint
afs.checkpoints
afs.fs
```

Compatibility aliases from the first SDK preview remain available:

```ts
afs.repo
afs.repos
```

## Workspace API

Create, list, inspect, fork, and delete workspaces. A workspace is the
Redis-backed file tree agents and tools work against.

### Syntax

```ts
await afs.workspace.create({
  name: string;
  description?: string;
  templateSlug?: string;
}): Promise<Workspace>
```

```ts
await afs.workspace.list(): Promise<Workspace[]>
await afs.workspace.get(workspace: string | { name: string }): Promise<Workspace>
await afs.workspace.fork({ source: string, name: string }): Promise<{
  source: string;
  workspace: string;
  created: boolean;
}>
await afs.workspace.delete(workspace: string | { name: string }): Promise<{
  workspace: string;
  deleted: boolean;
}>
```

### Return Value

`Workspace` includes at least `name`, and may include server-provided fields:

```ts
type Workspace = {
  id?: string;
  name: string;
  description?: string;
  database_id?: string;
  database_name?: string;
  template_slug?: string;
  [key: string]: unknown;
};
```

### Example

```ts
const workspace = await afs.workspace.create({
  name: "review-run",
  description: "Scratch space for the review agent",
});

const fork = await afs.workspace.fork({
  source: workspace.name,
  name: "review-run-alt",
});
```

## Checkpoint API

Checkpoints are explicit saved moments in the workspace timeline. File edits
change live state; they do not create checkpoints automatically.

### Syntax

```ts
await afs.checkpoint.list(workspace: string | { name: string }): Promise<Checkpoint[]>
```

```ts
await afs.checkpoint.create({
  workspace: string;
  checkpoint?: string;
}): Promise<{ workspace: string; checkpoint: string; created: boolean }>
```

```ts
await afs.checkpoint.restore({
  workspace: string;
  checkpoint: string;
}): Promise<{ workspace: string; checkpoint: string; restored: boolean }>
```

The preview `repo` field is still accepted by `create()` and `restore()`, but
new code should use `workspace`.

### Return Value

```ts
type Checkpoint = {
  id: string;
  name: string;
  created_at?: string;
  file_count?: number;
  folder_count?: number;
  total_bytes?: number;
  is_head?: boolean;
  [key: string]: unknown;
};
```

### Example

```ts
await afs.checkpoint.create({
  workspace: "review-run",
  checkpoint: "before-fix",
});

await afs.checkpoint.restore({
  workspace: "review-run",
  checkpoint: "before-fix",
});
```

## Mount API

`afs.fs.mount()` issues workspace-scoped MCP tokens and returns an isolated SDK
mount. This is not a kernel FUSE or NFS mount.

### Syntax

```ts
const fs = await afs.fs.mount({
  workspaces: [{ name: "foobar" }],
  mode: "rw",
  tokenName: "optional token label",
});
```

```ts
type MountInput = {
  workspaces?: Array<{ name: string }>;
  mode?: "ro" | "rw" | "rw-checkpoint";
  tokenName?: string;
};
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

- With one mounted workspace, `/src/file.ts` is workspace-relative.
- With multiple mounted workspaces, paths must start with the workspace name,
  such as `/api/src/file.ts` or `/web/package.json`.
- Paths are normalized as POSIX paths and cannot contain `..`.

## MountedFS API

Use `MountedFS` for workspace file operations, search, local materialization,
and shell execution.

### Properties

```ts
fs.workspaceNames: string[]
fs.localRoot: string | undefined
```

The preview `repoNames` property still works.

### File Methods

```ts
await fs.readFile(path: string): Promise<string>
await fs.writeFile(path: string, content: string | Uint8Array): Promise<void>
await fs.listFiles(path = "/", depth = 1): Promise<FileListItem[]>
```

`readFile()` returns text. It throws if the path is a directory or if the
control plane marks the file as binary. `writeFile()` sends text content through
the workspace MCP token.

```ts
type FileListItem = {
  path: string;
  name: string;
  kind: "file" | "dir" | "symlink" | string;
  size?: number;
  modified_at?: string;
  target?: string;
};
```

### Search Methods

```ts
await fs.glob(pattern: string, options?: {
  path?: string;
  kind?: "file" | "dir" | "symlink" | "any";
  limit?: number;
}): Promise<unknown>
```

```ts
await fs.grep(pattern: string, options?: Record<string, unknown>): Promise<unknown>
```

`glob()` matches workspace paths. `grep()` searches file contents and forwards
extra options to the hosted `file_grep` MCP tool.

### Checkpoint Method

```ts
await fs.checkpoint(name?: string): Promise<Array<{
  workspace: string;
  checkpoint: string;
  created: boolean;
}>>
```

Creates a checkpoint for each mounted workspace.

### Local Materialization

```ts
await fs.syncFromRemote(): Promise<string>
await fs.syncToRemote(): Promise<void>
fs.mapAbsoluteWorkspacePaths(command: string): string
await fs.close(): Promise<void>
```

`syncFromRemote()` creates a temporary local directory and downloads mounted
workspaces into it. `syncToRemote()` writes created and modified local text
files back through MCP. `close()` removes the temporary local directory.

The preview `mapAbsoluteRepoPaths()` helper still works, but new code should
use `mapAbsoluteWorkspacePaths()`.

## Bash API

Run shell commands against the mounted workspace. The command executes with
`/bin/bash` after the workspace has been materialized into a temporary local
directory.

### Syntax

```ts
const result = await fs.bash().exec(command, options?);
```

```ts
type BashExecOptions = {
  cwd?: string;
  env?: Record<string, string | undefined>;
  timeoutMs?: number;
};
```

### Return Value

```ts
type BashResult = {
  stdout: string;
  stderr: string;
  exitCode: number | null;
  signal: NodeJS.Signals | null;
  command: string;
  mappedCommand: string;
};
```

### Behavior

`bash().exec()` materializes workspaces, rewrites absolute workspace paths such
as `/foobar/src/README.md` to the isolated local directory, runs the command,
then syncs created and modified text files back to AFS.

Nonzero exit codes are returned in `exitCode`. They do not throw by themselves.

### Example

```ts
const result = await fs.bash().exec("npm test", {
  cwd: "foobar",
  timeoutMs: 120_000,
});

if (result.exitCode !== 0) {
  console.error(result.stderr);
}
```

## Low-Level MCP Client

Use `MCPHttpClient` when you need direct MCP access. Most applications should
prefer `AFS`.

### Syntax

```ts
import { MCPHttpClient } from "redis-afs";

const client = new MCPHttpClient({ apiKey: "afs_..." });
await client.callTool<T>("workspace_list");
await client.request<T>("tools/call", {
  name: "workspace_list",
  arguments: {},
});
```

## Errors

The SDK throws `AFSError` for API, validation, binary-read, and command wrapper
errors.

```ts
error.status  // HTTP status, when available
error.code    // JSON-RPC error code, when available
error.payload // raw server or command payload, when available
```

## Compatibility

The first SDK preview used `repo` language. These aliases remain available, but
new code should use `workspace` language:

```ts
afs.repo
afs.repos
fs.repoNames
fs.mapAbsoluteRepoPaths(command)
```

## Current Limits

- `readFile()` returns text only. Binary file reads throw.
- `writeFile()` treats `Uint8Array` content as UTF-8 text.
- `syncToRemote()` writes created and modified text files, but does not
  currently propagate local file deletion.
- `MountedFS` does not yet expose `mkdir`, `rename`, `delete`, `stat`, streams,
  or binary file helpers.
