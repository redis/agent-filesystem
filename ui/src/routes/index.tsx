import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { Button, Loader } from "@redis-ui/components";
import styled from "styled-components";
import {
  PageStack,
  StatCard,
  StatGrid,
  StatDetail,
  StatLabel,
  StatValue,
} from "../components/afs-kit";
import {
  CodeBlock,
  InlineCode,
  CrossLinkCard,
  CrossLinkText,
  CrossLinkTitle,
  CrossLinkDesc,
  CrossLinkArrow,
} from "../components/doc-kit";
import { AddDatabaseDialog } from "../components/add-database-dialog";
import { AgentHeroAnimation } from "../components/agent-hero-animation";
import { LiveTopologyCard } from "../components/live-topology-card";
import { formatBytes } from "../foundation/api/afs";
import { useDatabaseScope, useScopedAgents, useScopedWorkspaceSummaries } from "../foundation/database-scope";
import { queryClient } from "../foundation/query-client";
import {
  agentsQueryOptions,
  databasesQueryOptions,
  workspaceSummariesQueryOptions,
} from "../foundation/hooks/use-afs";

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
  const { databases, saveDatabase, isLoading: databasesLoading } = useDatabaseScope();

  if (databasesLoading || workspacesQuery.isLoading || agentsQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const hasDatabase = databases.length > 0;

  if (!hasDatabase) {
    return <GettingStartedView saveDatabase={saveDatabase} hasDatabase={false} />;
  }

  const workspaces = workspacesQuery.data;

  if (workspaces.length === 0) {
    return <GettingStartedView saveDatabase={saveDatabase} hasDatabase={true} />;
  }

  /* ── Dashboard ── */
  const workspacesWithCheckpoints = workspaces.filter((workspace) => workspace.checkpointCount > 0).length;
  const checkpointCount = workspaces.reduce((sum, workspace) => sum + workspace.checkpointCount, 0);
  const totalBytes = workspaces.reduce((sum, workspace) => sum + workspace.totalBytes, 0);
  const connectedAgents = agentsQuery.data.length;
  const checkpointCoverage = workspaces.length === 0 ? 0 : Math.round((workspacesWithCheckpoints / workspaces.length) * 100);

  return <DashboardView databases={databases} workspaces={workspaces} agents={agentsQuery.data} checkpointCount={checkpointCount} checkpointCoverage={checkpointCoverage} totalBytes={totalBytes} />;
}

function DashboardView({ databases, workspaces, agents, checkpointCount, checkpointCoverage, totalBytes }: {
  databases: { length: number };
  workspaces: { length: number }[];
  agents: unknown[];
  checkpointCount: number;
  checkpointCoverage: number;
  totalBytes: number;
}) {
  const navigate = useNavigate();
  const connectedAgents = agents.length;

  return (
    <PageStack>
      <StatGrid>
        <ClickableStatCard onClick={() => navigate({ to: "/workspaces" })}>
          <div>
            <StatLabel>Workspaces</StatLabel>
            <StatValue>{workspaces.length}</StatValue>
          </div>
          <StatDetail>
            {workspaces.length} workspace{workspaces.length === 1 ? "" : "s"} registered across{" "}
            {databases.length} database{databases.length === 1 ? "" : "s"}.
          </StatDetail>
        </ClickableStatCard>
        <ClickableStatCard onClick={() => navigate({ to: "/workspaces" })}>
          <div>
            <StatLabel>Stored Data</StatLabel>
            <StatValue>{formatBytes(totalBytes)}</StatValue>
          </div>
          <StatDetail>Total durable content tracked across all workspaces.</StatDetail>
        </ClickableStatCard>
        <ClickableStatCard onClick={() => navigate({ to: "/workspaces" })}>
          <div>
            <StatLabel>Checkpoints</StatLabel>
            <StatValue>{checkpointCount}</StatValue>
          </div>
          <StatDetail>{checkpointCoverage}% of workspaces have checkpoint history.</StatDetail>
        </ClickableStatCard>
        <ClickableStatCard onClick={() => navigate({ to: "/agents" })}>
          <div>
            <StatLabel>Connected Agents</StatLabel>
            <StatValue>{connectedAgents}</StatValue>
          </div>
          <StatDetail>
            {connectedAgents === 0
              ? "No agents are currently connected."
              : `${connectedAgents} live ${connectedAgents === 1 ? "agent" : "agents"} reporting workspace sessions.`}
          </StatDetail>
        </ClickableStatCard>
      </StatGrid>
      <LiveTopologyCard agents={agents as any} workspaces={workspaces as any} />
    </PageStack>
  );
}

/* ── Getting Started (empty-state) ── */

import type { SaveDatabaseInput } from "../foundation/types/afs";
import { useQuickstartMutation } from "../foundation/hooks/use-afs";

