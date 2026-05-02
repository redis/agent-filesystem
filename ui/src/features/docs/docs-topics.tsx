import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import styled from "styled-components";
import {
  CalloutBox,
  CmdTable,
  CodeBlock,
  CrossLinkArrow,
  CrossLinkCard,
  CrossLinkDesc,
  CrossLinkText,
  CrossLinkTitle,
  DocHeading,
  DocHero,
  DocHeroSub,
  DocHeroTitle,
  DocPage,
  DocProse,
  DocSection,
  DocSubheading,
  InlineCode,
  Step,
} from "../../components/doc-kit";
import { searchBenchmark } from "../../foundation/performance-data";
import { pythonSdkSample, typescriptSdkSample } from "./afs-samples";
import { HighlightedCode } from "./syntax-code";

const docsReferenceBaseHref = "https://github.com/redis/agent-filesystem/blob/main/docs";
const referenceDocHref = {
  cli: `${docsReferenceBaseHref}/reference/cli.md`,
  mcp: `${docsReferenceBaseHref}/reference/mcp.md`,
  python: `${docsReferenceBaseHref}/reference/python.md`,
  typescript: `${docsReferenceBaseHref}/reference/typescript.md`,
} as const;

export type DocsTopicId =
  | "how-it-works"
  | "cli"
  | "workspaces"
  | "local-files"
  | "mcp-agents"
  | "typescript-sdk"
  | "python-sdk"
  | "self-managed"
  | "performance"
  | "faq";

export type DocsTopic = {
  id: DocsTopicId;
  path:
    | "/docs/how-it-works"
    | "/docs/cli"
    | "/docs/workspaces"
    | "/docs/local-files"
    | "/docs/mcp-agents"
    | "/docs/typescript-sdk"
    | "/docs/python-sdk"
    | "/docs/self-managed"
    | "/docs/performance"
    | "/docs/faq";
  eyebrow: string;
  title: string;
  summary: string;
  sections: ReadonlyArray<{
    heading: string;
    body: ReactNode;
  }>;
  related: ReadonlyArray<DocsTopicId>;
};

const DefinitionGrid = styled.div`
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;

  @media (max-width: 720px) {
    grid-template-columns: 1fr;
  }
`;

const DefinitionItem = styled.div`
  display: grid;
  gap: 6px;
  padding: 14px;
  border: 1px solid var(--afs-line, #e6e6e6);
  border-radius: 8px;
  background: var(--afs-panel, #ffffff);
`;

const DefinitionTitle = styled.div`
  color: var(--afs-ink, #282828);
  font-size: 13px;
  font-weight: 800;
  line-height: 1.35;
`;

const DefinitionText = styled.div`
  color: var(--afs-muted, #6d6e71);
  font-size: 13px;
  line-height: 1.55;
`;

const BenchmarkRows = styled.div`
  display: grid;
  margin-top: 4px;
  border-top: 1px solid var(--afs-line, #e6e6e6);
`;

const BenchmarkRow = styled.div`
  display: grid;
  grid-template-columns: minmax(120px, 1fr) minmax(92px, auto) minmax(120px, 0.9fr) minmax(120px, 0.9fr) minmax(180px, 1.3fr);
  gap: 12px;
  align-items: center;
  padding: 13px 0;
  border-bottom: 1px solid var(--afs-line, #e6e6e6);

  @media (max-width: 860px) {
    grid-template-columns: 1fr;
    align-items: start;
    gap: 5px;
  }
`;

const BenchmarkName = styled.div`
  color: var(--afs-ink, #282828);
  font-size: 13px;
  font-weight: 800;
`;

const BenchmarkValue = styled.div`
  color: var(--afs-ink, #282828);
  font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
  font-size: 18px;
  font-weight: 800;
  line-height: 1.2;
`;

const BenchmarkDetail = styled.div`
  color: var(--afs-muted, #6d6e71);
  font-size: 12px;
  line-height: 1.4;
`;

const BenchmarkSummary = styled.div`
  color: var(--afs-muted, #6d6e71);
  font-size: 13px;
  line-height: 1.45;
`;

const FAQList = styled.div`
  display: grid;
  border-top: 1px solid var(--afs-line, #e6e6e6);
`;

const FAQItem = styled.div`
  display: grid;
  gap: 7px;
  padding: 16px 0;
  border-bottom: 1px solid var(--afs-line, #e6e6e6);
`;

const FAQQuestion = styled.div`
  color: var(--afs-ink, #282828);
  font-size: 14px;
  font-weight: 800;
  line-height: 1.4;
`;

const ReferenceDocLink = styled.a`
  color: var(--afs-accent, #064ea2);
  font-weight: 800;
  text-decoration: none;

  &:hover {
    text-decoration: underline;
    text-underline-offset: 4px;
  }
`;

