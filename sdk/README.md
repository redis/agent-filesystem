# AFS SDKs

This directory contains first-pass agent SDKs for the AFS control plane:

- `typescript/` publishes the `@redis/afs-sdk` package.
- `python/` publishes the `redis-afs-sdk` package with the `redis_afs` import.

Both SDKs use the hosted MCP endpoint as the stable agent-facing transport:

- control-plane tokens call workspace lifecycle tools such as `workspace_create`
  and `mcp_token_issue`;
- mounted filesystem objects use workspace-scoped MCP tokens for file reads,
  writes, searches, and checkpoints.

## Authentication

Set `AFS_API_KEY` in the process environment or pass the key directly to the
client constructor. Set `AFS_API_BASE_URL` only when targeting a local or
Self-managed control plane; otherwise the SDKs use `https://afs.cloud`.

## TypeScript

```bash
npm install @redis/afs-sdk
```

```ts
import { AFS } from "@redis/afs-sdk";

const afs = new AFS({ apiKey: process.env.AFS_API_KEY });
const repo = await afs.repo.create({ name: "foobar" });

const fs = await afs.fs.mount({
  repos: [{ name: repo.name }],
  mode: "rw",
});

await fs.writeFile("/src/README.md", "hello world");
const result = await fs.bash().exec("cat /foobar/src/README.md");
console.log(result.stdout);
await fs.close();
```

## Python

```bash
pip install redis-afs-sdk
```

```python
import os
from redis_afs import AFS

afs = AFS(api_key=os.environ["AFS_API_KEY"])
repo = afs.repo.create(name="foobar")

fs = afs.fs.mount(
    repos=[{"name": repo["name"]}],
    mode="rw",
)

fs.write_file("/src/README.md", "hello world")
result = fs.bash().exec("cat /foobar/src/README.md")
print(result.stdout)
fs.close()
```

## Mount Semantics

`fs.mount()` creates an isolated SDK mount, not a kernel FUSE/NFS mount. For
one mounted repo, `/path/to/file` is treated as repo-relative. For multiple
repos, use `/<repo-name>/path/to/file`.

`bash().exec()` materializes each repo into a temporary local directory, rewrites
absolute repo paths such as `/foobar/src/README.md` to that isolated directory,
runs the shell command, then writes created and modified files back through MCP.
The current alpha sync path supports file create/update; remote file deletion is
waiting on a dedicated hosted file-delete API.

## Test Locally

```bash
npm --prefix sdk/typescript run check
npm --prefix sdk/typescript test
```

```bash
PYTHONPATH=sdk/python/src python3 -m unittest discover -s sdk/python/tests
```

## Publish

Those install commands work from a clean machine only after publishing the
packages to their registries:

```bash
cd sdk/typescript
npm publish --access public
```

```bash
cd sdk/python
python3 -m build
python3 -m twine upload dist/*
```

Before publishing, install from the local checkout:

```bash
npm install ./sdk/typescript
python3 -m pip install ./sdk/python
```
