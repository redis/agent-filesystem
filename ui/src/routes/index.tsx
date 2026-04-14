import { createFileRoute, Link } from "@tanstack/react-router";
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
import { AgentHeroAnimation } from "../components/agent-hero-animation";
import { formatBytes } from "../foundation/api/afs";
import { useDatabaseScope, useScopedAgents, useScopedWorkspaceSummaries } from "../foundation/database-scope";

export const Route = createFileRoute("/")({
  component: OverviewPage,
});

function OverviewPage() {
  const workspacesQuery = useScopedWorkspaceSummaries();
  const agentsQuery = useScopedAgents();
  const { databases, isLoading: databasesLoading } = useDatabaseScope();

  if (databasesLoading || workspacesQuery.isLoading || agentsQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  if (databases.length === 0) {
    return (
      <PageStack>
        <EmptyStateLayout>
          <EmptyStateContent>
            <ProductName>Agent Filesystem</ProductName>
            <Headline>Durable workspaces for AI agents, backed by Redis</Headline>
            <Description>
              Agent Filesystem gives every agent a persistent, checkpointed workspace.
              Browse files, create recovery points, and track activity across workspaces
              all from one UI.
            </Description>
            <Link to="/databases">
              <CTAButton size="large">Add database to get started</CTAButton>
            </Link>
          </EmptyStateContent>

          <FeatureGrid>
            <FeatureCard>
              <FeatureIcon>01</FeatureIcon>
              <FeatureTitle>Connect a Redis database</FeatureTitle>
              <FeatureBody>
                Point AFS at any Redis instance. Workspaces, files, and checkpoints are stored as
                Redis keys with no extra infrastructure needed.
              </FeatureBody>
            </FeatureCard>
            <FeatureCard>
              <FeatureIcon>02</FeatureIcon>
              <FeatureTitle>Create workspaces</FeatureTitle>
              <FeatureBody>
                Each workspace is an isolated filesystem an agent can read, write, and checkpoint.
                Browse and edit files directly in the studio.
              </FeatureBody>
            </FeatureCard>
            <FeatureCard>
              <FeatureIcon>03</FeatureIcon>
              <FeatureTitle>Checkpoint and recover</FeatureTitle>
              <FeatureBody>
                Save named snapshots at any point. Compare history, restore previous state, and
                keep a full audit trail of every change.
              </FeatureBody>
            </FeatureCard>
          </FeatureGrid>
        </EmptyStateLayout>
      </PageStack>
    );
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

  return (
    <PageStack>
      <AgentHeroAnimation />
      <StatGrid>
        <StatCard>
          <div>
            <StatLabel>Workspaces</StatLabel>
            <StatValue>{workspaces.length}</StatValue>
          </div>
          <StatDetail>
            {workspaces.length} workspace{workspaces.length === 1 ? "" : "s"} registered across{" "}
            {databases.length} database{databases.length === 1 ? "" : "s"}.
          </StatDetail>
        </StatCard>
        <StatCard>
          <div>
            <StatLabel>Stored Data</StatLabel>
            <StatValue>{formatBytes(totalBytes)}</StatValue>
          </div>
          <StatDetail>Total durable content tracked across all workspaces.</StatDetail>
        </StatCard>
        <StatCard>
          <div>
            <StatLabel>Checkpoints</StatLabel>
            <StatValue>{checkpointCount}</StatValue>
          </div>
          <StatDetail>{checkpointCoverage}% of workspaces have checkpoint history.</StatDetail>
        </StatCard>
        <StatCard>
          <div>
            <StatLabel>Connected Agents</StatLabel>
            <StatValue>{connectedAgents}</StatValue>
          </div>
          <StatDetail>
            {connectedAgents === 0
              ? "No agents are currently connected."
              : `${connectedAgents} live ${connectedAgents === 1 ? "agent" : "agents"} reporting workspace sessions.`}
          </StatDetail>
        </StatCard>
      </StatGrid>
    </PageStack>
  );
}

/* ── Empty state layout ── */

const EmptyStateLayout = styled.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 48px;
  padding: 40px 0 20px;
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

/* ── Feature cards ── */

const FeatureGrid = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  width: 100%;

  @media (max-width: 900px) {
    grid-template-columns: 1fr;
  }
`;

const FeatureCard = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  padding: 20px;
  background: var(--afs-panel);
`;

const FeatureIcon = styled.div`
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 10px;
  background: var(--afs-accent-soft);
  color: var(--afs-accent);
  font-size: 12px;
  font-weight: 800;
  margin-bottom: 12px;
`;

const FeatureTitle = styled.div`
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
  line-height: 1.45;
`;

const FeatureBody = styled.p`
  margin: 8px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.65;
`;
