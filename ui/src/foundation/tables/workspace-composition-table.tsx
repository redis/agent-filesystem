import { Typography } from "@redis-ui/components";
import { Table } from "@redis-ui/table";
import type { ColumnDef, SortingState } from "@redis-ui/table";
import { useNavigate } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import type { ReactNode } from "react";
import styled from "styled-components";
import { BotIcon } from "../../components/lucide-icons";
import { compareValues } from "../sort-compare";
import { shortDateTime } from "../time-format";
import type { AFSWorkspaceCompositionSummary } from "../types/afs";
import { StatusNameCell, StatusNameLine } from "./status-name-cell";
import * as S from "./workspace-table.styles";

type WorkspaceCompositionSortField =
  | "name"
  | "mountCount"
  | "connectedAgentCount"
  | "updatedAt";

type Props = {
  rows: AFSWorkspaceCompositionSummary[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  toolbarAction?: ReactNode;
  onOpenWorkspace: (workspace: AFSWorkspaceCompositionSummary) => void;
};

function mountedVolumeLabel(
  volume: AFSWorkspaceCompositionSummary["mountedVolumes"][number],
) {
  return `${volume.name || volume.id} at ${volume.mountPath}${
    volume.readonly ? " ro" : ""
  }`;
}

function sortValue(
  workspace: AFSWorkspaceCompositionSummary,
  field: WorkspaceCompositionSortField,
) {
  switch (field) {
    case "connectedAgentCount":
      return workspace.connectedAgentCount;
    default:
      return workspace[field];
  }
}

export function WorkspaceCompositionTable({
  rows,
  loading = false,
  error = false,
  errorMessage = "Unable to load workspaces. Please retry.",
  toolbarAction,
  onOpenWorkspace,
}: Props) {
  const navigate = useNavigate();
  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] =
    useState<WorkspaceCompositionSortField>("updatedAt");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("desc");

  const filteredRows = useMemo(() => {
    const query = search.trim().toLowerCase();
    const baseRows =
      query === ""
        ? rows
        : rows.filter((workspace) =>
            [
              workspace.name,
              workspace.description ?? "",
              workspace.id,
              ...workspace.mountedVolumes.map(mountedVolumeLabel),
            ].some((value) => value.toLowerCase().includes(query)),
          );

    return [...baseRows].sort((left, right) =>
      compareValues(sortValue(left, sortBy), sortValue(right, sortBy), sortDirection),
    );
  }, [rows, search, sortBy, sortDirection]);

  const sorting = useMemo<SortingState>(
    () => [{ id: sortBy, desc: sortDirection === "desc" }],
    [sortBy, sortDirection],
  );
  const isFiltering = search.trim() !== "";

  const columns = useMemo(
    () =>
      [
        {
          accessorKey: "name",
          header: "Name",
          size: 280,
          enableSorting: true,
          cell: ({ row }) => {
            const workspace = row.original;
            return (
              <StatusNameCell
                active={workspace.connectedAgentCount > 0}
                icon={<BotIcon customSize={16} />}
                statusLabel={
                  workspace.connectedAgentCount > 0
                    ? "Connected agents"
                    : "No connected agents"
                }
              >
                <StatusNameLine>
                  <WorkspaceNameButton
                    type="button"
                    onClick={(event) => {
                      event.stopPropagation();
                      onOpenWorkspace(workspace);
                    }}
                  >
                    {workspace.name}
                  </WorkspaceNameButton>
                </StatusNameLine>
                <S.StatusCaption title={workspace.description ?? workspace.id}>
                  {workspace.description?.trim() || workspace.id}
                </S.StatusCaption>
              </StatusNameCell>
            );
          },
        },
        {
          accessorKey: "mountCount",
          header: "Volumes",
          size: 260,
          enableSorting: true,
          cell: ({ row }) => {
            const volumes = row.original.mountedVolumes;
            if (volumes.length === 0) {
              return (
                <Typography.Body component="span" color="secondary">
                  No volumes
                </Typography.Body>
              );
            }
            return (
              <VolumeNameList>
                {volumes.map((volume) => {
                  const label = volume.name?.trim() || volume.id;
                  return (
                    <VolumeNameButton
                      key={volume.id}
                      type="button"
                      title={`Open volume ${label}${volume.readonly ? " (read-only)" : ""}`}
                      onClick={(event) => {
                        event.stopPropagation();
                        void navigate({
                          to: "/volumes/$volumeId",
                          params: { volumeId: volume.id },
                        });
                      }}
                    >
                      {label}
                      {volume.readonly ? <ReadOnlyTag>ro</ReadOnlyTag> : null}
                    </VolumeNameButton>
                  );
                })}
              </VolumeNameList>
            );
          },
        },
        {
          accessorKey: "connectedAgentCount",
          header: "Agents",
          size: 90,
          enableSorting: true,
          cell: ({ row }) => (
            <S.CountCell>
              <Typography.Body component="span">
                {row.original.connectedAgentCount}
              </Typography.Body>
            </S.CountCell>
          ),
        },
        {
          accessorKey: "updatedAt",
          header: "Last updated",
          size: 160,
          enableSorting: true,
          cell: ({ row }) => (
            <S.Stack>
              <Typography.Body component="span">
                {shortDateTime(row.original.updatedAt)}
              </Typography.Body>
              {row.original.lastActivityAt ? (
                <S.StatusCaption>
                  Activity {shortDateTime(row.original.lastActivityAt)}
                </S.StatusCaption>
              ) : null}
            </S.Stack>
          ),
        },
      ] as ColumnDef<AFSWorkspaceCompositionSummary>[],
    [onOpenWorkspace, navigate],
  );

  return (
    <S.TableBlock>
      <S.HeadingWrap style={{ padding: 0 }}>
        <S.SearchInput
          value={search}
          onChange={setSearch}
          placeholder="Search Agent Workspaces, volumes, paths..."
        />
        {toolbarAction}
      </S.HeadingWrap>

      {loading ? <S.EmptyState>Loading Agent Workspaces...</S.EmptyState> : null}
      {error ? <S.EmptyState role="alert">{errorMessage}</S.EmptyState> : null}
      {!loading && !error && filteredRows.length === 0 ? (
        <S.EmptyState>
          {isFiltering
            ? "No Agent Workspaces match the current filter."
            : "No Agent Workspaces yet. Add one to start composing volumes for agents."}
        </S.EmptyState>
      ) : null}

      {!loading && !error && filteredRows.length > 0 ? (
        <S.TableCard>
          <S.DenseTableViewport>
            <Table
              columns={columns}
              data={filteredRows}
              getRowId={(row) => row.id}
              sorting={sorting}
              manualSorting
              onSortingChange={(nextState) => {
                if (nextState.length === 0) {
                  setSortBy("updatedAt");
                  setSortDirection("desc");
                  return;
                }

                const next = nextState[0];
                setSortBy(next.id as WorkspaceCompositionSortField);
                setSortDirection(next.desc ? "desc" : "asc");
              }}
              enableSorting
              stripedRows
              onRowClick={(rowData) => onOpenWorkspace(rowData)}
            />
          </S.DenseTableViewport>
        </S.TableCard>
      ) : null}
    </S.TableBlock>
  );
}

const WorkspaceNameButton = styled(S.WorkspaceNameButton)`
  && {
    font-size: 15px;
    font-weight: 700;
  }
`;

const VolumeNameList = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: flex-start;
  min-width: 0;
`;

const VolumeNameButton = styled.button`
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  padding: 2px 0;
  border: 0;
  background: transparent;
  color: var(--afs-accent, #2563eb);
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  text-align: left;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  &:hover,
  &:focus-visible {
    text-decoration: underline;
    outline: none;
  }
`;

const ReadOnlyTag = styled.span`
  flex-shrink: 0;
  padding: 1px 6px;
  border-radius: 6px;
  border: 1px solid var(--afs-line);
  background: var(--afs-panel);
  color: var(--afs-muted);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
`;
