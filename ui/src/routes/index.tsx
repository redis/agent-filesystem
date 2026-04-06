import { createFileRoute, Link } from "@tanstack/react-router";
import { Button, Loader, Typography } from "@redislabsdev/redis-ui-components";
import styled from "styled-components";
import {
  CardHeader,
  EventList,
  HeroBody,
  HeroCard,
  HeroLayout,
  HeroMetaGrid,
  InlineActions,
  MetaItem,
  MetaRow,
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
  ToneChip,
  WorkspaceCard,
  WorkspaceGrid,
} from "../components/afs-kit";
import { getAFSClientMode, formatBytes } from "../foundation/api/afs";
import { useActivity, useWorkspaceSummaries } from "../foundation/hooks/use-afs";
import type {
  AFSDraftState,
  AFSWorkspaceSource,
  AFSWorkspaceStatus,
} from "../foundation/types/afs";

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
  const backendMode = getAFSClientMode();
  const activity = (activityQuery.data ?? []).map((event) => ({
    ...event,
    title: event.workspaceName ? `${event.workspaceName}: ${event.title}` : event.title,
  }));
  const dirtyWorkspaces = workspaces.filter((workspace) => workspace.draftState === "dirty").length;
  const checkpointCount = workspaces.reduce((sum, workspace) => sum + workspace.checkpointCount, 0);
  const healthyWorkspaces = workspaces.filter((workspace) => workspace.status === "healthy").length;
  const importedWorkspaces = workspaces.filter((workspace) => workspace.source !== "blank").length;
  const totalBytes = workspaces.reduce((sum, workspace) => sum + workspace.totalBytes, 0);
  const regionCount = new Set(workspaces.map((workspace) => workspace.region)).size;
  const featuredWorkspaces = [...workspaces]
    .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt))
    .slice(0, 4);

  return (
    <PageStack>
      <HeroCard>
        <HeroLayout>
          <HeroBody>
            <SectionTitle
              eyebrow="Filesystem Operations Surface"
              title="See workspace health, draft pressure, and checkpoint recovery in one console"
              body="The first screen should immediately answer three questions: what needs attention, where edits are still mutable, and which checkpoint lets you recover without guesswork."
            />
            <InlineActions>
              <Link to="/workspaces">
                <Button size="medium">Open workspace catalog</Button>
              </Link>
              <Link to="/activity">
                <Button size="medium" variant="secondary-fill">
                  Review activity feed
                </Button>
              </Link>
            </InlineActions>
            <FlowGrid>
              <FlowCard>
                <FlowStep>01</FlowStep>
                <FlowTitle>Pick the workspace</FlowTitle>
                <FlowBody>
                  Start from fleet health and open the workspace that is drifting, syncing, or ready
                  for a checkpoint.
                </FlowBody>
              </FlowCard>
              <FlowCard>
                <FlowStep>02</FlowStep>
                <FlowTitle>Inspect mutable state</FlowTitle>
                <FlowBody>
                  Separate working copy edits from saved head so operators can see whether a change is
                  tentative or canonical.
                </FlowBody>
              </FlowCard>
              <FlowCard>
                <FlowStep>03</FlowStep>
                <FlowTitle>Checkpoint with intent</FlowTitle>
                <FlowBody>
                  Every restore or savepoint should feel like a deliberate move in an operational
                  loop, not an afterthought.
                </FlowBody>
              </FlowCard>
            </FlowGrid>
          </HeroBody>

          <HeroMetaGrid>
            <MetaItem>
              <MetaLabel>Current backend</MetaLabel>
              <MetaValue>
                {backendMode === "http" ? "HTTP control plane" : "Local AFS demo store"}
              </MetaValue>
              <MetaBody>
                {backendMode === "http"
                  ? "The console is reading workspace summaries and activity from the running service."
                  : "Demo mode stays useful while the live control plane is still being wired up."}
              </MetaBody>
            </MetaItem>
            <MetaItem>
              <MetaLabel>Mutable pressure</MetaLabel>
              <MetaValue>{dirtyWorkspaces} dirty workspaces</MetaValue>
              <MetaBody>
                Draft state stays visible at the fleet level so it is easy to spot pending edits.
              </MetaBody>
            </MetaItem>
            <MetaItem>
              <MetaLabel>Checkpoint coverage</MetaLabel>
              <MetaValue>{checkpointCount} recovery points</MetaValue>
              <MetaBody>
                The UI should make rollback confidence feel built in, not hidden behind a workflow.
              </MetaBody>
            </MetaItem>
            <MetaItem>
              <MetaLabel>Footprint</MetaLabel>
              <MetaValue>
                {workspaces.length} workspaces across {regionCount || 0} regions
              </MetaValue>
              <MetaBody>Redis-backed filesystems stay legible even as the fleet spreads out.</MetaBody>
            </MetaItem>
          </HeroMetaGrid>
        </HeroLayout>
      </HeroCard>

      <StatGrid>
        <StatCard>
          <div>
            <StatLabel>Workspace Fleet</StatLabel>
            <StatValue>{workspaces.length}</StatValue>
          </div>
          <StatDetail>
            {healthyWorkspaces} healthy and {dirtyWorkspaces} holding draft edits.
          </StatDetail>
        </StatCard>
        <StatCard>
          <div>
            <StatLabel>Imported Sources</StatLabel>
            <StatValue>{importedWorkspaces}</StatValue>
          </div>
          <StatDetail>Git and Redis Cloud imports should read as first-class entry paths.</StatDetail>
        </StatCard>
        <StatCard>
          <div>
            <StatLabel>Stored Content</StatLabel>
            <StatValue>{formatBytes(totalBytes)}</StatValue>
          </div>
          <StatDetail>Size is useful when a workspace becomes a long-lived memory or artifact set.</StatDetail>
        </StatCard>
        <StatCard>
          <div>
            <StatLabel>Recovery Depth</StatLabel>
            <StatValue>{checkpointCount}</StatValue>
          </div>
          <StatDetail>Checkpoint history should stay close to the live editing surface.</StatDetail>
        </StatCard>
      </StatGrid>

      <SectionGrid>
        <SectionCard $span={7}>
          <SectionHeader>
            <SectionTitle
              eyebrow="Fleet Signals"
              title="Hot workspaces"
              body="These cards should feel like operational readouts: source, mutable state, content footprint, and the Redis mapping you will touch if you open the studio."
            />
          </SectionHeader>
          <WorkspaceGrid>
            {featuredWorkspaces.map((workspace) => (
              <WorkspaceCard key={workspace.id}>
                <CardHeader>
                  <div>
                    <Typography.Heading component="h3" size="S">
                      {workspace.name}
                    </Typography.Heading>
                    <Typography.Body color="secondary" component="p">
                      {workspace.databaseName} · {workspace.redisKey}
                    </Typography.Body>
                  </div>
                  <ToneChip $tone={workspace.status}>{statusLabel(workspace.status)}</ToneChip>
                </CardHeader>
                <InlineActions>
                  <ToneChip $tone={workspace.draftState}>
                    {draftStateLabel(workspace.draftState)}
                  </ToneChip>
                  <ToneChip $tone={workspace.source}>{sourceLabel(workspace.source)}</ToneChip>
                </InlineActions>
                <WorkspaceFacts>
                  <FactTile>
                    <FactLabel>Content</FactLabel>
                    <FactValue>{workspace.fileCount} files</FactValue>
                  </FactTile>
                  <FactTile>
                    <FactLabel>Footprint</FactLabel>
                    <FactValue>{formatBytes(workspace.totalBytes)}</FactValue>
                  </FactTile>
                  <FactTile>
                    <FactLabel>Checkpoints</FactLabel>
                    <FactValue>{workspace.checkpointCount}</FactValue>
                  </FactTile>
                  <FactTile>
                    <FactLabel>Updated</FactLabel>
                    <FactValue>{new Date(workspace.updatedAt).toLocaleDateString()}</FactValue>
                  </FactTile>
                </WorkspaceFacts>
                <MetaRow>
                  <Tag>{workspace.region}</Tag>
                  <Tag>{workspace.folderCount} folders</Tag>
                  <Tag>{new Date(workspace.lastCheckpointAt).toLocaleDateString()}</Tag>
                </MetaRow>
              </WorkspaceCard>
            ))}
          </WorkspaceGrid>
        </SectionCard>

        <SectionCard $span={5}>
          <SectionHeader>
            <SectionTitle
              eyebrow="Audit Pulse"
              title="Recent activity"
              body="The event feed is the fastest way to validate that the console reflects real Redis-backed mutations."
            />
          </SectionHeader>
          <EventList events={activity} />
        </SectionCard>
      </SectionGrid>

      <SectionGrid>
        <SectionCard $span={4}>
          <SectionTitle
            eyebrow="Operator Loop"
            title="What this UI should make obvious"
            body="The console is working when these decisions require almost no interpretation from the operator."
          />
          <Checklist>
            <ChecklistItem>
              <ChecklistTitle>Which state is canonical?</ChecklistTitle>
              <ChecklistBody>Saved head, working copy, and checkpoints must read as clearly distinct views.</ChecklistBody>
            </ChecklistItem>
            <ChecklistItem>
              <ChecklistTitle>Can I safely roll back?</ChecklistTitle>
              <ChecklistBody>Recovery actions should always be adjacent to the history they affect.</ChecklistBody>
            </ChecklistItem>
            <ChecklistItem>
              <ChecklistTitle>What changed recently?</ChecklistTitle>
              <ChecklistBody>The audit trail belongs in the same visual language as file and checkpoint work.</ChecklistBody>
            </ChecklistItem>
          </Checklist>
        </SectionCard>

        <SectionCard $span={8}>
          <SectionTitle
            eyebrow="Studio Intent"
            title="The three surfaces to reinforce"
            body="Every route in the UI can ladder back to these product truths, which keeps the design opinionated instead of drifting toward generic admin chrome."
          />
          <InsightGrid>
            <InsightCard>
              <InsightKicker>Canonical State</InsightKicker>
              <InsightTitle>Redis-backed head is the stable reference point</InsightTitle>
              <InsightBody>
                The saved head should feel like the trustworthy baseline operators compare everything
                else against.
              </InsightBody>
            </InsightCard>
            <InsightCard>
              <InsightKicker>Mutable State</InsightKicker>
              <InsightTitle>Working copy edits are live, visible, and intentionally temporary</InsightTitle>
              <InsightBody>
                Dirty draft state needs visual tension so in-progress edits are noticeable without
                looking like errors.
              </InsightBody>
            </InsightCard>
            <InsightCard>
              <InsightKicker>Recovery State</InsightKicker>
              <InsightTitle>Checkpoint history should feel like an instrument panel</InsightTitle>
              <InsightBody>
                Recovery points earn trust when the UI makes them tangible, recent, and easy to
                compare against the current workspace.
              </InsightBody>
            </InsightCard>
          </InsightGrid>
        </SectionCard>
      </SectionGrid>
    </PageStack>
  );
}

