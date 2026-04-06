import { createFileRoute, Link } from "@tanstack/react-router";
import { Button, Loader, Typography } from "@redislabsdev/redis-ui-components";
import {
  EventList,
  HeroBody,
  HeroCard,
  HeroLayout,
  HeroMetaGrid,
  InlineActions,
  MetaItem,
  PageStack,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
  StatCard,
  StatGrid,
  Tag,
  ToneChip,
  WorkspaceCard,
  WorkspaceGrid,
} from "../components/afs-kit";
import { useActivity, useWorkspaceSummaries } from "../foundation/hooks/use-afs";
import { getAFSClientMode } from "../foundation/api/afs";

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

  return (
    <PageStack>
      <HeroCard>
        <HeroLayout>
          <HeroBody>
            <SectionTitle
              eyebrow="Redis Cloud Frame"
              title="AFS workspace management in a Redis-native shell"
              body="This first cut uses the multi-cluster-manager frame for navigation and titles, while the content area becomes a custom AFS control surface for workspaces, checkpoints, audit activity, and the browser/editor studio."
            />
            <InlineActions>
              <Link to="/workspaces">
                <Button size="medium">Open workspace catalog</Button>
              </Link>
            </InlineActions>
          </HeroBody>
          <HeroMetaGrid>
            <MetaItem>
              <Typography.Body color="secondary" component="p">
                Default mental model
              </Typography.Body>
              <Typography.Heading component="h3" size="S">
                Workspace, Draft, Checkpoint
              </Typography.Heading>
            </MetaItem>
            <MetaItem>
              <Typography.Body color="secondary" component="p">
                Editor mode
              </Typography.Body>
              <Typography.Heading component="h3" size="S">
                Browser + inline draft editing
              </Typography.Heading>
            </MetaItem>
            <MetaItem>
              <Typography.Body color="secondary" component="p">
                Integration target
              </Typography.Body>
              <Typography.Heading component="h3" size="S">
                Redis Cloud embedded experience
              </Typography.Heading>
            </MetaItem>
            <MetaItem>
              <Typography.Body color="secondary" component="p">
                Current data source
              </Typography.Body>
              <Typography.Heading component="h3" size="S">
                {backendMode === "http" ? "HTTP control plane" : "Local AFS demo store"}
              </Typography.Heading>
            </MetaItem>
          </HeroMetaGrid>
        </HeroLayout>
      </HeroCard>

      <StatGrid>
        <StatCard>
          <Typography.Body color="secondary" component="p">
            Workspaces
          </Typography.Body>
          <Typography.Heading component="h2" size="L">
            {workspaces.length}
          </Typography.Heading>
        </StatCard>
        <StatCard>
          <Typography.Body color="secondary" component="p">
            Healthy workspaces
          </Typography.Body>
          <Typography.Heading component="h2" size="L">
            {healthyWorkspaces}
          </Typography.Heading>
        </StatCard>
        <StatCard>
          <Typography.Body color="secondary" component="p">
            Dirty workspaces
          </Typography.Body>
          <Typography.Heading component="h2" size="L">
            {dirtyWorkspaces}
          </Typography.Heading>
        </StatCard>
        <StatCard>
          <Typography.Body color="secondary" component="p">
            Checkpoints
          </Typography.Body>
          <Typography.Heading component="h2" size="L">
            {checkpointCount}
          </Typography.Heading>
        </StatCard>
      </StatGrid>

      <SectionGrid>
        <SectionCard $span={7}>
          <SectionHeader>
            <SectionTitle
              title="Live workspaces"
              body="Healthy and active AFS workspaces surfaced in a Cloud-style catalog."
            />
          </SectionHeader>
          <WorkspaceGrid>
            {workspaces.slice(0, 4).map((workspace) => (
              <WorkspaceCard key={workspace.id}>
                <Typography.Heading component="h3" size="S">
                  {workspace.name}
                </Typography.Heading>
                <Typography.Body color="secondary" component="p">
                  {workspace.databaseName} · {workspace.redisKey}
                </Typography.Body>
                <InlineActions style={{ marginTop: 12 }}>
                  <ToneChip $tone={workspace.status}>{workspace.status}</ToneChip>
                  <ToneChip $tone={workspace.draftState}>{workspace.draftState}</ToneChip>
                  <ToneChip $tone={workspace.source}>{workspace.source}</ToneChip>
                </InlineActions>
                <div style={{ marginTop: 14, display: "flex", gap: 8, flexWrap: "wrap" }}>
                  <Tag>{workspace.region}</Tag>
                  <Tag>{workspace.checkpointCount} checkpoints</Tag>
                </div>
              </WorkspaceCard>
            ))}
          </WorkspaceGrid>
        </SectionCard>

        <SectionCard $span={5}>
          <SectionHeader>
            <SectionTitle
              title="Recent activity"
              body="The same event stream can later be replaced by AFS audit data from Redis."
            />
          </SectionHeader>
          <EventList events={activity} />
        </SectionCard>
      </SectionGrid>
    </PageStack>
  );
}
