import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redis-ui/components";
import { useEffect } from "react";
import { z } from "zod";
import styled from "styled-components";
import {
  PageStack,
  InlineActions,
  NoticeBody,
  NoticeCard,
  NoticeTitle,
} from "../components/afs-kit";
import { AgentSetupGuide } from "../features/agents/AgentSetupGuide";
import { useDatabaseScope, useScopedAgents } from "../foundation/database-scope";
import { agentsQueryOptions } from "../foundation/hooks/use-afs";
import { queryClient } from "../foundation/query-client";
import { AgentsTable } from "../foundation/tables/agents-table";
import type { AFSAgentSession } from "../foundation/types/afs";

const agentsSearchSchema = z.object({
  workspaceId: z.string().optional(),
  databaseId: z.string().optional(),
});

export const Route = createFileRoute("/agents")({
  validateSearch: agentsSearchSchema,
  loader: () =>
    queryClient.ensureQueryData({ ...agentsQueryOptions(null), revalidateIfStale: true }),
  component: AgentsPage,
});

function AgentsPage() {
  const navigate = useNavigate();
  const search = Route.useSearch();
  const { unavailableDatabases } = useDatabaseScope();
  const agentsQuery = useScopedAgents();
  const workspaceId = search.workspaceId;
  const databaseId = search.databaseId;

  const allAgents = agentsQuery.data ?? [];

  // Poll so the table updates live when agents connect / disconnect.
  useEffect(() => {
    const interval = setInterval(() => {
      void agentsQuery.refetch();
    }, 5000);
    return () => clearInterval(interval);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (agentsQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const rows = allAgents.filter((agent) => {
    if (workspaceId != null && agent.workspaceId !== workspaceId) {
      return false;
    }
    if (databaseId != null && agent.databaseId !== databaseId) {
      return false;
    }
    return true;
  });

  const isFiltered = workspaceId != null || databaseId != null;

  function openWorkspace(agent: AFSAgentSession) {
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: agent.workspaceId },
    });
  }

  /* Show the getting-started empty state only when there are truly no agents
     (not when the user has a filter active that happens to match nothing). */
  if (rows.length === 0 && !isFiltered && !agentsQuery.isError) {
    return (
      <PageStack>

        {unavailableDatabases.length > 0 ? (
          <NoticeCard $tone="warning" role="status">
            <NoticeTitle>Some databases are unavailable</NoticeTitle>
            <NoticeBody>
              Connected-agent results are partial while these databases are disconnected:{" "}
              {unavailableDatabases.map((database) => database.displayName || database.databaseName).join(", ")}.
            </NoticeBody>
          </NoticeCard>
        ) : null}
        <AgentsEmptyState />
      </PageStack>
    );
  }

  return (
    <PageStack>
      {unavailableDatabases.length > 0 ? (
        <NoticeCard $tone="warning" role="status">
          <NoticeTitle>Some databases are unavailable</NoticeTitle>
          <NoticeBody>
            Connected-agent results are partial while these databases are disconnected:{" "}
            {unavailableDatabases.map((database) => database.displayName || database.databaseName).join(", ")}.
          </NoticeBody>
        </NoticeCard>
      ) : null}
      <InlineActions>
        {isFiltered ? (
          <Button
            kind="ghost"
            size="small"
            onClick={() => {
              void navigate({ to: "/agents", search: {} });
            }}
          >
            Show all agents
          </Button>
        ) : null}
        <ActionsSpacer />
        <Button
          size="small"
          onClick={() => {
            void navigate({ to: "/agents/add" });
          }}
        >
          + Add Agent
        </Button>
      </InlineActions>
      <AgentsTable
        rows={rows}
        loading={agentsQuery.isLoading}
        error={agentsQuery.isError}
        onOpenWorkspace={openWorkspace}
      />
    </PageStack>
  );
}

const ActionsSpacer = styled.div`
  flex: 1;
`;

/* ── Empty state ── */

function AgentsEmptyState() {
  return (
    <EmptyLayout>
      <EmptyHeader>
        <EmptyIcon>
          <svg width="48" height="48" viewBox="0 0 48 48" fill="none">
            <rect width="48" height="48" rx="14" fill="var(--afs-accent-soft, rgba(37,99,235,0.1))" />
            <path
              d="M24 14v4m0 12v4m-10-10h4m12 0h4m-14.24-7.07 2.83 2.83m9.65 9.65 2.83 2.83m0-15.31-2.83 2.83m-9.65 9.65-2.83 2.83"
              stroke="var(--afs-accent, #2563eb)"
              strokeWidth="2"
              strokeLinecap="round"
            />
          </svg>
        </EmptyIcon>
        <EmptyTitle>No agents connected</EmptyTitle>
        <EmptyDesc>
          Connect an agent to start working with your workspaces. Agents sync
          files to Redis automatically and appear here in real time.
        </EmptyDesc>
      </EmptyHeader>
      <AgentSetupGuide compact />
    </EmptyLayout>
  );
}

const EmptyLayout = styled.div`
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 720px;
  margin: 0 auto;
  padding: 20px 0 0;
`;

const EmptyHeader = styled.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  text-align: center;
  margin-bottom: 8px;
`;

const EmptyIcon = styled.div`
  margin-bottom: 4px;
`;

const EmptyTitle = styled.h3`
  margin: 0;
  color: var(--afs-ink);
  font-size: 20px;
  font-weight: 700;
  letter-spacing: -0.01em;
`;

const EmptyDesc = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
  max-width: 480px;
`;
