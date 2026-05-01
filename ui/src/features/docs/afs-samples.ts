export const typescriptSdkSample = `import { AFS } from "redis-afs";

const afs = new AFS({ apiKey: process.env.AFS_API_KEY });
const workspace = await afs.workspace.create({ name: "foobar" });
const fs = await afs.fs.mount({
  workspaces: [{ name: workspace.name }],
  mode: "rw",
});

await fs.writeFile("/src/README.md", "hello world");
const result = await fs.bash().exec("cat /foobar/src/README.md");
console.log(result.stdout);
await fs.close();`;

export const pythonSdkSample = `import os
from redis_afs import AFS

afs = AFS(api_key=os.environ["AFS_API_KEY"])
workspace = afs.workspace.create(name="foobar")
fs = afs.fs.mount(
    workspaces=[{"name": workspace["name"]}],
    mode="rw",
)

fs.write_file("/src/README.md", "hello world")
result = fs.bash().exec("cat /foobar/src/README.md")
print(result.stdout)
fs.close()`;

export const cliGettingStartedSample = `afs auth login
afs ws create foobar
afs ws mount foobar ~/afs/foobar

echo "hello world" > ~/afs/foobar/src/README.md
cat ~/afs/foobar/src/README.md
afs cp create foobar first-readme`;

export const mcpGettingStartedSample = `{
  "mcpServers": {
    "agent-filesystem": {
      "url": "https://afs.cloud/mcp",
      "headers": {
        "Authorization": "Bearer \${AFS_TOKEN}"
      }
    }
  }
}`;