const howItWorksTopic: DocsTopic = {
  id: "how-it-works",
  path: "/docs/how-it-works",
  eyebrow: "Core model",
  title: "How AFS Works",
  summary:
    "The workspace model, Redis data path, local surfaces, and checkpoint semantics in one place.",
  sections: [
    {
      heading: "The Workspace Model",
      body: (
        <>
          <DocProse>
            AFS is workspace-first. A workspace is a complete file tree that can
            hold source code, prompts, notes, generated files, logs, and agent
            scratch state together. The workspace is not tied to one laptop or
            one container because Redis is the saved source of truth.
          </DocProse>
          <DocProse>
            The live workspace is the mutable state agents and local tools edit.
            Checkpoints are explicit restore points. Forks create a second line
            of work from an existing workspace so one agent can explore without
            disturbing the main path.
          </DocProse>
          <DefinitionGrid>
            <DefinitionItem>
              <DefinitionTitle>Workspace</DefinitionTitle>
              <DefinitionText>A named file tree backed by Redis.</DefinitionText>
            </DefinitionItem>
            <DefinitionItem>
              <DefinitionTitle>Live root</DefinitionTitle>
              <DefinitionText>The current editable workspace state.</DefinitionText>
            </DefinitionItem>
            <DefinitionItem>
              <DefinitionTitle>Checkpoint</DefinitionTitle>
              <DefinitionText>A saved point you can restore later.</DefinitionText>
            </DefinitionItem>
            <DefinitionItem>
              <DefinitionTitle>Fork</DefinitionTitle>
              <DefinitionText>A new workspace copied from another line of work.</DefinitionText>
            </DefinitionItem>
          </DefinitionGrid>
        </>
      ),
    },
    {
      heading: "The Data Path",
      body: (
        <>
          <DocProse>
            The CLI, web UI, and MCP tools all work against the same workspace
            model. In cloud or Self-managed mode they talk through a control
            plane; in standalone mode the CLI can talk directly to Redis.
          </DocProse>
          <CodeBlock>
            <code>{`afs CLI / Web UI / MCP tools
        |
control plane + workspace service
        |
Redis: metadata, manifests, blobs, live roots, activity
        |
sync directory, live mount, or direct MCP file tools`}</code>
          </CodeBlock>
          <DocProse>
            Redis stores workspace metadata, manifests, blobs, checkpoints, live
            roots, and activity. That lets normal tools work against local files
            while the durable state remains remote, checkpointable, searchable,
            and shareable.
          </DocProse>
        </>
      ),
    },
    {
      heading: "What Happens When You Edit",
      body: (
        <>
          <DocProse>
            In sync mode, AFS keeps a real local directory synchronized with the
            Redis-backed live workspace. In mount mode, AFS exposes the live
            workspace as a filesystem. In MCP mode, agent file tools read and
            write the live workspace directly.
          </DocProse>
          <DocProse>
            Edits change the live workspace; they do not automatically create a
            checkpoint. Create a checkpoint when the state is worth preserving,
            restore when you want to go back, and fork when you want parallel
            work without mixing histories.
          </DocProse>
          <CalloutBox $tone="tip">
            <DocProse>
              The important mental model is simple: files change live state,
              checkpoints save named moments, and Redis keeps the durable copy.
            </DocProse>
          </CalloutBox>
        </>
      ),
    },
  ],
  related: ["cli", "workspaces", "local-files"],
};

const cliTopic: DocsTopic = {
  id: "cli",
  path: "/docs/cli",
  eyebrow: "CLI Docs",
  title: "AFS CLI Workflow",
  summary:
    "Install, sign in, create or import a workspace, mount it locally, and use the daily commands.",
  sections: [
    {
      heading: "Fresh Setup",
      body: (
        <>
          <DocProse>
            Start with the CLI. It is the primary way to authenticate, mount a
            workspace, configure sync or mount mode, create checkpoints, and
            launch the MCP server.
          </DocProse>
          <DocProse>
            <ReferenceDocLink href={referenceDocHref.cli} rel="noreferrer" target="_blank">
              Full CLI command reference
            </ReferenceDocLink>
          </DocProse>
          <CodeBlock>
            <code>{`afs auth login
afs ws mount getting-started ~/getting-started`}</code>
          </CodeBlock>
          <DocProse>
            <InlineCode>afs auth login</InlineCode> connects the CLI to AFS Cloud or
            a control plane. <InlineCode>afs ws mount</InlineCode> mounts a
            workspace to a local folder and prompts when you omit values.
          </DocProse>
        </>
      ),
    },
    {
      heading: "The Whole Loop",
      body: (
        <>
          <Step n={1} title="Sign in">
            <InlineCode>afs auth login</InlineCode> connects your local CLI to AFS
            Cloud or a control plane. The CLI keeps the token locally so future
            commands can create and mount workspaces without another browser
            step.
          </Step>
          <Step n={2} title="Create a workspace">
            <InlineCode>afs ws create myworkspace</InlineCode> creates an
            empty workspace with an initial checkpoint. This workspace is the
            shared state agents and tools will work against.
          </Step>
          <Step n={3} title="Mount it locally">
            <InlineCode>afs ws mount myworkspace ~/afs/myworkspace</InlineCode>{" "}
            starts the local AFS runtime and exposes the workspace at{" "}
            <InlineCode>~/afs/myworkspace</InlineCode>.
            Use your editor, shell, and agents there like any other directory.
          </Step>
          <Step n={4} title="Checkpoint the good state">
            <InlineCode>afs cp create myworkspace before-refactor</InlineCode>{" "}
            saves a named restore point. Live edits are immediate, while
            checkpoints are deliberate moments in the workspace timeline.
          </Step>
        </>
      ),
    },
    {
      heading: "Create, Import, And Start",
      body: (
        <>
          <DocProse>
            Create an empty workspace when you are starting fresh. Import a
            directory when you already have code or project state that should
            become checkpointable.
          </DocProse>
          <CodeBlock>
            <code>{`# New workspace
afs ws create demo
afs ws mount demo ~/demo

# Existing directory
afs ws import --mount-at-source demo ~/src/demo`}</code>
          </CodeBlock>
          <DocProse>
            <InlineCode>afs ws mount</InlineCode> creates the local connection.
            <InlineCode> --mount-at-source</InlineCode> imports an existing
            directory and mounts it in one step.
          </DocProse>
        </>
      ),
    },
    {
      heading: "Daily Commands",
      body: (
        <CmdTable>
          <thead>
            <tr>
              <th>Command</th>
              <th>Use it for</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td><InlineCode>afs status</InlineCode></td>
              <td>Check daemon status, configuration, and local mounts.</td>
            </tr>
            <tr>
              <td><InlineCode>afs ws list</InlineCode></td>
              <td>See available workspaces.</td>
            </tr>
            <tr>
              <td><InlineCode>afs ws mount</InlineCode></td>
              <td>Mount a workspace at a local folder.</td>
            </tr>
            <tr>
              <td><InlineCode>afs cp create</InlineCode></td>
              <td>Save the current live workspace as a restore point.</td>
            </tr>
            <tr>
              <td><InlineCode>afs fs demo grep TODO</InlineCode></td>
              <td>Search workspace files directly through AFS.</td>
            </tr>
            <tr>
              <td><InlineCode>afs ws unmount</InlineCode></td>
              <td>Unmount a workspace while preserving local files by default.</td>
            </tr>
          </tbody>
        </CmdTable>
      ),
    },
    {
      heading: "Persistent Config",
      body: (
        <>
          <DocProse>
            Use the key-based config commands when you need to inspect or change
            saved settings.
          </DocProse>
          <CodeBlock>
            <code>{`afs config get redis.url
afs config set config.source self-managed
afs config set controlPlane.url http://127.0.0.1:8091
afs config set mode sync
afs config list`}</code>
          </CodeBlock>
        </>
      ),
    },
  ],
  related: ["workspaces", "local-files", "mcp-agents"],
};

