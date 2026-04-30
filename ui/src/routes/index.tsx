import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redis-ui/components";
import { Check, Copy } from "lucide-react";
import { useState } from "react";
import styled from "styled-components";
import {
  PageStack,
  StatCard,
  StatGrid,
  StatDetail,
  StatLabel,
  StatValue,
  TabButton,
  Tabs,
} from "../components/afs-kit";
import { AgentHeroAnimation } from "../components/agent-hero-animation";
import { GettingStartedOnboardingDialog } from "../components/getting-started-onboarding-dialog";
import { LiveTopologyCard } from "../components/live-topology-card";
import {
  cliGettingStartedSample,
  mcpGettingStartedSample,
  pythonSdkSample,
  typescriptSdkSample,
} from "../features/docs/afs-samples";
import { formatBytes } from "../foundation/api/afs";
import { useDatabaseScope, useScopedAgents, useScopedWorkspaceSummaries } from "../foundation/database-scope";
import { queryClient } from "../foundation/query-client";
import {
  agentsQueryOptions,
  databasesQueryOptions,
  useQuickstartMutation,
  workspaceSummariesQueryOptions,
} from "../foundation/hooks/use-afs";
import type { AFSWorkspaceDetail, AFSWorkspaceSummary } from "../foundation/types/afs";

const gettingStartedSamples = [
  {
    label: "CLI",
    title: "Install the CLI, log in, then work in a local workspace.",
    terminalTitle: "AFS CLI",
    setupCommand: "curl -fsSL https://afs.cloud/install.sh | bash",
    language: "shell",
    code: cliGettingStartedSample,
  },
  {
    label: "MCP",
    title: "Register the hosted MCP endpoint and keep the bearer token in AFS_TOKEN.",
    terminalTitle: "MCP config",
    setupCommand: "codex mcp add agent-filesystem --transport http https://afs.cloud/mcp --bearer-token-env AFS_TOKEN",
    language: "json",
    code: mcpGettingStartedSample,
  },
  {
    label: "TypeScript",
    title: "Install the SDK; the client reads AFS_API_KEY from the environment.",
    terminalTitle: "TypeScript SDK",
    setupCommand: "npm install redis-afs",
    language: "typescript",
    code: typescriptSdkSample,
  },
  {
    label: "Python",
    title: "Install the SDK; the client reads AFS_API_KEY from the environment.",
    terminalTitle: "Python SDK",
    setupCommand: "pip install redis-afs",
    language: "python",
    code: pythonSdkSample,
  },
] as const;

type GettingStartedSample = (typeof gettingStartedSamples)[number];
type GettingStartedLanguage = (typeof gettingStartedSamples)[number]["language"];

export const Route = createFileRoute("/")({
  loader: async () => {
    await Promise.all([
      queryClient.ensureQueryData({ ...databasesQueryOptions(), revalidateIfStale: true }),
      queryClient.ensureQueryData({
        ...workspaceSummariesQueryOptions(null),
        revalidateIfStale: true,
      }),
      queryClient.ensureQueryData({ ...agentsQueryOptions(null), revalidateIfStale: true }),
    ]);
  },
  component: OverviewPage,
});

