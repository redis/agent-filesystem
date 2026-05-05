import assert from "node:assert/strict";
import test from "node:test";
import { AFS, MountedFS, _testing } from "../dist/index.js";

test("normalizes MCP endpoints", () => {
  assert.equal(_testing.normalizeMCPEndpoint("https://afs.cloud"), "https://afs.cloud/mcp");
  assert.equal(_testing.normalizeMCPEndpoint("https://afs.cloud/mcp"), "https://afs.cloud/mcp");
});

test("workspace.create calls the control-plane MCP tool", async () => {
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

  const workspace = await afs.workspace.create({ name: "foobar" });

  assert.equal(workspace.name, "foobar");
  assert.equal(calls[0].params.name, "workspace_create");
});

test("single-workspace mounts allow workspace-relative paths", async () => {
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
  assert.deepEqual(fs.workspaceNames, ["foobar"]);
});

test("checkpoint.create and checkpoint.restore round-trip through MCP", async () => {
  const calls = [];
  const afs = new AFS({
    apiKey: "test",
    baseUrl: "https://afs.cloud",
    fetch: async (_url, init) => {
      const body = JSON.parse(String(init.body));
      calls.push(body);
      let structuredContent;
      if (body.params.name === "checkpoint_create") {
        structuredContent = {
          workspace: body.params.arguments.workspace,
          checkpoint: body.params.arguments.checkpoint,
          created: true,
        };
      } else if (body.params.name === "checkpoint_restore") {
        structuredContent = {
          workspace: body.params.arguments.workspace,
          checkpoint: body.params.arguments.checkpoint,
          restored: true,
        };
      } else {
        throw new Error(`unexpected tool ${body.params.name}`);
      }
      return new Response(
        JSON.stringify({
          jsonrpc: "2.0",
          id: body.id,
          result: { structuredContent },
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    },
  });

  const created = await afs.checkpoint.create({ workspace: "repo", checkpoint: "unchanged-head" });
  const restored = await afs.checkpoint.restore({ workspace: "repo", checkpoint: "unchanged-head" });

  assert.equal(created.created, true);
  assert.equal(created.checkpoint, "unchanged-head");
  assert.equal(restored.restored, true);
  assert.equal(restored.checkpoint, "unchanged-head");
  assert.deepEqual(
    calls.map((call) => call.params.name),
    ["checkpoint_create", "checkpoint_restore"],
  );
});
