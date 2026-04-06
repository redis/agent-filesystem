import { createFileRoute, Link } from "@tanstack/react-router";
import { Button, Loader } from "@redislabsdev/redis-ui-components";
import styled from "styled-components";
import {
  EventList,
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
import { useActivity, useWorkspaceSummaries } from "../foundation/hooks/use-afs";

export const Route = createFileRoute("/")({
  component: OverviewPage,
});

function OverviewPage() {
  const workspacesQuery = useWorkspaceSummaries();
  const activityQuery = useActivity(5);

  if (workspacesQuery.isLoading || activityQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const workspaces = workspacesQuery.data ?? [];
  const activity = (activityQuery.data ?? []).map((event) => ({
    ...event,
    title: event.workspaceName ? `${event.workspaceName}: ${event.title}` : event.title,
  }));

  if (workspaces.length === 0) {
    return (
      <PageStack>
        <HeroCard>
          <HeroLayout>
            <HeroBody>
              <SectionTitle
                eyebrow="Getting Started"
                title="Create your first agent workspace"
                body="Agent Filesystem gives every agent a durable workspace you can browse in the browser, checkpoint before risky changes, and restore when you need to recover fast. Start blank, import existing work, or register a managed workspace and operate on it from one place."
              />
              <InlineActions>
                <Link to="/workspaces">
                  <Button size="medium">Add workspace</Button>
                </Link>
              </InlineActions>
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
  const importedWorkspaces = workspaces.filter((workspace) => workspace.source !== "blank").length;
  const blankWorkspaces = workspaces.length - importedWorkspaces;
  const regionCount = new Set(workspaces.map((workspace) => workspace.region)).size;
  const latestCheckpointAt = workspaces
    .filter((workspace) => workspace.checkpointCount > 0)
    .map((workspace) => workspace.lastCheckpointAt)
    .sort((left, right) => right.localeCompare(left))[0];
  const largestWorkspace = [...workspaces].sort((left, right) => right.totalBytes - left.totalBytes)[0];
  const checkpointCoverage = workspaces.length === 0 ? 0 : Math.round((workspacesWithCheckpoints / workspaces.length) * 100);

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
      </StatGrid>

      <SectionGrid>
        <SectionCard $span={7}>
          <SectionHeader>
            <SectionTitle
              eyebrow="Health"
              title=""
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

        <SectionCard $span={5}>
          <SectionHeader>
            <SectionTitle
              eyebrow="Recent Activity"
              title="Latest activity"
              body="Recent activity belongs on the overview as a quick confidence check that the registry, studio, and audit feed are all moving together."
            />
          </SectionHeader>
          <EventList events={activity} />
        </SectionCard>
      </SectionGrid>

      <SectionGrid>
        <SectionCard $span={6}>
          <SectionHeader>
            <SectionTitle
              eyebrow="Recovery"
              title="Checkpoint readiness"
              body="Checkpoint data is most useful when it is summarized as readiness, not buried inside an individual workspace."
            />
          </SectionHeader>
          <HighlightGrid>
            <HighlightCard>
              <HighlightLabel>Coverage</HighlightLabel>
              <HighlightValue>{checkpointCoverage}%</HighlightValue>
              <HighlightBody>Workspaces already carrying checkpoint history.</HighlightBody>
            </HighlightCard>
            <HighlightCard>
              <HighlightLabel>Without checkpoints</HighlightLabel>
              <HighlightValue>{workspaces.length - workspacesWithCheckpoints}</HighlightValue>
              <HighlightBody>Workspaces that should create their first recovery point.</HighlightBody>
            </HighlightCard>
            <HighlightCard>
              <HighlightLabel>Average depth</HighlightLabel>
              <HighlightValue>
                {workspaces.length === 0 ? "0.0" : (checkpointCount / workspaces.length).toFixed(1)}
              </HighlightValue>
              <HighlightBody>Average number of checkpoints per workspace.</HighlightBody>
            </HighlightCard>
            <HighlightCard>
              <HighlightLabel>Latest checkpoint</HighlightLabel>
              <HighlightValue>
                {latestCheckpointAt ? new Date(latestCheckpointAt).toLocaleDateString() : "n/a"}
              </HighlightValue>
              <HighlightBody>Most recent checkpoint creation time across the fleet.</HighlightBody>
            </HighlightCard>
          </HighlightGrid>
        </SectionCard>

        <SectionCard $span={6}>
          <SectionHeader>
            <SectionTitle
              eyebrow="Posture"
              title="Workspace mix"
              body="A compact mix view helps you see how the registry is composed without turning Overview into another catalog."
            />
          </SectionHeader>
          <HighlightGrid>
            <HighlightCard>
              <HighlightLabel>Imported</HighlightLabel>
              <HighlightValue>{importedWorkspaces}</HighlightValue>
              <HighlightBody>Workspaces brought in from Git or Redis Cloud.</HighlightBody>
            </HighlightCard>
            <HighlightCard>
              <HighlightLabel>Blank</HighlightLabel>
              <HighlightValue>{blankWorkspaces}</HighlightValue>
              <HighlightBody>Workspaces created directly inside Agent Filesystem.</HighlightBody>
            </HighlightCard>
            <HighlightCard>
              <HighlightLabel>Regions</HighlightLabel>
              <HighlightValue>{regionCount}</HighlightValue>
              <HighlightBody>Distinct regions represented in the current registry.</HighlightBody>
            </HighlightCard>
            <HighlightCard>
              <HighlightLabel>Largest workspace</HighlightLabel>
              <HighlightValue>{formatBytes(largestWorkspace.totalBytes)}</HighlightValue>
              <HighlightBody>{largestWorkspace.name}</HighlightBody>
            </HighlightCard>
          </HighlightGrid>
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
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.84), rgba(243, 246, 251, 0.92)),
    rgba(255, 255, 255, 0.86);
`;

const StarterPreview = styled.div`
  position: relative;
  min-height: 230px;
  border-radius: 22px;
  border: 1px solid var(--afs-line);
  background:
    radial-gradient(circle at center, rgba(170, 59, 255, 0.12), transparent 48%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.92), rgba(246, 249, 255, 0.92));
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
  background: linear-gradient(90deg, #aa3bff, #47bfff);
`;

const HighlightGrid = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(2, minmax(0, 1fr));

  @media (max-width: 720px) {
    grid-template-columns: 1fr;
  }
`;

const HighlightCard = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 20px;
  padding: 18px;
  background: rgba(255, 255, 255, 0.78);
`;

const HighlightLabel = styled.div`
  color: var(--afs-muted);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const HighlightValue = styled.div`
  margin-top: 10px;
  color: var(--afs-ink);
  font-size: 1.9rem;
  font-weight: 700;
  letter-spacing: -0.04em;
`;

const HighlightBody = styled.p`
  margin: 8px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.65;
`;
