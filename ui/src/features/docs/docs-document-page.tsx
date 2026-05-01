import { Bot } from "lucide-react";
import styled, { css } from "styled-components";
import { searchBenchmark } from "../../foundation/performance-data";
import { pythonSdkSample, typescriptSdkSample } from "./afs-samples";
import { HighlightedCode } from "./syntax-code";
import type { CodeLanguage } from "./syntax-code";

const docsReferenceBaseHref = "https://github.com/redis/agent-filesystem/blob/main/docs";
const agentGuideHref = `${docsReferenceBaseHref}/agent-filesystem.md`;

const referenceDocs = [
  { label: "CLI Reference", href: `${docsReferenceBaseHref}/cli-reference.md` },
  { label: "TypeScript SDK Reference", href: `${docsReferenceBaseHref}/typescript-reference.md` },
  { label: "Python SDK Reference", href: `${docsReferenceBaseHref}/python-reference.md` },
  { label: "MCP Reference", href: `${docsReferenceBaseHref}/mcp-reference.md` },
] as const;

const tocItems = [
  { href: "#terms", label: "AFS basics" },
  { href: "#getting-started", label: "Getting started" },
  { href: "#how-it-works", label: "How AFS works" },
  { href: "#cli", label: "CLI workflow" },
  { href: "#workspaces", label: "Workspaces and checkpoints" },
  { href: "#local-files", label: "Sync, mount, and local files" },
  { href: "#mcp", label: "MCP and agents" },
  { href: "#sdk", label: "SDKs" },
  { href: "#deployments", label: "Deployments" },
  { href: "#performance", label: "Performance" },
  ...referenceDocs,
] as const;

