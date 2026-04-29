# `@redis/afs-sdk`

TypeScript SDK for the AFS control plane and agent filesystem mounts.

```bash
npm install @redis/afs-sdk
```

```ts
import { AFS } from "@redis/afs-sdk";

const afs = new AFS({ apiKey: process.env.AFS_API_KEY });
const repo = await afs.repo.create({ name: "foobar" });
const fs = await afs.fs.mount({ repos: [{ name: repo.name }], mode: "rw" });

await fs.writeFile("/src/README.md", "hello world");
const result = await fs.bash().exec("cat /foobar/src/README.md");
console.log(result.stdout);
await fs.close();
```

The API key can also be read automatically from `AFS_API_KEY`:

```bash
export AFS_API_KEY="afs_..."
```

Set `AFS_API_BASE_URL` to point at a local or Self-managed control plane. If not
provided, the SDK defaults to `https://afs.cloud`.

## Test

```bash
npm run check
npm test
```
