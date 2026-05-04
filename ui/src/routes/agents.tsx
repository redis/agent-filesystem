import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redis-ui/components";
import { useEffect } from "react";
import styled from "styled-components";
import { z } from "zod";
import {
  InlineActions,
  NoticeBody,
  NoticeCard,
  NoticeTitle,
  PageStack,
  SectionCard,
  SectionGrid,
  TabButton,
  Tabs,
} from "../components/afs-kit";
import { AgentSetupGuide } from "../features/agents/AgentSetupGuide";
import { useAuthSession } from "../foundation/auth-context";
import { useDatabaseScope } from "../foundation/database-scope";
import {
  useActivity,
  useAgents,
  useWorkspaceSummaries,
} from "../foundation/hooks/use-afs";
import { ActivityTable } from "../foundation/tables/activity-table";
import { AgentsTable } from "../foundation/tables/agents-table";
import type {
  AFSActivityEvent,
  AFSAgentSession,
} from "../foundation/types/afs";

type AgentsTab = "active" | "history";

const agentsSearchSchema = z.object({
  workspaceId: z.string().optional(),
  databaseId: z.string().optional(),
  tab: z.enum(["active", "history"]).optional(),
});

export const Route = createFileRoute("/agents")({
  validateSearch: agentsSearchSchema,
  component: AgentsPage,
});

function AgentsPage() {
  const navigate = useNavigate();
  const auth = useAuthSession();
  const search = Route.useSearch();
  const { unavailableDatabases } = useDatabaseScope();
  const queriesEnabled = !auth.isLoading && (!auth.config.enabled || auth.isAuthenticated);
  const agentsQuery = useAgents(null, queriesEnabled);
  const activityQuery = useActivity(search.databaseId ?? null, 100, queriesEnabled);
  const workspacesQuery = useWorkspaceSummaries(null, queriesEnabled);

  const tab: AgentsTab = search.tab ?? "active";
  const workspaceId = search.workspaceId;
  const databaseId = search.databaseId;
  const allAgents = agentsQuery.data ?? [];

  useEffect(() => {
    const interval = setInterval(() => {
      void agentsQuery.refetch();
      void activityQuery.refetch();
    }, 5000);
    return () => clearInterval(interval);
  }, [activityQuery, agentsQuery]);

  const currentConnections = allAgents.filter((agent) => {
    if (workspaceId != null && agent.workspaceId !== workspaceId) {
      return false;
    }
    if (databaseId != null && agent.databaseId !== databaseId) {
      return false;
    }
    return true;
  });

  const connectionHistory = (activityQuery.data ?? [])
    .filter((event) => event.kind === "session.started")
    .filter((event) => {
      if (workspaceId != null && event.workspaceId !== workspaceId) {
        return false;
      }
      if (databaseId != null && event.databaseId !== databaseId) {
        return false;
      }
      return true;
    })
    .sort((left, right) => new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime());

  const isFiltered = workspaceId != null || databaseId != null;

  function openWorkspace(agent: AFSAgentSession) {
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: agent.workspaceId },
      search: agent.databaseId ? { databaseId: agent.databaseId } : {},
    });
  }

  function openActivity(event: AFSActivityEvent) {
    if (event.workspaceId == null) {
      return;
    }
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: event.workspaceId },
      search: event.databaseId ? { databaseId: event.databaseId } : {},
    });
  }

  function setTab(nextTab: AgentsTab) {
    void navigate({
      to: "/agents",
      search: {
        ...(workspaceId ? { workspaceId } : {}),
        ...(databaseId ? { databaseId } : {}),
        ...(nextTab === "active" ? {} : { tab: nextTab }),
      },
      replace: true,
    });
  }

  if (auth.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  if (agentsQuery.isLoading && tab === "active") {
    return <Loader data-testid="loader--spinner" />;
  }

  if (tab === "active" && currentConnections.length === 0 && connectionHistory.length === 0 && !isFiltered && !agentsQuery.isError) {
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
        <Tabs>
          <TabButton $active onClick={() => setTab("active")}>
            Active Agents
          </TabButton>
          <TabButton $active={false} onClick={() => setTab("history")}>
            Connection History
          </TabButton>
        </Tabs>
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
            Agent results are partial while these databases are disconnected:{" "}
            {unavailableDatabases.map((database) => database.displayName || database.databaseName).join(", ")}.
          </NoticeBody>
        </NoticeCard>
      ) : null}

      <Tabs>
        <TabButton $active={tab === "active"} onClick={() => setTab("active")}>
          Active Agents
        </TabButton>
        <TabButton $active={tab === "history"} onClick={() => setTab("history")}>
          Connection History
        </TabButton>
      </Tabs>

      {isFiltered ? (
        <InlineActions>
          <Button
            kind="ghost"
            size="small"
            onClick={() => {
              void navigate({
                to: "/agents",
                search: tab === "active" ? {} : { tab },
              });
            }}
          >
            Show all
          </Button>
        </InlineActions>
      ) : null}

      {tab === "active" ? (
        <AgentsTable
          rows={currentConnections}
          workspaces={workspacesQuery.data}
          loading={agentsQuery.isLoading}
          error={agentsQuery.isError}
          // intentionally no toolbarAction — agents register themselves over MCP
          // (Claude Code, Codex, etc.). The empty state below explains the
          // setup path; once any agent is connected, this table is observable
          // only.
          onOpenWorkspace={openWorkspace}
        />
      ) : null}

      {tab === "history" ? (
        <SectionGrid>
          <SectionCard $span={12}>
            <ActivityTable
              rows={connectionHistory}
              loading={activityQuery.isLoading}
              error={activityQuery.isError}
              errorMessage={activityQuery.error instanceof Error ? activityQuery.error.message : undefined}
              hideTypeColumn
              onOpenActivity={openActivity}
            />
          </SectionCard>
        </SectionGrid>
      ) : null}
    </PageStack>
  );
}

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
          Connect an agent to start working with your workspaces. Agents sync files to Redis automatically and appear here in real time.
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