export function DocsDocumentPage() {
  const firstContentsColumn = tocItems.slice(0, 7);
  const secondContentsColumn = tocItems.slice(7);

  const renderContentsItem = (item: (typeof tocItems)[number]) => {
    const isExternal = item.href.startsWith("http");
    return (
      <li key={item.label}>
        <ContentsLink
          href={item.href}
          rel={isExternal ? "noreferrer" : undefined}
          target={isExternal ? "_blank" : undefined}
        >
          {item.label}
        </ContentsLink>
      </li>
    );
  };

  return (
    <PageShell>
      <Article>
        <ArticleHeader>
          <AgentDocLink href={agentGuideHref} rel="noreferrer" target="_blank">
            <Bot aria-hidden="true" size={16} strokeWidth={2.25} />
            For Agents -&gt; <span>Docs/agent-filesystem.md</span>
          </AgentDocLink>
          <h1>Agent Filesystem</h1>
          <Lead>
            AFS gives agents a filesystem-shaped workspace backed by Redis. The
            CLI, SDKs, web UI, local sync or mount runtime, and MCP tools all
            work against the same live workspace model.
          </Lead>

          <ContentsSection aria-labelledby="contents-heading">
            <ContentsHeading id="contents-heading">Contents</ContentsHeading>
            <ContentsGrid>
              <ContentsList>{firstContentsColumn.map(renderContentsItem)}</ContentsList>
              <ContentsList start={firstContentsColumn.length + 1}>
                {secondContentsColumn.map(renderContentsItem)}
              </ContentsList>
            </ContentsGrid>
          </ContentsSection>
        </ArticleHeader>

        <DocSection id="terms">
          <SectionEyebrow>Plain English</SectionEyebrow>
          <h2>AFS Basics</h2>
          <p>
            Agent Filesystem (AFS) gives agents a shared, versioned file tree
            backed by Redis. In AFS, each file tree is a workspace. A workspace
            can be mounted at any local directory, so agents can read and write
            shared state through ordinary filesystem paths.
          </p>
          <p>
            When a workspace is mounted, the local directory becomes a live
            view of the Redis-backed workspace. Changes written to that
            directory sync back to Redis, and changes from other mounted agents
            sync back down to the local directory.
          </p>
          <TerminalBlock code={`afs ws mount memories /agent/memories`} />
          <p>
            This mounts the <InlineCode>memories</InlineCode> workspace at{" "}
            <InlineCode>/agent/memories</InlineCode>. Agent 1 can edit files in
            that folder, Agent 2 can mount the same workspace somewhere else,
            and both agents still work against one shared filesystem.
          </p>
          <p>
            Redis stores the workspace metadata, file manifests, file contents,
            checkpoints, and activity. Local edits update the live workspace
            state immediately. When an agent reaches an important moment, it can
            create a checkpoint to save that state as a named restore point.
          </p>

          <SystemDiagram aria-label="AFS system diagram">
            <DiagramRow>
              <DiagramNode>
                <strong>Agent 1</strong>
                <span>/agent/memories</span>
              </DiagramNode>
              <DiagramNode>
                <strong>Agent 2</strong>
                <span>/tmp/memories</span>
              </DiagramNode>
              <DiagramNode>
                <strong>Agent 3</strong>
                <span>MCP or SDK tools</span>
              </DiagramNode>
            </DiagramRow>
            <DiagramArrow>mount to the same</DiagramArrow>
            <DiagramNode>
              <strong>Workspace: memories</strong>
              <span>one shared live file tree</span>
            </DiagramNode>
            <DiagramArrow>stores live state and checkpoints in</DiagramArrow>
            <DiagramNode>
              <strong>Redis</strong>
              <span>metadata, manifests, file contents, checkpoints, and activity</span>
            </DiagramNode>
            <DiagramCaption>Many local views, one Redis-backed workspace.</DiagramCaption>
          </SystemDiagram>

          <DefinitionList>
            <div>
              <dt>AFS</dt>
              <dd>The system that turns Redis-backed workspace state into files agents can use.</dd>
            </div>
            <div>
              <dt>Workspace</dt>
              <dd>A named file tree with one live editable state.</dd>
            </div>
            <div>
              <dt>Live state</dt>
              <dd>The current version of the workspace. Edits update this immediately.</dd>
            </div>
            <div>
              <dt>Checkpoint</dt>
              <dd>An explicit restore point you create when the live state is worth saving.</dd>
            </div>
          </DefinitionList>

          <Note>
            AFS is not Git. Checkpoints are explicit save points for workspace
            state; Git can still live inside a workspace if your project uses it.
          </Note>
        </DocSection>

        <DocSection id="getting-started">
          <h2>Getting Started</h2>
          <p>
            Start with the CLI if you want files on disk. Start with an SDK if
            an agent or app should create workspaces and mount them in process.
            Both paths end up in the same Redis-backed workspace state.
          </p>

          <h3>The shortest CLI path</h3>
          <NumberedList>
            <li>
              <strong>Sign in.</strong> Authenticate the local CLI to AFS Cloud
              or a Self-managed control plane.
            </li>
            <li>
              <strong>Create a workspace.</strong> The workspace is the shared
              file tree agents and local tools will edit.
            </li>
            <li>
              <strong>Mount it locally.</strong> Run <InlineCode>afs ws mount</InlineCode>{" "}
              so editors, shells, test runners, and agents can use the files.
            </li>
            <li>
              <strong>Checkpoint useful state.</strong> Live edits are immediate;
              checkpoints are the deliberate restore points.
            </li>
          </NumberedList>

          <TerminalBlock
            code={`afs auth login
afs ws create getting-started
afs ws mount getting-started ~/afs/getting-started

echo "hello world" > ~/afs/getting-started/README.md
afs cp create getting-started first-local-edit`}
          />

          <Note>
            The core loop is small: authenticate once, mount a workspace, edit
            files, and checkpoint the state
            worth keeping.
          </Note>
        </DocSection>

        <DocSection id="how-it-works">
          <SectionEyebrow>Core model</SectionEyebrow>
          <h2>How AFS Works</h2>
          <p>
            Every surface talks to the same workspace model. The CLI, browser,
            MCP server, and SDKs may look different, but they all read from and
            write to the Redis-backed live workspace state.
          </p>

          <NumberedList>
            <li>
              <strong>A client chooses a workspace.</strong> That can be a CLI
              command, a browser action, an SDK call, or an MCP tool call.
            </li>
            <li>
              <strong>The control plane resolves the workspace.</strong> It
              applies auth, finds the workspace metadata, and routes the
              operation to the workspace service.
            </li>
            <li>
              <strong>Redis holds the source of truth.</strong> Metadata,
              manifests, file blobs, the live root, checkpoints, and activity
              are stored there.
            </li>
            <li>
              <strong>Local files are a surface, not the source of truth.</strong>{" "}
              Sync mode writes changes back to AFS, and mount mode serves the
              same workspace through a filesystem path.
            </li>
          </NumberedList>

          <p>
            Edits change live state. They do not automatically create a
            checkpoint. Create a checkpoint before a risky change, after a good
            result, or before handing a workspace to another agent.
          </p>
        </DocSection>

        <DocSection id="cli">
          <SectionEyebrow>Daily operation</SectionEyebrow>
          <h2>CLI Workflow</h2>
          <p>
            The CLI owns authentication, workspace mount, local lifecycle,
            config, checkpoints, search, and the built-in MCP server.
          </p>
          <p>
            <ReferenceInlineLink href={referenceDocs[0].href} rel="noreferrer" target="_blank">
              Full CLI command reference
            </ReferenceInlineLink>
          </p>

          <h3>Fresh setup</h3>
          <TerminalBlock
            code={`afs auth login
afs ws mount getting-started ~/getting-started`}
          />

          <h3>Create, import, and start</h3>
          <p>
            Create an empty workspace when you are starting fresh. Import a
            directory when existing local files should become checkpointable.
          </p>
          <TerminalBlock
            code={`# New workspace
afs ws create demo
afs ws mount demo ~/demo

# Existing directory
afs ws import --mount-at-source demo ~/src/demo`}
          />

          <h3>Daily commands</h3>
          <MarkdownTable>
            <thead>
              <tr>
                <th>Command</th>
                <th>Use it for</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <InlineCode>afs status</InlineCode>
                </td>
                <td>Check daemon status, configuration, and local mounts.</td>
              </tr>
              <tr>
                <td>
                  <InlineCode>afs ws list</InlineCode>
                </td>
                <td>See available workspaces.</td>
              </tr>
              <tr>
                <td>
                  <InlineCode>afs ws mount</InlineCode>
                </td>
                <td>Mount a workspace at a local folder.</td>
              </tr>
              <tr>
                <td>
                  <InlineCode>afs cp create</InlineCode>
                </td>
                <td>Save the current live workspace as a restore point.</td>
              </tr>
              <tr>
                <td>
                  <InlineCode>afs fs grep --workspace demo TODO</InlineCode>
                </td>
                <td>Search workspace files directly through AFS.</td>
              </tr>
              <tr>
                <td>
                  <InlineCode>afs ws unmount</InlineCode>
                </td>
                <td>Unmount a workspace while preserving local files by default.</td>
              </tr>
            </tbody>
          </MarkdownTable>

          <h3>Persistent config</h3>
          <TerminalBlock
            code={`afs config get redis.url
afs config set config.source self-managed
afs config set controlPlane.url http://127.0.0.1:8091
afs config set mode sync
afs config list`}
          />
        </DocSection>

        <DocSection id="workspaces">
          <SectionEyebrow>State management</SectionEyebrow>
          <h2>Workspaces and Checkpoints</h2>
          <p>
            Workspaces are the durable unit of collaboration. You create one for
            a project, import one from an existing folder, mount it for local
            work, fork it for parallel work, and checkpoint the states that
            matter.
          </p>

          <TerminalBlock
            code={`afs ws create demo
afs ws import --mount-at-source demo ~/src/demo
afs ws list
afs ws fork demo demo-experiment

afs cp create demo before-refactor
afs cp list demo
afs cp restore demo before-refactor`}
          />

          <MarkdownTable>
            <thead>
              <tr>
                <th>Pattern</th>
                <th>Recommended habit</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Before an agent run</td>
                <td>Create a checkpoint such as <InlineCode>before-agent</InlineCode>.</td>
              </tr>
              <tr>
                <td>Before a risky refactor</td>
                <td>Checkpoint, run the change, then checkpoint the accepted result.</td>
              </tr>
              <tr>
                <td>For parallel experiments</td>
                <td>Fork the workspace and send each agent to a separate fork.</td>
              </tr>
              <tr>
                <td>For handoff</td>
                <td>Checkpoint the current state and share the workspace name.</td>
              </tr>
            </tbody>
          </MarkdownTable>

          <Warning>
            Restoring a checkpoint overwrites the live workspace state. Create a
            fresh checkpoint first if the current state might matter later.
          </Warning>
        </DocSection>

        <DocSection id="local-files">
          <SectionEyebrow>Local surfaces</SectionEyebrow>
          <h2>Sync, Mount, and Local Files</h2>
          <p>
            Sync mode and live mounts are the supported local execution
            surfaces. Sync mode is the recommended default because it gives
            editors, language servers, shell tools, and test runners a real
            local directory.
          </p>

          <h3>Sync mode</h3>
          <TerminalBlock
            code={`afs ws mount demo ~/afs/demo
cd ~/afs/demo`}
          />

          <h3>Live mount mode</h3>
          <TerminalBlock
            code={`afs config set --mode mount --mount-backend nfs
afs ws mount demo ~/afs/demo
afs ws unmount demo`}
          />

          <h3>Import hygiene</h3>
          <p>
            Add a <InlineCode>.afsignore</InlineCode> before importing large
            local projects so dependency caches, build output, logs, and
            machine-local files stay out of the workspace timeline.
          </p>
          <CodeBlock
            code={`node_modules/
.venv/
dist/
*.log
.DS_Store`}
          />
        </DocSection>

        <DocSection id="mcp">
          <SectionEyebrow>Agent access</SectionEyebrow>
          <h2>MCP and Agent Workflows</h2>
          <p>
            <InlineCode>afs mcp</InlineCode> starts the workspace-first MCP
            server over stdio. It is meant to be launched by an MCP client so an
            agent can use workspace-scoped file, search, and checkpoint tools.
          </p>
          <p>
            <ReferenceInlineLink href={referenceDocs[3].href} rel="noreferrer" target="_blank">
              Full MCP tool reference
            </ReferenceInlineLink>
          </p>

          <CodeBlock
            code={`{
  "mcpServers": {
    "afs": {
      "command": "/absolute/path/to/afs",
      "args": ["mcp", "--workspace", "demo", "--profile", "workspace-rw"]
    }
  }
}`}
          />

          <MarkdownTable>
            <thead>
              <tr>
                <th>Profile</th>
                <th>Scope</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <InlineCode>workspace-ro</InlineCode>
                </td>
                <td>Workspace-bound read-only file tools.</td>
              </tr>
              <tr>
                <td>
                  <InlineCode>workspace-rw</InlineCode>
                </td>
                <td>Workspace-bound read/write file tools. This is the default.</td>
              </tr>
              <tr>
                <td>
                  <InlineCode>workspace-rw-checkpoint</InlineCode>
                </td>
                <td>Read/write file tools plus checkpoint operations.</td>
              </tr>
              <tr>
                <td>
                  <InlineCode>admin-ro</InlineCode>
                </td>
                <td>Broad read-only workspace administration.</td>
              </tr>
              <tr>
                <td>
                  <InlineCode>admin-rw</InlineCode>
                </td>
                <td>Broad read/write workspace administration.</td>
              </tr>
            </tbody>
          </MarkdownTable>

          <TerminalBlock
            code={`afs ws create demo
afs cp create demo before-agent
afs mcp --workspace demo --profile workspace-rw-checkpoint`}
          />
        </DocSection>

        <DocSection id="sdk">
          <SectionEyebrow>In-process access</SectionEyebrow>
          <h2>SDKs</h2>
          <p>
            Use the SDKs when an agent application should create workspaces,
            mint workspace-scoped access, edit files, search, checkpoint, and
            run commands without requiring the user to manage a local mount.
          </p>
          <ReferenceLinkRow>
            <ReferenceInlineLink href={referenceDocs[1].href} rel="noreferrer" target="_blank">
              TypeScript command reference
            </ReferenceInlineLink>
            <ReferenceInlineLink href={referenceDocs[2].href} rel="noreferrer" target="_blank">
              Python command reference
            </ReferenceInlineLink>
          </ReferenceLinkRow>

          <h3>TypeScript</h3>
          <TerminalBlock
            code={`npm install redis-afs
export AFS_API_KEY="afs_..."

# Optional for Self-managed control planes
export AFS_API_BASE_URL="http://127.0.0.1:8091"`}
          />
          <CodeBlock code={typescriptSdkSample} language="typescript" />

          <h3>Python</h3>
          <TerminalBlock
            code={`pip install redis-afs
export AFS_API_KEY="afs_..."

# Optional for Self-managed control planes
export AFS_API_BASE_URL="http://127.0.0.1:8091"`}
          />
          <CodeBlock code={pythonSdkSample} language="python" />

          <MarkdownTable>
            <thead>
              <tr>
                <th>SDK helper</th>
                <th>Use it for</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <InlineCode>workspace.create</InlineCode>
                </td>
                <td>Create a Redis-backed workspace.</td>
              </tr>
              <tr>
                <td>
                  <InlineCode>workspace.fork</InlineCode>
                </td>
                <td>Branch a workspace into a separate line of work.</td>
              </tr>
              <tr>
                <td>
                  <InlineCode>checkpoint.create</InlineCode>
                </td>
                <td>Save a deliberate restore point.</td>
              </tr>
              <tr>
                <td>
                  <InlineCode>fs.mount</InlineCode>
                </td>
                <td>Open an isolated in-process mount.</td>
              </tr>
              <tr>
                <td>
                  <InlineCode>fs.grep</InlineCode>
                </td>
                <td>Search file contents without a local mount.</td>
              </tr>
              <tr>
                <td>
                  <InlineCode>fs.bash().exec</InlineCode>
                </td>
                <td>Run shell commands against materialized workspace files.</td>
              </tr>
            </tbody>
          </MarkdownTable>
        </DocSection>

        <DocSection id="deployments">
          <SectionEyebrow>Runtime modes</SectionEyebrow>
          <h2>Deployments</h2>
          <p>
            AFS can run through AFS Cloud, through a Self-managed control plane,
            or in standalone CLI mode directly against Redis.
          </p>

          <MarkdownTable>
            <thead>
              <tr>
                <th>Mode</th>
                <th>Use it when</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Cloud-hosted</td>
                <td>You want browser auth, hosted UI, and managed workspace access.</td>
              </tr>
              <tr>
                <td>Self-managed</td>
                <td>You run your own control plane and UI with your own Redis database.</td>
              </tr>
              <tr>
                <td>Standalone</td>
                <td>You want the CLI to talk directly to Redis without the hosted UI.</td>
              </tr>
            </tbody>
          </MarkdownTable>

          <h3>Local Self-managed development</h3>
          <TerminalBlock
            code={`make web-dev
# control plane: http://127.0.0.1:8091
# Vite UI:      printed by the dev server`}
          />

          <h3>Point the CLI at a control plane</h3>
          <TerminalBlock
            code={`afs config set config.source self-managed
afs config set controlPlane.url http://127.0.0.1:8091
afs ws mount getting-started ~/getting-started`}
          />
        </DocSection>

        <DocSection id="performance">
          <SectionEyebrow>Search</SectionEyebrow>
          <h2>Performance</h2>
          <p>
            Literal <InlineCode>afs fs grep</InlineCode> uses the Redis Search
            indexed path when it is available, then verifies candidate file
            contents through AFS. Regex searches use the non-indexed traversal
            path.
          </p>

          <BenchmarkMeta>
            {searchBenchmark.corpus} on {searchBenchmark.environment}
          </BenchmarkMeta>

          <MarkdownTable>
            <thead>
              <tr>
                <th>Search</th>
                <th>AFS</th>
                <th>BSD grep</th>
                <th>ripgrep</th>
                <th>Read it as</th>
              </tr>
            </thead>
            <tbody>
              {searchBenchmark.metrics.map((metric) => (
                <tr key={metric.name}>
                  <td>{metric.name}</td>
                  <td>{metric.afs}</td>
                  <td>{metric.grep}</td>
                  <td>{metric.ripgrep}</td>
                  <td>{metric.summary}</td>
                </tr>
              ))}
            </tbody>
          </MarkdownTable>

          <TerminalBlock
            code={`afs fs grep --workspace demo "TODO"
afs fs grep -l -i --workspace demo "disk full"
afs fs grep --workspace demo -E "error|warning"`}
          />

          <Note>
            Use <InlineCode>afs fs grep</InlineCode> for ordinary literal searches
            over a Redis-backed workspace. Use <InlineCode>rg</InlineCode> on a
            synced or mounted workspace for regex-heavy scans.
          </Note>
        </DocSection>

      </Article>
    </PageShell>
  );
}