function GettingStartedView({ saveDatabase, hasDatabase }: { saveDatabase: (input: SaveDatabaseInput) => Promise<void>; hasDatabase: boolean }) {
  const [showAddDb, setShowAddDb] = useState(false);
  const navigate = useNavigate();
  const quickstartMutation = useQuickstartMutation();

  const handleQuickstart = async () => {
    try {
      const result = await quickstartMutation.mutateAsync({});
      navigate({
        to: "/workspaces/$workspaceId",
        params: { workspaceId: result.workspaceId },
        search: { welcome: true },
      });
    } catch {
      // Error is stored in quickstartMutation.error
    }
  };

  return (
    <PageStack>
      <EmptyStateLayout>
        <AgentHeroAnimation />
        <EmptyStateContent>
          <ProductName>Agent Filesystem</ProductName>
          <Headline>
            {hasDatabase
              ? "Create your first workspace"
              : "Fast, durable filesystem workspaces for AI agents, backed by Redis"}
          </Headline>
          <Description>
            {hasDatabase
              ? "Your database is connected. Now create a workspace — an isolated file tree an agent can read, write, and checkpoint."
              : "Give every AI agent a persistent, checkpointed workspace. Browse files, create recovery points, and track activity — all from one UI."}
          </Description>
        </EmptyStateContent>

        {/* ── Onboarding paths ── */}
        <OnboardingPaths>
          <OnboardingCard $primary>
            <OnboardingCardIcon>
              <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
              </svg>
            </OnboardingCardIcon>
            <OnboardingCardTitle>{hasDatabase ? "Create a workspace" : "Quick Start"}</OnboardingCardTitle>
            <OnboardingCardDesc>
              {hasDatabase
                ? "Create a workspace with sample files and start exploring — all in one click."
                : "Connect to local Redis, create a workspace with sample files, and start exploring — all in one click."}
            </OnboardingCardDesc>
            <CTAButton
              size="large"
              onClick={handleQuickstart}
              disabled={quickstartMutation.isPending}
            >
              {quickstartMutation.isPending ? "Setting up..." : "Create my first workspace"}
            </CTAButton>
            {quickstartMutation.isError && (
              <QuickstartError>
                {quickstartMutation.error?.message?.includes("cannot connect")
                  ? "Could not connect to Redis at localhost:6379. Start Redis locally or add a remote database instead."
                  : quickstartMutation.error?.message ?? "Something went wrong."}
              </QuickstartError>
            )}
            {hasDatabase && (
              <Link to="/workspaces">
                <SecondaryButton size="large">Add empty workspace</SecondaryButton>
              </Link>
            )}
            {!hasDatabase && (
              <OnboardingCardHint>
                Requires Redis running on localhost:6379
              </OnboardingCardHint>
            )}
          </OnboardingCard>

          {!hasDatabase && (
            <>
              <OnboardingDivider>
                <OnboardingDividerLine />
                <OnboardingDividerLabel>or</OnboardingDividerLabel>
                <OnboardingDividerLine />
              </OnboardingDivider>

              <OnboardingCard>
                <OnboardingCardIcon>
                  <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <ellipse cx="12" cy="5" rx="9" ry="3" />
                    <path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3" />
                    <path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5" />
                  </svg>
                </OnboardingCardIcon>
                <OnboardingCardTitle>Connect a remote database</OnboardingCardTitle>
                <OnboardingCardDesc>
                  Point AFS at any Redis instance — local, hosted, or Redis Cloud.
                  You'll create workspaces manually after connecting.
                </OnboardingCardDesc>
                <SecondaryButton size="large" onClick={() => setShowAddDb(true)}>
                  Add database manually
                </SecondaryButton>
              </OnboardingCard>
            </>
          )}
        </OnboardingPaths>

        {/* ── Steps ── */}
        <StepsSection>
          <StepsSectionTitle>How it works</StepsSectionTitle>

          <StepCard>
            <StepNumber>01</StepNumber>
            <StepBody>
              <StepTitle>Workspace = isolated filesystem</StepTitle>
              <StepDesc>
                Each workspace is a self-contained file tree an agent can read, write, and
                checkpoint. Workspaces are stored entirely in Redis — no local state required.
              </StepDesc>
            </StepBody>
          </StepCard>

          <StepCard>
            <StepNumber>02</StepNumber>
            <StepBody>
              <StepTitle>Checkpoints = instant rollback</StepTitle>
              <StepDesc>
                Save a checkpoint before risky operations. If something goes wrong, restore
                to any previous state in seconds. Think of it as git commits for your workspace.
              </StepDesc>
            </StepBody>
          </StepCard>

          <StepCard>
            <StepNumber>03</StepNumber>
            <StepBody>
              <StepTitle>Connect agents via CLI or MCP</StepTitle>
              <StepDesc>
                Mount a workspace as a local directory, or give agents direct access via
                MCP tools. Either way, every file operation is durable and trackable.
              </StepDesc>
              <CodeBlock>
                <code>{`# mount a workspace locally
afs workspace use my-project && afs up

# or connect via MCP tools
{ "mcpServers": { "afs": { "command": "afs", "args": ["mcp"] } } }`}</code>
              </CodeBlock>
            </StepBody>
          </StepCard>
        </StepsSection>

        {/* ── Learn more ── */}
        <CrossLinkCard as={Link} to="/agent-guide" style={{ width: "100%" }}>
          <CrossLinkText>
            <CrossLinkTitle>Agent Guide</CrossLinkTitle>
            <CrossLinkDesc>
              Full reference for MCP tools, CLI commands, workflows, and best practices.
            </CrossLinkDesc>
          </CrossLinkText>
          <CrossLinkArrow>&rarr;</CrossLinkArrow>
        </CrossLinkCard>
      </EmptyStateLayout>

      <AddDatabaseDialog
        isOpen={showAddDb}
        onClose={() => setShowAddDb(false)}
        saveDatabase={saveDatabase}
      />
    </PageStack>
  );
}

