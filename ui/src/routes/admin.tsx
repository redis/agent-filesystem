import { createFileRoute } from "@tanstack/react-router";
import { Loader } from "@redis-ui/components";
import { useMemo, useState } from "react";
import styled from "styled-components";
import {
  EmptyState,
  NoticeBody,
  NoticeCard,
  NoticeTitle,
  PageStack,
  SectionCard,
  SectionHeader,
  SectionTitle,
  StatCard,
  StatDetail,
  StatGrid,
  StatLabel,
  StatValue,
  TabButton,
  Tabs,
  TextInput,
} from "../components/afs-kit";
import { formatBytes } from "../foundation/api/afs";
import { isCloudAdminConfig, useAuthSession } from "../foundation/auth-context";
import {
  useAdminAgents,
  useAdminDatabases,
  useAdminOverview,
  useAdminUsers,
  useAdminWorkspaceSummaries,
} from "../foundation/hooks/use-afs";
import { shortDateTime } from "../foundation/time-format";
import type {
  AFSAdminOverview,
  AFSAdminUser,
  AFSAgentSession,
  AFSDatabase,
  AFSWorkspaceSummary,
} from "../foundation/types/afs";

export const Route = createFileRoute("/admin")({
  component: AdminPage,
});

type AdminView = "overview" | "users" | "databases" | "workspaces" | "agents";

const VIEWS: ReadonlyArray<{ id: AdminView; label: string }> = [
  { id: "overview", label: "Overview" },
  { id: "users", label: "Users" },
  { id: "databases", label: "Databases" },
  { id: "workspaces", label: "Workspaces" },
  { id: "agents", label: "Agents" },
];

function AdminPage() {
  const auth = useAuthSession();
  const isAdmin = isCloudAdminConfig(auth.config);
  const [view, setView] = useState<AdminView>("overview");
  const [search, setSearch] = useState("");
  const enabled = !auth.isLoading && isAdmin;

  const overviewQuery = useAdminOverview(enabled);
  const usersQuery = useAdminUsers(enabled);
  const databasesQuery = useAdminDatabases(enabled);
  const workspacesQuery = useAdminWorkspaceSummaries(enabled);
  const agentsQuery = useAdminAgents(enabled);

  const users = usersQuery.data ?? [];
  const databases = databasesQuery.data ?? [];
  const workspaces = workspacesQuery.data ?? [];
  const agents = agentsQuery.data ?? [];

  const filteredUsers = useMemo(
    () => filterRows(users, search, (item) => [
      item.subject,
      item.label,
      item.sources.join(" "),
    ]),
    [search, users],
  );
  const filteredDatabases = useMemo(
    () => filterRows(databases, search, (item) => [
      item.id,
      item.name,
      item.description,
      item.ownerSubject,
      item.ownerLabel,
      item.redisAddr,
      item.managementType,
      item.purpose,
    ]),
    [databases, search],
  );
  const filteredWorkspaces = useMemo(
    () => filterRows(workspaces, search, (item) => [
      item.id,
      item.name,
      item.databaseName,
      item.databaseId,
      item.ownerSubject,
      item.ownerLabel,
      item.cloudAccount,
      item.redisKey,
      item.region,
    ]),
    [search, workspaces],
  );
  const filteredAgents = useMemo(
    () => filterRows(agents, search, (item) => [
      item.sessionId,
      item.agentId,
      item.label,
      item.ownerSubject,
      item.ownerLabel,
      item.workspaceName,
      item.workspaceId,
      item.databaseName,
      item.databaseId,
      item.hostname,
      item.localPath,
      item.state,
    ]),
    [agents, search],
  );

  if (auth.isLoading) {
    return (
      <PageStack>
        <CenteredState>
          <Loader data-testid="loader--admin-auth" />
        </CenteredState>
      </PageStack>
    );
  }

  if (!isAdmin) {
    return (
      <PageStack>
        <NoticeCard $tone="neutral" role="status">
          <NoticeTitle>Page not found</NoticeTitle>
          <NoticeBody>The requested page is not available for this account.</NoticeBody>
        </NoticeCard>
      </PageStack>
    );
  }

  return (
    <PageStack>
      <AdminHeader>
        <Tabs role="tablist" aria-label="Admin views">
          {VIEWS.map((item) => (
            <TabButton
              key={item.id}
              type="button"
              role="tab"
              $active={view === item.id}
              aria-selected={view === item.id}
              onClick={() => {
                setView(item.id);
                setSearch("");
              }}
            >
              {item.label}
            </TabButton>
          ))}
        </Tabs>
        {view !== "overview" ? (
          <SearchInput
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={`Search ${view}`}
            aria-label={`Search ${view}`}
          />
        ) : null}
      </AdminHeader>

      {view === "overview" ? (
        <OverviewPanel
          loading={overviewQuery.isLoading}
          error={overviewQuery.error}
          overview={overviewQuery.data}
        />
      ) : null}
      {view === "users" ? <UsersPanel loading={usersQuery.isLoading} error={usersQuery.error} rows={filteredUsers} /> : null}
      {view === "databases" ? <DatabasesPanel loading={databasesQuery.isLoading} error={databasesQuery.error} rows={filteredDatabases} /> : null}
      {view === "workspaces" ? <WorkspacesPanel loading={workspacesQuery.isLoading} error={workspacesQuery.error} rows={filteredWorkspaces} /> : null}
      {view === "agents" ? <AgentsPanel loading={agentsQuery.isLoading} error={agentsQuery.error} rows={filteredAgents} /> : null}
    </PageStack>
  );
}