function statusLabel(status: AFSWorkspaceStatus) {
  if (status === "healthy") return "Healthy";
  if (status === "syncing") return "Syncing";
  return "Attention";
}

function draftStateLabel(state: AFSDraftState) {
  return state === "dirty" ? "Draft dirty" : "Draft clean";
}

function sourceLabel(source: AFSWorkspaceSource) {
  if (source === "git-import") return "Git import";
  if (source === "cloud-import") return "Cloud import";
  return "Blank";
}

const FlowGrid = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(3, minmax(0, 1fr));

  @media (max-width: 860px) {
    grid-template-columns: 1fr;
  }
`;

const FlowCard = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 20px;
  padding: 16px 18px;
  background: rgba(255, 255, 255, 0.78);
`;

const FlowStep = styled.div`
  color: var(--afs-amber);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.14em;
  text-transform: uppercase;
`;

const FlowTitle = styled.div`
  margin-top: 10px;
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
`;

const FlowBody = styled.p`
  margin: 8px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const MetaLabel = styled.span`
  color: var(--afs-muted);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const MetaValue = styled.span`
  color: var(--afs-ink);
  font-size: 1.1rem;
  font-weight: 700;
  line-height: 1.35;
`;

const MetaBody = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const WorkspaceFacts = styled.div`
  display: grid;
  gap: 10px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  margin-top: 16px;
`;

