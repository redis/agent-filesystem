import { forwardRef, useImperativeHandle, useState } from "react";
import { Button } from "@redislabsdev/redis-ui-components";
import { Link } from "@tanstack/react-router";
import styled, { keyframes } from "styled-components";
import { getControlPlaneURL } from "../foundation/api/afs";

type Props = {
  workspaceName: string;
  onDismiss: () => void;
};

export type ConnectAgentBannerHandle = {
  jumpToStep: (s: 1 | 2 | 3) => void;
};

export const ConnectAgentBanner = forwardRef<ConnectAgentBannerHandle, Props>(
  function ConnectAgentBanner({ workspaceName, onDismiss }, ref) {
  const [step, setStep] = useState<1 | 2 | 3>(1);

  useImperativeHandle(ref, () => ({
    jumpToStep: (s: 1 | 2 | 3) => setStep(s),
  }));
  const [copied, setCopied] = useState<string | null>(null);

  const controlPlaneUrl = getControlPlaneURL();
  const mountPath = `~/afs/${workspaceName}`;

  const installCmd = `curl -fsSL "${controlPlaneUrl}/v1/cli" -o /tmp/afs && mv /tmp/afs /usr/local/bin/afs && chmod +x /usr/local/bin/afs`;

  const cliSetup = `afs config set --control-plane-url "${controlPlaneUrl}" && afs up ${workspaceName}`;

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
    <Banner>
      <BannerHeader>
        <BannerHeaderLeft>
          <BannerIcon>
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
              <polyline points="22 4 12 14.01 9 11.01" />
            </svg>
          </BannerIcon>
          <div>
            <BannerTitle>Workspace created! Now connect an agent.</BannerTitle>
            <BannerSubtitle>
              Your <strong>{workspaceName}</strong> workspace is ready with sample files.
              Follow the steps below to connect an AI agent.
            </BannerSubtitle>
          </div>
        </BannerHeaderLeft>
        <DismissButton type="button" onClick={onDismiss} aria-label="Dismiss">
          &times;
        </DismissButton>
      </BannerHeader>

      <StepNav>
        <StepNavButton $active={step === 1} onClick={() => setStep(1)}>
          <StepNavNumber $active={step === 1}>1</StepNavNumber>
          Run CLI
        </StepNavButton>
        <StepNavButton $active={step === 2} onClick={() => setStep(2)}>
          <StepNavNumber $active={step === 2}>2</StepNavNumber>
          MCP (optional)
        </StepNavButton>
        <StepNavButton $active={step === 3} onClick={() => setStep(3)}>
          <StepNavNumber $active={step === 3}>3</StepNavNumber>
          What's next
        </StepNavButton>
      </StepNav>

      {step === 1 && (
        <StepContent>
          <SubStepLabel>Step 1 — Download the CLI</SubStepLabel>
          <StepDescription>
            Install the <code>afs</code> binary directly from this server.
          </StepDescription>
          <CodeContainer>
            <CodePre>{installCmd}</CodePre>
            <CopyButton
              type="button"
              onClick={() => copyToClipboard(installCmd, "install")}
            >
              {copied === "install" ? "Copied!" : "Copy"}
            </CopyButton>
          </CodeContainer>
          <StepHint>
            Requires write access to <code>/usr/local/bin</code>. Prefix
            with <code>sudo</code> if needed, or change the path.
          </StepHint>

          <SubStepDivider />

          <SubStepLabel>Step 2 — Connect and mount the workspace</SubStepLabel>
          <StepDescription>
            Point the CLI at this server and sync the workspace to a local directory.
          </StepDescription>
          <CodeContainer>
            <CodePre>{cliSetup}</CodePre>
            <CopyButton
              type="button"
              onClick={() => copyToClipboard(cliSetup, "cli")}
            >
              {copied === "cli" ? "Copied!" : "Copy"}
            </CopyButton>
          </CodeContainer>
          <StepHint>
            The workspace will appear at <code>{mountPath}/</code> on your machine.
          </StepHint>

          <NextButtonRow>
            <NextButton type="button" onClick={() => setStep(2)}>
              Next &rarr;
            </NextButton>
          </NextButtonRow>
        </StepContent>
      )}

      {step === 2 && (
        <StepContent>
          <StepDescription>
            Add this to your agent's MCP configuration (Claude Desktop, Cursor, Windsurf, etc.).
            The agent gets tools to read, write, checkpoint, and restore workspace files.
          </StepDescription>
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
            After adding, restart your agent. It will have access to all AFS workspaces
            including <strong>{workspaceName}</strong>.
          </StepHint>

          <NextButtonRow>
            <NextButton type="button" onClick={() => setStep(3)}>
              Next &rarr;
            </NextButton>
          </NextButtonRow>
        </StepContent>
      )}

      {step === 3 && (
        <StepContent>
          <StepDescription>
            Once an agent is connected, try these things:
          </StepDescription>
          <NextStepsList>
            <NextStep>
              <NextStepIcon>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
                  <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
                </svg>
              </NextStepIcon>
              <div>
                <NextStepTitle>Have the agent edit a file</NextStepTitle>
                <NextStepDesc>
                  Ask it to modify <code>examples/hello.py</code> — add a new function,
                  fix a bug, or refactor the code.
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
                  Save a snapshot before and after changes. If something breaks, restore
                  instantly.
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
                  Every file operation is tracked. Check the Activity tab to see what
                  your agent did.
                </NextStepDesc>
              </div>
            </NextStep>
          </NextStepsList>
          <LearnMoreRow>
            <LearnMoreLink as={Link} to="/agent-guide">
              Read the full Agent Guide &rarr;
            </LearnMoreLink>
          </LearnMoreRow>
        </StepContent>
      )}
    </Banner>
  );
});

