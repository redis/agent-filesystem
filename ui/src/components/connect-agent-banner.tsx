import { useEffect, useRef, useState } from "react";
import { Button } from "@redis-ui/components";
import { Link } from "@tanstack/react-router";
import styled, { keyframes } from "styled-components";
import { SurfaceCard } from "./card-shell";
import { getControlPlaneURL } from "../foundation/api/afs";
import { canonicalWorkspaceName, displayWorkspaceName } from "../foundation/workspace-display";

type Props = {
  workspaceId: string;
  workspaceName: string;
  workspaceLabel?: string;
  agentConnected: boolean;
  onDismiss: () => void;
};

type Tab = "cli" | "mcp";
type Step = "connect" | "finished";

export function ConnectAgentBanner({
  workspaceName,
  workspaceLabel,
  agentConnected,
  onDismiss,
}: Props) {
  const [step, setStep] = useState<Step>(agentConnected ? "finished" : "connect");
  const [tab, setTab] = useState<Tab>("cli");
  const [copied, setCopied] = useState<string | null>(null);
  const [showConnectedDialog, setShowConnectedDialog] = useState(false);
  const hadAgentBefore = useRef(agentConnected);

  const controlPlaneUrl = getControlPlaneURL();
  const displayName = workspaceLabel?.trim() || displayWorkspaceName(workspaceName);
  const mountPath = `~/afs/${canonicalWorkspaceName(workspaceName)}`;
  const downloadCmd = `curl -fsSL ${controlPlaneUrl}/install.sh | bash`;
  const loginCmd = `afs auth login`;
  const mountCmd = `afs ws mount ${canonicalWorkspaceName(workspaceName)} ${mountPath}`;
  const mcpLocalConfig = JSON.stringify(
    {
      mcpServers: {
        [`afs-${canonicalWorkspaceName(workspaceName)}`]: {
          command: "afs",
          args: ["mcp", "--workspace", workspaceName, "--profile", "workspace-rw"],
        },
      },
    },
    null,
    2,
  );

  useEffect(() => {
    if (agentConnected && !hadAgentBefore.current && step === "connect") {
      setShowConnectedDialog(true);
    }
    hadAgentBefore.current = agentConnected;
  }, [agentConnected, step]);

  function copyToClipboard(text: string, label: string) {
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(label);
      setTimeout(() => setCopied(null), 2000);
    });
  }

  function handleConnectedNext() {
    setShowConnectedDialog(false);
    setStep("finished");
  }

  return (
    <Banner>
      <BannerHeader>
        <BannerHeaderLeft>
          <BannerTitle>
            {step === "connect" ? "Connect your first agent" : "You're all set!"}
          </BannerTitle>
          <BannerSubtitle>
            {step === "connect" ? (
              <>
                Run three commands to link an agent to{" "}
                <strong>{displayName}</strong>. We'll detect the connection
                automatically.
              </>
            ) : (
              <>
                Your agent is connected to <strong>{displayName}</strong>.
                Here are a few things to try next.
              </>
            )}
          </BannerSubtitle>
        </BannerHeaderLeft>
        <DismissButton type="button" onClick={onDismiss} aria-label="Dismiss">
          &times;
        </DismissButton>
      </BannerHeader>

      {step === "connect" && (
        <>
          <TabBar role="tablist">
            <TabItem
              $active={tab === "cli"}
              onClick={() => setTab("cli")}
              role="tab"
              aria-selected={tab === "cli"}
            >
              <TabRecommended>Recommended</TabRecommended>
              CLI
            </TabItem>
            <TabItem
              $active={tab === "mcp"}
              onClick={() => setTab("mcp")}
              role="tab"
              aria-selected={tab === "mcp"}
            >
              MCP (alternative)
            </TabItem>
          </TabBar>

          <StepContent>
            {tab === "cli" ? (
              <>
                <SubStepLabel>1 &mdash; Install the CLI</SubStepLabel>
                <StepDescription>
                  Detects your OS and architecture, installs{" "}
                  <code>afs</code> into <code>~/.afs/bin</code>, and adds that
                  directory to your shell PATH automatically when needed.
                </StepDescription>
                <CodeContainer>
                  <CodePre>{downloadCmd}</CodePre>
                  <CopyButton
                    type="button"
                    onClick={() => copyToClipboard(downloadCmd, "download")}
                  >
                    {copied === "download" ? "Copied!" : "Copy"}
                  </CopyButton>
                </CodeContainer>

                <SubStepDivider />

                <SubStepLabel>2 &mdash; Sign in</SubStepLabel>
                <StepDescription>
                  Opens a browser window, signs you in, and links the CLI to
                  your account.
                </StepDescription>
                <CodeContainer>
                  <CodePre>{loginCmd}</CodePre>
                  <CopyButton
                    type="button"
                    onClick={() => copyToClipboard(loginCmd, "login")}
                  >
                    {copied === "login" ? "Copied!" : "Copy"}
                  </CopyButton>
                </CodeContainer>

                <SubStepDivider />

                <SubStepLabel>3 &mdash; Mount the workspace</SubStepLabel>
                <StepDescription>
                  Mounts the workspace at <code>{mountPath}/</code> so any
                  agent on this machine can read and write files.
                </StepDescription>
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
                <SubStepLabel>MCP configuration</SubStepLabel>
                <StepDescription>
                  Prefer an MCP-native agent (Claude Desktop, Cursor,
                  Windsurf)? Add this to your agent's MCP config. You still
                  need to install the CLI and run <code>afs auth login</code>{" "}
                  first.
                </StepDescription>
                <CodeContainer>
                  <CodePre>{mcpLocalConfig}</CodePre>
                  <CopyButton
                    type="button"
                    onClick={() => copyToClipboard(mcpLocalConfig, "mcp")}
                  >
                    {copied === "mcp" ? "Copied!" : "Copy"}
                  </CopyButton>
                </CodeContainer>
                <StepHint>
                  Restart your agent after saving. This config locks MCP access
                  to <strong>{displayName}</strong> with the standard
                  read/write workspace profile.
                </StepHint>
              </>
            )}
          </StepContent>

          <WaitingBar>
            <WaitingSpinner aria-hidden />
            <WaitingText>Waiting for your agent to connect&hellip;</WaitingText>
          </WaitingBar>
        </>
      )}

      {step === "finished" && (
        <StepContent>
          <ConnectedChip>
            <ChipDot />
            Agent connected to <strong>{displayName}</strong>
          </ConnectedChip>
          <NextStepsList>
            <NextStep>
              <NextStepIcon>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
                  <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
                </svg>
              </NextStepIcon>
              <div>
                <NextStepTitle>Ask your agent to edit a file</NextStepTitle>
                <NextStepDesc>
                  Try <code>examples/hello.py</code>. Add a function, fix a
                  bug, or refactor the code.
                </NextStepDesc>
              </div>
            </NextStep>
            <NextStep>
              <NextStepIcon>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <polyline points="16 16 12 12 8 16" />
                  <line x1="12" y1="12" x2="12" y2="21" />
                  <path d="M20.39 18.39A5 5 0 0 0 18 9h-1.26A8 8 0 1 0 3 16.3" />
                </svg>
              </NextStepIcon>
              <div>
                <NextStepTitle>Create a checkpoint</NextStepTitle>
                <NextStepDesc>
                  Snapshot the workspace before risky changes. Restore any
                  checkpoint in seconds if something breaks.
                </NextStepDesc>
              </div>
            </NextStep>
            <NextStep>
              <NextStepIcon>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
                  <polyline points="14 2 14 8 20 8" />
                  <line x1="16" y1="13" x2="8" y2="13" />
                  <line x1="16" y1="17" x2="8" y2="17" />
                </svg>
              </NextStepIcon>
              <div>
                <NextStepTitle>Browse the activity log</NextStepTitle>
                <NextStepDesc>
                  Every read, write, and checkpoint is tracked. See exactly
                  what your agent did, and when.
                </NextStepDesc>
              </div>
            </NextStep>
          </NextStepsList>
          <FinishedActions>
            <LearnMoreLink as={Link} to="/agent-guide">
              Read the Agent Guide &rarr;
            </LearnMoreLink>
            <Button size="large" onClick={onDismiss}>
              Close
            </Button>
          </FinishedActions>
        </StepContent>
      )}

      {showConnectedDialog ? (
        <ConnectedOverlay>
          <ConnectedCard role="alertdialog" aria-modal="true">
            <ConnectedBigIcon aria-hidden>
              <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                <polyline points="20 6 9 17 4 12" />
              </svg>
            </ConnectedBigIcon>
            <ConnectedCardTitle>Agent Connected!</ConnectedCardTitle>
            <ConnectedCardBody>
              Your agent is linked to <strong>{displayName}</strong>.
            </ConnectedCardBody>
            <Button size="large" onClick={handleConnectedNext}>
              Next &rarr;
            </Button>
          </ConnectedCard>
        </ConnectedOverlay>
      ) : null}
    </Banner>
  );
}

