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

export const agentBootstrapPrompt = `I want you to use Agent Filesystem (AFS) — a persistent, checkpointed workspace backed by Redis.

Please do the following:

1. Install the AFS CLI:
   curl -fsSL https://afs.cloud/install.sh | bash

2. Authenticate (this will open a browser):
   afs auth login

3. Mount the "getting-started" workspace locally:
   afs ws mount getting-started ~/afs/getting-started

4. From now on, treat ~/afs/getting-started as your persistent home for files, notes, and project state. Use \`afs cp create\` to checkpoint before risky changes.

Confirm when you're connected and tell me what's in the workspace.`;

export const agentMcpPrompt = `Connect to Agent Filesystem (AFS) via MCP.

Add this server to your MCP configuration:

{
  "mcpServers": {
    "agent-filesystem": {
      "url": "https://afs.cloud/mcp",
      "headers": { "Authorization": "Bearer <YOUR_TOKEN>" }
    }
  }
}

Once connected, call list_workspaces, then read the contents of the "getting-started" workspace and summarize what's there.`;