function TerminalBlock({ code }: { code: string }) {
  return (
    <Pre $terminal>
      {code.split("\n").map((line, index) => (
        <TerminalLine key={`${index}-${line}`}>
          {renderTerminalLine(line)}
        </TerminalLine>
      ))}
    </Pre>
  );
}

function CodeBlock({ code, language }: { code: string; language?: CodeLanguage }) {
  return <Pre>{language ? <HighlightedCode code={code} language={language} /> : code}</Pre>;
}

function renderTerminalLine(line: string) {
  if (!line.trim()) {
    return "\u00a0";
  }
  if (line.trimStart().startsWith("#")) {
    return <TerminalComment>{line}</TerminalComment>;
  }
  return (
    <>
      <TerminalPrompt>&gt; </TerminalPrompt>
      <TerminalCommandText>{line}</TerminalCommandText>
    </>
  );
}

const documentMeasure = css`
  width: min(100%, 840px);
`;

const PageShell = styled.div`
  ${documentMeasure}
  margin: 0 auto;
  padding: 36px 32px 72px 52px;
  color: var(--afs-ink, #1f2328);

  @media (max-width: 980px) {
    padding: 24px 18px 56px 28px;
  }
`;

const Article = styled.article`
  h1,
  h2,
  h3,
  p,
  ul,
  ol,
  dl,
  pre,
  table {
    margin-top: 0;
  }

  h1 {
    margin-bottom: 12px;
    color: var(--afs-ink, #1f2328);
    font-size: 36px;
    font-weight: 780;
    letter-spacing: 0;
    line-height: 1.12;
  }

  h2 {
    margin-bottom: 12px;
    color: var(--afs-ink, #1f2328);
    font-size: 24px;
    font-weight: 760;
    letter-spacing: 0;
    line-height: 1.25;
  }

  h3 {
    margin: 26px 0 8px;
    color: var(--afs-ink, #1f2328);
    font-size: 16px;
    font-weight: 760;
    letter-spacing: 0;
    line-height: 1.4;
  }

  p,
  li,
  dd,
  td {
    color: var(--afs-muted, #4e5961);
    font-size: 15px;
    line-height: 1.72;
  }

  p {
    margin-bottom: 14px;
  }

  strong {
    color: var(--afs-ink, #1f2328);
    font-weight: 760;
  }
`;