function OverviewPage() {
  const workspacesQuery = useScopedWorkspaceSummaries();
  const agentsQuery = useScopedAgents();
  const { databases, isLoading: databasesLoading } = useDatabaseScope();
  const [onboardingWorkspace, setOnboardingWorkspace] = useState<AFSWorkspaceDetail | null>(null);

  if (databasesLoading || workspacesQuery.isLoading || agentsQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const hasDatabase = databases.length > 0;

  const workspaces = workspacesQuery.data;
  let content;
  if (!hasDatabase) {
    content = <GettingStartedView hasDatabase={false} onQuickstartCreated={setOnboardingWorkspace} />;
  } else if (workspaces.length === 0) {
    content = <GettingStartedView hasDatabase={true} onQuickstartCreated={setOnboardingWorkspace} />;
  } else {
    /* ── Dashboard ── */
    const workspacesWithCheckpoints = workspaces.filter((workspace) => workspace.checkpointCount > 0).length;
    const checkpointCount = workspaces.reduce((sum, workspace) => sum + workspace.checkpointCount, 0);
    const totalBytes = workspaces.reduce((sum, workspace) => sum + workspace.totalBytes, 0);
    const checkpointCoverage = workspaces.length === 0 ? 0 : Math.round((workspacesWithCheckpoints / workspaces.length) * 100);

    content = (
      <DashboardView
        databases={databases}
        workspaces={workspaces}
        agents={agentsQuery.data}
        checkpointCount={checkpointCount}
        checkpointCoverage={checkpointCoverage}
        totalBytes={totalBytes}
      />
    );
  }

  return (
    <>
      {content}
      {onboardingWorkspace ? (
        <GettingStartedOnboardingDialog
          open
          workspaceId={onboardingWorkspace.id}
          workspaceName={onboardingWorkspace.name}
          databaseName={onboardingWorkspace.databaseName}
          fileCount={onboardingWorkspace.fileCount}
          folderCount={onboardingWorkspace.folderCount}
          onClose={() => setOnboardingWorkspace(null)}
        />
      ) : null}
    </>
  );
}

function DashboardView({ databases, workspaces, agents, checkpointCount, checkpointCoverage, totalBytes }: {
  databases: { length: number };
  workspaces: AFSWorkspaceSummary[];
  agents: unknown[];
  checkpointCount: number;
  checkpointCoverage: number;
  totalBytes: number;
}) {
  const navigate = useNavigate();
  const connectedAgents = agents.length;

  return (
    <PageStack>
      <OverviewStatGrid>
        <ClickableStatCard onClick={() => navigate({ to: "/workspaces" })}>
          <div>
            <StatLabel>Workspaces</StatLabel>
            <StatValue>{workspaces.length}</StatValue>
          </div>
          <StatDetail>
            Across {databases.length} database{databases.length === 1 ? "" : "s"}.
          </StatDetail>
        </ClickableStatCard>
        <ClickableStatCard onClick={() => navigate({ to: "/workspaces" })}>
          <div>
            <StatLabel>Stored Data</StatLabel>
            <StatValue>{formatBytes(totalBytes)}</StatValue>
          </div>
          <StatDetail>Durable content in Redis.</StatDetail>
        </ClickableStatCard>
        <ClickableStatCard onClick={() => navigate({ to: "/workspaces" })}>
          <div>
            <StatLabel>Checkpoints</StatLabel>
            <StatValue>{checkpointCount}</StatValue>
          </div>
          <StatDetail>{checkpointCoverage}% with history.</StatDetail>
        </ClickableStatCard>
        <ClickableStatCard onClick={() => navigate({ to: "/agents" })}>
          <div>
            <StatLabel>Agents</StatLabel>
            <StatValue>{connectedAgents}</StatValue>
          </div>
          <StatDetail>
            {connectedAgents === 0
              ? "No live agents."
              : `${connectedAgents} live ${connectedAgents === 1 ? "session" : "sessions"}.`}
          </StatDetail>
        </ClickableStatCard>
      </OverviewStatGrid>
      <LiveTopologyCard agents={agents as any} workspaces={workspaces as any} />
      <CliQuickstartCard />
      <TemplatesLinkCard as={Link} to="/templates">
        <TemplatesLinkCopy>
          <TemplatesLinkEyebrow>Templates</TemplatesLinkEyebrow>
          <TemplatesLinkTitle>Start from a prepared workspace</TemplatesLinkTitle>
          <TemplatesLinkText>
            Browse shared-memory, wiki, coding-standards, and team-planning
            templates when you want a seeded workspace instead of a blank one.
          </TemplatesLinkText>
        </TemplatesLinkCopy>
        <TemplatesLinkArrow>&rarr;</TemplatesLinkArrow>
      </TemplatesLinkCard>
    </PageStack>
  );
}

function CliQuickstartCard() {
  const [activeHintIndex, setActiveHintIndex] = useState(0);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const activeHint = gettingStartedSamples[activeHintIndex];

  function copySnippet(key: string, value: string) {
    if (typeof navigator === "undefined" || !navigator.clipboard) {
      return;
    }
    void navigator.clipboard.writeText(value).then(() => {
      setCopiedKey(key);
      window.setTimeout(() => {
        setCopiedKey((current) => (current === key ? null : current));
      }, 1600);
    });
  }

  return (
    <CliQuickstart>
      <CliQuickstartCopy>
        <CliQuickstartEyebrow>Getting Started</CliQuickstartEyebrow>
        <CliQuickstartTitle>Start from the AFS CLI or an SDK.</CliQuickstartTitle>
        <CliLessonTabs role="tablist" aria-label="Getting started examples">
          {gettingStartedSamples.map((hint, index) => (
            <CliLessonTab
              key={hint.label}
              type="button"
              role="tab"
              aria-selected={activeHintIndex === index}
              $active={activeHintIndex === index}
              onClick={() => setActiveHintIndex(index)}
            >
              {hint.label}
            </CliLessonTab>
          ))}
        </CliLessonTabs>
        <CliLessonDetail>
          <CliLessonTitle>{activeHint.title}</CliLessonTitle>
        </CliLessonDetail>
      </CliQuickstartCopy>
      <OverviewTerminal sample={activeHint} copiedKey={copiedKey} onCopy={copySnippet} />
    </CliQuickstart>
  );
}

function GettingStartedView({
  hasDatabase,
  onQuickstartCreated,
}: {
  hasDatabase: boolean;
  onQuickstartCreated: (workspace: AFSWorkspaceDetail) => void;
}) {
  const quickstartMutation = useQuickstartMutation();
  const quickstartErrorMessage = quickstartMutation.isError
    ? quickstartMutation.error.message || "Something went wrong."
    : null;

  const handleQuickstart = async () => {
    try {
      const result = await quickstartMutation.mutateAsync({});
      onQuickstartCreated(result.workspace);
    } catch {
      // Error is stored in quickstartMutation.error
    }
  };

  return (
    <PageStack>
      <HeroLayout>
        <HeroEyebrow>Agent Filesystem</HeroEyebrow>
        <HeroAnimationWrap>
          <AgentHeroAnimation />
        </HeroAnimationWrap>
        <Headline>
          A filesystem your AI agents can trust.
        </Headline>
        <Description>
          Give every agent a persistent, checkpointed workspace backed by
          Redis. Edit files, snapshot state, and replay history &mdash; all
          from one place.
        </Description>

        <CTABlock>
          <PrimaryCTA
            size="large"
            onClick={handleQuickstart}
            disabled={quickstartMutation.isPending}
          >
            {quickstartMutation.isPending
              ? "Setting up\u2026"
              : "Create my first workspace \u2192"}
          </PrimaryCTA>
          <CTAHint>
            {hasDatabase
              ? "We'll preload sample files so you can explore in seconds."
              : "Requires Redis running on localhost:6379"}
          </CTAHint>
          {quickstartErrorMessage ? (
            <QuickstartError>
              {quickstartErrorMessage.includes("cannot connect")
                ? "Could not connect to Redis at localhost:6379. Start Redis locally or add a remote database instead."
                : quickstartErrorMessage}
            </QuickstartError>
          ) : null}
        </CTABlock>

        <BenefitsGrid>
          <Benefit>
            <BenefitIcon>
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <ellipse cx="12" cy="5" rx="9" ry="3" />
                <path d="M3 5v14a9 3 0 0 0 18 0V5" />
                <path d="M3 12a9 3 0 0 0 18 0" />
              </svg>
            </BenefitIcon>
            <BenefitTitle>Persistent by default</BenefitTitle>
            <BenefitDesc>
              Workspaces live in Redis &mdash; no local state to sync,
              restore, or lose when you switch machines.
            </BenefitDesc>
          </Benefit>
          <Benefit>
            <BenefitIcon>
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 12a9 9 0 1 0 9-9" />
                <polyline points="3 4 3 12 11 12" />
              </svg>
            </BenefitIcon>
            <BenefitTitle>Checkpoint &amp; rollback</BenefitTitle>
            <BenefitDesc>
              Snapshot before risky changes. Restore the workspace to any
              previous state in seconds when an agent goes off the rails.
            </BenefitDesc>
          </Benefit>
          <Benefit>
            <BenefitIcon>
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <polyline points="16 18 22 12 16 6" />
                <polyline points="8 6 2 12 8 18" />
              </svg>
            </BenefitIcon>
            <BenefitTitle>CLI &amp; MCP ready</BenefitTitle>
            <BenefitDesc>
              Mount workspaces locally with one command, or plug them into
              any MCP-capable agent &mdash; Claude, Cursor, Windsurf.
            </BenefitDesc>
          </Benefit>
        </BenefitsGrid>

        <FooterLink as={Link} to="/agent-guide">
          Read the full Agent Guide &rarr;
        </FooterLink>
      </HeroLayout>
    </PageStack>
  );
}

/* ── Styled components ── */

const HeroLayout = styled.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 24px 0 32px;
  max-width: 880px;
  margin: 0 auto;
`;

const HeroEyebrow = styled.div`
  color: var(--afs-accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
`;

const HeroAnimationWrap = styled.div`
  margin: 12px 0 8px;
  width: 100%;
  display: flex;
  justify-content: center;
`;

const Headline = styled.h2`
  margin: 8px 0 12px;
  color: var(--afs-ink);
  font-size: 42px;
  font-weight: 700;
  line-height: 1.1;
  letter-spacing: 0;
  max-width: 18ch;

  @media (max-width: 720px) {
    font-size: 32px;
  }
`;

const Description = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 17px;
  line-height: 1.55;
  max-width: 56ch;
`;

const CTABlock = styled.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  margin: 28px 0 8px;
  width: 100%;
`;

const PrimaryCTA = styled(Button)`
  && {
    padding-left: 28px;
    padding-right: 28px;
    font-size: 15px;
    box-shadow: 0 10px 28px color-mix(in srgb, var(--afs-accent) 30%, transparent);
  }
`;

const CTAHint = styled.div`
  color: var(--afs-muted);
  font-size: 13px;
`;

const QuickstartError = styled.div`
  color: #dc2626;
  font-size: 13px;
  line-height: 1.5;
  padding: 10px 14px;
  background: #fef2f2;
  border-radius: 10px;
  max-width: 480px;
`;

const BenefitsGrid = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  width: 100%;
  margin-top: 40px;

  @media (max-width: 760px) {
    grid-template-columns: 1fr;
  }
`;

const Benefit = styled.div`
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  text-align: left;
  gap: 10px;
  padding: 22px 22px 24px;
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background: var(--afs-panel);
  transition: border-color 180ms ease, transform 180ms ease;

  &:hover {
    border-color: color-mix(in srgb, var(--afs-accent, #2563eb) 30%, var(--afs-line));
    transform: translateY(-2px);
  }
`;

const BenefitIcon = styled.div`
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 12px;
  background: var(--afs-accent-soft, color-mix(in srgb, var(--afs-accent, #2563eb) 12%, transparent));
  color: var(--afs-accent, #2563eb);
`;

const BenefitTitle = styled.div`
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
  letter-spacing: -0.01em;
`;

const BenefitDesc = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 13.5px;
  line-height: 1.6;
`;

const FooterLink = styled.a`
  margin-top: 32px;
  color: var(--afs-accent, #2563eb);
  font-size: 14px;
  font-weight: 600;
  text-decoration: none;

  &:hover {
    text-decoration: underline;
  }
`;

const OverviewStatGrid = styled(StatGrid)`
  container-type: inline-size;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  align-items: stretch;

  @media (max-width: 1080px) {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }

  @media (max-width: 700px) {
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 10px;
  }

  ${StatCard} {
    min-width: 0;
  }

  ${StatLabel},
  ${StatValue},
  ${StatDetail} {
    min-width: 0;
    overflow-wrap: anywhere;
  }

  ${StatLabel} {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  ${StatValue} {
    font-size: clamp(1.8rem, 3vw, 2.55rem);
  }

  ${StatDetail} {
    display: -webkit-box;
    overflow: hidden;
    line-height: 1.35;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
  }

  @container (max-width: 760px) {
    ${StatCard} {
      min-height: 82px;
      justify-content: flex-start;
    }

    ${StatDetail} {
      display: none;
    }
  }

  @container (max-width: 520px) {
    gap: 8px;

    ${StatCard} {
      gap: 6px;
      min-height: 74px;
      padding: 12px 10px;
    }

    ${StatLabel} {
      font-size: 10px;
      letter-spacing: 0;
    }

    ${StatValue} {
      font-size: 1.6rem;
      letter-spacing: 0;
    }
  }

  @container (max-width: 380px) {
    gap: 6px;

    ${StatCard} {
      min-height: 68px;
      padding: 10px 8px;
    }

    ${StatLabel} {
      font-size: 9px;
    }

    ${StatValue} {
      font-size: 1.3rem;
    }
  }
`;

function OverviewTerminal({
  sample,
  copiedKey,
  onCopy,
}: {
  sample: GettingStartedSample;
  copiedKey: string | null;
  onCopy: (key: string, value: string) => void;
}) {
  const setupKey = `${sample.label}-setup`;

  return (
    <OverviewTerminalFrame>
      <OverviewTerminalTopBar>
        <OverviewTerminalDots aria-hidden="true">
          <span />
          <span />
          <span />
        </OverviewTerminalDots>
        <OverviewTerminalTitle>{sample.terminalTitle}</OverviewTerminalTitle>
      </OverviewTerminalTopBar>
      <OverviewTerminalBody>
        <OverviewSetupLine>
          <OverviewSetupCode>
            <code className="language-shell">{highlightCode(sample.setupCommand, "shell")}</code>
          </OverviewSetupCode>
          <OverviewCopyButton
            type="button"
            aria-label={`Copy ${sample.label} setup command`}
            title={copiedKey === setupKey ? "Copied" : "Copy setup command"}
            onClick={() => onCopy(setupKey, sample.setupCommand)}
          >
            {copiedKey === setupKey ? <Check size={16} strokeWidth={1.9} /> : <Copy size={16} strokeWidth={1.9} />}
          </OverviewCopyButton>
        </OverviewSetupLine>
        <OverviewCodePane>
          <HighlightedOverviewCode code={sample.code} language={sample.language} />
        </OverviewCodePane>
      </OverviewTerminalBody>
    </OverviewTerminalFrame>
  );
}

function HighlightedOverviewCode({
  code,
  language,
}: {
  code: string;
  language: GettingStartedLanguage;
}) {
  return (
    <OverviewTerminalCode>
      <code className={`language-${language}`}>{highlightCode(code, language)}</code>
    </OverviewTerminalCode>
  );
}

function highlightCode(code: string, language: GettingStartedLanguage) {
  const lines = code.split("\n");
  return lines.flatMap((line, index) => {
    const parts = highlightLine(line, language, index);
    if (index === lines.length - 1) {
      return parts;
    }
    return [...parts, "\n"];
  });
}

function highlightLine(line: string, language: GettingStartedLanguage, lineIndex: number) {
  const pattern = highlightPattern(language);
  const parts: React.ReactNode[] = [];
  let cursor = 0;
  for (const match of line.matchAll(pattern)) {
    const value = match[0];
    const index = match.index ?? 0;
    if (index > cursor) {
      parts.push(line.slice(cursor, index));
    }
    parts.push(
      <span key={`${language}-${lineIndex}-${index}-${value}`} className={highlightClass(value, line, index, language)}>
        {value}
      </span>,
    );
    cursor = index + value.length;
  }
  if (cursor < line.length) {
    parts.push(line.slice(cursor));
  }
  return parts;
}

function highlightPattern(language: GettingStartedLanguage) {
  switch (language) {
    case "typescript":
      return /\/\/.*|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`\\])*`|\b(?:await|const|from|import|new|process)\b|\b[A-Z][A-Za-z0-9_]*\b|\b\d+\b/g;
    case "python":
      return /#.*|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|\b(?:from|import|as|print)\b|\b[A-Z][A-Za-z0-9_]*\b|\b\d+\b/g;
    case "shell":
      return /#.*|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|\b(?:afs|bash|cat|codex|curl|echo|export|npm|pip|sh)\b|--[A-Za-z0-9-]+|\$[A-Za-z_][A-Za-z0-9_]*/g;
    case "json":
      return /"(?:\\.|[^"\\])*"|\b(?:true|false|null)\b|\b\d+\b/g;
  }
}

function highlightClass(
  value: string,
  line: string,
  index: number,
  language: GettingStartedLanguage,
) {
  if (value.startsWith("//") || value.startsWith("#")) {
    return "code-token-comment";
  }
  if (language === "json" && line.slice(index + value.length).trimStart().startsWith(":")) {
    return "code-token-key";
  }
  if (value.startsWith("\"") || value.startsWith("'") || value.startsWith("`")) {
    return "code-token-string";
  }
  if (value.startsWith("--") || value.startsWith("$")) {
    return "code-token-option";
  }
  if (/^\d+$/.test(value) || /^(true|false|null)$/.test(value)) {
    return "code-token-number";
  }
  if (/^[A-Z]/.test(value)) {
    return "code-token-type";
  }
  return "code-token-keyword";
}