function OverviewPanel({
  loading,
  error,
  overview,
}: {
  loading: boolean;
  error: unknown;
  overview?: AFSAdminOverview;
}) {
  if (loading) return <CenteredLoader />;
  if (error) return <PanelError error={error} />;
  if (overview == null) return <EmptyState>No admin overview data.</EmptyState>;

  return (
    <StatGrid>
      <StatCard>
        <StatLabel>Users</StatLabel>
        <StatValue>{overview.userCount}</StatValue>
        <StatDetail>Derived from owners, tokens, and sessions</StatDetail>
      </StatCard>
      <StatCard>
        <StatLabel>Databases</StatLabel>
        <StatValue>{overview.databaseCount}</StatValue>
        <StatDetail>{overview.unavailableDatabaseCount} unavailable</StatDetail>
      </StatCard>
      <StatCard>
        <StatLabel>Workspaces</StatLabel>
        <StatValue>{overview.workspaceCount}</StatValue>
        <StatDetail>{overview.fileCount.toLocaleString()} files</StatDetail>
      </StatCard>
      <StatCard>
        <StatLabel>Agents</StatLabel>
        <StatValue>{overview.agentCount}</StatValue>
        <StatDetail>{overview.activeAgentCount} active, {overview.staleAgentCount} stale</StatDetail>
      </StatCard>
      <StatCard>
        <StatLabel>Stored</StatLabel>
        <StatValue>{formatBytes(overview.totalBytes)}</StatValue>
        <StatDetail>Total workspace footprint</StatDetail>
      </StatCard>
    </StatGrid>
  );
}

function UsersPanel({ loading, error, rows }: { loading: boolean; error: unknown; rows: AFSAdminUser[] }) {
  if (loading) return <CenteredLoader />;
  if (error) return <PanelError error={error} />;
  return (
    <SectionCard $span={12}>
      <SectionHeader>
        <SectionTitle title="Users" body="Accounts discovered from ownership, access tokens, and agent sessions." />
      </SectionHeader>
      <DataTable empty="No users matched." rowCount={rows.length}>
        <thead>
          <tr>
            <th>User</th>
            <th>Databases</th>
            <th>Workspaces</th>
            <th>Agents</th>
            <th>MCP tokens</th>
            <th>Last seen</th>
            <th>Sources</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.subject}>
              <td><PrimaryCell title={row.subject}>{row.label || row.subject}<MetaLine>{row.subject}</MetaLine></PrimaryCell></td>
              <td>{row.databaseCount}</td>
              <td>{row.workspaceCount}</td>
              <td>{row.agentSessionCount}</td>
              <td>{row.mcpTokenCount}</td>
              <td>{formatDate(row.lastSeenAt)}</td>
              <td>{row.sources.join(", ")}</td>
            </tr>
          ))}
        </tbody>
      </DataTable>
    </SectionCard>
  );
}

