import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { Button, Loader } from "@redislabsdev/redis-ui-components";
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

export const Route = createFileRoute("/")({
  component: OverviewPage,
});

function OverviewPage() {
  const workspacesQuery = useScopedWorkspaceSummaries();
  const agentsQuery = useScopedAgents();
  const { databases, saveDatabase, isLoading: databasesLoading } = useDatabaseScope();

  if (databasesLoading || workspacesQuery.isLoading || agentsQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  if (databases.length === 0) {
    return <GettingStartedView saveDatabase={saveDatabase} />;
  }

  const workspaces = workspacesQuery.data;

  if (workspaces.length === 0) {
    return (
      <PageStack>
        <EmptyStateLayout>
          <EmptyStateContent>
            <Headline>No workspaces in the catalog yet</Headline>
            <Description>
              Create your first workspace to start managing files, checkpoints, and activity
              across your connected databases.
            </Description>
            <Link to="/workspaces">
              <CTAButton size="large">Add workspace</CTAButton>
            </Link>
          </EmptyStateContent>
        </EmptyStateLayout>
      </PageStack>
    );
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

function GettingStartedView({ saveDatabase }: { saveDatabase: (input: SaveDatabaseInput) => Promise<void> }) {
  const [showAddDb, setShowAddDb] = useState(false);

  return (
    <PageStack>
      <EmptyStateLayout>
        <AgentHeroAnimation />
        <EmptyStateContent>
          <ProductName>Agent Filesystem</ProductName>
          <Headline>
            Fast, durable filesystem workspaces for AI agents, backed by Redis
          </Headline>
          <Description>
            Give every AI agent a persistent, checkpointed workspace.
            Browse files, create recovery points, and track activity — all from one UI.
          </Description>
          <CTAButton size="large" onClick={() => setShowAddDb(true)}>
            Add database to get started
          </CTAButton>
        </EmptyStateContent>

        {/* ── Steps ── */}
        <StepsSection>
          <StepsSectionTitle>Get up and running in 3 steps</StepsSectionTitle>

          <StepCard>
            <StepNumber>01</StepNumber>
            <StepBody>
              <StepTitle>Connect a Redis database</StepTitle>
              <StepDesc>
                AFS stores workspaces, files, and checkpoints as Redis keys — no extra
                infrastructure needed. Click <strong>Add database</strong> above to point AFS
                at any Redis instance (local or remote).
              </StepDesc>
            </StepBody>
          </StepCard>

          <StepCard>
            <StepNumber>02</StepNumber>
            <StepBody>
              <StepTitle>Create a workspace</StepTitle>
              <StepDesc>
                A workspace is an isolated filesystem an agent can read, write, and
                checkpoint. Create one from the <strong>Workspaces</strong> page, or let
                an agent create its own via the CLI or MCP tools.
              </StepDesc>
            </StepBody>
          </StepCard>

          <StepCard>
            <StepNumber>03</StepNumber>
            <StepBody>
              <StepTitle>Connect an agent</StepTitle>
              <StepDesc>
                Use the CLI to mount a workspace as a local directory. The agent reads and
                writes files normally — AFS syncs everything to Redis in the background.
              </StepDesc>
              <CodeBlock>
                <code>{`# select and mount a workspace
afs workspace use my-project
afs up

# the agent works in ~/afs/my-project/ with normal file I/O`}</code>
              </CodeBlock>
              <StepDesc style={{ marginTop: 12 }}>
                Alternatively, agents can connect via{" "}
                <strong>MCP</strong> (Model Context Protocol) for tool-based access. Add
                the following to your agent's MCP configuration:
              </StepDesc>
              <CodeBlock>
                <code>{`{
  "mcpServers": {
    "agent-filesystem": {
      "command": "/absolute/path/to/afs",
      "args": ["mcp"]
    }
  }
}`}</code>
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
    border-color: var(--afs-accent, #6366f1);
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