const CliQuickstart = styled.section`
  display: grid;
  grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr);
  gap: 20px;
  align-items: stretch;
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background: var(--afs-panel-strong);
  padding: 20px;

  @media (max-width: 980px) {
    grid-template-columns: 1fr;
  }

  @media (max-width: 640px) {
    padding: 16px;
  }

  [data-skin="situation-room"] && {
    border-radius: var(--afs-r-2);
    border-color: var(--afs-line-strong);
    background: var(--afs-bg-1);
  }
`;

const CliQuickstartCopy = styled.div`
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
  min-width: 0;
`;

const CliQuickstartEyebrow = styled.div`
  color: var(--afs-accent, #2563eb);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
`;

const CliQuickstartTitle = styled.h2`
  margin: 0;
  color: var(--afs-ink);
  font-size: 18px;
  font-weight: 750;
  line-height: 1.25;
  letter-spacing: 0;
`;

const CliLessonTabs = styled(Tabs)`
  display: flex;
  flex-wrap: wrap;
  width: 100%;
  max-width: 100%;
  overflow-x: visible;
  overscroll-behavior-x: contain;
  scroll-snap-type: x proximity;
  scrollbar-width: thin;

  @media (max-width: 640px) {
    flex-wrap: nowrap;
    overflow-x: auto;
  }
`;

