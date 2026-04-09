import { createFileRoute, Link } from "@tanstack/react-router";
import { Button, Loader } from "@redislabsdev/redis-ui-components";
import styled from "styled-components";
import {
  HeroBody,
  HeroCard,
  HeroLayout,
  InlineActions,
  PageStack,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
  StatCard,
  StatDetail,
  StatGrid,
  StatLabel,
  StatValue,
  Tag,
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

  const workspaces = workspacesQuery.data;

  if (workspaces.length === 0) {
    return (
      <PageStack>
        <HeroCard>
          <HeroLayout>
            <HeroBody>
              <SectionTitle
                eyebrow={selectedDatabase == null ? "Database Scope" : "Getting Started"}
                title={
                  selectedDatabase == null
                    ? "Choose a Redis database"
                    : `No workspaces in ${selectedDatabase.displayName} yet`
                }
                body={
                  selectedDatabase == null
                    ? "Use the database selector in the page header to choose the active Redis database for Overview, Workspaces, and Activity."
                    : "This database is open and ready. Add a workspace here to start filling Overview, Workspaces, and Activity for this scope."
                }
              />
              {selectedDatabase != null ? (
                <InlineActions>
                  <Link to="/workspaces">
                    <Button size="medium">Add workspace</Button>
                  </Link>
                </InlineActions>
              ) : null}
              <BenefitGrid>
                <BenefitCard>
                  <BenefitKicker>01</BenefitKicker>
                  <BenefitTitle>Bring in real working state</BenefitTitle>
                  <BenefitBody>
                    Start with a blank workspace, a Git import, or a Redis Cloud import instead of rebuilding context by hand.
                  </BenefitBody>
                </BenefitCard>
                <BenefitCard>
                  <BenefitKicker>02</BenefitKicker>
                  <BenefitTitle>Edit, inspect, and keep history close</BenefitTitle>
                  <BenefitBody>
                    Browse files in the studio, keep mutable state visible, and make checkpoints part of the normal workflow.
                  </BenefitBody>
                </BenefitCard>
                <BenefitCard>
                  <BenefitKicker>03</BenefitKicker>
                  <BenefitTitle>Recover without guesswork</BenefitTitle>
                  <BenefitBody>
                    Open a workspace and use its checkpoint view to compare history, restore safely, and understand what changed.
                  </BenefitBody>
                </BenefitCard>
              </BenefitGrid>
            </HeroBody>

            <StarterPanel>
              <StarterPreview>
                <StarterCore>Agent Filesystem</StarterCore>
                <StarterNode $x="8%" $y="14%">
                  Blank workspace
                </StarterNode>
                <StarterNode $x="58%" $y="18%">
                  Git import
                </StarterNode>
                <StarterNode $x="18%" $y="66%">
                  Browser studio
                </StarterNode>
                <StarterNode $x="56%" $y="70%">
                  Checkpoints
                </StarterNode>
              </StarterPreview>
              <StarterList>
                <StarterListTitle>What you unlock</StarterListTitle>
                <StarterListItem>One place to manage durable workspaces for code, prompts, memories, and artifacts.</StarterListItem>
                <StarterListItem>A clear path from draft edits to saved checkpoints.</StarterListItem>
                <StarterListItem>A registry, studio, and activity view that stay in sync.</StarterListItem>
              </StarterList>
              <StarterTags>
                <Tag>Blank workspace</Tag>
                <Tag>Git import</Tag>
                <Tag>Redis Cloud import</Tag>
              </StarterTags>
            </StarterPanel>
          </HeroLayout>
        </HeroCard>
      </PageStack>
    );
  }

  const activeWorkspaces = workspaces.filter((workspace) => workspace.status !== "attention").length;
  const attentionWorkspaces = workspaces.filter((workspace) => workspace.status === "attention").length;
  const dirtyWorkspaces = workspaces.filter((workspace) => workspace.draftState === "dirty").length;
  const workspacesWithCheckpoints = workspaces.filter((workspace) => workspace.checkpointCount > 0).length;
  const checkpointCount = workspaces.reduce((sum, workspace) => sum + workspace.checkpointCount, 0);
  const totalBytes = workspaces.reduce((sum, workspace) => sum + workspace.totalBytes, 0);
  const largestWorkspace = [...workspaces].sort((left, right) => right.totalBytes - left.totalBytes)[0];
  const checkpointCoverage = workspaces.length === 0 ? 0 : Math.round((workspacesWithCheckpoints / workspaces.length) * 100);
  const averageDepth = workspaces.length === 0 ? "0.0" : (checkpointCount / workspaces.length).toFixed(1);

  return (
    <PageStack>
      <StatGrid>
        <StatCard>
          <div>
            <StatLabel>Workspaces</StatLabel>
            <StatValue>{workspaces.length}</StatValue>
          </div>
          <StatDetail>{activeWorkspaces} active and {attentionWorkspaces} inactive.</StatDetail>
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
            <StatLabel>Checkpoints Created</StatLabel>
            <StatValue>{checkpointCount}</StatValue>
          </div>
          <StatDetail>{checkpointCoverage}% of workspaces currently have checkpoint history.</StatDetail>
        </StatCard>
        <StatCard>
          <div>
            <StatLabel>Attention Needed</StatLabel>
            <StatValue>{attentionWorkspaces + dirtyWorkspaces}</StatValue>
          </div>
          <StatDetail>{dirtyWorkspaces} workspace{dirtyWorkspaces === 1 ? "" : "s"} currently carrying draft changes.</StatDetail>
        </StatCard>
        <StatCard>
          <div>
            <StatLabel>Largest Workspace</StatLabel>
            <StatValue>{formatBytes(largestWorkspace.totalBytes)}</StatValue>
          </div>
          <StatDetail>{largestWorkspace.name}</StatDetail>
        </StatCard>
        <StatCard>
          <div>
            <StatLabel>Average Depth</StatLabel>
            <StatValue>{averageDepth}</StatValue>
          </div>
          <StatDetail>Average number of checkpoints per workspace.</StatDetail>
        </StatCard>
      </StatGrid>

      <SectionGrid>
        <SectionCard $span={12}>
          <SectionHeader>
            <SectionTitle
              eyebrow="Health"
              title="Workspace health"
              body={
                selectedDatabase == null
                  ? "Health signals appear here once a database is open and carrying workspaces."
                  : `Health signals for ${selectedDatabase.displayName}.`
              }
            />
          </SectionHeader>
          <SignalList>
            <SignalRow>
              <SignalText>
                <SignalTitle>Active workspaces</SignalTitle>
                <SignalBody>Healthy and syncing workspaces that are currently in rotation.</SignalBody>
              </SignalText>
              <SignalMetric>{activeWorkspaces}</SignalMetric>
              <SignalBar>
                <SignalFill $value={ratio(activeWorkspaces, workspaces.length)} />
              </SignalBar>
            </SignalRow>
            <SignalRow>
              <SignalText>
                <SignalTitle>Inactive workspaces</SignalTitle>
                <SignalBody>Workspaces that need attention before they should be trusted as ready.</SignalBody>
              </SignalText>
              <SignalMetric>{attentionWorkspaces}</SignalMetric>
              <SignalBar>
                <SignalFill $value={ratio(attentionWorkspaces, workspaces.length)} />
              </SignalBar>
            </SignalRow>
            <SignalRow>
              <SignalText>
                <SignalTitle>Dirty working state</SignalTitle>
                <SignalBody>Workspaces carrying draft changes that have not been checkpointed yet.</SignalBody>
              </SignalText>
              <SignalMetric>{dirtyWorkspaces}</SignalMetric>
              <SignalBar>
                <SignalFill $value={ratio(dirtyWorkspaces, workspaces.length)} />
              </SignalBar>
            </SignalRow>
            <SignalRow>
              <SignalText>
                <SignalTitle>Checkpoint coverage</SignalTitle>
                <SignalBody>Workspaces with at least one checkpoint already recorded.</SignalBody>
              </SignalText>
              <SignalMetric>{workspacesWithCheckpoints}</SignalMetric>
              <SignalBar>
                <SignalFill $value={ratio(workspacesWithCheckpoints, workspaces.length)} />
              </SignalBar>
            </SignalRow>
          </SignalList>
        </SectionCard>
      </SectionGrid>
    </PageStack>
  );
}

