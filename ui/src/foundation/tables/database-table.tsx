import { Typography } from "@redislabsdev/redis-ui-components";
import { CheckThinIcon } from "@redislabsdev/redis-ui-icons/monochrome";
import { Table } from "@redislabsdev/redis-ui-table";
import type { ColumnDef } from "@redislabsdev/redis-ui-table";
import { useMemo } from "react";
import styled from "styled-components";
import type { AFSDatabaseScopeRecord } from "../database-scope";
import * as S from "./workspace-table.styles";

type Props = {
  rows: AFSDatabaseScopeRecord[];
  loading?: boolean;
  error?: boolean;
  errorMessage?: string;
  selectedDatabaseId: string | null;
  onSelectDatabase: (databaseId: string) => void;
  onEditDatabase: (databaseId: string) => void;
  onDeleteDatabase: (databaseId: string) => void;
};

function selectedCellProps(database: AFSDatabaseScopeRecord, selectedDatabaseId: string | null) {
  if (database.id !== selectedDatabaseId) {
    return {};
  }

  return {
    style: {
      background: "rgba(71, 191, 255, 0.08)",
    },
  };
}

export function DatabaseTable({
  rows,
  loading = false,
  error = false,
  errorMessage = "Unable to load databases. Please retry.",
  selectedDatabaseId,
  onSelectDatabase,
  onEditDatabase,
  onDeleteDatabase,
}: Props) {
  const columns = useMemo(
    () =>
      [
        {
          id: "selected",
          header: "",
          size: 20,
          minSize: 20,
          maxSize: 20,
          enableSorting: false,
          getCellProps: (database) => selectedCellProps(database, selectedDatabaseId),
          cell: ({ row }) => (
            <SelectionCell>
              {row.original.id === selectedDatabaseId ? <CheckThinIcon size="S" /> : null}
            </SelectionCell>
          ),
        },
        {
          accessorKey: "displayName",
          header: "Name",
          size: 120,
          enableSorting: false,
          getCellProps: (database) => selectedCellProps(database, selectedDatabaseId),
          cell: ({ row }) => (
            <S.Stack>
              <PlainText>{row.original.displayName || row.original.databaseName}</PlainText>
              {row.original.description ? (
                <Typography.Body color="secondary" component="span">
                  {row.original.description}
                </Typography.Body>
              ) : null}
            </S.Stack>
          ),
        },
        {
          accessorKey: "endpointLabel",
          header: "Endpoint",
          size: 120,
          enableSorting: false,
          getCellProps: (database) => selectedCellProps(database, selectedDatabaseId),
          cell: ({ row }) => row.original.endpointLabel || "Not configured",
        },
        {
          accessorKey: "workspaceCount",
          header: "Workspaces",
          size: 60,
          enableSorting: false,
          getCellProps: (database) => selectedCellProps(database, selectedDatabaseId),
        },
        {
          id: "actions",
          header: "",
          size: 60,
          enableSorting: false,
          getCellProps: (database) => selectedCellProps(database, selectedDatabaseId),
          cell: ({ row }) => (
            <S.ActionRow>
              <S.TextActionButton
                type="button"
                onClick={(event) => {
                  event.stopPropagation();
                  onEditDatabase(row.original.id);
                }}
              >
                Edit
              </S.TextActionButton>
              <S.DangerActionButton
                type="button"
                onClick={(event) => {
                  event.stopPropagation();
                  onDeleteDatabase(row.original.id);
                }}
              >
                Remove
              </S.DangerActionButton>
            </S.ActionRow>
          ),
        },
      ] as ColumnDef<AFSDatabaseScopeRecord>[],
    [onDeleteDatabase, onEditDatabase, onSelectDatabase, selectedDatabaseId],
  );

  return (
    <S.TableCard>
      {loading ? <S.EmptyState>Loading databases...</S.EmptyState> : null}
      {error ? <S.EmptyState role="alert">{errorMessage}</S.EmptyState> : null}
      {!loading && !error && rows.length === 0 ? (
        <S.EmptyState>No databases have been configured yet.</S.EmptyState>
      ) : null}

      {!loading && !error && rows.length > 0 ? (
        <S.TableViewport>
          <Table
            columns={columns}
            data={rows}
            getRowId={(row) => row.id}
            stripedRows
            onRowClick={(rowData) => onSelectDatabase(rowData.id)}
          />
        </S.TableViewport>
      ) : null}
    </S.TableCard>
  );
}

const SelectionCell = styled.div`
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  min-width: 16px;
  color: var(--afs-ink);
`;

const PlainText = styled.span`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 400;
`;
