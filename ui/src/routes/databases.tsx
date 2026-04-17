import { createFileRoute } from "@tanstack/react-router";
import { Button } from "@redis-ui/components";
import { useMemo, useState } from "react";
import styled from "styled-components";
import {
  PageStack,
} from "../components/afs-kit";
import { AddDatabaseDialog } from "../components/add-database-dialog";
import { DatabaseTable } from "../foundation/tables/database-table";
import { useDatabaseScope } from "../foundation/database-scope";

export const Route = createFileRoute("/databases")({
  component: DatabasesPage,
});

type DialogMode = "create" | "edit" | null;

function DatabasesPage() {
  const { databases, saveDatabase, setDefaultDatabase, removeDatabase, reconcileCatalog, isLoading, errorMessage } = useDatabaseScope();
  const [dialogMode, setDialogMode] = useState<DialogMode>(null);
  const [editingDatabaseId, setEditingDatabaseId] = useState<string | null>(null);
  const [pageMessage, setPageMessage] = useState<string | null>(null);
  const [isReconciling, setIsReconciling] = useState(false);

  const editingDatabase = useMemo(
    () => databases.find((database) => database.id === editingDatabaseId) ?? null,
    [databases, editingDatabaseId],
  );

  function closeDialog() {
    setDialogMode(null);
    setEditingDatabaseId(null);
  }

  function openCreateDialog() {
    setDialogMode("create");
    setEditingDatabaseId(null);
  }

  function openEditDialog(databaseId: string) {
    setDialogMode("edit");
    setEditingDatabaseId(databaseId);
  }

  function deleteDatabase(databaseId: string) {
    const database = databases.find((item) => item.id === databaseId);
    const confirmed = window.confirm(
      `Remove database "${database?.displayName || database?.databaseName || databaseId}" from the control plane configuration list?`,
    );
    if (!confirmed) return;

    void removeDatabase(databaseId).then(() => {
      if (editingDatabaseId === databaseId) closeDialog();
    });
  }

  async function makeDefaultDatabase(databaseId: string) {
    try {
      setPageMessage(null);
      await setDefaultDatabase(databaseId);
      const database = databases.find((item) => item.id === databaseId);
      setPageMessage(`Default database set to ${database?.displayName || database?.databaseName || databaseId}.`);
    } catch (error) {
      setPageMessage(error instanceof Error ? error.message : "Unable to update the default database.");
    }
  }

  async function repairCatalog() {
    try {
      setPageMessage(null);
      setIsReconciling(true);
      await reconcileCatalog();
      setPageMessage("Catalog repair completed.");
    } catch (error) {
      setPageMessage(error instanceof Error ? error.message : "Unable to repair the catalog.");
    } finally {
      setIsReconciling(false);
    }
  }

  return (
    <PageStack>
      {pageMessage ? <StatusMessage>{pageMessage}</StatusMessage> : null}

      <DatabaseTable
        rows={databases}
        loading={isLoading}
        error={errorMessage != null}
        errorMessage={errorMessage ?? undefined}
        onEditDatabase={openEditDialog}
        onSetDefaultDatabase={makeDefaultDatabase}
        onRemoveDatabase={deleteDatabase}
        toolbarAction={
          <div style={{ display: "flex", gap: 8 }}>
            <RefreshButton size="medium" onClick={repairCatalog} disabled={isReconciling}>
              {isReconciling ? "Refreshing..." : "Refresh"}
            </RefreshButton>
            <Button size="medium" onClick={openCreateDialog}>
              Add database
            </Button>
          </div>
        }
      />

      <AddDatabaseDialog
        isOpen={dialogMode != null}
        onClose={closeDialog}
        saveDatabase={saveDatabase}
        mode={dialogMode === "edit" ? "edit" : "create"}
        editingDatabase={editingDatabase}
        onDelete={deleteDatabase}
      />
    </PageStack>
  );
}

const RefreshButton = styled(Button)`
  && {
    white-space: nowrap;
    background: transparent;
    border-color: var(--afs-line-strong);
    color: var(--afs-ink-soft);
  }

  &&:hover:not(:disabled) {
    background: var(--afs-panel);
    border-color: var(--afs-muted);
  }
`;

const StatusMessage = styled.div`
  color: var(--afs-muted-ink);
  font-size: 14px;
`;
