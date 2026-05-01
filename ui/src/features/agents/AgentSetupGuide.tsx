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
import { useAuthSession } from "../../foundation/auth-context";

type Props = {
  compact?: boolean;
};

type Tab = "install" | "mcp";
type InstallMethod = "curl" | "brew" | "manual";

export function AgentSetupGuide({ compact = false }: Props) {
  const [tab, setTab] = useState<Tab>("install");
  const [method, setMethod] = useState<InstallMethod>("curl");
  const [copied, setCopied] = useState<string | null>(null);
  const controlPlaneUrl = getControlPlaneURL();
  const { config: authConfig } = useAuthSession();
  const isCloud = authConfig.productMode === "cloud";
  const installCmd = `curl -fsSL ${controlPlaneUrl}/install.sh | bash`;
  const loginCmd = `afs auth login`;
  const mountCmd = `afs ws mount <workspace> ~/workspace`;
  const manualCmd = `curl -fsSL "${controlPlaneUrl}/v1/cli?os=$(uname -s)&arch=$(uname -m)" -o afs && chmod +x afs`;
  const manualConfigCmd = `afs auth login --self-hosted --url ${controlPlaneUrl}`;

  const mcpConfig = JSON.stringify(
    {
      mcpServers: {
        "agent-filesystem": {
          command: "afs",
          args: ["mcp", "--workspace", "my-workspace", "--profile", "workspace-rw"],
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
      <TabBar role="tablist">
        <TabItem
          $active={tab === "install"}
          role="tab"
          aria-selected={tab === "install"}
          onClick={() => setTab("install")}
        >
          Install CLI
        </TabItem>
        <TabItem
          $active={tab === "mcp"}
          role="tab"
          aria-selected={tab === "mcp"}
          onClick={() => setTab("mcp")}
        >
          MCP config
        </TabItem>
      </TabBar>

      {tab === "install" ? (
        <PanelCard>
          <MethodPills role="tablist">
            <MethodPill
              $active={method === "curl"}
              onClick={() => setMethod("curl")}
            >
              curl
            </MethodPill>
            <MethodPill
              $active={method === "brew"}
              $disabled
              aria-disabled
              title="Coming soon"
              onClick={() => {}}
            >
              Homebrew
              <PillBadge>soon</PillBadge>
            </MethodPill>
            <MethodPill
              $active={method === "manual"}
              onClick={() => setMethod("manual")}
            >
              Manual
            </MethodPill>
          </MethodPills>

          {method === "curl" && (
            <>
              <StepNumber>1</StepNumber>
              <StepTitle>Install the CLI</StepTitle>
              <StepDesc>
                One command. Detects your OS and architecture, drops{" "}
                <InlineCode>afs</InlineCode> into{" "}
                <InlineCode>~/.afs/bin</InlineCode>, and adds that directory to
                your shell PATH automatically when needed.
                {!isCloud && (
                  <>
                    {" "}
                    Also points the CLI at this control plane (
                    <InlineCode>{controlPlaneUrl}</InlineCode>) automatically.
                  </>
                )}
              </StepDesc>
              <CodeContainer>
                <CodePre>{installCmd}</CodePre>
                <CopyButton
                  type="button"
                  onClick={() => copyToClipboard(installCmd, "install")}
                >
                  {copied === "install" ? "Copied!" : "Copy"}
                </CopyButton>
              </CodeContainer>

              {isCloud ? (
                <>
                  <StepDivider />

                  <StepNumber>2</StepNumber>
                  <StepTitle>Sign in</StepTitle>
                  <StepDesc>
                    Opens a browser window and links this CLI to your account.
                  </StepDesc>
                  <CodeContainer>
                    <CodePre>{loginCmd}</CodePre>
                    <CopyButton
                      type="button"
                      onClick={() => copyToClipboard(loginCmd, "login")}
                    >
                      {copied === "login" ? "Copied!" : "Copy"}
                    </CopyButton>
                  </CodeContainer>
                  <StepHint>
                    After a workspace is mounted, the agent appears on this
                    page with a live status indicator.
                  </StepHint>

                  <StepDivider />

                  <StepNumber>3</StepNumber>
                  <StepTitle>Mount a workspace</StepTitle>
                  <StepDesc>
                    Pick any workspace and mount it to a local folder.
                  </StepDesc>
                  <CodeContainer>
                    <CodePre>{mountCmd}</CodePre>
                    <CopyButton
                      type="button"
                      onClick={() => copyToClipboard(mountCmd, "mount")}
                    >
                      {copied === "mount" ? "Copied!" : "Copy"}
                    </CopyButton>
                  </CodeContainer>
                </>
              ) : (
                <>
                  <StepDivider />

                  <StepNumber>2</StepNumber>
                  <StepTitle>Mount a workspace</StepTitle>
                  <StepDesc>
                    The installer points the CLI at this control plane. Mount
                    any workspace to a local folder when you are ready to work.
                  </StepDesc>
                  <CodeContainer>
                    <CodePre>{mountCmd}</CodePre>
                    <CopyButton
                      type="button"
                      onClick={() => copyToClipboard(mountCmd, "mount")}
                    >
                      {copied === "mount" ? "Copied!" : "Copy"}
                    </CopyButton>
                  </CodeContainer>
                  <StepHint>
                    Once the workspace is mounted, the agent appears here with
                    a live status indicator.
                  </StepHint>
                </>
              )}
            </>
          )}

          {method === "manual" && (
            <>
              <StepTitle>Download the binary directly</StepTitle>
              <StepDesc>
                Prefer not to pipe a script to bash? Download the{" "}
                <InlineCode>afs</InlineCode> binary yourself. Make it
                executable and move it somewhere on your PATH.
              </StepDesc>
              <CodeContainer>
                <CodePre>{manualCmd}</CodePre>
                <CopyButton
                  type="button"
                  onClick={() => copyToClipboard(manualCmd, "manual")}
                >
                  {copied === "manual" ? "Copied!" : "Copy"}
                </CopyButton>
              </CodeContainer>
              <StepHint>
                On macOS you may need to strip the quarantine attribute:{" "}
                <InlineCode>xattr -d com.apple.quarantine ./afs</InlineCode>.
              </StepHint>

              {!isCloud && (
                <>
                  <StepDivider />

                  <StepTitle>Point the CLI at this control plane</StepTitle>
                  <StepDesc>
                    The install script does this automatically; when
                    downloading manually, run it yourself.
                  </StepDesc>
                  <CodeContainer>
                    <CodePre>{manualConfigCmd}</CodePre>
                    <CopyButton
                      type="button"
                      onClick={() =>
                        copyToClipboard(manualConfigCmd, "manual-config")
                      }
                    >
                      {copied === "manual-config" ? "Copied!" : "Copy"}
                    </CopyButton>
                  </CodeContainer>
                </>
              )}
            </>
          )}
        </PanelCard>
      ) : (
        <PanelCard>
          <StepTitle>Connect via MCP</StepTitle>
          <StepDesc>
            For agents that speak the Model Context Protocol (Claude Desktop,
            Cursor, Windsurf, etc.), add AFS as an MCP server. The default MCP
            flow is workspace-bound and file-focused, so agents get the file
            tools they need without a broad management surface.
          </StepDesc>
          <StepDesc>
            Add the following to your agent's MCP configuration, then replace{" "}
            <InlineCode>my-workspace</InlineCode> with the workspace you want
            to expose (e.g.{" "}
            <InlineCode>claude_desktop_config.json</InlineCode> or{" "}
            <InlineCode>.claude/settings.json</InlineCode>):
          </StepDesc>
          <CodeContainer>
            <CodePre>{mcpConfig}</CodePre>
            <CopyButton
              type="button"
              onClick={() => copyToClipboard(mcpConfig, "mcp")}
            >
              {copied === "mcp" ? "Copied!" : "Copy"}
            </CopyButton>
          </CodeContainer>
          <StepHint>
            You still need the <InlineCode>afs</InlineCode> CLI installed
            {isCloud ? (
              <>
                {" "}and signed in (via <InlineCode>afs auth login</InlineCode>)
                for MCP to authenticate against your workspaces.
              </>
            ) : (
              <>
                {" "}and pointed at this control plane (the install script
                handles that) for MCP to reach your workspaces.
              </>
            )}
          </StepHint>
        </PanelCard>
      )}

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

/* ── Styled components ── */

const Layout = styled.div<{ $compact: boolean }>`
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 720px;
  padding: ${({ $compact }) => ($compact ? "0" : "20px 0 0")};
`;

const TabBar = styled.div`
  display: flex;
  gap: 0;
  border-bottom: 1px solid var(--afs-line);
`;

const TabItem = styled.button<{ $active?: boolean }>`
  position: relative;
  border: none;
  background: transparent;
  color: ${(p) => (p.$active ? "var(--afs-accent, #2563eb)" : "var(--afs-muted)")};
  font-size: 14px;
  font-weight: 700;
  padding: 12px 18px;
  cursor: pointer;
  transition: color 120ms ease;

  &:hover {
    color: var(--afs-accent, #2563eb);
  }

  &::after {
    content: "";
    position: absolute;
    left: 0;
    right: 0;
    bottom: -1px;
    height: 2px;
    background: ${(p) => (p.$active ? "var(--afs-accent, #2563eb)" : "transparent")};
  }
`;

const PanelCard = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  padding: 24px;
  background: var(--afs-panel-strong);
`;

const MethodPills = styled.div`
  display: flex;
  width: fit-content;
  gap: 6px;
  padding: 4px;
  border-radius: 999px;
  background: var(--afs-line);
  margin-bottom: 20px;
`;

const MethodPill = styled.button<{ $active?: boolean; $disabled?: boolean }>`
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: none;
  background: ${(p) => (p.$active ? "var(--afs-panel)" : "transparent")};
  color: ${(p) =>
    p.$disabled
      ? "color-mix(in srgb, var(--afs-muted) 70%, transparent)"
      : p.$active
        ? "var(--afs-ink)"
        : "var(--afs-muted)"};
  font-size: 13px;
  font-weight: 700;
  padding: 6px 14px;
  border-radius: 999px;
  cursor: ${(p) => (p.$disabled ? "not-allowed" : "pointer")};
  transition: background 120ms ease, color 120ms ease;
  box-shadow: ${(p) => (p.$active ? "0 1px 3px rgba(0,0,0,0.06)" : "none")};

  &:hover {
    color: ${(p) => (p.$disabled ? "inherit" : "var(--afs-ink)")};
  }
`;

const PillBadge = styled.span`
  padding: 1px 6px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--afs-muted) 18%, transparent);
  color: var(--afs-muted);
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.04em;
  text-transform: uppercase;
`;

const StepNumber = styled.div`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--afs-accent, #2563eb);
  color: #fff;
  font-size: 12px;
  font-weight: 800;
  margin-bottom: 8px;
`;

const StepTitle = styled.h4`
  margin: 0 0 8px;
  color: var(--afs-ink);
  font-size: 16px;
  font-weight: 700;
`;

const StepDesc = styled.p`
  margin: 0 0 12px;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.65;
`;

const StepDivider = styled.div`
  height: 1px;
  background: var(--afs-line);
  margin: 22px 0 18px;
`;

const StepHint = styled.p`
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