/* ── Styled components ── */

const fadeIn = keyframes`
  from { opacity: 0; transform: translateY(-8px); }
  to   { opacity: 1; transform: translateY(0); }
`;

const popIn = keyframes`
  from { opacity: 0; transform: scale(0.92); }
  to   { opacity: 1; transform: scale(1); }
`;

const spin = keyframes`
  to { transform: rotate(360deg); }
`;

const Banner = styled.div`
  position: relative;
  animation: ${fadeIn} 300ms ease;
  border: 1.5px solid var(--afs-accent, #2563eb);
  border-radius: 16px;
  background: var(--afs-panel);
  overflow: hidden;
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--afs-accent, #2563eb) 8%, transparent);
`;

const BannerHeader = styled.div`
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 24px 16px;
`;

const BannerHeaderLeft = styled.div`
  min-width: 0;
`;

const BannerTitle = styled.div`
  color: var(--afs-ink);
  font-size: 18px;
  font-weight: 700;
  line-height: 1.3;
  letter-spacing: -0.01em;
`;

const BannerSubtitle = styled.div`
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.55;
  margin-top: 4px;
`;

const DismissButton = styled.button`
  flex-shrink: 0;
  border: none;
  background: transparent;
  color: var(--afs-muted);
  font-size: 22px;
  line-height: 1;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 6px;

  &:hover {
    background: var(--afs-line);
    color: var(--afs-ink);
  }
`;

