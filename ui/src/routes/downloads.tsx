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

export const Route = createFileRoute("/downloads")({
  component: DownloadsPage,
});

function DownloadsPage() {
  return (
    <DocPage>
      {/* ── Hero ── */}
      <DocHero>
        <DocHeroTitle>Get AFS Running</DocHeroTitle>
        <DocHeroSub>
          The fastest way to get started is Docker Compose — one command gives
          you Redis, the control plane, and the full web UI. You can also build
          from source for local CLI usage.
        </DocHeroSub>
      </DocHero>

      {/* ── Docker (primary) ── */}
      <DocSection>
        <DocHeading>Quickstart with Docker</DocHeading>
        <DocProse>
          Docker Compose runs the full AFS stack: a Redis instance, the control
          plane server, and the embedded web UI. Everything is pre-configured to
          work together.
        </DocProse>

        <CalloutBox $tone="tip">
          <DocProse>
            <strong>This is the recommended way to get started.</strong> No build
            tools required — just Docker.
          </DocProse>
        </CalloutBox>

        <Step n={1} title="Prerequisites">
          Install{" "}
          <a href="https://docs.docker.com/get-docker/" target="_blank" rel="noreferrer">
            Docker
          </a>{" "}
          and{" "}
          <a href="https://docs.docker.com/compose/install/" target="_blank" rel="noreferrer">
            Docker Compose
          </a>{" "}
          if you don't have them already.
        </Step>

        <Step n={2} title="Clone and start">
          <CodeBlock>
            <code>{`git clone <repo-url>
cd agent-filesystem
docker compose up`}</code>
          </CodeBlock>
          <DocProse>
            Docker builds the AFS image (first run takes a few minutes), starts
            Redis, and launches the control plane with the embedded UI.
          </DocProse>
        </Step>

        <Step n={3} title="Open the UI">
          <DocProse>
            Navigate to{" "}
            <a href="http://localhost:8091" target="_blank" rel="noreferrer">
              <InlineCode>http://localhost:8091</InlineCode>
            </a>{" "}
            in your browser. You'll see the AFS dashboard. Add the Docker Redis
            as a database (it's already running at{" "}
            <InlineCode>localhost:6379</InlineCode>).
          </DocProse>
        </Step>

        <Step n={4} title="Connect the CLI (optional)">
          <DocProse>
            If you also want to use the <InlineCode>afs</InlineCode> CLI on your
            host machine, point it at the Docker Redis:
          </DocProse>
          <CodeBlock>
            <code>{`./afs setup
# When prompted for Redis address, enter: localhost:6379`}</code>
          </CodeBlock>
        </Step>
      </DocSection>

      {/* ── Build from source ── */}
      <DocSection>
        <DocHeading>Build from Source</DocHeading>
        <DocProse>
          For local CLI usage or development, build AFS directly on your
          machine. This gives you the <InlineCode>afs</InlineCode> CLI and
          optionally the control plane server.
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

        <Step n={2} title="Build">
          <CodeBlock>
            <code>{`git clone <repo-url>
cd agent-filesystem

# CLI only (no UI):
make afs

# CLI + control plane with embedded UI:
make commands`}</code>
          </CodeBlock>
        </Step>

        <Step n={3} title="Setup and run">
          <CodeBlock>
            <code>{`# Interactive setup (configures Redis connection)
./afs setup

# Create a workspace
./afs workspace create my-project

# Start syncing
./afs up`}</code>
          </CodeBlock>
        </Step>

        <Step n={4} title="Run the web UI (optional)">
          <DocProse>
            For development, run the control plane and UI together:
          </DocProse>
          <CodeBlock>
            <code>{`make web-dev`}</code>
          </CodeBlock>
          <DocProse>
            Or run the control plane with the embedded UI:
          </DocProse>
          <CodeBlock>
            <code>{`make afs-control-plane
./afs-control-plane --listen 0.0.0.0:8091`}</code>
          </CodeBlock>
        </Step>
      </DocSection>

      {/* ── System requirements ── */}
      <DocSection>
        <DocHeading>Supported Platforms</DocHeading>
        <CmdTable>
          <thead>
            <tr>
              <th>Platform</th>
              <th>CLI</th>
              <th>Mount (sync)</th>
              <th>Mount (FUSE/NFS)</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>macOS (Apple Silicon)</td>
              <td>&#10003;</td>
              <td>&#10003;</td>
              <td>&#10003; (NFS)</td>
            </tr>
            <tr>
              <td>macOS (Intel)</td>
              <td>&#10003;</td>
              <td>&#10003;</td>
              <td>&#10003; (NFS)</td>
            </tr>
            <tr>
              <td>Linux (x86_64)</td>
              <td>&#10003;</td>
              <td>&#10003;</td>
              <td>&#10003; (FUSE)</td>
            </tr>
            <tr>
              <td>Linux (ARM64)</td>
              <td>&#10003;</td>
              <td>&#10003;</td>
              <td>&#10003; (FUSE)</td>
            </tr>
            <tr>
              <td>Docker</td>
              <td>&#10003;</td>
              <td>&#10003;</td>
              <td>&#8212;</td>
            </tr>
          </tbody>
        </CmdTable>
      </DocSection>

      {/* ── Cross-links ── */}
      <CrossLinkCard as={Link} to="/docs">
        <CrossLinkText>
          <CrossLinkTitle>Learn the concepts</CrossLinkTitle>
          <CrossLinkDesc>
            Architecture overview, deployment modes, and getting-started
            walkthrough.
          </CrossLinkDesc>
        </CrossLinkText>
        <CrossLinkArrow>&rarr;</CrossLinkArrow>
      </CrossLinkCard>

      <CrossLinkCard as={Link} to="/agent-guide">
        <CrossLinkText>
          <CrossLinkTitle>Set up for AI agents</CrossLinkTitle>
          <CrossLinkDesc>
            MCP configuration, available tools, and agent-specific workflows.
          </CrossLinkDesc>
        </CrossLinkText>
        <CrossLinkArrow>&rarr;</CrossLinkArrow>
      </CrossLinkCard>
    </DocPage>
  );
}