const CliLessonTab = styled(TabButton)`
  flex: 0 0 auto;
  scroll-snap-align: start;
  white-space: nowrap;

  [data-skin="situation-room"] && {
    letter-spacing: 0;
  }
`;

const CliLessonDetail = styled.div`
  display: grid;
  gap: 6px;
  min-width: 0;
`;

const CliLessonTitle = styled.h3`
  margin: 0;
  color: var(--afs-ink);
  font-size: 16px;
  font-weight: 750;
  line-height: 1.25;
`;

const OverviewTerminalFrame = styled.div`
  overflow: hidden;
  align-self: stretch;
  min-width: 0;
  border: 1px solid var(--afs-line, #e6e6e6);
  border-radius: 8px;
  background: #101820;
  box-shadow: var(--afs-shadow, none);

  [data-skin="situation-room"] && {
    border-radius: var(--afs-r-2);
    border-color: var(--afs-line);
  }
`;

const OverviewTerminalTopBar = styled.div`
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  gap: 14px;
  min-height: 34px;
  padding: 0 14px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  background: #1a242e;
`;

const OverviewTerminalDots = styled.div`
  display: flex;
  gap: 6px;

  span {
    width: 10px;
    height: 10px;
    border-radius: 999px;
    background: #6d6e71;
  }

  span:nth-child(1) {
    background: #dc2626;
  }

  span:nth-child(2) {
    background: #f59e0b;
  }

  span:nth-child(3) {
    background: #16a34a;
  }
`;

