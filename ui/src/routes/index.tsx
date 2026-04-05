import { createFileRoute, Link } from "@tanstack/react-router";
import { Button, Loader, Typography } from "@redislabsdev/redis-ui-components";
import { EventList, HeroBody, HeroCard, HeroLayout, HeroMetaGrid, InlineActions, MetaItem, PageStack, SectionCard, SectionGrid, SectionHeader, SectionTitle, StatCard, StatGrid, Tag, ToneChip, WorkspaceCard, WorkspaceGrid } from "../components/raf-kit";
import { useWorkspaces } from "../foundation/hooks/use-raf";
import { getRAFClientMode } from "../foundation/api/raf";

export const Route = createFileRoute("/")({
  component: OverviewPage,
});

function OverviewPage() {
  const workspacesQuery = useWorkspaces();

  if (workspacesQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const workspaces = workspacesQuery.data ?? [];
  const backendMode = getRAFClientMode();
  const sessions = workspaces.flatMap((workspace) => workspace.sessions);
  const activity = workspaces
    .flatMap((workspace) =>
      workspace.activity.map((event) => ({
        ...event,
        title: `${workspace.name}: ${event.title}`,
      })),
    )
    .sort((left, right) => right.createdAt.localeCompare(left.createdAt))
    .slice(0, 5);
  const dirtySessions = sessions.filter((session) => session.status === "dirty").length;

  return (
    <PageStack>
      <HeroCard>
        <HeroLayout>
          <HeroBody>
            <SectionTitle
              eyebrow="Redis Cloud Frame"
              title="RAF workspace management in a Redis-native shell"
              body="This first cut uses the multi-cluster-manager frame for navigation and titles, while the content area becomes a custom RAF control surface for workspaces, sessions, savepoints, and the browser/editor studio."
            />
            <InlineActions>
              <Link to="/workspaces">
                <Button size="medium">Open workspace catalog</Button>
              </Link>
              <Link to="/sessions">
                <Button size="medium" variant="secondary-fill">
                  Review sessions
                </Button>
              </Link>
            </InlineActions>
          </HeroBody>
          <HeroMetaGrid>
            <MetaItem>
              <Typography.Body color="secondary" component="p">
                Default mental model
              </Typography.Body>
              <Typography.Heading component="h3" size="S">
                Workspace, Session, Savepoint
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
                {backendMode === "http" ? "HTTP control plane" : "Local RAF demo store"}
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
            Sessions
          </Typography.Body>
          <Typography.Heading component="h2" size="L">
            {sessions.length}
          </Typography.Heading>
        </StatCard>
        <StatCard>
          <Typography.Body color="secondary" component="p">
            Dirty sessions
          </Typography.Body>
          <Typography.Heading component="h2" size="L">
            {dirtySessions}
          </Typography.Heading>
        </StatCard>
        <StatCard>
          <Typography.Body color="secondary" component="p">
            Savepoints
          </Typography.Body>
          <Typography.Heading component="h2" size="L">
            {sessions.reduce((sum, session) => sum + session.savepoints.length, 0)}
          </Typography.Heading>
        </StatCard>
      </StatGrid>

      <SectionGrid>
        <SectionCard $span={7}>
          <SectionHeader>
            <SectionTitle
              title="Live workspaces"
              body="Healthy and active RAF workspaces surfaced in a Cloud-style catalog."
            />
          </SectionHeader>
          <WorkspaceGrid>
            {workspaces.slice(0, 4).map((workspace) => (
              <WorkspaceCard key={workspace.id}>
                <Typography.Heading component="h3" size="S">
                  {workspace.name}
                </Typography.Heading>
                <Typography.Body color="secondary" component="p">
                  {workspace.description}
                </Typography.Body>
                <InlineActions style={{ marginTop: 12 }}>
                  <ToneChip $tone={workspace.status}>{workspace.status}</ToneChip>
                  <ToneChip $tone={workspace.source}>{workspace.source}</ToneChip>
                </InlineActions>
                <div style={{ marginTop: 14, display: "flex", gap: 8, flexWrap: "wrap" }}>
                  <Tag>{workspace.region}</Tag>
                  <Tag>{workspace.sessions.length} sessions</Tag>
                </div>
              </WorkspaceCard>
            ))}
          </WorkspaceGrid>
        </SectionCard>

        <SectionCard $span={5}>
          <SectionHeader>
            <SectionTitle
              title="Recent activity"
              body="The same event stream can later be replaced by RAF audit data from Redis."
            />
          </SectionHeader>
          <EventList events={activity} />
        </SectionCard>
      </SectionGrid>
    </PageStack>
  );
}
