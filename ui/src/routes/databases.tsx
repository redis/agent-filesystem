import { createFileRoute } from "@tanstack/react-router";
import { Button } from "@redis-ui/components";
import { useMemo, useState } from "react";
import styled from "styled-components";
import {
  DialogActions,
  DialogBody,
  DialogCard,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
  PageStack,
} from "../components/afs-kit";
import { AddDatabaseDialog } from "../components/add-database-dialog";
import { RefreshCwIcon } from "../components/lucide-icons";
import { databasesQueryOptions } from "../foundation/hooks/use-afs";
import { queryClient } from "../foundation/query-client";
import { DatabaseTable, DatabaseSummaryStrip } from "../foundation/tables/database-table";
import { useDatabaseScope } from "../foundation/database-scope";
import { useDrawerCommands } from "../foundation/drawer-context";
import type { CommandsDrawerConfig } from "../foundation/drawer-context";

const DATABASE_COMMANDS: CommandsDrawerConfig = {
  title: "Manage databases",
  subline: "Switch the active Redis database for new workspaces.",
  sections: [
    {
      title: "List databases",
      description: "Every database your control plane knows about.",
      command: "afs database list",
    },
    {
      title: "Switch active database",
      description: "Pick which database new workspaces land in.",
      command: "afs database use my-database",
    },
    {
      title: "Reset to default",
      description: "Clear local override; fall back to the control-plane default.",
      command: "afs database use auto",
    },
  ],
};

export const Route = createFileRoute("/databases")({
  loader: () =>
    queryClient.ensureQueryData({ ...databasesQueryOptions(), revalidateIfStale: true }),
  component: DatabasesPage,
});

type DialogMode = "create" | "edit" | null;

function DatabasesPage() {
  useDrawerCommands(DATABASE_COMMANDS);
  const { databases, saveDatabase, setDefaultDatabase, removeDatabase, reconcileCatalog, isLoading, errorMessage } = useDatabaseScope();
  const [dialogMode, setDialogMode] = useState<DialogMode>(null);
  const [editingDatabaseId, setEditingDatabaseId] = useState<string | null>(null);
  const [pageMessage, setPageMessage] = useState<string | null>(null);
  const [isReconciling, setIsReconciling] = useState(false);
  const [pendingDefaultId, setPendingDefaultId] = useState<string | null>(null);
  const [isApplyingDefault, setIsApplyingDefault] = useState(false);

  const pendingDefaultDatabase = useMemo(
    () => databases.find((database) => database.id === pendingDefaultId) ?? null,
    [databases, pendingDefaultId],
  );

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

  function requestMakeDefaultDatabase(databaseId: string) {
    setPendingDefaultId(databaseId);
  }

  function cancelMakeDefaultDatabase() {
    if (isApplyingDefault) return;
    setPendingDefaultId(null);
  }

  async function confirmMakeDefaultDatabase() {
    if (pendingDefaultId == null) return;
    const databaseId = pendingDefaultId;
    try {
      setPageMessage(null);
      setIsApplyingDefault(true);
      await setDefaultDatabase(databaseId);
      const database = databases.find((item) => item.id === databaseId);
      setPageMessage(`Default database set to ${database?.displayName || database?.databaseName || databaseId}.`);
      setPendingDefaultId(null);
    } catch (error) {
      setPageMessage(error instanceof Error ? error.message : "Unable to update the default database.");
    } finally {
      setIsApplyingDefault(false);
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

      <DatabaseSummaryStrip rows={databases} />

      <DatabaseTable
        rows={databases}
        loading={isLoading}
        error={errorMessage != null}
        errorMessage={errorMessage ?? undefined}
        onEditDatabase={openEditDialog}
        onSetDefaultDatabase={requestMakeDefaultDatabase}
        onRemoveDatabase={deleteDatabase}
        toolbarAction={
          <div style={{ display: "flex", gap: 8 }}>
            <RefreshButton
              size="medium"
              onClick={repairCatalog}
              disabled={isReconciling}
              aria-label={isReconciling ? "Refreshing databases" : "Refresh databases"}
              aria-busy={isReconciling}
              title={isReconciling ? "Refreshing..." : "Refresh databases"}
            >
              <RefreshCwIcon customSize={16} />
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

      {pendingDefaultDatabase ? (
        <DialogOverlay
          role="dialog"
          aria-modal="true"
          aria-labelledby="set-default-dialog-title"
          onClick={cancelMakeDefaultDatabase}
        >
          <ConfirmCard onClick={(event) => event.stopPropagation()}>
            <DialogHeader>
              <DialogTitle id="set-default-dialog-title">
                Make &ldquo;{pendingDefaultDatabase.displayName || pendingDefaultDatabase.databaseName}&rdquo; the default database?
              </DialogTitle>
            </DialogHeader>
            <DialogBody>
              The default database is the one used automatically when you create a new
              workspace, so you don&rsquo;t have to pick a database every time. It also
              becomes the suggested target for imports and quick actions across the app.
              Existing workspaces stay attached to whichever database they were created in
              &mdash; only future workspaces are affected.
            </DialogBody>
            <DialogActions style={{ justifyContent: "flex-end", marginTop: 20 }}>
              <Button
                variant="secondary-fill"
                size="medium"
                onClick={cancelMakeDefaultDatabase}
                disabled={isApplyingDefault}
              >
                Cancel
              </Button>
              <Button
                size="medium"
                onClick={confirmMakeDefaultDatabase}
                disabled={isApplyingDefault}
              >
                {isApplyingDefault ? "Setting..." : "Make default"}
              </Button>
            </DialogActions>
          </ConfirmCard>
        </DialogOverlay>
      ) : null}
    </PageStack>
  );
}

const ConfirmCard = styled(DialogCard)`
  width: min(480px, 100%);
`;

const RefreshButton = styled(Button)`
  && {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 38px;
    min-width: 38px;
    padding-inline: 0;
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