const workspacesTopic: DocsTopic = {
  id: "workspaces",
  path: "/docs/workspaces",
  eyebrow: "State management",
  title: "Workspaces And Checkpoints",
  summary:
    "How to create, import, mount, fork, checkpoint, and restore AFS workspaces.",
  sections: [
    {
      heading: "Workspace Lifecycle",
      body: (
        <>
          <DocProse>
            Workspaces are the durable unit of collaboration in AFS. You create
            one for a project, import one from an existing folder, mount it for
            local work, and optionally fork it for parallel work.
          </DocProse>
          <CodeBlock>
            <code>{`afs ws create demo
afs ws import --mount-at-source demo ~/src/demo
afs ws list
afs ws fork demo demo-experiment`}</code>
          </CodeBlock>
          <DocProse>
            <InlineCode>--mount-at-source</InlineCode> keeps the imported
            directory mounted after import. <InlineCode>fork</InlineCode>{" "}
            creates another AFS workspace, preserving the source workspace as
            its own line of work.
          </DocProse>
        </>
      ),
    },
    {
      heading: "Checkpoint Discipline",
      body: (
        <>
          <DocProse>
            A checkpoint is a named restore point inside a workspace. File edits,
            MCP writes, sync changes, and mount writes update live state first.
            They become a durable named point when you create a checkpoint.
          </DocProse>
          <CodeBlock>
            <code>{`afs cp create demo before-refactor
afs cp list demo
afs cp restore demo before-refactor`}</code>
          </CodeBlock>
          <CalloutBox $tone="warn">
            <DocProse>
              Restoring a checkpoint overwrites the live workspace state. Create
              a fresh checkpoint first if the current state might matter later.
            </DocProse>
          </CalloutBox>
        </>
      ),
    },
    {
      heading: "Useful Patterns",
      body: (
        <DefinitionGrid>
          <DefinitionItem>
            <DefinitionTitle>Before an agent run</DefinitionTitle>
            <DefinitionText>Create a checkpoint such as <InlineCode>before-agent</InlineCode>.</DefinitionText>
          </DefinitionItem>
          <DefinitionItem>
            <DefinitionTitle>Before a risky refactor</DefinitionTitle>
            <DefinitionText>Checkpoint, run the change, then checkpoint the accepted result.</DefinitionText>
          </DefinitionItem>
          <DefinitionItem>
            <DefinitionTitle>For parallel experiments</DefinitionTitle>
            <DefinitionText>Fork the workspace and send each agent to a separate fork.</DefinitionText>
          </DefinitionItem>
          <DefinitionItem>
            <DefinitionTitle>For handoff</DefinitionTitle>
            <DefinitionText>Checkpoint the current state and share the workspace name.</DefinitionText>
          </DefinitionItem>
        </DefinitionGrid>
      ),
    },
  ],
  related: ["cli", "local-files", "mcp-agents"],
};

