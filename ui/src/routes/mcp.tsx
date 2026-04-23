import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redis-ui/components";
import { useEffect, useMemo, useState } from "react";
import { z } from "zod";
import {
  DialogError,
  InlineActions,
  NoticeBody,
  NoticeCard,
  NoticeTitle,
  PageStack,
} from "../components/afs-kit";
import styled from "styled-components";
import { CreateMCPAccessDialog } from "../features/agents/CreateMCPAccessDialog";
import { LocalMCPAccessDialog } from "../features/agents/LocalMCPAccessDialog";
import { useAuthSession } from "../foundation/auth-context";
import { useDatabaseScope } from "../foundation/database-scope";
import {
  useAllMCPAccessTokens,
  useDatabases,
  useRevokeMCPAccessTokenMutation,
  useWorkspaceSummaries,
} from "../foundation/hooks/use-afs";
import { MCPServersTable } from "../foundation/tables/mcp-servers-table";
import type { AFSMCPToken } from "../foundation/types/afs";

const mcpSearchSchema = z.object({
  workspaceId: z.string().optional(),
  databaseId: z.string().optional(),
});

export const Route = createFileRoute("/mcp")({
  validateSearch: mcpSearchSchema,
  component: MCPPage,
});

function MCPPage() {
  const navigate = useNavigate();
  const auth = useAuthSession();
  const search = Route.useSearch();
  const { unavailableDatabases } = useDatabaseScope();
  const queriesEnabled = !auth.isLoading && (!auth.config.enabled || auth.isAuthenticated);
  const databasesQuery = useDatabases(queriesEnabled);
  const workspacesQuery = useWorkspaceSummaries(search.databaseId ?? null, queriesEnabled);
  const allTokensQuery = useAllMCPAccessTokens(queriesEnabled);
  const revokeMCPAccessToken = useRevokeMCPAccessTokenMutation();

  const [createOpen, setCreateOpen] = useState(false);
  const [localOpen, setLocalOpen] = useState(false);

  const workspaceId = search.workspaceId;
  const databaseId = search.databaseId;
  const allWorkspaces = workspacesQuery.data ?? [];
  const allTokens = allTokensQuery.data ?? [];
  const databases = databasesQuery.data ?? [];

  useEffect(() => {
    const interval = setInterval(() => {
      void allTokensQuery.refetch();
    }, 5000);
    return () => clearInterval(interval);
  }, [allTokensQuery]);

  const workspaceNameById = useMemo(
    () => new Map(allWorkspaces.map((workspace) => [workspace.id, workspace.name])),
    [allWorkspaces],
  );
  const databaseNameById = useMemo(
    () => new Map(databases.map((database) => [database.id, database.name])),
    [databases],
  );

  const filteredTokens = allTokens
    .filter((token) => token.revokedAt == null || token.revokedAt === "")
    .filter((token) => {
      if (workspaceId != null && token.workspaceId !== workspaceId) {
        return false;
      }
      if (databaseId != null && token.databaseId !== databaseId) {
        return false;
      }
      return true;
    })
    .sort((left, right) => new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime());

  const isFiltered = workspaceId != null || databaseId != null;

  async function revokeToken(token: AFSMCPToken) {
    await revokeMCPAccessToken.mutateAsync({
      databaseId: token.databaseId,
      workspaceId: token.workspaceId,
      tokenId: token.id,
    });
  }

  if (auth.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  return (
    <PageStack>
      {unavailableDatabases.length > 0 ? (
        <NoticeCard $tone="warning" role="status">
          <NoticeTitle>Some databases are unavailable</NoticeTitle>
          <NoticeBody>
            MCP results are partial while these databases are disconnected:{" "}
            {unavailableDatabases.map((database) => database.displayName || database.databaseName).join(", ")}.
          </NoticeBody>
        </NoticeCard>
      ) : null}

      {isFiltered ? (
        <InlineActions>
          <Button
            kind="ghost"
            size="small"
            onClick={() => {
              void navigate({ to: "/mcp", search: {} });
            }}
          >
            Show all
          </Button>
        </InlineActions>
      ) : null}

      {allTokensQuery.error instanceof Error ? (
        <DialogError role="alert">{allTokensQuery.error.message}</DialogError>
      ) : null}
      <MCPServersTable
        rows={filteredTokens}
        loading={allTokensQuery.isLoading}
        error={allTokensQuery.isError}
        workspaceNameById={workspaceNameById}
        databaseNameById={databaseNameById}
        revoking={revokeMCPAccessToken.isPending}
        onRevoke={(token) => void revokeToken(token)}
        toolbarAction={(
          <HeaderActions>
            <Button
              size="medium"
              variant="secondary-fill"
              onClick={() => setLocalOpen(true)}
            >
              Local MCP
            </Button>
            <Button size="medium" onClick={() => setCreateOpen(true)}>
              Add MCP
            </Button>
          </HeaderActions>
        )}
      />

      <CreateMCPAccessDialog
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        workspaces={allWorkspaces}
        initialWorkspaceId={workspaceId}
        initialDatabaseId={databaseId}
      />
      <LocalMCPAccessDialog
        isOpen={localOpen}
        onClose={() => setLocalOpen(false)}
        workspaces={allWorkspaces}
        initialWorkspaceId={workspaceId}
        initialDatabaseId={databaseId}
      />
    </PageStack>
  );
}

const HeaderActions = styled.div`
  display: flex;
  flex-wrap: nowrap;
  gap: 10px;
  align-items: center;
  flex-shrink: 0;
  white-space: nowrap;
`;
