# `redis-afs`

TypeScript SDK for creating AFS workspaces, mounting them in-process, reading
and writing files, checkpointing work, and running shell commands against an
isolated AFS-backed workspace.

## Install

```bash
npm install redis-afs
```

## Quick Start

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

## Authentication

```bash
export AFS_API_KEY="afs_..."
```

Set `AFS_API_BASE_URL` to target a local or Self-managed control plane. If not
provided, the SDK defaults to `https://afs.cloud`.

## API Reference

See [api-docs.md](api-docs.md) for the full TypeScript API surface, including
workspace management, checkpoints, mount semantics, file operations, shell
execution, low-level MCP access, and current limitations.

## Test

From `sdk/typescript`:

```bash
npm install
npm run check
npm test
```

From the project root:

```bash
npm --prefix sdk/typescript install
npm --prefix sdk/typescript run check
npm --prefix sdk/typescript test
```