function ratio(value: number, total: number) {
  if (total <= 0 || value <= 0) {
    return 0;
  }

  return Math.max(8, Math.min(100, Math.round((value / total) * 100)));
}

const BenefitGrid = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(3, minmax(0, 1fr));

  @media (max-width: 900px) {
    grid-template-columns: 1fr;
  }
`;

const BenefitCard = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 20px;
  padding: 16px 18px;
  background: rgba(255, 255, 255, 0.78);
`;

const BenefitKicker = styled.div`
  color: var(--afs-accent);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const BenefitTitle = styled.div`
  margin-top: 10px;
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
  line-height: 1.45;
`;

const BenefitBody = styled.p`
  margin: 8px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.65;
`;

const StarterPanel = styled.div`
  display: grid;
  gap: 14px;
  border: 1px solid var(--afs-line);
  border-radius: 24px;
  padding: 18px;
  background: #fff;
`;

const StarterPreview = styled.div`
  position: relative;
  min-height: 230px;
  border-radius: 22px;
  border: 1px solid var(--afs-line);
  background: #fff;
  overflow: hidden;
`;

const StarterCore = styled.div`
  position: absolute;
  inset: 50% auto auto 50%;
  transform: translate(-50%, -50%);
  min-width: 168px;
  padding: 16px 18px;
  border-radius: 20px;
  text-align: center;
  border: 1px solid var(--afs-line-strong);
  background: #fff;
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
  box-shadow: 0 14px 28px rgba(8, 6, 13, 0.08);
