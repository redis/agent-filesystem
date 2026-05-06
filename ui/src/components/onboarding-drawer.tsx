// OnboardingDrawer — right-anchored slide-over for the dual-path landing.
//
// Pattern adapted from the Redis Cloud "Context Surfaces" Quick Start drawer:
// keep the landing visible behind, scroll-and-copy linear steps, no nested
// tabs, named-step substitution placeholders.
//
// Also exports `CommandsDrawer` — same shell, but renders per-page command
// reference sections instead of onboarding prompts.

import { Link } from "@tanstack/react-router";
import { Check, Copy } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import type { ReactNode } from "react";
import styled, { keyframes } from "styled-components";
import { SurfaceCard } from "./card-shell";
import { AgentPromptCard } from "./agent-prompt-card";
import {
  agentBootstrapPrompt,
  agentMcpPrompt,
} from "../features/docs/afs-samples";
import type { DrawerCommandSection } from "../foundation/drawer-context";

// Slide animation timing — keep in sync with the CSS @keyframes durations
// on Backdrop and DrawerShell below.
const SLIDE_MS = 220;

// Drives the open/close animation. CSS keyframes play automatically on
// mount (slide-in). On close request, flip `closing` to play the reverse
// keyframes; after SLIDE_MS we call the parent's onClose to unmount.
function useDrawerAnimation(onCloseParent: () => void) {
  const [closing, setClosing] = useState(false);

  const handleClose = useCallback(() => {
    setClosing(true);
    window.setTimeout(onCloseParent, SLIDE_MS);
  }, [onCloseParent]);

  return { closing, handleClose };
}

export type OnboardingPath = "agent" | "cli";

export type OnboardingStatus = "idle" | "creating" | "ready" | "error";

type Props = {
  path: OnboardingPath;
  status: OnboardingStatus;
  errorMessage?: string | null;
  workspaceName?: string;
  onClose: () => void;
  onRetry?: () => void;
};

export function OnboardingDrawer({
  path,
  status,
  errorMessage,
  workspaceName = "getting-started",
  onClose,
  onRetry,
}: Props) {
  const { closing, handleClose } = useDrawerAnimation(onClose);

  // Close on Escape — matches the close X in the header.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") handleClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [handleClose]);

  const headerTitle =
    path === "agent" ? "Connect your agent" : "Connect via CLI";
  const headerSubline =
    path === "agent"
      ? "Paste a prompt into your agent. It installs the CLI and connects."
      : "Install the CLI, authenticate, and mount the workspace from your shell.";

  return (
    <Backdrop $closing={closing} onClick={handleClose} role="presentation">
      <DrawerShell
        $closing={closing}
        role="dialog"
        aria-modal="true"
        aria-label={headerTitle}
        onClick={(e) => e.stopPropagation()}
      >
        <DrawerHeader>
          <DrawerTitleStack>
            <DrawerEyebrow>Quick start</DrawerEyebrow>
            <DrawerTitle>{headerTitle}</DrawerTitle>
            <DrawerSubline>{headerSubline}</DrawerSubline>
          </DrawerTitleStack>
          <CloseButton type="button" onClick={handleClose} aria-label="Close">
            ×
          </CloseButton>
        </DrawerHeader>

        <DrawerBody>
          <StatusChip
            status={status}
            workspaceName={workspaceName}
            errorMessage={errorMessage}
            onRetry={onRetry}
          />
          {path === "agent" ? (
            <AgentBody />
          ) : (
            <CliBody workspaceName={workspaceName} />
          )}
        </DrawerBody>
      </DrawerShell>
    </Backdrop>
  );
}

// ─── Path card — peer-choice entry ───────────────────────────────────

type CardProps = {
  tone?: "primary" | "secondary";
  title: ReactNode;
  description: ReactNode;
  buttonLabel: string;
  badge?: string;
  onClick: () => void;
  disabled?: boolean;
};

