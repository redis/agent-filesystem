import { createFileRoute, Link } from "@tanstack/react-router";
import { Button, Typography } from "@redis-ui/components";
import { useMemo, useState } from "react";
import styled from "styled-components";
import { PageStack } from "../components/afs-kit";
import { SurfaceCard } from "../components/card-shell";
import { getControlPlaneURL } from "../foundation/api/afs";

export const Route = createFileRoute("/mcp_/connect")({
  component: ConnectMCPPage,
});

type Client = "claude-code" | "codex";

function ConnectMCPPage() {
  const [client, setClient] = useState<Client>("claude-code");
  const endpoint = useMemo(
    () => `${getControlPlaneURL().replace(/\/+$/, "")}/mcp`,
    [],
  );

  return (
    <PageStack>
      <BackRow>
        <Link to="/mcp">
          <BackLink>← Back to MCP</BackLink>
        </Link>
      </BackRow>

      <Header>
        <Typography.Heading size="L">Connect an agent to AFS</Typography.Heading>
        <Lede>
          Every agent connects to the same MCP endpoint with a bearer token.
          Pick your client below for step-by-step instructions.
        </Lede>
      </Header>

      <EndpointBlock>
        <EndpointLabel>Endpoint</EndpointLabel>
        <EndpointValue>{endpoint}</EndpointValue>
      </EndpointBlock>

      <ClientTabs role="tablist">
        <ClientTab
          role="tab"
          aria-selected={client === "claude-code"}
          $selected={client === "claude-code"}
          onClick={() => setClient("claude-code")}
        >
          Claude Code
        </ClientTab>
        <ClientTab
          role="tab"
          aria-selected={client === "codex"}
          $selected={client === "codex"}
          onClick={() => setClient("codex")}
        >
          Codex
        </ClientTab>
      </ClientTabs>

      {client === "claude-code" ? (
        <ClaudeCodeInstructions endpoint={endpoint} />
      ) : (
        <CodexInstructions endpoint={endpoint} />
      )}

      <Footer>
        <Link to="/mcp">
          <Button size="medium" variant="secondary-fill">
            Back to MCP
          </Button>
        </Link>
      </Footer>
    </PageStack>
  );
}

/* -------------------------------------------------------------------------- */
/* Claude Code                                                                */
/* -------------------------------------------------------------------------- */

function ClaudeCodeInstructions({ endpoint }: { endpoint: string }) {
  const cliAdd = `claude mcp add --scope user --transport http agent-filesystem \\
  '${endpoint}' \\
  --header 'Authorization: Bearer <PASTE_TOKEN>'`;

  const jsonConfig = JSON.stringify(
    {
      mcpServers: {
        "agent-filesystem": {
          url: endpoint,
          headers: {
            Authorization: "Bearer <PASTE_TOKEN>",
          },
        },
      },
    },
    null,
    2,
  );

  return (
    <Card>
      <Subheading>1. Issue a token</Subheading>
      <Body>
        On the <Link to="/mcp"><InlineLink>MCP page</InlineLink></Link>, click
        <strong> Create token</strong>. Pick <strong>Control plane</strong> to
        let the agent manage workspaces itself, or <strong>Workspace</strong>{" "}
        if you want the agent scoped to a single workspace. Copy the token
        from the success dialog — it's shown once.
      </Body>

      <Subheading>2. Register the server</Subheading>
      <Body>
        Run this in your terminal, substituting the token you just copied:
      </Body>
      <CodeBlock label="Terminal">{cliAdd}</CodeBlock>
      <Hint>
        <code>--scope user</code> installs it for every Claude Code session on
        this machine. Use <code>--scope project</code> if you only want it in
        the current working directory.
      </Hint>

      <Subheading>Alternative — edit the config file directly</Subheading>
      <Body>
        If you prefer editing JSON, add this to{" "}
        <code>~/.claude/mcp.json</code>:
      </Body>
      <CodeBlock label="~/.claude/mcp.json">{jsonConfig}</CodeBlock>

      <Subheading>3. Verify</Subheading>
      <Body>
        Open Claude Code, run <code>/mcp</code>, and confirm{" "}
        <code>agent-filesystem</code> shows as connected. If you used a
        control-plane token, ask Claude "what workspaces do I have?" — it
        should call <code>workspace_list</code> and report back.
      </Body>

      <Subheading>Note on Claude Code desktop</Subheading>
      <Body>
        The <code>/plugin</code> manager isn't in the desktop app yet, but
        MCP servers registered via the CLI work in both the terminal and
        desktop app. Install once via CLI, use anywhere.
      </Body>
    </Card>
  );
}

/* -------------------------------------------------------------------------- */
/* Codex                                                                      */
/* -------------------------------------------------------------------------- */