const localFilesTopic: DocsTopic = {
  id: "local-files",
  path: "/docs/local-files",
  eyebrow: "Local surfaces",
  title: "Sync, Mount, And Local Files",
  summary:
    "How AFS exposes Redis-backed workspaces to editors, shells, scripts, and local tools.",
  sections: [
    {
      heading: "Sync Mode",
      body: (
        <>
          <DocProse>
            Sync mode is the recommended default. AFS keeps a real local folder
            synchronized with the Redis-backed live workspace so editors,
            language servers, test runners, and shell tools can operate normally.
          </DocProse>
          <CodeBlock>
            <code>{`afs ws mount demo ~/afs/demo
cd ~/afs/demo`}</code>
          </CodeBlock>
          <DocProse>
            Unmounting with <InlineCode>afs ws unmount demo</InlineCode> stops
            managing the local folder; it does not delete the workspace from
            Redis.
          </DocProse>
        </>
      ),
    },
    {
      heading: "Live Mount Mode",
      body: (
        <>
          <DocProse>
            Mount mode exposes the Redis-backed workspace directly as a
            filesystem. It is useful when you want the local path to be a live
            view instead of a synchronized folder.
          </DocProse>
          <CodeBlock>
            <code>{`afs config set --mode mount --mount-backend nfs
afs ws mount demo ~/afs/demo
afs ws unmount demo`}</code>
          </CodeBlock>
          <DocProse>
            On macOS AFS uses NFS; on Linux it uses FUSE. Sync mode is usually
            the friendlier default for cloud-connected and editor-heavy work.
          </DocProse>
        </>
      ),
    },
    {
      heading: "Import Hygiene",
      body: (
        <>
          <DocProse>
            Add a <InlineCode>.afsignore</InlineCode> file before importing
            large local projects. It uses gitignore-style patterns and keeps
            build output, dependency caches, logs, and machine-local files out
            of the workspace.
          </DocProse>
          <CodeBlock>
            <code>{`node_modules/
.venv/
dist/
*.log
.DS_Store`}</code>
          </CodeBlock>
        </>
      ),
    },
  ],
  related: ["cli", "workspaces", "performance"],
};

const mcpAgentsTopic: DocsTopic = {
  id: "mcp-agents",
  path: "/docs/mcp-agents",
  eyebrow: "Agent access",
  title: "MCP And Agent Workflows",
  summary:
    "Connect agents to AFS workspaces with the built-in MCP server and permission profiles.",
  sections: [
    {
      heading: "Run The MCP Server",
      body: (
        <>
          <DocProse>
            <InlineCode>afs mcp</InlineCode> starts the workspace-first MCP
            server over stdio. It is meant to be launched by an MCP client, not
            used as a long-running web server.
          </DocProse>
          <DocProse>
            <ReferenceDocLink href={referenceDocHref.mcp} rel="noreferrer" target="_blank">
              Full MCP tool reference
            </ReferenceDocLink>
          </DocProse>
          <CodeBlock>
            <code>{`{
  "mcpServers": {
    "afs": {
      "command": "/absolute/path/to/afs",
      "args": ["mcp", "--workspace", "demo", "--profile", "workspace-rw"]
    }
  }
}`}</code>
          </CodeBlock>
        </>
      ),
    },
    {
      heading: "SDK Mounts",
      body: (
        <>
          <DocProse>
            Use the TypeScript or Python SDK when an agent application should
            create workspaces, mint workspace-scoped access, and run commands
            against an isolated in-process mount.
          </DocProse>
          <DocProse>
            Install with <InlineCode>npm install redis-afs</InlineCode> or{" "}
            <InlineCode>pip install redis-afs</InlineCode>.
          </DocProse>
          <DocSubheading>TypeScript</DocSubheading>
          <CodeBlock>
            <code>
              <HighlightedCode code={typescriptSdkSample} language="typescript" />
            </code>
          </CodeBlock>
          <DocSubheading>Python</DocSubheading>
          <CodeBlock>
            <code>
              <HighlightedCode code={pythonSdkSample} language="python" />
            </code>
          </CodeBlock>
        </>
      ),
    },
    {
      heading: "Permission Profiles",
      body: (
        <CmdTable>
          <thead>
            <tr>
              <th>Profile</th>
              <th>Scope</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td><InlineCode>workspace-ro</InlineCode></td>
              <td>Workspace-bound read-only file tools.</td>
            </tr>
            <tr>
              <td><InlineCode>workspace-rw</InlineCode></td>
              <td>Workspace-bound read/write file tools. This is the default.</td>
            </tr>
            <tr>
              <td><InlineCode>workspace-rw-checkpoint</InlineCode></td>
              <td>Read/write file tools plus checkpoint operations.</td>
            </tr>
            <tr>
              <td><InlineCode>admin-ro</InlineCode></td>
              <td>Broad read-only workspace administration.</td>
            </tr>
            <tr>
              <td><InlineCode>admin-rw</InlineCode></td>
              <td>Broad read/write workspace administration.</td>
            </tr>
          </tbody>
        </CmdTable>
      ),
    },
    {
      heading: "Agent Operating Loop",
      body: (
        <>
          <DocProse>
            Give each agent a workspace, make the MCP server name obvious, and
            checkpoint before and after important work. MCP file tools update
            live workspace state just like sync and mount writes.
          </DocProse>
          <CodeBlock>
            <code>{`afs ws create demo
afs cp create demo before-agent
afs mcp --workspace demo --profile workspace-rw-checkpoint`}</code>
          </CodeBlock>
          <CalloutBox $tone="tip">
            <DocProse>
              For repeatable team setups, use workspace templates after the CLI
              works. Templates can generate MCP config and agent setup copy for
              a named workspace.
            </DocProse>
          </CalloutBox>
        </>
      ),
    },
  ],
  related: ["typescript-sdk", "python-sdk", "workspaces"],
};

