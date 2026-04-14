import { Typography } from "@redislabsdev/redis-ui-components";
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
  onEditDatabase: (databaseId: string) => void;
  onDeleteDatabase: (databaseId: string) => void;
};

export function DatabaseTable({
  rows,
  loading = false,
  error = false,
  errorMessage = "Unable to load databases. Please retry.",
  onEditDatabase,
  onDeleteDatabase,
}: Props) {
  const columns = useMemo(
    () =>
      [
        {
          accessorKey: "displayName",
          header: "Name",
          size: 120,
          enableSorting: false,
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
          cell: ({ row }) => row.original.endpointLabel || "Not configured",
        },
        {
          accessorKey: "workspaceCount",
          header: "Workspaces",
          size: 60,
          enableSorting: false,
        },
        {
          id: "actions",
          header: "",
          size: 60,
          enableSorting: false,
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
    [onDeleteDatabase, onEditDatabase],
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
          />
        </S.TableViewport>
      ) : null}
    </S.TableCard>
  );
}

const PlainText = styled.span`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 400;
`;
