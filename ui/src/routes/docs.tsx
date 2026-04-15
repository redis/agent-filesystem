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
  Step,
  CmdTable,
  CrossLinkCard,
  CrossLinkText,
  CrossLinkTitle,
  CrossLinkDesc,
  CrossLinkArrow,
} from "../components/doc-kit";

export const Route = createFileRoute("/docs")({
  component: DocsPage,
});

function DocsPage() {
  return (
    <DocPage>
      {/* ── Hero ── */}
      <DocHero>
        <DocHeroTitle>Getting Started with Agent Filesystem</DocHeroTitle>
        <DocHeroSub>
          AFS gives every agent a persistent, checkpointed workspace backed by
          Redis. This guide walks you through the core concepts and gets you up
          and running.
        </DocHeroSub>
      </DocHero>

      {/* ── What is AFS ── */}
      <DocSection>
        <DocHeading>What is AFS?</DocHeading>
        <DocProse>
          Agent Filesystem (AFS) is a workspace system for AI agents, backed by
          Redis. It gives agents a filesystem-shaped way to work with data
          without being tied to one machine's local disk.
        </DocProse>
        <DocProse>
          Agents already know how to read files, write files, search trees, and
          work in directories. AFS takes that familiar interface and adds
          checkpointing, forking, and remote persistence so workspace state
          survives restarts, moves between machines, and can be rolled back at
          any time.
        </DocProse>
        <CalloutBox $tone="info">
          <DocProse>
            <strong>In short:</strong> AFS is a workspace system for agents,
            backed by Redis, with real directories for real tools.
          </DocProse>
        </CalloutBox>
      </DocSection>

      {/* ── How it works ── */}
      <DocSection>
        <DocHeading>How It Works</DocHeading>
        <DocProse>
          The architecture has four layers. Redis is the canonical source of
          truth; everything else reads from or writes to it.
        </DocProse>

        <DocSubheading>Data flow</DocSubheading>
        <CodeBlock>
          <code>{`  AI Agents (Claude, GPT, custom)
        │
        ▼
  AFS CLI / MCP Server          ◀── agents interact here
        │
        ▼
  Redis (source of truth)       ◀── workspaces, files, checkpoints
        │
        ▼
  Web UI (this app)             ◀── browse, monitor, manage`}</code>
        </CodeBlock>

        <DocSubheading>Operating modes</DocSubheading>
        <DocProse>
          <strong>Sync mode (recommended)</strong> — Redis is canonical. AFS
          keeps a local directory in sync with the workspace in Redis. Agents
          work in the local directory with normal tools; changes sync back to
          Redis automatically.
        </DocProse>
        <DocProse>
          <strong>Mount mode</strong> — AFS exposes the Redis-backed workspace
          as a POSIX filesystem via FUSE (Linux) or NFS (macOS). No local copy;
          reads and writes go directly to Redis.
        </DocProse>
        <DocProse>
          <strong>MCP mode</strong> — Agents interact with workspaces through
          MCP tools exposed by{" "}
          <InlineCode>afs mcp</InlineCode>. No local directory
          needed.
        </DocProse>
      </DocSection>

      {/* ── Deployment modes ── */}
      <DocSection>
        <DocHeading>Deployment Modes</DocHeading>
        <DocProse>
          AFS supports two deployment modes depending on your needs.
        </DocProse>

        <DocSubheading>Managed mode (Docker)</DocSubheading>
        <DocProse>
          Docker Compose runs the full stack: Redis, the control plane, and the
          web UI — all pre-configured. Best for teams, demos, or when you want
          the full visual management experience. The CLI can connect to the
          Docker Redis for local sync and mount.
        </DocProse>
        <CodeBlock>
          <code>{`docker compose up
# Open http://localhost:8091`}</code>
        </CodeBlock>

        <DocSubheading>Local mode (CLI only)</DocSubheading>
        <DocProse>
          Run the <InlineCode>afs</InlineCode> CLI directly against your own
          Redis instance. No control plane or web UI required. Best for
          single-developer or agent-only workflows where you just need
          workspaces, sync, and checkpoints.
        </DocProse>
        <CodeBlock>
          <code>{`afs setup          # configure Redis connection
afs workspace create my-project
afs up             # start syncing`}</code>
        </CodeBlock>

        <CalloutBox $tone="tip">
          <DocProse>
            See the{" "}
            <Link to="/downloads" style={{ color: "var(--afs-accent)" }}>
              Downloads page
            </Link>{" "}
            to download a pre-built CLI binary.
          </DocProse>
        </CalloutBox>
      </DocSection>

      {/* ── Getting started: Managed ── */}
      <DocSection>
        <DocHeading>Getting Started — Managed Mode</DocHeading>
        <DocProse>
          The fastest path. Docker Compose handles Redis, the control plane, and
          the web UI.
        </DocProse>

        <Step n={1} title="Start the stack">
          <CodeBlock>
            <code>{`git clone <repo-url>
cd agent-filesystem
docker compose up`}</code>
          </CodeBlock>
        </Step>

        <Step n={2} title="Open the web UI">
          Navigate to{" "}
          <InlineCode>http://localhost:8091</InlineCode>.
          Add the Docker Redis as a database at{" "}
          <InlineCode>redis:6379</InlineCode>{" "}
          (or <InlineCode>localhost:6379</InlineCode> from the host).
        </Step>

        <Step n={3} title="Create a workspace">
          Use the web UI to create your first workspace, or connect the CLI:
          <CodeBlock>
            <code>{`afs setup   # point at localhost:6379
afs workspace create my-project
afs up`}</code>
          </CodeBlock>
        </Step>

        <Step n={4} title="Manage from the UI">
          Browse files, create checkpoints, view agent sessions, and track
          activity — all from the web dashboard.
        </Step>
      </DocSection>

      {/* ── Getting started: Local ── */}
      <DocSection>
        <DocHeading>Getting Started — Local Mode</DocHeading>
        <DocProse>
          Build from source and use the CLI directly. Bring your own Redis.
        </DocProse>

        <Step n={1} title="Prerequisites">
          <CmdTable>
            <thead>
              <tr>
                <th>Requirement</th>
                <th>Version</th>
                <th>Notes</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>Go</td>
                <td>1.22.2+</td>
                <td>Required for CLI and control plane</td>
              </tr>
              <tr>
                <td>Node.js</td>
                <td>20+</td>
                <td>Only needed if building the web UI</td>
              </tr>
              <tr>
                <td>Redis</td>
                <td>7+</td>
                <td>Running instance (local or remote)</td>
              </tr>
              <tr>
                <td>C compiler</td>
                <td>gcc or clang</td>
                <td>Only needed for the optional Redis module</td>
              </tr>
            </tbody>
          </CmdTable>
        </Step>

        <Step n={2} title="Build AFS">
          <CodeBlock>
            <code>{`git clone <repo-url>
cd agent-filesystem

# CLI only (no UI):
make afs

# CLI + control plane with embedded UI:
make commands

# Everything (CLI, control plane, Redis module):
make all`}</code>
          </CodeBlock>
        </Step>

        <Step n={3} title="Install to PATH">
          <CodeBlock>
            <code>{`make install`}</code>
          </CodeBlock>
          <DocProse>
            Installs <InlineCode>afs</InlineCode> to{" "}
            <InlineCode>/usr/local/bin</InlineCode> so you can run it from
            anywhere.
          </DocProse>
        </Step>

        <Step n={4} title="Run setup">
          The interactive setup wizard configures your Redis connection and local
          sync directory.
          <CodeBlock>
            <code>{`afs setup`}</code>
          </CodeBlock>
          <CalloutBox $tone="tip">
            <DocProse>
              This creates <InlineCode>afs.config.json</InlineCode> next to the
              binary. You can edit it directly later.
            </DocProse>
          </CalloutBox>
        </Step>

        <Step n={5} title="Create a workspace and start syncing">
          <CodeBlock>
            <code>{`afs workspace create my-project
afs up`}</code>
          </CodeBlock>
          <DocProse>
            Your workspace is now at{" "}
            <InlineCode>~/afs/my-project/</InlineCode>. Use normal tools to
            work with files — changes sync to Redis automatically.
          </DocProse>
        </Step>

        <Step n={6} title="Create checkpoints">
          Save a named snapshot. You can restore to any checkpoint later.
          <CodeBlock>
            <code>{`afs checkpoint create before-refactor`}</code>
          </CodeBlock>
        </Step>
      </DocSection>

      {/* ── CLI reference ── */}
      <DocSection>
        <DocHeading>Key CLI Commands</DocHeading>
        <CmdTable>
          <thead>
            <tr>
              <th>Command</th>
              <th>Description</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>
                <InlineCode>afs setup</InlineCode>
              </td>
              <td>Interactive first-time configuration</td>
            </tr>
            <tr>
              <td>
                <InlineCode>afs up</InlineCode>
              </td>
              <td>Start syncing (or mounting) workspaces</td>
            </tr>
            <tr>
              <td>
                <InlineCode>afs down</InlineCode>
              </td>
              <td>Stop services and unmount</td>
            </tr>
            <tr>
              <td>
                <InlineCode>afs status</InlineCode>
              </td>
              <td>Show current status</td>
            </tr>
            <tr>
              <td>
                <InlineCode>afs workspace create &lt;name&gt;</InlineCode>
              </td>
              <td>Create a new workspace</td>
            </tr>
            <tr>
              <td>
                <InlineCode>afs workspace list</InlineCode>
              </td>
              <td>List all workspaces</td>
            </tr>
            <tr>
              <td>
                <InlineCode>afs workspace use &lt;name&gt;</InlineCode>
              </td>
              <td>Set the current workspace</td>
            </tr>
            <tr>
              <td>
                <InlineCode>
                  afs workspace import &lt;name&gt; &lt;dir&gt;
                </InlineCode>
              </td>
              <td>Import existing directory as a workspace</td>
            </tr>
            <tr>
              <td>
                <InlineCode>afs workspace fork &lt;name&gt; &lt;new&gt;</InlineCode>
              </td>
              <td>Fork workspace for parallel work</td>
            </tr>
            <tr>
              <td>
                <InlineCode>afs checkpoint create &lt;name&gt;</InlineCode>
              </td>
              <td>Save current state as a checkpoint</td>
            </tr>
            <tr>
              <td>
                <InlineCode>afs checkpoint list</InlineCode>
              </td>
              <td>List all checkpoints</td>
            </tr>
            <tr>
              <td>
                <InlineCode>afs checkpoint restore &lt;name&gt;</InlineCode>
              </td>
              <td>Restore workspace to a checkpoint</td>
            </tr>
            <tr>
              <td>
                <InlineCode>afs mcp</InlineCode>
              </td>
              <td>Start the MCP server for agent integration</td>
            </tr>
            <tr>
              <td>
                <InlineCode>afs grep &lt;pattern&gt;</InlineCode>
              </td>
              <td>Search workspace files directly in Redis</td>
            </tr>
          </tbody>
        </CmdTable>
      </DocSection>

      {/* ── Cross-link to Agent Guide ── */}
      <CrossLinkCard as={Link} to="/agent-guide">
        <CrossLinkText>
          <CrossLinkTitle>Setting up for AI agents?</CrossLinkTitle>
          <CrossLinkDesc>
            See the Agent Guide for MCP configuration, available tools, and
            agent-specific workflows.
          </CrossLinkDesc>
        </CrossLinkText>
        <CrossLinkArrow>&rarr;</CrossLinkArrow>
      </CrossLinkCard>
    </DocPage>
  );
}
