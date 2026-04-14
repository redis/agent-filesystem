import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redislabsdev/redis-ui-components";
import { z } from "zod";
import {
  PageStack,
  InlineActions,
} from "../components/afs-kit";
import { useScopedAgents } from "../foundation/database-scope";
import { AgentsTable } from "../foundation/tables/agents-table";
import type { AFSAgentSession } from "../foundation/types/afs";

const agentsSearchSchema = z.object({
  workspaceId: z.string().optional(),
  databaseId: z.string().optional(),
});

export const Route = createFileRoute("/agents")({
  validateSearch: agentsSearchSchema,
  component: AgentsPage,
});

function AgentsPage() {
  const navigate = useNavigate();
  const search = Route.useSearch();
  const agentsQuery = useScopedAgents();
  const workspaceId = search.workspaceId;
  const databaseId = search.databaseId;

  if (agentsQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const rows = agentsQuery.data.filter((agent) => {
    if (workspaceId != null && agent.workspaceId !== workspaceId) {
      return false;
    }
    if (databaseId != null && agent.databaseId !== databaseId) {
      return false;
    }
    return true;
  });

  function openWorkspace(agent: AFSAgentSession) {
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: agent.workspaceId },
      search: agent.databaseId ? { databaseId: agent.databaseId } : {},
    });
  }

  return (
    <PageStack>
      {workspaceId != null || databaseId != null ? (
        <InlineActions>
          <Button
            kind="ghost"
            size="small"
            onClick={() => {
              void navigate({ to: "/agents", search: {} });
            }}
          >
            Show all agents
          </Button>
        </InlineActions>
      ) : null}
      <AgentsTable
        rows={rows}
        loading={agentsQuery.isLoading}
        error={agentsQuery.isError}
        onOpenWorkspace={openWorkspace}
      />
    </PageStack>
  );
}