/* ── Styled components ── */

const EmptyStateLayout = styled.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 48px;
  padding: 40px 0 20px;
  max-width: 780px;
  margin: 0 auto;
`;

const EmptyStateContent = styled.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  max-width: 560px;
  text-align: center;
`;

const ProductName = styled.div`
  color: var(--afs-accent);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
`;

const Headline = styled.h2`
  margin: 0;
  color: var(--afs-ink);
  font-size: clamp(1.5rem, 3vw, 2rem);
  font-weight: 700;
  line-height: 1.25;
  letter-spacing: -0.02em;
`;

const Description = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 15px;
  line-height: 1.65;
`;

const CTAButton = styled(Button)`
  && {
    margin-top: 8px;
  }
`;

const ClickableStatCardWrap = styled.div`
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
`;

function ClickableStatCard({ onClick, children }: { onClick: () => void; children: React.ReactNode }) {
  return (
    <ClickableStatCardWrap onClick={onClick}>
      <StatCard>{children}</StatCard>
    </ClickableStatCardWrap>
  );
}

/* ── Steps ── */

const StepsSection = styled.div`
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
`;

const StepsSectionTitle = styled.h3`
  margin: 0;
  color: var(--afs-ink);
  font-size: 18px;
  font-weight: 700;
  letter-spacing: -0.01em;
`;

const StepCard = styled.div`
  display: flex;
  gap: 16px;
  align-items: flex-start;
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  padding: 20px;
  background: var(--afs-panel);
`;

const StepNumber = styled.div`
  flex-shrink: 0;
  width: 32px;
  height: 32px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 800;
  color: var(--afs-accent);
  background: var(--afs-accent-soft);
`;

const StepBody = styled.div`
  flex: 1;
  min-width: 0;
`;

const StepTitle = styled.div`
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
  line-height: 1.45;
  margin-bottom: 6px;
`;

const StepDesc = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.65;
`;

/* ── Onboarding paths ── */

const OnboardingPaths = styled.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0;
  width: 100%;
`;

const OnboardingCard = styled.div<{ $primary?: boolean }>`
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  border: 1.5px solid ${(p) => (p.$primary ? "var(--afs-accent, #2563eb)" : "var(--afs-line)")};
  border-radius: 20px;
  padding: 32px 28px 28px;
  background: var(--afs-panel);
  text-align: center;
  width: 100%;
  max-width: 480px;
  transition: border-color 180ms ease, box-shadow 180ms ease;

  ${(p) =>
    p.$primary &&
    `
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--afs-accent, #2563eb) 12%, transparent);
  `}
`;

const OnboardingCardIcon = styled.div`
  display: flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: 14px;
  background: var(--afs-accent-soft, #fef2f1);
  color: var(--afs-accent, #2563eb);
`;

const OnboardingCardTitle = styled.div`
  color: var(--afs-ink);
  font-size: 18px;
  font-weight: 700;
  letter-spacing: -0.01em;
`;

const OnboardingCardDesc = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.65;
  max-width: 380px;
`;

const OnboardingCardHint = styled.div`
  color: var(--afs-muted);
  font-size: 12px;
  opacity: 0.7;
`;

const QuickstartError = styled.div`
  color: #dc2626;
  font-size: 13px;
  line-height: 1.5;
  padding: 8px 12px;
  background: #fef2f2;
  border-radius: 8px;
  width: 100%;
`;

const SecondaryButton = styled(Button)`
  && {
    background: transparent;
    border: 1.5px solid var(--afs-line);
    color: var(--afs-ink);

    &:hover {
      border-color: var(--afs-accent, #2563eb);
      color: var(--afs-accent, #2563eb);
    }
  }
`;

const OnboardingDivider = styled.div`
  display: flex;
  align-items: center;
  gap: 16px;
  width: 100%;
  max-width: 480px;
  padding: 16px 0;
`;

const OnboardingDividerLine = styled.div`
  flex: 1;
  height: 1px;
  background: var(--afs-line);
`;

const OnboardingDividerLabel = styled.span`
  color: var(--afs-muted);
  font-size: 13px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
`;