const ArticleHeader = styled.header`
  display: grid;
  gap: 0;
`;

const AgentDocLink = styled.a`
  display: inline-flex;
  align-items: center;
  gap: 7px;
  width: fit-content;
  margin-bottom: 14px;
  color: var(--afs-accent, #0b6bcb);
  font-family: var(--afs-mono, "SF Mono", "Fira Code", "Cascadia Code", monospace);
  font-size: 13px;
  font-weight: 700;
  line-height: 1.4;
  text-decoration: none;

  svg {
    color: #16a34a;
    flex: 0 0 auto;
  }

  span {
    color: var(--afs-muted, #5f6b73);
    font-weight: 500;
  }

  &:hover {
    text-decoration: underline;
    text-underline-offset: 4px;
  }
`;

const Lead = styled.p`
  max-width: 720px;
  color: var(--afs-muted, #4e5961);
  font-size: 18px;
  line-height: 1.68;
`;

const ContentsSection = styled.nav`
  margin: 28px 0 0;
  padding: 16px 0 0;
  border-top: 1px solid var(--afs-line, #dfe3e6);
  border-bottom: 1px solid var(--afs-line, #dfe3e6);
`;

const ContentsHeading = styled.h2`
  margin: 0 0 12px;
  color: var(--afs-ink, #1f2328);
  font-size: 20px;
  font-weight: 760;
  letter-spacing: 0;
  line-height: 1.3;
`;