export function OnboardingPathCard({
  tone = "primary",
  title,
  description,
  buttonLabel,
  badge,
  onClick,
  disabled,
}: CardProps) {
  return (
    <PathCard $tone={tone}>
      {badge ? <PathBadge>{badge}</PathBadge> : null}
      <PathTitle>{title}</PathTitle>
      <PathDesc>{description}</PathDesc>
      <ChooseButton type="button" onClick={onClick} disabled={disabled} $tone={tone}>
        {buttonLabel} →
      </ChooseButton>
    </PathCard>
  );
}

// ─── Bodies ──────────────────────────────────────────────────────────

function AgentBody() {
  return (
    <BodyStack>
      <AgentPromptCard
        tone="primary"
        eyebrow="Step 1 — Paste this into your agent"
        title="Claude, Cursor, Codex, or any agent CLI."
        description="Your agent installs the AFS CLI, signs in, and mounts the workspace. ~30 seconds."
        prompt={agentBootstrapPrompt}
      />
      <AgentPromptCard
        eyebrow="Or — connect via MCP"
        title="Wire AFS into your agent over MCP."
        description={
          <>
            No CLI install needed. Replace <Mono>&lt;YOUR_TOKEN&gt;</Mono> with a
            token from MCP access.
          </>
        }
        prompt={agentMcpPrompt}
        footer={
          <>
            Generate a token at{" "}
            <FooterLink as={Link} to="/mcp">
              /mcp
            </FooterLink>
            .
          </>
        }
      />
    </BodyStack>
  );
}

function CliBody({ workspaceName }: { workspaceName: string }) {
  const steps = [
    {
      title: "Install the CLI",
      command: "curl -fsSL https://afs.cloud/install.sh | bash",
    },
    {
      title: "Authenticate",
      command: "afs auth login",
    },
    {
      title: "Mount the workspace",
      command: `afs ws mount ${workspaceName} ~/afs/${workspaceName}`,
    },
    {
      title: "Start using it",
      command: `cd ~/afs/${workspaceName}\necho "hello" > notes.txt\nafs cp create ${workspaceName} first-note`,
    },
  ];

  return (
    <BodyStack>
      {steps.map((step, idx) => (
        <NumberedStep
          key={idx}
          n={idx + 1}
          title={step.title}
          command={step.command}
        />
      ))}
      <FooterRow>
        <FooterLink as={Link} to="/docs/cli">
          Read the full CLI guide ↗
        </FooterLink>
      </FooterRow>
    </BodyStack>
  );
}

function NumberedStep({
  n,
  title,
  command,
}: {
  n: number;
  title: string;
  command: string;
}) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      /* ignore */
    }
  }

  return (
    <StepRow>
      <StepNumber>{n}</StepNumber>
      <StepBody>
        <StepTitle>{title}</StepTitle>
        <StepCommandBlock>
          <StepCommandText>{command}</StepCommandText>
          <StepCopyButton type="button" onClick={copy} aria-label="Copy command">
            {copied ? <Check size={14} strokeWidth={2.4} /> : <Copy size={14} strokeWidth={2.4} />}
          </StepCopyButton>
        </StepCommandBlock>
      </StepBody>
    </StepRow>
  );
}

// ─── Status chip ─────────────────────────────────────────────────────

function StatusChip({
  status,
  workspaceName,
  errorMessage,
  onRetry,
}: {
  status: OnboardingStatus;
  workspaceName: string;
  errorMessage?: string | null;
  onRetry?: () => void;
}) {
  if (status === "idle") return null;
  const tone =
    status === "ready" ? "ok" : status === "error" ? "warn" : "info";
  return (
    <Chip $tone={tone}>
      <ChipDot $tone={tone} />
      {status === "creating" ? (
        <span>
          Creating <Mono>{workspaceName}</Mono> workspace…
        </span>
      ) : status === "ready" ? (
        <span>
          Workspace <Mono>{workspaceName}</Mono> is ready.
        </span>
      ) : (
        <>
          <span>{errorMessage || "Could not create workspace."}</span>
          {onRetry ? (
            <RetryButton type="button" onClick={onRetry}>
              retry
            </RetryButton>
          ) : null}
        </>
      )}
    </Chip>
  );
}