const typescriptSdkTopic: DocsTopic = {
  id: "typescript-sdk",
  path: "/docs/typescript-sdk",
  eyebrow: "SDK",
  title: "TypeScript SDK",
  summary:
    "Create workspaces, mount them in-process, edit files, search, checkpoint, and run shell commands from Node.js.",
  sections: [
    {
      heading: "Install And Connect",
      body: (
        <>
          <DocProse>
            Use the TypeScript SDK when an agent app should work with AFS
            directly instead of asking a user to create a local mount first.
            The client uses the hosted MCP endpoint by default and can point at
            a Self-managed control plane with one environment variable.
          </DocProse>
          <DocProse>
            <ReferenceDocLink href={referenceDocHref.typescript} rel="noreferrer" target="_blank">
              TypeScript command reference
            </ReferenceDocLink>
          </DocProse>
          <CodeBlock>
            <code>{`npm install redis-afs
export AFS_API_KEY="afs_..."

# Optional for Self-managed control planes
export AFS_API_BASE_URL="http://127.0.0.1:8091"`}</code>
          </CodeBlock>
          <CmdTable>
            <thead>
              <tr>
                <th>Setting</th>
                <th>Value</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><InlineCode>import</InlineCode></td>
                <td><InlineCode>{`import { AFS } from "redis-afs"`}</InlineCode></td>
              </tr>
              <tr>
                <td><InlineCode>runtime</InlineCode></td>
                <td>Node.js 18 or newer.</td>
              </tr>
              <tr>
                <td><InlineCode>default endpoint</InlineCode></td>
                <td><InlineCode>https://afs.cloud/mcp</InlineCode></td>
              </tr>
            </tbody>
          </CmdTable>
        </>
      ),
    },
    {
      heading: "Create And Mount",
      body: (
        <>
          <DocProse>
            The mount is isolated inside the SDK process. It issues
            workspace-scoped MCP tokens, then gives your app file, search,
            checkpoint, and shell helpers.
          </DocProse>
          <CodeBlock>
            <code>
              <HighlightedCode code={typescriptSdkSample} language="typescript" />
            </code>
          </CodeBlock>
        </>
      ),
    },
    {
      heading: "Daily SDK Methods",
      body: (
        <CmdTable>
          <thead>
            <tr>
              <th>Method</th>
              <th>Use it for</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td><InlineCode>afs.workspace.create()</InlineCode></td>
              <td>Create a Redis-backed workspace.</td>
            </tr>
            <tr>
              <td><InlineCode>afs.workspace.fork()</InlineCode></td>
              <td>Branch an existing workspace into a separate line of work.</td>
            </tr>
            <tr>
              <td><InlineCode>afs.checkpoint.create()</InlineCode></td>
              <td>Save a deliberate restore point.</td>
            </tr>
            <tr>
              <td><InlineCode>afs.fs.mount()</InlineCode></td>
              <td>Open an isolated in-process mount.</td>
            </tr>
            <tr>
              <td><InlineCode>fs.readFile()</InlineCode> / <InlineCode>fs.writeFile()</InlineCode></td>
              <td>Read and write text files through workspace-scoped MCP tools.</td>
            </tr>
            <tr>
              <td><InlineCode>fs.glob()</InlineCode> / <InlineCode>fs.grep()</InlineCode></td>
              <td>Find paths and search file contents without a local mount.</td>
            </tr>
            <tr>
              <td><InlineCode>fs.bash().exec()</InlineCode></td>
              <td>Run shell commands after materializing the workspace locally.</td>
            </tr>
          </tbody>
        </CmdTable>
      ),
    },
    {
      heading: "Path And Command Semantics",
      body: (
        <>
          <DocProse>
            With one mounted workspace, paths like{" "}
            <InlineCode>/src/index.ts</InlineCode> are workspace-relative. With
            multiple mounted workspaces, prefix paths with the workspace name,
            such as <InlineCode>/api/src/index.ts</InlineCode>.
          </DocProse>
          <DocProse>
            <InlineCode>bash().exec()</InlineCode> downloads mounted workspaces
            into a temporary local directory, rewrites absolute workspace paths
            to that directory, runs <InlineCode>/bin/bash</InlineCode>, then
            syncs created and modified text files back to AFS.
          </DocProse>
        </>
      ),
    },
  ],
  related: ["python-sdk", "mcp-agents", "workspaces"],
};