function CodexInstructions({ endpoint }: { endpoint: string }) {
  const cliAdd = `codex mcp add agent-filesystem \\
  --transport http '${endpoint}' \\
  --bearer-token-env AFS_TOKEN`;

  const tomlConfig = `[mcp_servers.agent-filesystem]
url = "${endpoint}"
bearer_token_env_var = "AFS_TOKEN"`;

  return (
    <Card>
      <Subheading>1. Issue a token</Subheading>
      <Body>
        On the <Link to="/mcp"><InlineLink>MCP page</InlineLink></Link>, click
        <strong> Create token</strong>. Pick <strong>Control plane</strong> for
        workspace management tools, or <strong>Workspace</strong> for
        file-tools scoped to one workspace. Copy the token from the success
        dialog — it's shown once.
      </Body>

      <Subheading>2. Export the token as an environment variable</Subheading>
      <Body>
        Codex reads bearer tokens from env vars so they don't land in config
        files or shell history. Add this to your shell profile:
      </Body>
      <CodeBlock label="~/.zshrc or ~/.bashrc">{`export AFS_TOKEN='<PASTE_TOKEN>'`}</CodeBlock>

      <Subheading>3. Register the server</Subheading>
      <CodeBlock label="Terminal">{cliAdd}</CodeBlock>

      <Subheading>Alternative — edit the config file directly</Subheading>
      <Body>
        Add this to <code>~/.codex/config.toml</code>:
      </Body>
      <CodeBlock label="~/.codex/config.toml">{tomlConfig}</CodeBlock>

      <Subheading>4. Verify</Subheading>
      <Body>
        Start a Codex session, run <code>codex mcp list</code> (or{" "}
        <code>/mcp</code> inside the app), and confirm{" "}
        <code>agent-filesystem</code> is connected. Works in Codex CLI, Codex
        app, and the VS Code extension.
      </Body>
    </Card>
  );
}

/* -------------------------------------------------------------------------- */
/* Shared building blocks                                                     */
/* -------------------------------------------------------------------------- */

function CodeBlock({ label, children }: { label?: string; children: string }) {
  const [copied, setCopied] = useState(false);

  function copy() {
    void navigator.clipboard.writeText(children).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    });
  }

  return (
    <CodeBlockWrap>
      <CodeBlockHead>
        <CodeBlockLabel>{label ?? "snippet"}</CodeBlockLabel>
        <Button size="small" variant="secondary-fill" onClick={copy}>
          {copied ? "Copied!" : "Copy"}
        </Button>
      </CodeBlockHead>
      <Pre>{children}</Pre>
    </CodeBlockWrap>
  );
}

/* ── Styled ── */

const BackRow = styled.div`
  margin-bottom: -4px;
`;

const BackLink = styled.span`
  color: var(--afs-muted);
  font-size: 13px;
  cursor: pointer;

  &:hover {
    color: var(--afs-accent, #2563eb);
  }
`;

const Header = styled.div`
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-width: 78ch;
`;

const Lede = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 15px;
  line-height: 1.55;
`;

const EndpointBlock = styled.div`
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 16px;
  border: 1px solid var(--afs-line);
  border-radius: 12px;
  background: var(--afs-panel);
`;

const EndpointLabel = styled.div`
  color: var(--afs-muted);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  flex-shrink: 0;
`;

const EndpointValue = styled.code`
  color: var(--afs-ink);
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 14px;
  overflow-x: auto;
`;

const ClientTabs = styled.div`
  display: inline-flex;
  gap: 4px;
  padding: 4px;
  border: 1px solid var(--afs-line);
  border-radius: 12px;
  background: var(--afs-panel);
  align-self: flex-start;
`;

const ClientTab = styled.button<{ $selected: boolean }>`
  padding: 8px 16px;
  border-radius: 8px;
  border: 0;
  background: ${({ $selected }) =>
    $selected ? "var(--afs-accent, #2563eb)" : "transparent"};
  color: ${({ $selected }) => ($selected ? "#fff" : "var(--afs-ink)")};
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  transition: background 120ms ease, color 120ms ease;

  &:hover {
    background: ${({ $selected }) =>
      $selected
        ? "var(--afs-accent, #2563eb)"
        : "color-mix(in srgb, var(--afs-accent, #2563eb) 12%, transparent)"};
  }
`;

const Card = styled(SurfaceCard)`
  padding: 28px 32px;
  display: flex;
  flex-direction: column;
  gap: 10px;
`;

const Subheading = styled.h3`
  margin: 16px 0 4px;
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;

  &:first-child {
    margin-top: 0;
  }
`;

const Body = styled.p`
  margin: 0;
  color: var(--afs-ink-soft, var(--afs-ink));
  font-size: 14px;
  line-height: 1.6;

  code {
    font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    font-size: 12.5px;
    padding: 1px 6px;
    border-radius: 6px;
    border: 1px solid var(--afs-line);
    background: var(--afs-panel);
  }
`;

const Hint = styled.p`
  margin: 4px 0 0;
  color: var(--afs-muted);
  font-size: 12.5px;
  line-height: 1.55;

  code {
    font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    font-size: 11.5px;
    padding: 1px 5px;
    border-radius: 5px;
    border: 1px solid var(--afs-line);
    background: var(--afs-panel);
  }
`;

const InlineLink = styled.span`
  color: var(--afs-accent, #2563eb);
  text-decoration: underline;
`;

const CodeBlockWrap = styled.div`
  display: flex;
  flex-direction: column;
  gap: 0;
  margin: 6px 0;
  border: 1px solid var(--afs-line);
  border-radius: 12px;
  overflow: hidden;
  background: rgba(15, 23, 42, 0.94);
`;

const CodeBlockHead = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  background: rgba(15, 23, 42, 0.98);
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
`;

const CodeBlockLabel = styled.span`
  color: #94a3b8;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
`;

const Pre = styled.pre`
  margin: 0;
  padding: 14px 16px;
  color: #e2e8f0;
  font-size: 12.5px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  overflow: auto;
  max-height: 280px;
`;

const Footer = styled.div`
  display: flex;
  justify-content: flex-end;
  padding-top: 8px;
`;
