import { createFileRoute, Link } from "@tanstack/react-router";
import {
  DocPage,
  DocHero,
  DocHeroTitle,
  DocHeroSub,
  DocSection,
  DocHeading,
  DocSubheading,
  DocProse,
  CodeBlock,
  InlineCode,
  CalloutBox,
  RawFileLink,
  CmdTable,
  CrossLinkCard,
  CrossLinkText,
  CrossLinkTitle,
  CrossLinkDesc,
  CrossLinkArrow,
} from "../components/doc-kit";

export const Route = createFileRoute("/agent-guide")({
  component: AgentGuidePage,
});

function AgentGuidePage() {
  return (
    <DocPage>
      {/* ── Hero ── */}
      <DocHero>
        <DocHeroTitle>Agent Guide</DocHeroTitle>
        <DocHeroSub>
          Everything an AI agent needs to start using Agent Filesystem.
          Configure MCP, create workspaces, read and write files, and manage
          checkpoints.
        </DocHeroSub>
        <div style={{ marginTop: 16, display: "flex", flexWrap: "wrap", gap: 12 }}>
          <RawFileLink href="/agent-guide.md" target="_blank">
            &#128196; View raw Markdown
          </RawFileLink>
        </div>
      </DocHero>

      {/* ── Quick start ── */}
      <DocSection>
        <DocHeading>Quick Start for Agents</DocHeading>
        <DocProse>
          To get an agent working with AFS, point it at the raw guide and tell
          it what to do:
        </DocProse>

        <CalloutBox $tone="tip">
          <DocProse>
            <strong>Tell your agent:</strong>
          </DocProse>
          <CodeBlock>
            <code>{`Read the Agent Filesystem guide at <your-host>/agent-guide.md
and set up a workspace called "my-project". Use the AFS MCP
server to create files, organize the project, and create a
checkpoint when you're done.`}</code>
          </CodeBlock>
        </CalloutBox>

        <DocProse>
          The agent will read the guide, understand the available tools, and
          start working with AFS workspaces autonomously.
        </DocProse>
      </DocSection>

      {/* ── MCP setup ── */}
      <DocSection>
        <DocHeading>MCP Server Configuration</DocHeading>
        <DocProse>
          AFS exposes workspace tools through the Model Context Protocol (MCP).
          Add the following to your agent's MCP configuration (e.g.{" "}
          <InlineCode>claude_desktop_config.json</InlineCode> or{" "}
          <InlineCode>.claude/settings.json</InlineCode>):
        </DocProse>

        <CodeBlock>
          <code>{`{
  "mcpServers": {
    "agent-filesystem": {
      "command": "/absolute/path/to/afs",
      "args": ["mcp"]
    }
  }
}`}</code>
        </CodeBlock>

        <CalloutBox $tone="warn">
          <DocProse>
            The <InlineCode>command</InlineCode> path must be{" "}
            <strong>absolute</strong>. Relative paths like{" "}
            <InlineCode>./afs</InlineCode> will not resolve correctly in most
            MCP hosts.
          </DocProse>
        </CalloutBox>

        <DocSubheading>How it works</DocSubheading>
        <DocProse>
          Running <InlineCode>afs mcp</InlineCode> starts a stdio-based MCP
          server that exposes workspace management tools. The agent communicates
          via JSON-RPC over stdin/stdout. The MCP server reads its configuration
          from <InlineCode>afs.config.json</InlineCode> (next to the binary).
        </DocProse>
      </DocSection>

      {/* ── Available tools ── */}
      <DocSection>
        <DocHeading>Available Workspace Tools</DocHeading>
        <DocProse>
          When connected via MCP, the agent has access to the following
          categories of tools:
        </DocProse>

        <CmdTable>
          <thead>
            <tr>
              <th>Category</th>
              <th>What the agent can do</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td><strong>Workspace management</strong></td>
              <td>
                Create, list, select, fork, and delete workspaces
              </td>
            </tr>
            <tr>
              <td><strong>File operations</strong></td>
              <td>
                Read, write, edit, delete, copy, move, and search files within a
                workspace
              </td>
            </tr>
            <tr>
              <td><strong>Navigation</strong></td>
              <td>
                List directories, tree view, find files by pattern, check
                existence
              </td>
            </tr>
            <tr>
              <td><strong>Search</strong></td>
              <td>
                Grep across workspace files directly in Redis (fast, no local
                mount needed)
              </td>
            </tr>
            <tr>
              <td><strong>Checkpoints</strong></td>
              <td>
                Create named snapshots, list history, restore to any checkpoint
              </td>
            </tr>
          </tbody>
        </CmdTable>

        <CalloutBox $tone="info">
          <DocProse>
            The MCP server is <strong>workspace-first</strong>: file edits are
            automatically tracked and can be checkpointed. The agent does not
            need to manage sync or mounts.
          </DocProse>
        </CalloutBox>
      </DocSection>

      {/* ── Common workflows ── */}
      <DocSection>
        <DocHeading>Common Agent Workflows</DocHeading>

        <DocSubheading>Create a workspace and add files</DocSubheading>
        <DocProse>
          Agents typically start by creating a workspace, then writing files
          into it:
        </DocProse>
        <CodeBlock>
          <code>{`# Via CLI (if agent has shell access)
./afs workspace create my-project
./afs workspace use my-project
./afs up

# Then work in ~/afs/my-project/ with normal file tools
echo "# README" > ~/afs/my-project/README.md`}</code>
        </CodeBlock>

        <DocSubheading>Import an existing directory</DocSubheading>
        <DocProse>
          To bring an existing project into AFS:
        </DocProse>
        <CodeBlock>
          <code>{`./afs workspace import my-project /path/to/existing/directory`}</code>
        </CodeBlock>

        <DocSubheading>Checkpoint before risky changes</DocSubheading>
        <DocProse>
          Always checkpoint before making large or risky modifications. This
          gives the agent a clean rollback point.
        </DocProse>
        <CodeBlock>
          <code>{`./afs checkpoint create before-refactor

# ... make changes ...

# If something goes wrong:
./afs checkpoint restore before-refactor`}</code>
        </CodeBlock>

        <DocSubheading>Search without mounting</DocSubheading>
        <DocProse>
          Agents can search workspace contents directly in Redis without needing
          a local mount:
        </DocProse>
        <CodeBlock>
          <code>{`./afs grep "TODO" --workspace my-project
./afs grep --workspace my-project --path /src -E "function|class"`}</code>
        </CodeBlock>

        <DocSubheading>Fork a workspace for parallel work</DocSubheading>
        <DocProse>
          Multiple agents can work in parallel by forking a workspace:
        </DocProse>
        <CodeBlock>
          <code>{`./afs workspace fork my-project my-project-experiment`}</code>
        </CodeBlock>
      </DocSection>

      {/* ── Configuration ── */}
      <DocSection>
        <DocHeading>Configuration Reference</DocHeading>
        <DocProse>
          AFS reads its configuration from{" "}
          <InlineCode>afs.config.json</InlineCode>, located next to the{" "}
          <InlineCode>afs</InlineCode> binary. Key fields:
        </DocProse>
        <CodeBlock>
          <code>{`{
  "redis": {
    "addr": "localhost:6379",
    "username": "",
    "password": "",
    "db": 0,
    "tls": false
  },
  "mode": "sync",
  "currentWorkspace": "my-project",
  "localPath": "~/afs"
}`}</code>
        </CodeBlock>
        <CmdTable>
          <thead>
            <tr>
              <th>Field</th>
              <th>Description</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td><InlineCode>redis.addr</InlineCode></td>
              <td>Redis host:port</td>
            </tr>
            <tr>
              <td><InlineCode>redis.password</InlineCode></td>
              <td>Redis auth password (empty for no auth)</td>
            </tr>
            <tr>
              <td><InlineCode>redis.tls</InlineCode></td>
              <td>Enable TLS connection</td>
            </tr>
            <tr>
              <td><InlineCode>mode</InlineCode></td>
              <td>
                <InlineCode>sync</InlineCode> (recommended),{" "}
                <InlineCode>mount</InlineCode>, or{" "}
                <InlineCode>none</InlineCode>
              </td>
            </tr>
            <tr>
              <td><InlineCode>currentWorkspace</InlineCode></td>
              <td>Default workspace name</td>
            </tr>
            <tr>
              <td><InlineCode>localPath</InlineCode></td>
              <td>Local directory for sync/mount</td>
            </tr>
          </tbody>
        </CmdTable>
      </DocSection>

      {/* ── Best practices ── */}
      <DocSection>
        <DocHeading>Best Practices</DocHeading>
        <CmdTable>
          <thead>
            <tr>
              <th>Practice</th>
              <th>Why</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>Checkpoint before risky operations</td>
              <td>Gives you an instant rollback point</td>
            </tr>
            <tr>
              <td>Use descriptive workspace names</td>
              <td>Makes it easy to identify workspaces in the UI and CLI</td>
            </tr>
            <tr>
              <td>Use sync mode (not mount) for most workflows</td>
              <td>Better compatibility with local tools and IDEs</td>
            </tr>
            <tr>
              <td>
                Add <InlineCode>.afsignore</InlineCode> for imports
              </td>
              <td>
                Exclude <InlineCode>node_modules/</InlineCode>,{" "}
                <InlineCode>.venv/</InlineCode>, build artifacts
              </td>
            </tr>
            <tr>
              <td>Use MCP for agent-only workflows</td>
              <td>No local mount needed; tools work directly with Redis</td>
            </tr>
            <tr>
              <td>Fork workspaces for parallel experiments</td>
              <td>Keeps the main workspace clean while agents explore</td>
            </tr>
          </tbody>
        </CmdTable>
      </DocSection>

      {/* ── Cross-link to Docs ── */}
      <CrossLinkCard as={Link} to="/docs">
        <CrossLinkText>
          <CrossLinkTitle>New to AFS?</CrossLinkTitle>
          <CrossLinkDesc>
            Read the full getting-started guide for installation, setup, and
            core concepts.
          </CrossLinkDesc>
        </CrossLinkText>
        <CrossLinkArrow>&rarr;</CrossLinkArrow>
      </CrossLinkCard>
    </DocPage>
  );
}