`;

const StarterNode = styled.div<{ $x: string; $y: string }>`
  position: absolute;
  left: ${({ $x }) => $x};
  top: ${({ $y }) => $y};
  padding: 10px 12px;
  border-radius: 999px;
  border: 1px solid var(--afs-line);
  background: rgba(255, 255, 255, 0.94);
  color: var(--afs-ink-soft);
  font-size: 12px;
  font-weight: 700;
`;

const StarterList = styled.div`
  display: grid;
  gap: 10px;
`;

const StarterListTitle = styled.div`
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
`;

const StarterListItem = styled.div`
  position: relative;
  padding-left: 18px;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.65;

  &::before {
    content: "";
    position: absolute;
    left: 0;
    top: 9px;
    width: 8px;
    height: 8px;
    border-radius: 999px;
    background: var(--afs-accent);
  }
`;

const StarterTags = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
`;

const SignalList = styled.div`
  display: grid;
  gap: 14px;
`;

const SignalRow = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: minmax(0, 1.4fr) auto minmax(120px, 180px);
  align-items: center;
  border: 1px solid var(--afs-line);
  border-radius: 20px;
  padding: 16px;
  background: rgba(255, 255, 255, 0.78);

  @media (max-width: 860px) {
    grid-template-columns: 1fr;
  }
`;

const SignalText = styled.div`
  display: grid;
  gap: 6px;
`;

const SignalTitle = styled.div`
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
`;

const SignalBody = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const SignalMetric = styled.div`
  color: var(--afs-ink);
  font-size: 1.8rem;
  font-weight: 700;
  letter-spacing: -0.04em;
`;

const SignalBar = styled.div`
  height: 10px;
  border-radius: 999px;
  background: rgba(8, 6, 13, 0.08);
  overflow: hidden;
`;

const SignalFill = styled.div<{ $value: number }>`
  width: ${({ $value }) => `${$value}%`};
  height: 100%;
  border-radius: inherit;
  background: var(--afs-accent);
`;
