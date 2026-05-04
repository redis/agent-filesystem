import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { Loader } from "@redis-ui/components";
import styled from "styled-components";
import { PageStack } from "../components/afs-kit";
import { AgentHeroAnimation } from "../components/agent-hero-animation";
import { OnboardingPathCard } from "../components/onboarding-drawer";
import type { OnboardingPath } from "../components/onboarding-drawer";
import { PublicLandingPage } from "../features/landing/PublicLandingPage";
import { useDrawer } from "../foundation/drawer-context";
import { afsApi } from "../foundation/api/afs";
import { useAuthSession } from "../foundation/auth-context";
import { useDatabaseScope, useScopedActivity, useScopedAgents, useScopedWorkspaceSummaries } from "../foundation/database-scope";
import { ActivityTable } from "../foundation/tables/activity-table";
import type { AFSActivityEvent } from "../foundation/types/afs";
import { queryClient } from "../foundation/query-client";
import {
  agentsQueryOptions,
  databasesQueryOptions,
  useQuickstartMutation,
  workspaceSummariesQueryOptions,
} from "../foundation/hooks/use-afs";
import type { AFSAgentSession, AFSWorkspaceSummary } from "../foundation/types/afs";

export const Route = createFileRoute("/")({
  loader: async () => {
    const authConfig = await afsApi.getAuthConfig().catch(() => null);
    if (authConfig == null || (authConfig.signInRequired && !authConfig.authenticated)) {
      return;
    }

    await Promise.all([
      queryClient.ensureQueryData({ ...databasesQueryOptions(), revalidateIfStale: true }),
      queryClient.ensureQueryData({
        ...workspaceSummariesQueryOptions(null),
        revalidateIfStale: true,
      }),
      queryClient.ensureQueryData({ ...agentsQueryOptions(null), revalidateIfStale: true }),
    ]);
  },
  component: HomeRoute,
});

function HomeRoute() {
  const auth = useAuthSession();

  if (auth.isLoading || auth.isSignedOut) {
    return <PublicLandingPage />;
  }

  return <OverviewPage />;
}