const pythonSdkTopic: DocsTopic = {
  id: "python-sdk",
  path: "/docs/python-sdk",
  eyebrow: "SDK",
  title: "Python SDK",
  summary:
    "Use Python to create AFS workspaces, mount them in-process, edit files, checkpoint, search, and run commands.",
  sections: [
    {
      heading: "Install And Connect",
      body: (
        <>
          <DocProse>
            The Python SDK mirrors the TypeScript shape with Python naming and a
            context-manager-friendly mount. Use it for agents, workers, and
            automation that should talk to AFS directly.
          </DocProse>
          <DocProse>
            <ReferenceDocLink href={referenceDocHref.python} rel="noreferrer" target="_blank">
              Python command reference
            </ReferenceDocLink>
          </DocProse>
          <CodeBlock>
            <code>{`pip install redis-afs
export AFS_API_KEY="afs_..."

# Optional for Self-managed control planes
export AFS_API_BASE_URL="http://127.0.0.1:8091"`}</code>
          </CodeBlock>
          <CmdTable>
            <thead>
              <tr>
                <th>Setting</th>
                <th>Value</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td><InlineCode>import</InlineCode></td>
                <td><InlineCode>from redis_afs import AFS</InlineCode></td>
              </tr>
              <tr>
                <td><InlineCode>runtime</InlineCode></td>
                <td>Python 3.10 or newer.</td>
              </tr>
              <tr>
                <td><InlineCode>default endpoint</InlineCode></td>
                <td><InlineCode>https://afs.cloud/mcp</InlineCode></td>
              </tr>
            </tbody>
          </CmdTable>
        </>
      ),
    },
    {
      heading: "Create And Mount",
      body: (
        <>
          <DocProse>
            The mount is isolated inside the SDK process. It can be closed
            manually or used as a context manager so temporary local state is
            cleaned up when your agent finishes.
          </DocProse>
          <CodeBlock>
            <code>
              <HighlightedCode code={pythonSdkSample} language="python" />
            </code>
          </CodeBlock>
          <CodeBlock>
            <code>
              <HighlightedCode
                code={`with afs.fs.mount(workspaces=[{"name": "foobar"}], mode="rw") as fs:
    fs.write_file("/README.md", "hello")`}
                language="python"
              />
            </code>
          </CodeBlock>
        </>
      ),
    },
    {
      heading: "Daily SDK Methods",
      body: (
        <CmdTable>
          <thead>
            <tr>
              <th>Method</th>
              <th>Use it for</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td><InlineCode>afs.workspace.create()</InlineCode></td>
              <td>Create a Redis-backed workspace.</td>
            </tr>
            <tr>
              <td><InlineCode>afs.workspace.fork()</InlineCode></td>
              <td>Branch an existing workspace into a separate line of work.</td>
            </tr>
            <tr>
              <td><InlineCode>afs.checkpoint.create()</InlineCode></td>
              <td>Save a deliberate restore point.</td>
            </tr>
            <tr>
              <td><InlineCode>afs.fs.mount()</InlineCode></td>
              <td>Open an isolated in-process mount.</td>
            </tr>
            <tr>
              <td><InlineCode>fs.read_file()</InlineCode> / <InlineCode>fs.write_file()</InlineCode></td>
              <td>Read and write text files through workspace-scoped MCP tools.</td>
            </tr>
            <tr>
              <td><InlineCode>fs.glob()</InlineCode> / <InlineCode>fs.grep()</InlineCode></td>
              <td>Find paths and search file contents without a local mount.</td>
            </tr>
            <tr>
              <td><InlineCode>fs.bash().exec()</InlineCode></td>
              <td>Run shell commands after materializing the workspace locally.</td>
            </tr>
          </tbody>
        </CmdTable>
      ),
    },
    {
      heading: "Path And Command Semantics",
      body: (
        <>
          <DocProse>
            With one mounted workspace, paths like{" "}
            <InlineCode>/src/app.py</InlineCode> are workspace-relative. With
            multiple mounted workspaces, prefix paths with the workspace name,
            such as <InlineCode>/api/app.py</InlineCode>.
          </DocProse>
          <DocProse>
            <InlineCode>bash().exec()</InlineCode> downloads mounted workspaces
            into a temporary local directory, rewrites absolute workspace paths
            to that directory, runs <InlineCode>/bin/bash</InlineCode>, then
            syncs created and modified text files back to AFS.
          </DocProse>
        </>
      ),
    },
  ],
  related: ["typescript-sdk", "mcp-agents", "workspaces"],
};

const selfManagedTopic: DocsTopic = {
  id: "self-managed",
  path: "/docs/self-managed",
  eyebrow: "Deployment",
  title: "Deployments: Cloud, Self-managed, and Standalone",
  summary:
    "Choose the right control-plane and Redis topology for local development, teams, and agent-only workflows.",
  sections: [
    {
      heading: "Three Ways To Run AFS",
      body: (
        <DefinitionGrid>
          <DefinitionItem>
            <DefinitionTitle>Cloud-hosted</DefinitionTitle>
            <DefinitionText>Use AFS Cloud for browser auth, hosted UI, and managed workspace access.</DefinitionText>
          </DefinitionItem>
          <DefinitionItem>
            <DefinitionTitle>Self-managed</DefinitionTitle>
            <DefinitionText>Run your own control plane and UI, usually with your own Redis database.</DefinitionText>
          </DefinitionItem>
          <DefinitionItem>
            <DefinitionTitle>Standalone</DefinitionTitle>
            <DefinitionText>Use the CLI directly against Redis without the browser UI or control plane.</DefinitionText>
          </DefinitionItem>
        </DefinitionGrid>
      ),
    },
    {
      heading: "Local Self-Managed Development",
      body: (
        <>
          <DocProse>
            The checkout has a single local web-dev path that starts the control
            plane and Vite UI together. Use it when you want to work on the
            product surface rather than only the CLI.
          </DocProse>
          <CodeBlock>
            <code>{`make web-dev
# control plane: http://127.0.0.1:8091
# Vite UI:      printed by the dev server`}</code>
          </CodeBlock>
          <DocProse>
            The Databases page manages the Redis databases where workspaces are
            hosted. Redis remains the canonical store for manifests, blobs,
            checkpoints, live roots, and workspace metadata.
          </DocProse>
        </>
      ),
    },
    {
      heading: "CLI Configuration",
      body: (
        <>
          <DocProse>
            Point the CLI at a Self-managed control plane when you want the CLI
            and UI to share the same workspace catalog.
          </DocProse>
          <CodeBlock>
            <code>{`afs config set config.source self-managed
afs config set controlPlane.url http://127.0.0.1:8091
afs ws mount getting-started ~/getting-started`}</code>
          </CodeBlock>
          <DocProse>
            For standalone mode, configure Redis directly with{" "}
            <InlineCode>redis.url</InlineCode> and run the same workspace and
            checkpoint commands without the hosted UI.
          </DocProse>
        </>
      ),
    },
  ],
  related: ["cli", "how-it-works", "mcp-agents"],
};

