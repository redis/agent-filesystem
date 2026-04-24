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
import { AccessTokenEmptyState } from "../components/access-token-empty-state";
import { MCPConnectionPanel } from "../components/mcp-connection-panel";
import { useAuthSession } from "../foundation/auth-context";
import { useDatabaseScope } from "../foundation/database-scope";
import {
  useAllMCPAccessTokens,
  useControlPlaneTokens,
  useDatabases,
  useRevokeControlPlaneTokenMutation,
  useRevokeMCPAccessTokenMutation,
  useWorkspaceSummaries,
} from "../foundation/hooks/use-afs";
import { AccessTokensTable } from "../foundation/tables/access-tokens-table";
import { isControlPlaneScope, type AFSMCPToken } from "../foundation/types/afs";

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
  const workspaceTokensQuery = useAllMCPAccessTokens(queriesEnabled);
  const controlPlaneTokensQuery = useControlPlaneTokens(queriesEnabled);
  const revokeWorkspaceToken = useRevokeMCPAccessTokenMutation();
  const revokeControlPlaneToken = useRevokeControlPlaneTokenMutation();

  const [createOpen, setCreateOpen] = useState(false);
  const [localOpen, setLocalOpen] = useState(false);

  const workspaceId = search.workspaceId;
  const databaseId = search.databaseId;
  const allWorkspaces = workspacesQuery.data ?? [];
  const databases = databasesQuery.data ?? [];

  useEffect(() => {
    const interval = setInterval(() => {
      void workspaceTokensQuery.refetch();
      void controlPlaneTokensQuery.refetch();
    }, 5000);
    return () => clearInterval(interval);
  }, [workspaceTokensQuery, controlPlaneTokensQuery]);

  const workspaceNameById = useMemo(
    () => new Map(allWorkspaces.map((workspace) => [workspace.id, workspace.name])),
    [allWorkspaces],
  );
  const databaseNameById = useMemo(
    () => new Map(databases.map((database) => [database.id, database.name])),
    [databases],
  );

  // Merge workspace-scoped tokens and control-plane tokens into one list.
  // Client-side filter for control-plane tokens (defensive — the backend
  // should only return control-plane entries to that query). Workspace list
  // already excludes scope-empty/control-plane rows by virtue of requiring a
  // database binding on the server.
  const allTokens: AFSMCPToken[] = useMemo(() => {
    const workspace = (workspaceTokensQuery.data ?? []).filter(
      (token) => !isControlPlaneScope(token.scope),
    );
    const controlPlane = (controlPlaneTokensQuery.data ?? []).filter((token) =>
      isControlPlaneScope(token.scope),
    );
    return [...controlPlane, ...workspace];
  }, [workspaceTokensQuery.data, controlPlaneTokensQuery.data]);

  const filteredTokens = allTokens
    .filter((token) => !token.revokedAt || token.revokedAt === "")
    .filter((token) => {
      // Control-plane tokens are never workspace/database filtered.
      if (isControlPlaneScope(token.scope)) return true;
      if (workspaceId != null && token.workspaceId !== workspaceId) {
        return false;
      }
      if (databaseId != null && token.databaseId !== databaseId) {
        return false;
      }
      return true;
    })
    .sort(
      (left, right) => new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime(),
    );

  const isFiltered = workspaceId != null || databaseId != null;
  const isLoading = workspaceTokensQuery.isLoading || controlPlaneTokensQuery.isLoading;
  const loadError =
    workspaceTokensQuery.error instanceof Error
      ? workspaceTokensQuery.error
      : controlPlaneTokensQuery.error instanceof Error
        ? controlPlaneTokensQuery.error
        : null;
  const isError = Boolean(workspaceTokensQuery.isError || controlPlaneTokensQuery.isError);

  async function revokeToken(token: AFSMCPToken) {
    if (isControlPlaneScope(token.scope)) {
      await revokeControlPlaneToken.mutateAsync(token.id);
      return;
    }
    await revokeWorkspaceToken.mutateAsync({
      databaseId: token.databaseId,
      workspaceId: token.workspaceId,
      tokenId: token.id,
    });
  }

  if (auth.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const revoking = revokeWorkspaceToken.isPending || revokeControlPlaneToken.isPending;

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

      <MCPConnectionPanel />

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

      {loadError ? <DialogError role="alert">{loadError.message}</DialogError> : null}

      {!isLoading && !isError && !isFiltered && filteredTokens.length === 0 ? (
        <AccessTokenEmptyState
          onCreateToken={() => setCreateOpen(true)}
          onCreateLocalToken={() => setLocalOpen(true)}
        />
      ) : (
        <AccessTokensTable
          rows={filteredTokens}
          loading={isLoading}
          error={isError}
          workspaceNameById={workspaceNameById}
          databaseNameById={databaseNameById}
          revoking={revoking}
          onRevoke={(token) => void revokeToken(token)}
          toolbarAction={(
            <HeaderActions>
              <Button
                size="medium"
                variant="secondary-fill"
                onClick={() => setLocalOpen(true)}
              >
                Local
              </Button>
              <Button size="medium" onClick={() => setCreateOpen(true)}>
                Create token
              </Button>
            </HeaderActions>
          )}
        />
      )}

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