// ─── Styled components ───────────────────────────────────────────────

const fadeIn = keyframes`
  from { background: rgba(8, 6, 13, 0); }
  to { background: rgba(8, 6, 13, 0.42); }
`;

const fadeOut = keyframes`
  from { background: rgba(8, 6, 13, 0.42); }
  to { background: rgba(8, 6, 13, 0); }
`;

const slideIn = keyframes`
  from { transform: translateX(100%); }
  to { transform: translateX(0); }
`;

const slideOut = keyframes`
  from { transform: translateX(0); }
  to { transform: translateX(100%); }
`;

const Backdrop = styled.div<{ $closing: boolean }>`
  position: fixed;
  inset: 0;
  z-index: 80;
  background: rgba(8, 6, 13, 0.42);
  display: flex;
  justify-content: flex-end;
  animation: ${(p) => (p.$closing ? fadeOut : fadeIn)} 200ms ease forwards;
`;

const DrawerShell = styled.aside<{ $closing: boolean }>`
  width: min(520px, 96vw);
  max-width: 96vw;
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--afs-panel);
  border-left: 1px solid var(--afs-line);
  box-shadow: -16px 0 48px rgba(8, 6, 13, 0.32);
  transform: translateX(0);
  animation: ${(p) => (p.$closing ? slideOut : slideIn)} 220ms cubic-bezier(0.32, 0.72, 0.24, 1) forwards;
  will-change: transform;
`;

const DrawerHeader = styled.div`
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 20px 22px 14px;
  border-bottom: 1px solid var(--afs-line);
`;

const DrawerTitleStack = styled.div`
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
`;

const DrawerEyebrow = styled.div`
  color: var(--afs-accent);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.14em;
  text-transform: uppercase;
`;

const DrawerTitle = styled.h2`
  margin: 0;
  color: var(--afs-ink);
  font-size: 22px;
  font-weight: 750;
  line-height: 1.2;
  letter-spacing: -0.01em;
`;

const DrawerSubline = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 13.5px;
  line-height: 1.5;
`;

const CloseButton = styled.button`
  flex: 0 0 auto;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  border: 1px solid var(--afs-line);
  background: transparent;
  color: var(--afs-muted);
  font-size: 22px;
  line-height: 1;
  cursor: pointer;
  transition: background 120ms ease, color 120ms ease;

  &:hover {
    background: var(--afs-bg-soft);
    color: var(--afs-ink);
  }
`;

const DrawerBody = styled.div`
  flex: 1;
  overflow-y: auto;
  padding: 18px 22px 28px;
  display: flex;
  flex-direction: column;
  gap: 16px;
`;

const BodyStack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 14px;
`;

// Status chip
const Chip = styled.div<{ $tone: "info" | "ok" | "warn" }>`
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  border-radius: 10px;
  border: 1px solid
    ${(p) =>
      p.$tone === "ok"
        ? "#10b981"
        : p.$tone === "warn"
          ? "#f59e0b"
          : "color-mix(in srgb, var(--afs-accent) 35%, var(--afs-line))"};
  background: ${(p) =>
    p.$tone === "ok"
      ? "#ecfdf5"
      : p.$tone === "warn"
        ? "#fffbeb"
        : "color-mix(in srgb, var(--afs-accent) 6%, var(--afs-panel))"};
  color: ${(p) =>
    p.$tone === "ok" ? "#047857" : p.$tone === "warn" ? "#92400e" : "var(--afs-ink)"};
  font-size: 13px;
  line-height: 1.5;
`;

