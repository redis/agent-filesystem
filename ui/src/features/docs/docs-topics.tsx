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
} from "../../components/doc-kit";
import { searchBenchmark } from "../../foundation/performance-data";

export type DocsTopicId =
  | "how-it-works"
  | "cli"
  | "workspaces"
  | "local-files"
  | "mcp-agents"
  | "self-managed"
  | "performance";

export type DocsTopic = {
  id: DocsTopicId;
  path:
    | "/docs/how-it-works"
    | "/docs/cli"
    | "/docs/workspaces"
    | "/docs/local-files"
    | "/docs/mcp-agents"
    | "/docs/self-managed"
    | "/docs/performance";
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
  eyebrow: "First run",
  title: "AFS CLI Workflow",
  summary:
    "Install, sign in, create or import a workspace, start the local surface, and use the daily commands.",
  sections: [
    {
      heading: "Fresh Setup",
      body: (
        <>
          <DocProse>
            Start with the CLI. It is the primary way to authenticate, select a
            workspace, start sync or mount mode, create checkpoints, and launch
            the MCP server.
          </DocProse>
          <CodeBlock>
            <code>{`afs login
afs setup
afs up`}</code>
          </CodeBlock>
          <DocProse>
            <InlineCode>afs login</InlineCode> connects the CLI to AFS Cloud or
            a control plane. <InlineCode>afs setup</InlineCode> walks through
            workspace and local path setup. <InlineCode>afs up</InlineCode>{" "}
            starts the saved workspace using the saved mode.
          </DocProse>
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
afs workspace create demo
afs workspace use demo
afs up

# Existing directory
afs workspace import demo ~/src/demo
afs up demo ~/src/demo`}</code>
          </CodeBlock>
          <DocProse>
            When a workspace is passed positionally to{" "}
            <InlineCode>afs up</InlineCode>, AFS saves that workspace and local
            path so future <InlineCode>afs up</InlineCode> runs can reuse them.
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
              <td>Check login, selected workspace, local path, and runtime state.</td>
            </tr>
            <tr>
              <td><InlineCode>afs workspace list</InlineCode></td>
              <td>See available workspaces.</td>
            </tr>
            <tr>
              <td><InlineCode>afs workspace current</InlineCode></td>
              <td>Print the workspace used when commands omit one.</td>
            </tr>
            <tr>
              <td><InlineCode>afs checkpoint create</InlineCode></td>
              <td>Save the current live workspace as a restore point.</td>
            </tr>
            <tr>
              <td><InlineCode>afs grep TODO</InlineCode></td>
              <td>Search workspace files directly through AFS.</td>
            </tr>
            <tr>
              <td><InlineCode>afs down</InlineCode></td>
              <td>Stop the local runtime when you are done.</td>
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
afs config set mount.path ~/afs/demo
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
    "How to create, import, select, fork, clone, checkpoint, and restore AFS workspaces.",
  sections: [
    {
      heading: "Workspace Lifecycle",
      body: (
        <>
          <DocProse>
            Workspaces are the durable unit of collaboration in AFS. You create
            one for a project, import one from an existing folder, select it for
            daily commands, and optionally fork it for parallel work.
          </DocProse>
          <CodeBlock>
            <code>{`afs workspace create demo
afs workspace import demo ~/src/demo
afs workspace list
afs workspace use demo
afs workspace fork demo demo-experiment
afs workspace clone demo ~/exports/demo`}</code>
          </CodeBlock>
          <DocProse>
            <InlineCode>clone</InlineCode> exports a workspace to a normal local
            directory. <InlineCode>fork</InlineCode> creates another AFS
            workspace, preserving the source workspace as its own line of work.
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
            <code>{`afs checkpoint create demo before-refactor
afs checkpoint list demo
afs checkpoint restore demo before-refactor`}</code>
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
            <code>{`afs workspace use demo
afs up --mode sync
cd ~/afs/demo`}</code>
          </CodeBlock>
          <DocProse>
            Stopping the daemon with <InlineCode>afs down</InlineCode> stops the
            local runtime; it does not delete the workspace from Redis.
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
            <code>{`afs config set mount.backend nfs
afs up demo ~/afs/demo --mode mount
afs down`}</code>
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
            <code>{`afs workspace create demo
afs checkpoint create demo before-agent
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
  related: ["workspaces", "cli", "self-managed"],
};

const selfManagedTopic: DocsTopic = {
  id: "self-managed",
  path: "/docs/self-managed",
  eyebrow: "Deployment",
  title: "Cloud, Self-Managed, And Standalone Modes",
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
            The repo has a single local web-dev path that starts the control
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
afs up --control-plane-url http://127.0.0.1:8091 getting-started`}</code>
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
            <code>{`afs grep "TODO" --workspace demo
afs grep -l -i --workspace demo "disk full"
afs grep -E "error|warning" --workspace demo`}</code>
          </CodeBlock>
        </>
      ),
    },
  ],
  related: ["cli", "local-files", "how-it-works"],
};

export const docsTopics = [
  howItWorksTopic,
  cliTopic,
  workspacesTopic,
  localFilesTopic,
  mcpAgentsTopic,
  selfManagedTopic,
  performanceTopic,
] as const satisfies ReadonlyArray<DocsTopic>;

export const docsTopicById = {
  "how-it-works": howItWorksTopic,
  cli: cliTopic,
  workspaces: workspacesTopic,
  "local-files": localFilesTopic,
  "mcp-agents": mcpAgentsTopic,
  "self-managed": selfManagedTopic,
  performance: performanceTopic,
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