function DatabasesPanel({ loading, error, rows }: { loading: boolean; error: unknown; rows: AFSDatabase[] }) {
  if (loading) return <CenteredLoader />;
  if (error) return <PanelError error={error} />;
  return (
    <SectionCard $span={12}>
      <SectionHeader>
        <SectionTitle title="Databases" body="Every configured Cloud database, including owner and health state." />
      </SectionHeader>
      <DataTable empty="No databases matched." rowCount={rows.length}>
        <thead>
          <tr>
            <th>Database</th>
            <th>Owner</th>
            <th>Workspaces</th>
            <th>Agents</th>
            <th>Endpoint</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.id}>
              <td><PrimaryCell title={row.id}>{row.name}<MetaLine>{row.id}</MetaLine></PrimaryCell></td>
              <td>{ownerLabel(row.ownerSubject, row.ownerLabel)}</td>
              <td>{row.workspaceCount}</td>
              <td>{row.activeSessionCount}</td>
              <td>{row.redisAddr}</td>
              <td>{row.connectionError ? `Unavailable: ${row.connectionError}` : "Healthy"}</td>
            </tr>
          ))}
        </tbody>
      </DataTable>
    </SectionCard>
  );
}

function WorkspacesPanel({ loading, error, rows }: { loading: boolean; error: unknown; rows: AFSWorkspaceSummary[] }) {
  if (loading) return <CenteredLoader />;
  if (error) return <PanelError error={error} />;
  return (
    <SectionCard $span={12}>
      <SectionHeader>
        <SectionTitle title="Workspaces" body="All workspace summaries across Cloud databases." />
      </SectionHeader>
      <DataTable empty="No workspaces matched." rowCount={rows.length}>
        <thead>
          <tr>
            <th>Workspace</th>
            <th>Owner</th>
            <th>Database</th>
            <th>Size</th>
            <th>Checkpoints</th>
            <th>Updated</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={`${row.databaseId}:${row.id}`}>
              <td><PrimaryCell title={row.id}>{row.name}<MetaLine>{row.id}</MetaLine></PrimaryCell></td>
              <td>{ownerLabel(row.ownerSubject, row.ownerLabel)}</td>
              <td><PrimaryCell>{row.databaseName}<MetaLine>{row.databaseId}</MetaLine></PrimaryCell></td>
              <td>{formatBytes(row.totalBytes)}<MetaLine>{row.fileCount} files</MetaLine></td>
              <td>{row.checkpointCount}</td>
              <td>{formatDate(row.updatedAt)}</td>
            </tr>
          ))}
        </tbody>
      </DataTable>
    </SectionCard>
  );
}

function AgentsPanel({ loading, error, rows }: { loading: boolean; error: unknown; rows: AFSAgentSession[] }) {
  if (loading) return <CenteredLoader />;
  if (error) return <PanelError error={error} />;
  return (
    <SectionCard $span={12}>
      <SectionHeader>
        <SectionTitle title="Agents" body="Active and stale sessions across all Cloud workspaces." />
      </SectionHeader>
      <DataTable empty="No agents matched." rowCount={rows.length}>
        <thead>
          <tr>
            <th>Agent</th>
            <th>Owner</th>
            <th>Workspace</th>
            <th>Database</th>
            <th>Host</th>
            <th>State</th>
            <th>Last seen</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.sessionId}>
              <td>
                <PrimaryCell title={row.sessionId}>
                  {agentSessionTitle(row)}
                  <MetaLine>{agentSessionMeta(row)}</MetaLine>
                </PrimaryCell>
              </td>
              <td>{ownerLabel(row.ownerSubject, row.ownerLabel)}</td>
              <td><PrimaryCell>{row.workspaceName}<MetaLine>{row.workspaceId}</MetaLine></PrimaryCell></td>
              <td><PrimaryCell>{row.databaseName || row.databaseId}<MetaLine>{row.databaseId}</MetaLine></PrimaryCell></td>
              <td><PrimaryCell>{row.hostname || "-"}<MetaLine>{row.localPath}</MetaLine></PrimaryCell></td>
              <td>{row.state}</td>
              <td>{formatDate(row.lastSeenAt)}</td>
            </tr>
          ))}
        </tbody>
      </DataTable>
    </SectionCard>
  );
}

