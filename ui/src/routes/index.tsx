import { createFileRoute, Link } from "@tanstack/react-router";
import { Button, Loader } from "@redislabsdev/redis-ui-components";
import styled from "styled-components";
import {
  PageStack,
  StatCard,
  StatDetail,
  StatGrid,
  StatLabel,
  StatValue,
} from "../components/afs-kit";
import { formatBytes } from "../foundation/api/afs";
import { useDatabaseScope, useScopedWorkspaceSummaries } from "../foundation/database-scope";

export const Route = createFileRoute("/")({
  component: OverviewPage,
});

function OverviewPage() {
  const workspacesQuery = useScopedWorkspaceSummaries();
  const { selectedDatabase } = useDatabaseScope();

  if (workspacesQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  /* ── No database selected ── */
  if (selectedDatabase == null) {
    return (
      <PageStack>
        <EmptyStateLayout>
          <EmptyStateContent>
            <ProductName>Agent Filesystem</ProductName>
            <Headline>Durable workspaces for AI agents, backed by Redis</Headline>
            <Description>
              Agent Filesystem gives every agent a persistent, checkpointed workspace.
              Browse files, create recovery points, and track activity across workspaces
              — all from one UI.
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
                Redis keys — no extra infrastructure needed.
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

  /* ── Database selected but no workspaces ── */
  if (workspaces.length === 0) {
    return (
      <PageStack>
        <EmptyStateLayout>
          <EmptyStateContent>
            <Headline>No workspaces in {selectedDatabase.displayName} yet</Headline>
            <Description>
              Create your first workspace to start managing files, checkpoints, and activity
              in this database.
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
  const checkpointCoverage = workspaces.length === 0 ? 0 : Math.round((workspacesWithCheckpoints / workspaces.length) * 100);

  return (
    <PageStack>
      <StatGrid>
        <StatCard>
          <div>
            <StatLabel>Workspaces</StatLabel>
            <StatValue>{workspaces.length}</StatValue>
          </div>
          <StatDetail>{workspaces.length} workspace{workspaces.length === 1 ? "" : "s"} registered in this database.</StatDetail>
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