const FactTile = styled.div`
  border-radius: 18px;
  padding: 12px 14px;
  background: rgba(255, 255, 255, 0.74);
  border: 1px solid var(--afs-line);
`;

const FactLabel = styled.div`
  color: var(--afs-muted);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
`;

const FactValue = styled.div`
  margin-top: 6px;
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 700;
`;

const Checklist = styled.div`
  display: grid;
  gap: 12px;
  margin-top: 18px;
`;

const ChecklistItem = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 18px;
  padding: 16px;
  background: rgba(255, 255, 255, 0.74);
`;

const ChecklistTitle = styled.div`
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
`;

const ChecklistBody = styled.p`
  margin: 8px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const InsightGrid = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin-top: 18px;

  @media (max-width: 960px) {
    grid-template-columns: 1fr;
  }
`;

const InsightCard = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 22px;
  padding: 18px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.84), rgba(243, 246, 251, 0.92)),
    rgba(255, 255, 255, 0.8);
`;

const InsightKicker = styled.div`
  color: var(--afs-accent);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const InsightTitle = styled.div`
  margin-top: 10px;
  color: var(--afs-ink);
  font-size: 16px;
  font-weight: 700;
  line-height: 1.45;
`;

const InsightBody = styled.p`
  margin: 8px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.65;
`;
