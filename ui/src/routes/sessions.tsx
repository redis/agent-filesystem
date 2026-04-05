import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader, Typography } from "@redislabsdev/redis-ui-components";
import {
  CardHeader,
  InlineActions,
  PageStack,
  SectionTitle,
  ToneChip,
  WorkspaceCard,
  WorkspaceGrid,
} from "../components/raf-kit";
import { useWorkspaces } from "../foundation/hooks/use-raf";

export const Route = createFileRoute("/sessions")({
  component: SessionsPage,
});

function SessionsPage() {
  const navigate = useNavigate();
  const workspacesQuery = useWorkspaces();

  if (workspacesQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const rows = (workspacesQuery.data ?? [])
    .flatMap((workspace) =>
      workspace.sessions.map((session) => ({
        workspaceId: workspace.id,
        workspaceName: workspace.name,
        session,
        savepoint: session.savepoints.find((item) => item.id === session.headSavepointId),
      })),
    )
    .sort((left, right) => right.session.updatedAt.localeCompare(left.session.updatedAt));

  return (
    <PageStack>
      <SectionTitle
        eyebrow="Session Catalog"
        title="Sessions across every RAF workspace"
        body="This page is a cross-workspace view for operators who want to scan dirty sessions, recent savepoints, and branch activity without opening each workspace individually."
      />

      <WorkspaceGrid>
        {rows.map((row) => (
          <WorkspaceCard key={row.session.id}>
            <CardHeader>
              <div>
                <Typography.Heading component="h3" size="S">
                  {row.session.name}
                </Typography.Heading>
                <Typography.Body color="secondary" component="p">
                  {row.workspaceName}
                </Typography.Body>
              </div>
              <InlineActions>
                <ToneChip $tone={row.session.status}>{row.session.status}</ToneChip>
              </InlineActions>
            </CardHeader>
            <Typography.Body color="secondary" component="p">
              {row.session.description}
            </Typography.Body>
            <InlineActions style={{ marginTop: 14 }}>
              <ToneChip $tone={row.session.kind === "imported" ? "cloud-import" : "git-import"}>
                {row.session.kind}
              </ToneChip>
              <span>{row.savepoint?.name ?? "No savepoint"}</span>
            </InlineActions>
            <Typography.Body color="secondary" component="p" style={{ marginTop: 14 }}>
              Updated {new Date(row.session.updatedAt).toLocaleString()}
            </Typography.Body>
            <InlineActions style={{ marginTop: 16 }}>
              <Button
                size="medium"
                onClick={() =>
                  void navigate({
                    to: "/workspaces/$workspaceId",
                    params: { workspaceId: row.workspaceId },
                  })
                }
              >
                Open studio
              </Button>
            </InlineActions>
          </WorkspaceCard>
        ))}
      </WorkspaceGrid>
    </PageStack>
  );
}