const TabBar = styled.div`
  display: flex;
  gap: 0;
  border-top: 1px solid var(--afs-line);
  border-bottom: 1px solid var(--afs-line);
  background: color-mix(in srgb, var(--afs-bg-soft, var(--afs-panel)) 60%, transparent);
`;

const TabItem = styled.button<{ $active?: boolean }>`
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: none;
  background: ${(p) => (p.$active ? "var(--afs-panel)" : "transparent")};
  color: ${(p) => (p.$active ? "var(--afs-accent, #2563eb)" : "var(--afs-muted)")};
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.01em;
  padding: 12px 20px;
  cursor: pointer;
  transition: background 120ms ease, color 120ms ease;
  border-right: 1px solid var(--afs-line);

  &:last-child {
    border-right: none;
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

  &:hover {
    color: var(--afs-accent, #2563eb);
  }
`;

const TabRecommended = styled.span`
  display: inline-flex;
  align-items: center;
  padding: 2px 7px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--afs-accent, #2563eb) 12%, transparent);
  color: var(--afs-accent, #2563eb);
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.06em;
  text-transform: uppercase;
`;

const StepContent = styled.div`
  padding: 22px 24px 24px;
  animation: ${fadeIn} 200ms ease;
`;

const SubStepLabel = styled.div`
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 700;
  margin-bottom: 8px;
`;

const SubStepDivider = styled.div`
  height: 1px;
  background: var(--afs-line);
  margin: 20px 0;
`;

const StepDescription = styled.p`
  margin: 0 0 14px;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.65;

  code {
    background: var(--afs-line);
    padding: 2px 5px;
    border-radius: 4px;
    font-size: 12.5px;
  }
`;

const StepHint = styled.p`
  margin: 12px 0 0;
  color: var(--afs-muted);
  font-size: 12px;
  line-height: 1.5;

  code {
    background: var(--afs-line);
    padding: 2px 5px;
    border-radius: 4px;
    font-size: 11.5px;
  }
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

const WaitingBar = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 24px;
  border-top: 1px solid var(--afs-line);
  background: color-mix(in srgb, var(--afs-accent, #2563eb) 5%, transparent);
`;

const WaitingSpinner = styled.span`
  display: inline-block;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 2px solid color-mix(in srgb, var(--afs-accent, #2563eb) 28%, transparent);
  border-top-color: var(--afs-accent, #2563eb);
  animation: ${spin} 720ms linear infinite;
`;

const WaitingText = styled.span`
  color: var(--afs-accent, #2563eb);
  font-size: 13px;
  font-weight: 600;
`;

const ConnectedChip = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  border-radius: 999px;
  background: #ecfdf5;
  color: #047857;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 20px;

  strong {
    font-weight: 700;
  }
`;

const ChipDot = styled.span`
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #10b981;
  box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.18);
`;

const NextStepsList = styled.div`
  display: flex;
  flex-direction: column;
  gap: 14px;
`;

const NextStep = styled.div`
  display: flex;
  gap: 12px;
  align-items: flex-start;
`;

const NextStepIcon = styled.div`
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--afs-accent-soft, rgba(37, 99, 235, 0.1));
  color: var(--afs-accent, #2563eb);
`;

const NextStepTitle = styled.div`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 600;
  line-height: 1.4;
`;

const NextStepDesc = styled.div`
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.55;
  margin-top: 2px;

  code {
    background: var(--afs-line);
    padding: 1px 5px;
    border-radius: 4px;
    font-size: 12px;
  }
`;

const FinishedActions = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px solid var(--afs-line);
  flex-wrap: wrap;
`;

const LearnMoreLink = styled.a`
  color: var(--afs-accent, #2563eb);
  font-size: 14px;
  font-weight: 600;
  text-decoration: none;

  &:hover {
    text-decoration: underline;
  }
`;

const ConnectedOverlay = styled.div`
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: color-mix(in srgb, var(--afs-panel) 82%, transparent);
  backdrop-filter: blur(6px);
  animation: ${fadeIn} 160ms ease;
  z-index: 5;
`;

const ConnectedCard = styled(SurfaceCard)`
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
  text-align: center;
  max-width: 360px;
  padding: 28px 28px 24px;
  box-shadow: 0 20px 44px rgba(8, 6, 13, 0.18);
  animation: ${popIn} 220ms cubic-bezier(0.2, 0.9, 0.32, 1.18);
`;

const ConnectedBigIcon = styled.div`
  display: flex;
  align-items: center;
  justify-content: center;
  width: 52px;
  height: 52px;
  border-radius: 50%;
  background: #ecfdf5;
  color: #059669;
`;

const ConnectedCardTitle = styled.div`
  color: var(--afs-ink);
  font-size: 22px;
  font-weight: 700;
  letter-spacing: -0.01em;
`;

const ConnectedCardBody = styled.p`
  margin: 0 0 8px;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.55;
`;