function OverviewPage() {
  const workspacesQuery = useScopedWorkspaceSummaries();
  const agentsQuery = useScopedAgents();
  const { databases, isLoading: databasesLoading } = useDatabaseScope();
  const quickstartMutation = useQuickstartMutation();
  const { open: openDrawer } = useDrawer();

  if (databasesLoading || workspacesQuery.isLoading || agentsQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const hasDatabase = databases.length > 0;
  const workspaces = workspacesQuery.data;
  // If the user already has any workspace, treat the quickstart as effectively
  // done — re-opening the drawer should not re-fire creation.
  const haveAnyWorkspace = workspaces.length > 0;

  function handleChoosePath(path: OnboardingPath) {
    openDrawer({ kind: "onboarding", path });
    const alreadyHandled =
      haveAnyWorkspace ||
      quickstartMutation.isSuccess ||
      quickstartMutation.isPending;
    if (!alreadyHandled && hasDatabase) {
      void quickstartMutation.mutateAsync({}).catch(() => undefined);
    }
  }

  if (!hasDatabase) {
    return <GettingStartedView hasDatabase={false} onChoosePath={handleChoosePath} />;
  }
  if (workspaces.length === 0) {
    return <GettingStartedView hasDatabase={true} onChoosePath={handleChoosePath} />;
  }
  return (
    <InspectorView
      workspaces={workspaces}
      agents={agentsQuery.data}
      onChoosePath={handleChoosePath}
    />
  );
}

// InspectorView — the new home page when you have at least one workspace.
//
// Replaces the old "Dashboard" of stat cards. The headline content is a live
// activity stream — what your CLI and agents are *currently doing*. Stats are
// reduced to a slim StatusHeader so the page reads as observability, not as
// a control surface.
function InspectorView({
  workspaces,
  agents,
  onChoosePath,
}: {
  workspaces: AFSWorkspaceSummary[];
  agents: AFSAgentSession[];
  onChoosePath: (path: OnboardingPath) => void;
}) {
  const navigate = useNavigate();
  const activityQuery = useScopedActivity(50);
  const connectedAgents = agents.length;
  const opsPerMin = computeOpsPerMin(activityQuery.data);

  function openActivity(event: AFSActivityEvent) {
    if (!event.workspaceId) return;
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: event.workspaceId },
      search: {
        ...(event.databaseId ? { databaseId: event.databaseId } : {}),
        tab: "activity",
      },
    });
  }

  return (
    <PageStack>
      <StatusHeader
        workspaces={workspaces.length}
        activeSessions={connectedAgents}
        opsPerMin={opsPerMin}
        loading={activityQuery.isLoading}
      />

      <ActiveAgentsPanel agents={agents} onOpenAgents={() => void navigate({ to: "/agents" })} />

      <ActivityCard>
        <ActivityCardHeader>
          <ActivityCardEyebrow>Live activity</ActivityCardEyebrow>
          <ActivityCardSub>
            What your CLI and agents are doing right now. Tail the full stream
            on any workspace.
          </ActivityCardSub>
        </ActivityCardHeader>
        <ActivityTable
          rows={activityQuery.data}
          loading={activityQuery.isLoading}
          error={activityQuery.isError}
          errorMessage={activityQuery.error instanceof Error ? activityQuery.error.message : undefined}
          onOpenActivity={openActivity}
        />
      </ActivityCard>

      <CliQuickstartCard onChoosePath={onChoosePath} />

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

// ActiveAgentsPanel — compact list of currently-connected agents and the
// workspace each is on. This is the "watch your CLI/agents work" cue for the
// Inspector page: dense, no animation, per-row state dot for at-a-glance
// activity. Replaces the old animated topology graph.
function ActiveAgentsPanel({ agents, onOpenAgents }: {
  agents: AFSAgentSession[];
  onOpenAgents: () => void;
}) {
  if (agents.length === 0) return null;
  // sort: most recently active first
  const ordered = [...agents].sort((a, b) =>
    Date.parse(b.lastSeenAt) - Date.parse(a.lastSeenAt),
  );
  const showCount = Math.min(6, ordered.length);
  const overflow = ordered.length - showCount;
  const visible = ordered.slice(0, showCount);

  return (
    <AgentsPanelCard>
      <AgentsPanelHeader>
        <AgentsPanelEyebrow>Active agents ({agents.length})</AgentsPanelEyebrow>
        <AgentsPanelLink type="button" onClick={onOpenAgents}>
          all agents &rarr;
        </AgentsPanelLink>
      </AgentsPanelHeader>
      <AgentsPanelList>
        {visible.map((agent) => (
          <AgentRow key={agent.sessionId}>
            <AgentDot $idle={isAgentIdle(agent)} />
            <AgentLabel>{agentDisplayLabel(agent)}</AgentLabel>
            <AgentArrow>&rarr;</AgentArrow>
            <AgentWorkspace>{agent.workspaceName}</AgentWorkspace>
            <AgentMeta>
              <AgentTag>{agent.clientKind}</AgentTag>
              <AgentTag>{agent.readonly ? "RO" : "RW"}</AgentTag>
              <AgentTag>{agent.operatingSystem}</AgentTag>
            </AgentMeta>
            <AgentSeen>{relativeAgentSeen(agent.lastSeenAt)}</AgentSeen>
          </AgentRow>
        ))}
        {overflow > 0 ? (
          <AgentRow>
            <AgentMore type="button" onClick={onOpenAgents}>
              + {overflow} more &rarr;
            </AgentMore>
          </AgentRow>
        ) : null}
      </AgentsPanelList>
    </AgentsPanelCard>
  );
}

function agentDisplayLabel(agent: AFSAgentSession) {
  return agent.label || agent.agentId || agent.hostname || agent.sessionId.slice(0, 12);
}

function isAgentIdle(agent: AFSAgentSession) {
  const last = Date.parse(agent.lastSeenAt);
  if (!Number.isFinite(last)) return true;
  return Date.now() - last > 30_000;
}

function relativeAgentSeen(iso: string) {
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return "—";
  const seconds = Math.max(0, Math.floor((Date.now() - t) / 1000));
  if (seconds < 5) return "just now";
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  return `${Math.floor(seconds / 3600)}h ago`;
}

// Compact inline status. Replaces the four stat cards with a single line that
// reads like a process header: live indicator, key counts, current op rate.
function StatusHeader({ workspaces, activeSessions, opsPerMin, loading }: {
  workspaces: number;
  activeSessions: number;
  opsPerMin: number;
  loading: boolean;
}) {
  return (
    <StatusBar>
      <StatusLive>
        <StatusDot $live={!loading} />
        <StatusLiveText>{loading ? "loading" : "live"}</StatusLiveText>
      </StatusLive>
      <StatusSep>·</StatusSep>
      <StatusItem>
        <StatusValue>{workspaces}</StatusValue>
        <StatusLabel>workspace{workspaces === 1 ? "" : "s"}</StatusLabel>
      </StatusItem>
      <StatusSep>·</StatusSep>
      <StatusItem>
        <StatusValue>{activeSessions}</StatusValue>
        <StatusLabel>active session{activeSessions === 1 ? "" : "s"}</StatusLabel>
      </StatusItem>
      <StatusSep>·</StatusSep>
      <StatusItem>
        <StatusValue>{opsPerMin}</StatusValue>
        <StatusLabel>ops/min</StatusLabel>
      </StatusItem>
    </StatusBar>
  );
}

// Count activity events whose createdAt falls within the last 60s.
function computeOpsPerMin(events: AFSActivityEvent[]) {
  const cutoff = Date.now() - 60_000;
  return events.reduce((count, e) => {
    const t = Date.parse(e.createdAt);
    return Number.isFinite(t) && t >= cutoff ? count + 1 : count;
  }, 0);
}

function CliQuickstartCard({
  onChoosePath,
}: {
  onChoosePath: (path: OnboardingPath) => void;
}) {
  return (
    <GettingStartedShell>
      <GettingStartedHeader>
        <CliQuickstartEyebrow>Getting Started</CliQuickstartEyebrow>
        <CliQuickstartTitle>Need to (re)open the quick start?</CliQuickstartTitle>
        <CliQuickstartSubline>
          Pick a path. Your <Mono>getting-started</Mono> workspace already
          exists — these just bring the instructions back.
        </CliQuickstartSubline>
      </GettingStartedHeader>

      <PathPair>
        <OnboardingPathCard
          tone="primary"
          badge="Recommended"
          title="Connect your agent"
          description="Paste a prompt into Claude, Cursor, Codex, or any MCP-capable agent."
          buttonLabel="Open"
          onClick={() => onChoosePath("agent")}
        />
        <OnboardingPathCard
          tone="secondary"
          title="Use the CLI"
          description="Install, authenticate, and mount the workspace from your shell."
          buttonLabel="Open"
          onClick={() => onChoosePath("cli")}
        />
      </PathPair>
    </GettingStartedShell>
  );
}

function GettingStartedView({
  hasDatabase,
  onChoosePath,
}: {
  hasDatabase: boolean;
  onChoosePath: (path: OnboardingPath) => void;
}) {
  return (
    <PageStack>
      <HeroLayout>
        <HeroEyebrow>Agent Filesystem</HeroEyebrow>
        <HeroAnimationWrap>
          <AgentHeroAnimation />
        </HeroAnimationWrap>
        <Headline>
          A filesystem for <Strike>humans</Strike> agents.
        </Headline>
        <Description>
          Built for AI agents &mdash; not a dashboard for you to click around.
          Point your agent here. It&rsquo;ll do the rest.
        </Description>

        <PathChoiceGrid>
          <OnboardingPathCard
            tone="primary"
            badge="Recommended"
            title="Connect your agent"
            description="Paste a prompt into Claude, Cursor, Codex, or any MCP-capable agent. It installs the CLI and connects."
            buttonLabel="Choose"
            onClick={() => onChoosePath("agent")}
            disabled={!hasDatabase}
          />
          <OnboardingPathCard
            tone="secondary"
            title="Use the CLI"
            description="Install the CLI, authenticate, and mount the getting-started workspace from your shell."
            buttonLabel="Choose"
            onClick={() => onChoosePath("cli")}
            disabled={!hasDatabase}
          />
        </PathChoiceGrid>

        {!hasDatabase ? (
          <DbWarning role="alert">
            Redis isn&rsquo;t reachable on <code>localhost:6379</code>. Start
            Redis locally or add a remote database to continue.
          </DbWarning>
        ) : null}

        <BenefitsGrid>
          <Benefit>
            <BenefitIcon>
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <polyline points="16 18 22 12 16 6" />
                <polyline points="8 6 2 12 8 18" />
              </svg>
            </BenefitIcon>
            <BenefitTitle>MCP-native</BenefitTitle>
            <BenefitDesc>
              Every workspace operation is an MCP tool call. Plug AFS into
              Claude, Cursor, Windsurf, or any MCP-capable runtime.
            </BenefitDesc>
          </Benefit>
          <Benefit>
            <BenefitIcon>
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 12a9 9 0 1 0 9-9" />
                <polyline points="3 4 3 12 11 12" />
              </svg>
            </BenefitIcon>
            <BenefitTitle>Checkpoints your agent can roll back to</BenefitTitle>
            <BenefitDesc>
              Agents snapshot before risky changes. Restore to any prior state
              when something goes off the rails.
            </BenefitDesc>
          </Benefit>
          <Benefit>
            <BenefitIcon>
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <ellipse cx="12" cy="5" rx="9" ry="3" />
                <path d="M3 5v14a9 3 0 0 0 18 0V5" />
                <path d="M3 12a9 3 0 0 0 18 0" />
              </svg>
            </BenefitIcon>
            <BenefitTitle>Persistent across sessions</BenefitTitle>
            <BenefitDesc>
              State lives in Redis. Switch machines, swap agents, resume
              tomorrow &mdash; the workspace is right where you left it.
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

const Strike = styled.s`
  color: var(--afs-muted);
  text-decoration-thickness: 2px;
  text-decoration-color: color-mix(in srgb, var(--afs-accent) 70%, transparent);
  margin-right: 0.18em;
  font-weight: 600;
`;

const Mono = styled.code`
  font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
  font-size: 0.92em;
  padding: 0 4px;
  border-radius: 4px;
  background: color-mix(in srgb, var(--afs-line) 60%, transparent);
  color: var(--afs-ink);
`;

const PathChoiceGrid = styled.div`
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
  width: 100%;
  margin: 28px 0 8px;

  @media (max-width: 720px) {
    grid-template-columns: 1fr;
  }
`;

const DbWarning = styled.div`
  margin-top: 4px;
  padding: 10px 14px;
  border-radius: 10px;
  border: 1px solid #f59e0b;
  background: #fffbeb;
  color: #92400e;
  font-size: 13px;
  line-height: 1.5;
  text-align: left;
  max-width: 640px;

  code {
    font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
    font-size: 0.9em;
    padding: 0 4px;
    border-radius: 4px;
    background: rgba(146, 64, 14, 0.08);
  }
`;

const BenefitsGrid = styled.div`
  display: grid;
  gap: 16px;
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

// ──────────────────────────────────────────────────────────────────────
// StatusHeader + ActivityCard styles (Inspector home)
// ──────────────────────────────────────────────────────────────────────

const StatusBar = styled.div`
  display: flex;
  align-items: baseline;
  gap: 12px;
  flex-wrap: wrap;
  padding: 14px 18px;
  border: 1px solid var(--afs-line);
  border-radius: 12px;
  background: var(--afs-bg-soft);
  font-family: var(--afs-mono, "Monaco", "Menlo", monospace);
  font-size: 13px;
`;

const StatusLive = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 6px;
`;

const StatusDot = styled.span<{ $live?: boolean }>`
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: ${(p) => (p.$live ? "#22c55e" : "var(--afs-muted)")};
  box-shadow: ${(p) => (p.$live ? "0 0 8px rgba(34, 197, 94, 0.5)" : "none")};
  animation: ${(p) => (p.$live ? "afs-status-pulse 2s ease-in-out infinite" : "none")};

  @keyframes afs-status-pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }
`;

const StatusLiveText = styled.span`
  color: var(--afs-accent);
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  font-size: 11px;
`;

const StatusSep = styled.span`
  color: var(--afs-line-strong);
`;

const StatusItem = styled.span`
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
`;

const StatusValue = styled.span`
  color: var(--afs-ink);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
`;

const StatusLabel = styled.span`
  color: var(--afs-muted);
  font-size: 12px;
`;

const ActivityCard = styled.section`
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 18px 22px;
  border: 1px solid var(--afs-line);
  border-radius: 14px;
  background: var(--afs-panel);
`;

// ──────────────────────────────────────────────────────────────────────
// ActiveAgentsPanel styles
// ──────────────────────────────────────────────────────────────────────

const AgentsPanelCard = styled.section`
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 18px;
  border: 1px solid var(--afs-line);
  border-radius: 12px;
  background: var(--afs-panel);
`;

const AgentsPanelHeader = styled.div`
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
`;

const AgentsPanelEyebrow = styled.h3`
  margin: 0;
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.02em;
`;

const AgentsPanelLink = styled.button`
  background: none;
  border: none;
  padding: 0;
  cursor: pointer;
  color: var(--afs-accent);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;

  &:hover {
    text-decoration: underline;
  }
`;

const AgentsPanelList = styled.ul`
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
`;

const AgentRow = styled.li`
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 0;
  border-bottom: 1px dashed var(--afs-line);
  font-family: var(--afs-mono, "Monaco", "Menlo", monospace);
  font-size: 12px;

  &:last-child {
    border-bottom: none;
  }
`;

const AgentDot = styled.span<{ $idle?: boolean }>`
  width: 8px;
  height: 8px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: ${(p) => (p.$idle ? "var(--afs-muted)" : "#22c55e")};
  box-shadow: ${(p) =>
    p.$idle
      ? "none"
      : "0 0 8px rgba(34, 197, 94, 0.65), 0 0 0 3px rgba(34, 197, 94, 0.18)"};
`;

const AgentLabel = styled.span`
  flex: 0 0 auto;
  min-width: 14ch;
  color: var(--afs-ink);
  font-weight: 600;
`;

const AgentArrow = styled.span`
  color: var(--afs-line-strong);
`;

const AgentWorkspace = styled.span`
  flex: 0 0 auto;
  color: var(--afs-accent);
`;

const AgentMeta = styled.span`
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-left: auto;
`;

const AgentTag = styled.span`
  padding: 1px 6px;
  border-radius: 4px;
  border: 1px solid var(--afs-line);
  color: var(--afs-muted);
  font-size: 10px;
  letter-spacing: 0.04em;
  text-transform: lowercase;
`;

const AgentSeen = styled.span`
  flex: 0 0 8ch;
  text-align: right;
  color: var(--afs-muted);
  font-size: 11px;
`;

const AgentMore = styled.button`
  background: none;
  border: none;
  padding: 0;
  margin: 0;
  cursor: pointer;
  color: var(--afs-accent);
  font-family: var(--afs-mono, "Monaco", "Menlo", monospace);
  font-size: 12px;

  &:hover {
    text-decoration: underline;
  }
`;

const ActivityCardHeader = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
`;

const ActivityCardEyebrow = styled.h2`
  margin: 0;
  color: var(--afs-ink);
  font-size: 16px;
  font-weight: 700;
  letter-spacing: -0.01em;
`;

const ActivityCardSub = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.5;
`;

const GettingStartedShell = styled.section`
  display: flex;
  flex-direction: column;
  gap: 18px;
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background: var(--afs-panel-strong);
  padding: 22px 22px 24px;

  @media (max-width: 640px) {
    padding: 18px;
  }

  [data-skin="situation-room"] && {
    border-radius: var(--afs-r-2);
    border-color: var(--afs-line-strong);
    background: var(--afs-bg-1);
  }
`;

const GettingStartedHeader = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
`;

const CliQuickstartSubline = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.5;
`;

const PathPair = styled.div`
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;

  @media (max-width: 720px) {
    grid-template-columns: 1fr;
  }
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