const OverviewTerminalTitle = styled.div`
  min-width: 0;
  overflow: hidden;
  color: #d1d3d4;
  font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const OverviewTerminalBody = styled.div`
  color: #f8f8f8;
  font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
  font-size: 13px;
  line-height: 1.65;
  overflow: hidden;

  @media (max-width: 640px) {
    font-size: 12px;
  }
`;

const OverviewSetupLine = styled.div`
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  min-height: 46px;
  padding: 8px 12px 8px 18px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.035);
`;

const OverviewSetupCode = styled.pre`
  margin: 0;
  min-width: 0;
  overflow-x: auto;
  color: #d1d3d4;
  font: inherit;
  line-height: 1.55;
  white-space: pre;

  code {
    font: inherit;
  }
`;

const OverviewCopyButton = styled.button`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  flex: 0 0 auto;
  border: 1px solid transparent;
  border-radius: 6px;
  background: transparent;
  color: #aab4c0;
  cursor: pointer;
  transition: background 140ms ease, border-color 140ms ease, color 140ms ease;

  &:hover {
    border-color: rgba(255, 255, 255, 0.16);
    background: rgba(255, 255, 255, 0.08);
    color: #ffffff;
  }

  &:focus-visible {
    outline: 2px solid var(--afs-accent, #dc2626);
    outline-offset: 1px;
  }
`;

const OverviewCodePane = styled.div`
  min-height: 286px;
  max-height: 420px;
  padding: 18px;
  overflow-x: auto;

  @media (max-width: 640px) {
    padding: 18px;
  }
`;

const OverviewTerminalCode = styled.pre`
  margin: 0;
  color: #ffffff;
  white-space: pre;

  code {
    font: inherit;
  }

  .code-token-comment {
    color: #8fa1b3;
  }

  .code-token-keyword {
    color: #7dd3fc;
  }

  .code-token-string {
    color: #bef264;
  }

  .code-token-key {
    color: #c4b5fd;
  }

  .code-token-type {
    color: #f0abfc;
  }

  .code-token-option,
  .code-token-number {
    color: #fdba74;
  }
`;

const TemplatesLinkCard = styled.a`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background: var(--afs-panel-strong);
  padding: 18px 20px;
  color: inherit;
  text-decoration: none;
  transition: border-color 180ms ease, transform 180ms ease, box-shadow 180ms ease;

  &:hover {
    border-color: var(--afs-accent, #2563eb);
    box-shadow: 0 6px 20px rgba(8, 6, 13, 0.08);
    transform: translateY(-2px);
  }

  [data-skin="situation-room"] && {
    border-radius: var(--afs-r-2);
    border-color: var(--afs-line-strong);
    background: var(--afs-bg-1);
  }

  @media (max-width: 640px) {
    align-items: flex-start;
  }
`;

const TemplatesLinkCopy = styled.span`
  display: grid;
  gap: 4px;
  min-width: 0;
`;

const TemplatesLinkEyebrow = styled.span`
  color: var(--afs-accent, #2563eb);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
`;

const TemplatesLinkTitle = styled.span`
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 750;
  line-height: 1.3;
`;

const TemplatesLinkText = styled.span`
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.5;
`;

const TemplatesLinkArrow = styled.span`
  color: var(--afs-accent, #2563eb);
  font-size: 22px;
  line-height: 1;
  flex: 0 0 auto;
`;

const ClickableStatCardWrap = styled.div`
  height: 100%;
  min-width: 0;
  cursor: pointer;
  transition: border-color 180ms ease, transform 180ms ease, box-shadow 180ms ease;
  border-radius: 16px;

  &:hover {
    transform: translateY(-2px);
    box-shadow: 0 6px 20px rgba(8, 6, 13, 0.08);
  }

  &:hover > * {
    border-color: var(--afs-accent, #2563eb);
  }

  > * {
    height: 100%;
  }
`;

function ClickableStatCard({ onClick, children }: { onClick: () => void; children: React.ReactNode }) {
  return (
    <ClickableStatCardWrap onClick={onClick}>
      <StatCard>{children}</StatCard>
    </ClickableStatCardWrap>
  );
}