const performanceTopic: DocsTopic = {
  id: "performance",
  path: "/docs/performance",
  eyebrow: "Benchmarks",
  title: "Search Performance",
  summary:
    "How to read the Redis Search benchmark and when to use indexed AFS grep versus local ripgrep.",
  sections: [
    {
      heading: "Latest Redis Search Snapshot",
      body: (
        <>
          <DocProse>
            The latest local benchmark used {searchBenchmark.corpus} on{" "}
            {searchBenchmark.environment}. Literal searches use the Redis Search
            indexed path when it is available, then AFS verifies candidate file
            contents.
          </DocProse>
          <BenchmarkRows>
            {searchBenchmark.metrics.map((metric) => (
              <BenchmarkRow key={metric.name}>
                <BenchmarkName>{metric.name}</BenchmarkName>
                <BenchmarkValue>{metric.afs}</BenchmarkValue>
                <BenchmarkDetail>BSD grep: {metric.grep}</BenchmarkDetail>
                <BenchmarkDetail>ripgrep: {metric.ripgrep}</BenchmarkDetail>
                <BenchmarkSummary>{metric.summary}</BenchmarkSummary>
              </BenchmarkRow>
            ))}
          </BenchmarkRows>
        </>
      ),
    },
    {
      heading: "How To Interpret It",
      body: (
        <>
          <DocProse>
            Simple literal searches are the indexed fast path. They are the
            reason AFS can search a Redis-backed workspace without first
            materializing every file into a local tree.
          </DocProse>
          <DocProse>
            Regex and advanced matching still fall back to the non-indexed
            traversal path. For regex-heavy scans inside a mounted or synced
            workspace, <InlineCode>rg</InlineCode> remains the right tool.
          </DocProse>
          <CodeBlock>
            <code>{`afs fs demo grep "TODO"
afs fs demo grep -l -i "disk full"
afs fs demo grep -E "error|warning"`}</code>
          </CodeBlock>
        </>
      ),
    },
  ],
  related: ["cli", "local-files", "how-it-works"],
};

const faqTopic: DocsTopic = {
  id: "faq",
  path: "/docs/faq",
  eyebrow: "Reference",
  title: "Frequently Asked Questions",
  summary:
    "Answers for data import, egress, large files, versioning, Git providers, POSIX behavior, and search speed.",
  sections: [
    {
      heading: "Common Questions",
      body: (
        <FAQList>
          <FAQItem>
            <FAQQuestion>
              My data lives in GitHub, GitLab, S3, or Drive. Can I sync it to AFS?
            </FAQQuestion>
            <DocProse>
              Today, the clean path is to bring that data into a local
              directory first, then import or sync it with AFS. For Git
              upstreams, clone or check out the Git project the way you already
              do, then run <InlineCode>afs ws import --mount-at-source</InlineCode>{" "}
              or <InlineCode>afs ws mount</InlineCode>. For non-Git systems
              like S3 or Google Drive, use their API or CLI in a small script
              and let AFS own the workspace state after the files land locally.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>How does egress metering work?</FAQQuestion>
            <DocProse>
              Local and Self-managed AFS do not add an AFS egress meter. Reads
              happen through sync, live mount, MCP tools, the CLI, or the
              control-plane API against your configured Redis/control-plane
              deployment. If you run AFS on hosted infrastructure, the normal
              provider bandwidth and Redis network policies still apply.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>
              Does AFS handle large files like datasets, models, and media?
            </FAQQuestion>
            <DocProse>
              Yes, within the shape of an agent workspace. AFS stores file
              content in Redis-backed external content keys, supports byte-range
              reads and writes in the mount path, and syncs changed large files
              in chunks. The default sync per-file cap is 2 GB, so keep
              generated artifacts and temporary bulk data out with{" "}
              <InlineCode>.afsignore</InlineCode> when they do not belong in the
              workspace timeline.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>Does my agent really need versioning?</FAQQuestion>
            <DocProse>
              If the agent is doing throwaway scratch work, maybe not. If the
              work needs human review, rollback, audit history, or parallel
              exploration, versioning becomes the safety rail. In AFS, edits
              update the live workspace, checkpoints save deliberate restore
              points, and forks let another agent explore a second line of work
              without clobbering the first one.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>Why can't I just use GitHub or GitLab?</FAQQuestion>
            <DocProse>
              You can, and you should keep using Git providers for source
              control, pull requests, and long-lived project history. AFS is for
              the live workspace around that source: generated files, prompts,
              logs, datasets, agent scratch state, checkpoints, forks, local
              sync, live mount, and MCP file tools. It gives agents a
              filesystem-shaped place to work before everything is ready to
              become a Git commit.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>
              How is this different from S3 or S3-style filesystems?
            </FAQQuestion>
            <DocProse>
              Object storage is excellent for durable blobs. AFS is a workspace
              system: it keeps a file tree, live edits, search, checkpoints,
              forks, and local execution surfaces together. Use S3-style storage
              for large durable objects and archival data; use AFS when agents
              need to edit a working tree, checkpoint the state, fork into
              parallel attempts, and keep normal tools pointed at a local path.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>Is AFS POSIX compatible?</FAQQuestion>
            <DocProse>
              AFS is designed to feel like a normal filesystem to editors,
              shells, agents, and sandboxes. Sync mode gives you a real local
              directory on disk. Live mount mode exposes the workspace through
              NFS on macOS and FUSE on Linux. That covers the everyday Unix tool
              workflow, but AFS does not pretend to be a perfect replacement for
              a mature shared POSIX filesystem in every multi-writer edge case.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>How fast is AFS?</FAQQuestion>
            <DocProse>
              For the current Redis Search benchmark, indexed literal{" "}
              <InlineCode>afs fs grep</InlineCode> runs in tens of milliseconds:
              17.35 ms for a rare literal and 42.56 ms for a common literal on
              a 4,000-file markdown corpus. Sync mode keeps a real local
              directory on disk for editors and tools, while live mount and MCP
              give agents direct access to the same Redis-backed workspace.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>Okay, but is AFS fast enough for agents?</FAQQuestion>
            <DocProse>
              AFS is built around time-to-useful-work. An agent can start from a
              workspace, search the tree, read the files it needs, write
              changes, and checkpoint the result without cloning or
              materializing everything up front. For regex-heavy searches, use{" "}
              <InlineCode>rg</InlineCode> on a synced or mounted workspace; for
              ordinary literal search, <InlineCode>afs fs grep</InlineCode> uses
              the indexed fast path when Redis Search is available.
            </DocProse>
          </FAQItem>
        </FAQList>
      ),
    },
  ],
  related: ["how-it-works", "cli", "self-managed"],
};