const ContentsGrid = styled.div`
  display: grid;
  grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr);
  column-gap: 32px;
  row-gap: 18px;
  margin: 0 0 14px;

  @media (max-width: 640px) {
    grid-template-columns: 1fr;
  }
`;

const ContentsList = styled.ol`
  display: grid;
  gap: 6px;
  margin: 0;
  padding-left: 22px;

  li {
    color: var(--afs-muted, #4e5961);
    font-size: 14px;
    line-height: 1.55;
  }
`;

const ContentsLink = styled.a`
  color: var(--afs-accent, #0b6bcb);
  text-decoration: none;

  &:hover {
    text-decoration: underline;
    text-underline-offset: 4px;
  }
`;

const ReferenceLink = styled.a`
  color: var(--afs-accent, #0b6bcb);
  font-weight: 760;
  text-decoration: none;

  &:hover {
    text-decoration: underline;
    text-underline-offset: 4px;
  }
`;

const ReferenceInlineLink = styled(ReferenceLink)`
  font-size: 14px;
`;

const ReferenceLinkRow = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 10px 18px;
  margin: 0 0 6px;
`;

const DocSection = styled.section`
  padding: 30px 0 0;
  scroll-margin-top: 28px;

  & + & {
    margin-top: 30px;
    border-top: 1px solid var(--afs-line, #dfe3e6);
  }
`;

const SectionEyebrow = styled.div`
  margin-bottom: 8px;
  color: var(--afs-accent, #0b6bcb);
  font-size: 12px;
  font-weight: 760;
  letter-spacing: 0;
  text-transform: uppercase;
`;

const NumberedList = styled.ol`
  display: grid;
  gap: 8px;
  margin-bottom: 18px;
  padding-left: 22px;
`;

const InlineCode = styled.code`
  padding: 2px 5px;
  background: color-mix(in srgb, var(--afs-panel, #f6f8fa) 84%, transparent);
  color: var(--afs-ink, #1f2328);
  font-family: var(--afs-mono, "SF Mono", "Fira Code", "Cascadia Code", monospace);
  font-size: 0.9em;
`;

const Pre = styled.pre<{ $terminal?: boolean }>`
  margin: 16px 0 22px;
  padding: 16px 18px;
  overflow-x: auto;
  border-left: 3px solid
    ${({ $terminal }) => ($terminal ? "#16a34a" : "var(--afs-line, #dfe3e6)")};
  background: ${({ $terminal }) => ($terminal ? "#07130d" : "var(--afs-panel, #f6f8fa)")};
  color: ${({ $terminal }) => ($terminal ? "#f8fafc" : "var(--afs-ink, #1f2328)")};
  font-family: var(--afs-mono, "SF Mono", "Fira Code", "Cascadia Code", monospace);
  font-size: 13px;
  line-height: 1.65;
  white-space: pre;
`;

const TerminalLine = styled.span`
  display: block;
  min-height: 1.65em;
`;

const TerminalPrompt = styled.span`
  color: #22c55e;
  font-weight: 800;
`;

const TerminalCommandText = styled.span`
  color: #f8fafc;
`;

const TerminalComment = styled.span`
  color: #94a3b8;
`;

const Note = styled.p`
  margin: 18px 0 0;
  padding: 0 0 0 16px;
  border-left: 3px solid #16a34a;
  color: var(--afs-muted, #4e5961);
`;

const Warning = styled(Note)`
  border-left-color: #b42318;
`;

const SystemDiagram = styled.figure`
  display: grid;
  gap: 10px;
  margin: 20px 0 22px;
  padding: 16px 0;
  border-top: 1px solid var(--afs-line, #dfe3e6);
  border-bottom: 1px solid var(--afs-line, #dfe3e6);
`;

const DiagramRow = styled.div`
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(136px, 1fr));
  gap: 10px;
`;

const DiagramNode = styled.div`
  min-width: 0;
  padding: 10px 12px;
  border: 1px solid var(--afs-line, #dfe3e6);
  border-radius: 6px;
  background: color-mix(in srgb, var(--afs-panel, #f6f8fa) 78%, transparent);

  strong {
    display: block;
    margin-bottom: 2px;
    color: var(--afs-ink, #1f2328);
    font-size: 13px;
    font-weight: 760;
    line-height: 1.35;
  }

  span {
    display: block;
    color: var(--afs-muted, #4e5961);
    font-size: 13px;
    line-height: 1.45;
  }
`;

const DiagramArrow = styled.div`
  color: var(--afs-muted, #4e5961);
  font-family: var(--afs-mono, "SF Mono", "Fira Code", "Cascadia Code", monospace);
  font-size: 12px;
  font-weight: 700;
  line-height: 1.4;
  text-align: center;
`;

const DiagramCaption = styled.figcaption`
  color: var(--afs-muted, #4e5961);
  font-size: 13px;
  line-height: 1.5;
  text-align: center;
`;

const DefinitionList = styled.dl`
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  column-gap: 28px;
  row-gap: 16px;
  margin: 20px 0 22px;

  div {
    min-width: 0;
  }

  dt {
    margin-bottom: 3px;
    color: var(--afs-ink, #1f2328);
    font-size: 14px;
    font-weight: 760;
    line-height: 1.4;
  }

  dd {
    margin: 0;
  }

  @media (max-width: 680px) {
    grid-template-columns: 1fr;
  }
`;

const MarkdownTable = styled.table`
  width: 100%;
  margin: 18px 0 24px;
  border-collapse: collapse;
  font-size: 14px;

  th {
    padding: 9px 10px 9px 0;
    border-bottom: 1px solid var(--afs-line, #dfe3e6);
    color: var(--afs-ink, #1f2328);
    font-size: 13px;
    font-weight: 760;
    letter-spacing: 0;
    line-height: 1.45;
    text-align: left;
    vertical-align: bottom;
  }

  td {
    padding: 10px 10px 10px 0;
    border-bottom: 1px solid var(--afs-line, #dfe3e6);
    vertical-align: top;
  }

  tr:last-child td {
    border-bottom: 0;
  }

  @media (max-width: 680px) {
    display: block;
    overflow-x: auto;
    white-space: nowrap;
  }
`;

const BenchmarkMeta = styled.p`
  color: var(--afs-muted, #4e5961);
  font-family: var(--afs-mono, "SF Mono", "Fira Code", "Cascadia Code", monospace);
  font-size: 13px;
`;