const ChipDot = styled.span<{ $tone: "info" | "ok" | "warn" }>`
  width: 8px;
  height: 8px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: ${(p) =>
    p.$tone === "ok" ? "#10b981" : p.$tone === "warn" ? "#f59e0b" : "var(--afs-accent)"};
  box-shadow: 0 0 0 3px
    color-mix(in srgb,
      ${(p) =>
        p.$tone === "ok" ? "#10b981" : p.$tone === "warn" ? "#f59e0b" : "var(--afs-accent)"}
      22%, transparent);
  ${(p) =>
    p.$tone === "info" ? "animation: afs-onboarding-pulse 1.6s ease-in-out infinite;" : ""}

  @keyframes afs-onboarding-pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.45; }
  }
`;

const RetryButton = styled.button`
  margin-left: auto;
  background: transparent;
  border: none;
  color: var(--afs-accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  cursor: pointer;
  padding: 0;

  &:hover {
    text-decoration: underline;
  }
`;

// Numbered step
const StepRow = styled.div`
  display: grid;
  grid-template-columns: 28px 1fr;
  gap: 12px;
  align-items: flex-start;
`;

const StepNumber = styled.div`
  width: 24px;
  height: 24px;
  margin-top: 4px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--afs-accent);
  color: var(--afs-ink-on-accent, #fff);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0;
`;

const StepBody = styled.div`
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
`;

const StepTitle = styled.div`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 700;
  letter-spacing: -0.01em;
`;

const StepCommandBlock = styled.div`
  position: relative;
  padding: 10px 44px 10px 14px;
  border-radius: 8px;
  background: #0d1117;
  border: 1px solid #1f2937;
  font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
  font-size: 12.5px;
  line-height: 1.55;
  overflow-x: auto;
`;

const StepCommandText = styled.code`
  color: #4ade80;
  white-space: pre;
  text-shadow: 0 0 6px rgba(74, 222, 128, 0.22);
`;

const StepCopyButton = styled.button`
  position: absolute;
  top: 6px;
  right: 6px;
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  border: 1px solid rgba(255, 255, 255, 0.18);
  background: transparent;
  color: #9ca3af;
  cursor: pointer;
  transition: background 120ms ease, color 120ms ease, border-color 120ms ease;

  &:hover {
    background: rgba(74, 222, 128, 0.12);
    color: #4ade80;
    border-color: #4ade80;
  }
`;

// Path card (peer choice)
const PathCard = styled(SurfaceCard)<{ $tone: "primary" | "secondary" }>`
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 22px 22px 20px;
  border: 1px solid
    ${(p) =>
      p.$tone === "primary"
        ? "color-mix(in srgb, var(--afs-accent) 30%, var(--afs-line))"
        : "var(--afs-line)"};
  background: ${(p) =>
    p.$tone === "primary"
      ? "linear-gradient(180deg, color-mix(in srgb, var(--afs-accent) 5%, var(--afs-panel)), var(--afs-panel))"
      : "var(--afs-panel)"};
  box-shadow: ${(p) =>
    p.$tone === "primary"
      ? "0 12px 32px color-mix(in srgb, var(--afs-accent) 14%, transparent)"
      : "0 10px 24px rgba(8, 6, 13, 0.08)"};
  text-align: left;
`;

const PathBadge = styled.span`
  align-self: flex-start;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  background: color-mix(in srgb, var(--afs-accent) 12%, transparent);
  color: var(--afs-accent);
`;

const PathTitle = styled.div`
  color: var(--afs-ink);
  font-size: 18px;
  font-weight: 750;
  letter-spacing: -0.01em;
`;

const PathDesc = styled.div`
  color: var(--afs-muted);
  font-size: 13.5px;
  line-height: 1.55;
`;