function DataTable(props: { children: React.ReactNode; empty: string; rowCount: number }) {
  return (
    <>
      <TableViewport>
        <AdminTable>{props.children}</AdminTable>
      </TableViewport>
      {props.rowCount === 0 ? <TableEmpty>{props.empty}</TableEmpty> : null}
    </>
  );
}

function CenteredLoader() {
  return (
    <SectionCard $span={12}>
      <CenteredState>
        <Loader data-testid="loader--admin" />
      </CenteredState>
    </SectionCard>
  );
}

function PanelError({ error }: { error: unknown }) {
  return (
    <NoticeCard $tone="danger" role="alert">
      <NoticeTitle>Unable to load admin data</NoticeTitle>
      <NoticeBody>{error instanceof Error ? error.message : "The admin request failed."}</NoticeBody>
    </NoticeCard>
  );
}

function filterRows<T>(rows: T[], search: string, values: (row: T) => Array<string | number | boolean | undefined>) {
  const query = search.trim().toLowerCase();
  if (query === "") return rows;
  return rows.filter((row) =>
    values(row).some((value) => String(value ?? "").toLowerCase().includes(query)),
  );
}

function agentSessionTitle(row: AFSAgentSession) {
  return row.sessionName?.trim() || row.agentName?.trim() || row.label?.trim() || row.agentId || row.sessionId;
}

function agentSessionMeta(row: AFSAgentSession) {
  const values = [row.agentName?.trim(), row.agentId?.trim(), row.sessionId].filter(Boolean);
  return values.join(" · ");
}

function ownerLabel(subject?: string, label?: string) {
  if (label?.trim()) {
    return (
      <PrimaryCell title={subject}>
        {label}
        {subject ? <MetaLine>{subject}</MetaLine> : null}
      </PrimaryCell>
    );
  }
  return subject?.trim() || "Shared / system";
}

function formatDate(value?: string) {
  return value?.trim() ? shortDateTime(value) : "-";
}

const AdminHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: center;

  @media (max-width: 760px) {
    align-items: stretch;
    flex-direction: column;
  }
`;

const SearchInput = styled(TextInput)`
  width: min(360px, 100%);
`;

const TableViewport = styled.div`
  width: 100%;
  overflow-x: auto;
  border: 1px solid var(--afs-line);
  border-radius: 14px;
`;

const AdminTable = styled.table`
  width: 100%;
  min-width: 880px;
  border-collapse: collapse;
  color: var(--afs-ink);
  font-size: 13px;

  th,
  td {
    padding: 13px 14px;
    border-bottom: 1px solid var(--afs-line);
    text-align: left;
    vertical-align: top;
  }

  th {
    color: var(--afs-muted);
    font-size: 11px;
    font-weight: 800;
    text-transform: uppercase;
  }

  tr:last-child td {
    border-bottom: 0;
  }

  [data-skin="situation-room"] && {
    font-family: var(--afs-font-mono);
  }
`;

const PrimaryCell = styled.div`
  display: grid;
  gap: 4px;
  min-width: 0;
`;

const MetaLine = styled.span`
  color: var(--afs-muted);
  font-size: 12px;
  line-height: 1.35;
  overflow-wrap: anywhere;
`;

const TableEmpty = styled(EmptyState)`
  margin-top: 12px;
  color: var(--afs-muted);
  font-size: 13px;
`;

const CenteredState = styled.div`
  min-height: 220px;
  display: grid;
  place-items: center;
`;
