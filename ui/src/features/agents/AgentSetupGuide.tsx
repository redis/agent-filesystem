import { useState } from "react";
import { Link } from "@tanstack/react-router";
import styled from "styled-components";
import {
  InlineCode,
  CrossLinkCard,
  CrossLinkText,
  CrossLinkTitle,
  CrossLinkDesc,
  CrossLinkArrow,
} from "../../components/doc-kit";
import { getControlPlaneURL } from "../../foundation/api/afs";

type Props = {
  compact?: boolean;
};

export function AgentSetupGuide({ compact = false }: Props) {
  const [copied, setCopied] = useState<string | null>(null);
  const controlPlaneUrl = getControlPlaneURL();
  const cliPath = `./afs`;
  const downloadCmd = `curl -fsSL "${controlPlaneUrl}/v1/cli?os=$(uname -s)&arch=$(uname -m)" -o "${cliPath}" && chmod +x "${cliPath}"`;
  const configCmd = `${cliPath} config --control-plane-url "${controlPlaneUrl}"`;

  const mcpConfig = JSON.stringify(
    {
      mcpServers: {
        "agent-filesystem": {
          command: "afs",
          args: ["mcp"],
        },
      },
    },
    null,
    2,
  );

  function copyToClipboard(text: string, label: string) {
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(label);
      setTimeout(() => setCopied(null), 2000);
    });
  }

  return (
    <Layout $compact={compact}>
      <SetupCard>
        <CardLabel>Recommended</CardLabel>
        <SetupTitle>Connect via CLI</SetupTitle>
        <SetupDesc>
          Use the <InlineCode>afs</InlineCode> CLI to mount a workspace as a
          local directory. The agent reads and writes files normally — AFS syncs
          everything to Redis in the background.
        </SetupDesc>

        <StepLabel>Step 1 — Download the CLI</StepLabel>
        <SetupDesc>
          Download the latest compatible <InlineCode>afs</InlineCode> binary for
          your machine. The command auto-detects OS and CPU architecture.
        </SetupDesc>
        <CodeContainer>
          <CodePre>{downloadCmd}</CodePre>
          <CopyButton type="button" onClick={() => copyToClipboard(downloadCmd, "download")}>
            {copied === "download" ? "Copied!" : "Copy"}
          </CopyButton>
        </CodeContainer>

        <StepDivider />

        <StepLabel>Step 2 — Configure the connection</StepLabel>
        <SetupDesc>
          Point the CLI at this control plane server. You only need to do this once per machine.
        </SetupDesc>
        <CodeContainer>
          <CodePre>{configCmd}</CodePre>
          <CopyButton type="button" onClick={() => copyToClipboard(configCmd, "config")}>
            {copied === "config" ? "Copied!" : "Copy"}
          </CopyButton>
        </CodeContainer>

        <StepDivider />

        <StepLabel>Step 3 — Select a workspace and start syncing</StepLabel>
        <SetupDesc>
          Pick an existing workspace (or create one) and start the sync agent.
        </SetupDesc>
        <CodeContainer>
          <CodePre>{`# select a workspace and start syncing
${cliPath} workspace use my-project
${cliPath} up

# the agent works in ~/afs/my-project/ with normal file I/O`}</CodePre>
        </CodeContainer>
        <SetupHint>
          Once <InlineCode>afs up</InlineCode> is running, the agent appears on
          the Agents page with a live status indicator.
        </SetupHint>
      </SetupCard>

      <SetupCard>
        <SetupTitle>Connect via MCP</SetupTitle>
        <SetupDesc>
          For AI agents that support the Model Context Protocol, add AFS as an
          MCP server. The agent gets tool-based access to workspaces, files, and
          checkpoints — no local mount needed.
        </SetupDesc>
        <SetupDesc>
          Add the following to your agent's MCP configuration (e.g.{" "}
          <InlineCode>claude_desktop_config.json</InlineCode> or{" "}
          <InlineCode>.claude/settings.json</InlineCode>):
        </SetupDesc>
        <CodeContainer>
          <CodePre>{mcpConfig}</CodePre>
          <CopyButton type="button" onClick={() => copyToClipboard(mcpConfig, "mcp")}>
            {copied === "mcp" ? "Copied!" : "Copy"}
          </CopyButton>
        </CodeContainer>
      </SetupCard>

      <LinksRow>
        <CrossLinkCard as={Link} to="/docs" style={{ flex: 1 }}>
          <CrossLinkText>
            <CrossLinkTitle>Getting Started</CrossLinkTitle>
            <CrossLinkDesc>
              Docker quickstart, build from source, and platform support.
            </CrossLinkDesc>
          </CrossLinkText>
          <CrossLinkArrow>&rarr;</CrossLinkArrow>
        </CrossLinkCard>
        <CrossLinkCard as={Link} to="/agent-guide" style={{ flex: 1 }}>
          <CrossLinkText>
            <CrossLinkTitle>Agent Guide</CrossLinkTitle>
            <CrossLinkDesc>
              Full MCP tool reference, CLI commands, and workflows.
            </CrossLinkDesc>
          </CrossLinkText>
          <CrossLinkArrow>&rarr;</CrossLinkArrow>
        </CrossLinkCard>
      </LinksRow>
    </Layout>
  );
}

const Layout = styled.div<{ $compact: boolean }>`
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 720px;
  margin: 0 auto;
  padding: ${({ $compact }) => ($compact ? "0" : "20px 0 0")};
`;

const SetupCard = styled.div`
  position: relative;
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  padding: 24px;
  background: var(--afs-panel-strong);
`;

const CardLabel = styled.span`
  position: absolute;
  top: -9px;
  left: 20px;
  padding: 2px 10px;
  border-radius: 999px;
  background: var(--afs-accent-soft);
  color: var(--afs-accent);
  border: 1px solid var(--afs-accent);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
`;

const SetupTitle = styled.h4`
  margin: 0 0 8px;
  color: var(--afs-ink);
  font-size: 16px;
  font-weight: 700;
`;

const SetupDesc = styled.p`
  margin: 0 0 12px;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.65;
`;

const StepLabel = styled.div`
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 700;
  margin-bottom: 8px;
`;

const StepDivider = styled.div`
  height: 1px;
  background: var(--afs-line);
  margin: 20px 0;
`;

const SetupHint = styled.p`
  margin: 12px 0 0;
  color: var(--afs-muted);
  font-size: 12px;
  line-height: 1.5;
`;

const CodeContainer = styled.div`
  background: #1e1e2e;
  border-radius: 10px;
  display: flex;
  flex-direction: column;
`;

const CodePre = styled.pre`
  margin: 0;
  padding: 16px 20px 12px;
  color: #cdd6f4;
  font-family: "SF Mono", "Fira Code", "Consolas", monospace;
  font-size: 13px;
  line-height: 1.6;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
`;

const CopyButton = styled.button`
  align-self: flex-end;
  margin: 0 12px 12px;
  border: 1px solid rgba(255, 255, 255, 0.15);
  background: rgba(255, 255, 255, 0.08);
  color: #cdd6f4;
  font-size: 12px;
  font-weight: 600;
  padding: 5px 14px;
  border-radius: 6px;
  cursor: pointer;
  transition: background 120ms ease;
  flex-shrink: 0;

  &:hover {
    background: rgba(255, 255, 255, 0.16);
  }
`;

const LinksRow = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: 1fr 1fr;

  @media (max-width: 640px) {
    grid-template-columns: 1fr;
  }
`;