const ChooseButton = styled.button<{ $tone: "primary" | "secondary" }>`
  margin-top: 10px;
  align-self: flex-start;
  padding: 8px 16px;
  border-radius: 8px;
  border: 1px solid
    ${(p) =>
      p.$tone === "primary"
        ? "var(--afs-accent)"
        : "var(--afs-line)"};
  background: ${(p) =>
    p.$tone === "primary"
      ? "var(--afs-accent)"
      : "var(--afs-panel)"};
  color: ${(p) =>
    p.$tone === "primary" ? "var(--afs-ink-on-accent, #fff)" : "var(--afs-ink)"};
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.02em;
  cursor: pointer;
  transition: background 120ms ease, transform 120ms ease;

  &:hover:not(:disabled) {
    background: ${(p) =>
      p.$tone === "primary"
        ? "color-mix(in srgb, var(--afs-accent) 90%, white)"
        : "var(--afs-bg-soft)"};
    transform: translateY(-1px);
  }

  &:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
`;

// Footer link (used by both bodies + status chip retry)
const FooterRow = styled.div`
  margin-top: 6px;
  font-size: 12.5px;
  color: var(--afs-muted);
`;

const FooterLink = styled.a`
  color: var(--afs-accent);
  text-decoration: none;
  font-weight: 600;

  &:hover {
    text-decoration: underline;
  }
`;

const Mono = styled.code`
  font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
  font-size: 0.92em;
  padding: 0 4px;
  border-radius: 4px;
  background: color-mix(in srgb, var(--afs-line) 60%, transparent);
  color: var(--afs-ink);
`;

// ─── CommandsDrawer ──────────────────────────────────────────────────
// Same shell as OnboardingDrawer, but renders an arbitrary list of
// command sections. Used by the global Help button when the current
// page has registered contextual commands.

export function CommandsDrawer({
  title,
  subline,
  sections,
  onClose,
}: {
  title: string;
  subline?: string;
  sections: DrawerCommandSection[];
  onClose: () => void;
}) {
  const { closing, handleClose } = useDrawerAnimation(onClose);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") handleClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [handleClose]);

  return (
    <Backdrop $closing={closing} onClick={handleClose} role="presentation">
      <DrawerShell
        $closing={closing}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onClick={(e) => e.stopPropagation()}
      >
        <DrawerHeader>
          <DrawerTitleStack>
            <DrawerEyebrow>Commands</DrawerEyebrow>
            <DrawerTitle>{title}</DrawerTitle>
            {subline ? <DrawerSubline>{subline}</DrawerSubline> : null}
          </DrawerTitleStack>
          <CloseButton type="button" onClick={handleClose} aria-label="Close">
            ×
          </CloseButton>
        </DrawerHeader>

        <DrawerBody>
          <BodyStack>
            {sections.map((section, idx) => (
              <CommandSectionRow key={idx} section={section} />
            ))}
          </BodyStack>
        </DrawerBody>
      </DrawerShell>
    </Backdrop>
  );
}

function CommandSectionRow({ section }: { section: DrawerCommandSection }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(section.command);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      /* ignore */
    }
  }

  return (
    <CommandRow>
      <CommandHeader>
        <CommandTitle>{section.title}</CommandTitle>
        {section.description ? (
          <CommandDescription>{section.description}</CommandDescription>
        ) : null}
      </CommandHeader>
      <StepCommandBlock>
        <StepCommandText>{section.command}</StepCommandText>
        <StepCopyButton type="button" onClick={copy} aria-label="Copy command">
          {copied ? <Check size={14} strokeWidth={2.4} /> : <Copy size={14} strokeWidth={2.4} />}
        </StepCopyButton>
      </StepCommandBlock>
    </CommandRow>
  );
}

const CommandRow = styled.div`
  display: flex;
  flex-direction: column;
  gap: 6px;
`;

const CommandHeader = styled.div`
  display: flex;
  flex-direction: column;
  gap: 2px;
`;

const CommandTitle = styled.div`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 700;
  letter-spacing: -0.01em;
`;

const CommandDescription = styled.div`
  color: var(--afs-muted);
  font-size: 12.5px;
  line-height: 1.45;
`;
