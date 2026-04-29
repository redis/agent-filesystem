import assert from "node:assert/strict";
import test from "node:test";
import { AFS, MountedFS, _testing } from "../dist/index.js";

test("normalizes MCP endpoints", () => {
  assert.equal(_testing.normalizeMCPEndpoint("https://afs.cloud"), "https://afs.cloud/mcp");
  assert.equal(_testing.normalizeMCPEndpoint("https://afs.cloud/mcp"), "https://afs.cloud/mcp");
});

test("repo.create calls the control-plane MCP tool", async () => {
  const calls = [];
  const afs = new AFS({
    apiKey: "test",
    baseUrl: "https://afs.cloud",
    fetch: async (_url, init) => {
      const body = JSON.parse(String(init.body));
      calls.push(body);
      return new Response(
        JSON.stringify({
          jsonrpc: "2.0",
          id: body.id,
          result: {
            structuredContent: { name: body.params.arguments.name },
          },
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    },
  });

  const repo = await afs.repo.create({ name: "foobar" });

  assert.equal(repo.name, "foobar");
  assert.equal(calls[0].params.name, "workspace_create");
});

test("single-repo mounts allow repo-relative paths", async () => {
  const files = new Map();
  const fakeClient = {
    async callTool(name, args = {}) {
      if (name === "file_write") {
        files.set(args.path, args.content);
        return { operation: "write" };
      }
      if (name === "file_read") {
        return { path: args.path, kind: "file", content: files.get(args.path) ?? "" };
      }
      throw new Error(`unexpected tool ${name}`);
    },
  };
  const fs = new MountedFS([{ name: "foobar", token: "token", client: fakeClient }], { mode: "rw" });

  await fs.writeFile("/src/README.md", "hello");

  assert.equal(files.get("/src/README.md"), "hello");
  assert.equal(await fs.readFile("/foobar/src/README.md"), "hello");
});