export const docsTopics = [
  howItWorksTopic,
  cliTopic,
  workspacesTopic,
  localFilesTopic,
  mcpAgentsTopic,
  typescriptSdkTopic,
  pythonSdkTopic,
  selfManagedTopic,
  performanceTopic,
  faqTopic,
] as const satisfies ReadonlyArray<DocsTopic>;

export const docsTopicById = {
  "how-it-works": howItWorksTopic,
  cli: cliTopic,
  workspaces: workspacesTopic,
  "local-files": localFilesTopic,
  "mcp-agents": mcpAgentsTopic,
  "typescript-sdk": typescriptSdkTopic,
  "python-sdk": pythonSdkTopic,
  "self-managed": selfManagedTopic,
  performance: performanceTopic,
  faq: faqTopic,
} satisfies Record<DocsTopicId, DocsTopic>;

export function DocsTopicPage({ topic }: { topic: DocsTopic }) {
  const relatedTopics = topic.related.map((id) => docsTopicById[id]);

  return (
    <DocPage>
      <DocHero>
        <BackLink to="/docs">Docs</BackLink>
        <TopicEyebrow>{topic.eyebrow}</TopicEyebrow>
        <DocHeroTitle>{topic.title}</DocHeroTitle>
        <DocHeroSub>{topic.summary}</DocHeroSub>
      </DocHero>

      {topic.sections.map((section) => (
        <DocSection key={section.heading}>
          <DocHeading>{section.heading}</DocHeading>
          <SectionBody>{section.body}</SectionBody>
        </DocSection>
      ))}

      <DocSection>
        <DocHeading>Keep Reading</DocHeading>
        <DocProse>These pages cover the next nearby parts of AFS.</DocProse>
        <DocsTopicLinks topics={relatedTopics} />
      </DocSection>
    </DocPage>
  );
}

export function DocsTopicLinks({ topics = docsTopics }: { topics?: ReadonlyArray<DocsTopic> }) {
  return (
    <TopicGrid>
      {topics.map((topic) => (
        <TopicCard as={Link} key={topic.id} to={topic.path}>
          <TopicCardBody>
            <TopicCardEyebrow>{topic.eyebrow}</TopicCardEyebrow>
            <TopicCardTitle>{topic.title}</TopicCardTitle>
            <TopicCardSummary>{topic.summary}</TopicCardSummary>
          </TopicCardBody>
          <CrossLinkArrow>&rarr;</CrossLinkArrow>
        </TopicCard>
      ))}
    </TopicGrid>
  );
}

const BackLink = styled(Link)`
  display: inline-flex;
  width: fit-content;
  margin-bottom: 14px;
  color: var(--afs-accent, #064ea2);
  font-size: 13px;
  font-weight: 800;
  text-decoration: none;

  &:hover {
    text-decoration: underline;
  }
`;

const TopicEyebrow = styled.div`
  margin-bottom: 8px;
  color: var(--afs-accent, #064ea2);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
`;

const SectionBody = styled.div`
  display: grid;
  gap: 14px;
  margin-top: 12px;
`;

const TopicGrid = styled.div`
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-top: 18px;

  @media (max-width: 760px) {
    grid-template-columns: 1fr;
  }
`;

const TopicCard = styled(CrossLinkCard)`
  align-items: flex-start;
  padding: 18px;
  border-radius: 8px;
`;

const TopicCardBody = styled(CrossLinkText)`
  display: grid;
  gap: 5px;
`;

const TopicCardEyebrow = styled.div`
  color: var(--afs-accent, #064ea2);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
`;

const TopicCardTitle = styled(CrossLinkTitle)`
  line-height: 1.35;
`;

const TopicCardSummary = styled(CrossLinkDesc)`
  line-height: 1.55;
`;