/* ── Styled components ── */

const fadeIn = keyframes`
  from { opacity: 0; transform: translateY(-8px); }
  to   { opacity: 1; transform: translateY(0); }
`;

const Banner = styled.div`
  animation: ${fadeIn} 300ms ease;
  border: 1.5px solid var(--afs-accent, #D82C20);
  border-radius: 16px;
  background: var(--afs-panel);
  overflow: hidden;
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--afs-accent, #D82C20) 8%, transparent);
`;

const BannerHeader = styled.div`
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 24px 16px;
`;

const BannerHeaderLeft = styled.div`
  display: flex;
  align-items: flex-start;
  gap: 14px;
`;

const BannerIcon = styled.div`
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: #ecfdf5;
  color: #059669;
`;

const BannerTitle = styled.div`
  color: var(--afs-ink);
  font-size: 16px;
  font-weight: 700;
  line-height: 1.3;
`;

const BannerSubtitle = styled.div`
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.5;
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

const StepNav = styled.div`
  display: flex;
  gap: 0;
  border-top: 1px solid var(--afs-line);
  border-bottom: 1px solid var(--afs-line);
`;

const StepNavButton = styled.button<{ $active?: boolean }>`
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: none;
  background: ${(p) =>
    p.$active
      ? "color-mix(in srgb, var(--afs-accent, #D82C20) 6%, transparent)"
      : "transparent"};
  color: ${(p) => (p.$active ? "var(--afs-accent, #D82C20)" : "var(--afs-muted)")};
  font-size: 13px;
  font-weight: 600;
  padding: 12px 16px;
  cursor: pointer;
  transition: background 120ms ease, color 120ms ease;
  border-right: 1px solid var(--afs-line);

  &:last-child {
    border-right: none;
  }

  &:hover {
    background: color-mix(in srgb, var(--afs-accent, #D82C20) 6%, transparent);
    color: var(--afs-accent, #D82C20);
  }
`;

const StepNavNumber = styled.span<{ $active?: boolean }>`
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  font-size: 11px;
  font-weight: 800;
  background: ${(p) =>
    p.$active ? "var(--afs-accent, #D82C20)" : "var(--afs-line)"};
  color: ${(p) => (p.$active ? "#fff" : "var(--afs-muted)")};
  transition: background 120ms ease, color 120ms ease;
`;

const StepContent = styled.div`
  padding: 20px 24px 24px;
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

const NextButtonRow = styled.div`
  display: flex;
  justify-content: flex-end;
  margin-top: 20px;
  padding-top: 16px;
  border-top: 1px solid var(--afs-line);
`;

const NextButton = styled.button`
  border: none;
  background: #2563eb;
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  padding: 8px 20px;
  border-radius: 8px;
  cursor: pointer;
  transition: opacity 120ms ease;

  &:hover {
    opacity: 0.85;
  }
`;

const StepDescription = styled.p`
  margin: 0 0 14px;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.65;
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
  background: var(--afs-accent-soft, #fef2f1);
  color: var(--afs-accent, #D82C20);
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

const LearnMoreRow = styled.div`
  margin-top: 20px;
  padding-top: 16px;
  border-top: 1px solid var(--afs-line);
`;

const LearnMoreLink = styled.a`
  color: var(--afs-accent, #D82C20);
  font-size: 14px;
  font-weight: 600;
  text-decoration: none;

  &:hover {
    text-decoration: underline;
  }
`;
