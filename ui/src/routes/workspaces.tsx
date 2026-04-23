import { createFileRoute, Outlet, useLocation, useNavigate, useRouter } from "@tanstack/react-router";
import { Button, Loader, Select } from "@redis-ui/components";
import { useEffect, useState } from "react";
import styled from "styled-components";
import {
  DialogActions,
  DialogBody,
  DialogCard,
  DialogCloseButton,
  DialogError,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
  Field,
  FormGrid,
  PageStack,
  TextInput,
} from "../components/afs-kit";
import { GettingStartedOnboardingDialog } from "../components/getting-started-onboarding-dialog";
import { FreeTierLimitDialog } from "../components/free-tier-limit-dialog";
import { CreateWorkspaceDialog } from "../features/workspaces/CreateWorkspaceDialog";

const FREE_TIER_WORKSPACE_LIMIT = 3;
import {
  agentsQueryOptions,
  useDeleteWorkspaceMutation,
  useUpdateWorkspaceMutation,
  useImportLocalMutation,
  useWorkspace,
  workspaceSummariesQueryOptions,
} from "../foundation/hooks/use-afs";
import {
  type AFSDatabaseScopeRecord,
  useDatabaseScope,
  useScopedAgents,
  useScopedWorkspaceSummaries,
} from "../foundation/database-scope";
import { queryClient } from "../foundation/query-client";
import { WorkspaceTable } from "../foundation/tables/workspace-table";
import type { AFSWorkspaceSummary } from "../foundation/types/afs";

export const Route = createFileRoute("/workspaces")({
  loader: async () => {
    await Promise.all([
      queryClient.ensureQueryData({
        ...workspaceSummariesQueryOptions(null),
        revalidateIfStale: true,
      }),
      queryClient.ensureQueryData({ ...agentsQueryOptions(null), revalidateIfStale: true }),
    ]);
  },
  component: WorkspacesPage,
});

type WorkspaceFormState = {
  name: string;
  description: string;
  databaseId: string;
  cloudAccount: string;
  databaseName: string;
  region: string;
};

type DialogMode = "create" | "edit" | null;

type ImportFormState = {
  name: string;
  path: string;
  description: string;
  databaseId: string;
};

function ImportDialog({
  open,
  onClose,
  databases,
}: {
  open: boolean;
  onClose: () => void;
  databases: AFSDatabaseScopeRecord[];
}) {
  const navigate = useNavigate();
  const importLocal = useImportLocalMutation();
  const [form, setForm] = useState<ImportFormState>({
    name: "",
    path: "",
    description: "",
    databaseId: preferredDatabase(databases)?.id ?? "",
  });
  const [error, setError] = useState<string | null>(null);

  if (!open) return null;

  function update<K extends keyof ImportFormState>(key: K, value: ImportFormState[K]) {
    setError(null);
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  return (
    <DialogOverlay
      style={{ zIndex: 1100 }}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <DialogCard>
        <DialogHeader>
          <div>
            <DialogTitle>Import from local directory</DialogTitle>
            <DialogBody>
              Create a workspace from a directory on this machine. Files are scanned and
              stored in Redis.
            </DialogBody>
          </div>
          <DialogCloseButton type="button" aria-label="Close" onClick={onClose}>
            &times;
          </DialogCloseButton>
        </DialogHeader>
        <FormGrid
          onSubmit={(event) => {
            event.preventDefault();
            if (form.name.trim() === "" || form.path.trim() === "") {
              setError("Name and path are required.");
              return;
            }
            importLocal.mutate(
              {
                databaseId: form.databaseId,
                name: form.name,
                path: form.path,
                description: form.description,
              },
              {
                onSuccess: (result) => {
                  onClose();
                  void navigate({
                    to: "/workspaces/$workspaceId",
                    params: { workspaceId: result.workspaceId },
                    search: { databaseId: result.databaseId, tab: "browse" },
                  });
                },
                onError: (err) => {
                  setError(err instanceof Error ? err.message : "Import failed.");
                },
              },
            );
          }}
        >
          <Field>
            Workspace name
            <TextInput
              autoFocus
              value={form.name}
              onChange={(e) => update("name", e.target.value)}
              placeholder="my-project"
            />
          </Field>

          <Field>
            Local directory path
            <TextInput
              value={form.path}
              onChange={(e) => update("path", e.target.value)}
              placeholder="~/code/my-project"
            />
          </Field>

          {databases.length > 1 && (
            <Field>
              Database
              <Select
                options={databases.map((db) => ({
                  value: db.id,
                  label: db.displayName || db.databaseName,
                }))}
                value={form.databaseId}
                onChange={(next) => update("databaseId", next as string)}
              />
            </Field>
          )}

          <Field>
            Description
            <TextInput
              value={form.description}
              onChange={(e) => update("description", e.target.value)}
              placeholder="Optional description"
            />
          </Field>

          {error && <DialogError role="alert">{error}</DialogError>}

          <DialogActions>
            <Button size="medium" type="button" variant="secondary-fill" onClick={onClose}>
              Cancel
            </Button>
            <Button size="medium" type="submit" disabled={importLocal.isPending}>
              {importLocal.isPending ? "Importing..." : "Import"}
            </Button>
          </DialogActions>
        </FormGrid>
      </DialogCard>
    </DialogOverlay>
  );
}

function createWorkspaceDefaults(database?: AFSDatabaseScopeRecord | null) {
  return {
    databaseId: database?.id ?? "",
    databaseName: database?.databaseName ?? "",
    cloudAccount: "Direct Redis",
    region: "",
  };
}

function workspaceEligibleDatabases(databases: AFSDatabaseScopeRecord[]) {
  return databases.filter((database) => database.canCreateWorkspaces);
}

function preferredDatabase(databases: AFSDatabaseScopeRecord[]) {
  const eligible = workspaceEligibleDatabases(databases);
  return eligible.find((database) => database.isDefault) ?? eligible[0] ?? null;
}

function createInitialFormState(database?: AFSDatabaseScopeRecord | null): WorkspaceFormState {
  const defaults = createWorkspaceDefaults(database);
  return {
    name: "",
    description: "",
    databaseId: defaults.databaseId,
    cloudAccount: defaults.cloudAccount,
    databaseName: defaults.databaseName,
    region: defaults.region,
  };
}

function workspaceRowKey(databaseId: string | undefined, workspaceId: string) {
  return `${databaseId ?? ""}:${workspaceId}`;
}

function WorkspacesPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const router = useRouter();
  const workspacesQuery = useScopedWorkspaceSummaries();
  const agentsQuery = useScopedAgents();
  const { databases } = useDatabaseScope();
  const eligibleDatabases = workspaceEligibleDatabases(databases);
  const updateWorkspace = useUpdateWorkspaceMutation();
  const deleteWorkspace = useDeleteWorkspaceMutation();

  const [importOpen, setImportOpen] = useState(false);
  const [dialogMode, setDialogMode] = useState<DialogMode>(null);
  const [editingWorkspaceId, setEditingWorkspaceId] = useState<string | null>(null);
  const [editingWorkspaceDatabaseId, setEditingWorkspaceDatabaseId] = useState<string | null>(null);
  const [onboardingWorkspace, setOnboardingWorkspace] = useState<AFSWorkspaceSummary | null>(null);
  const [workspaceToDelete, setWorkspaceToDelete] = useState<AFSWorkspaceSummary | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [freeTierDialogOpen, setFreeTierDialogOpen] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [form, setForm] = useState<WorkspaceFormState>(() =>
    createInitialFormState(preferredDatabase(databases)),
  );

  const editingWorkspaceQuery = useWorkspace(
    editingWorkspaceDatabaseId,
    editingWorkspaceId ?? "",
    dialogMode === "edit" && editingWorkspaceId != null,
  );

  useEffect(() => {
    const workspace = editingWorkspaceQuery.data;
    if (workspace == null || dialogMode !== "edit") {
      return;
    }

    setForm({
      name: workspace.name,
      description: workspace.description,
      databaseId: workspace.databaseId,
      cloudAccount: workspace.cloudAccount,
      databaseName: workspace.databaseName,
      region: workspace.region,
    });
  }, [dialogMode, editingWorkspaceQuery.data]);

  const workspaces = workspacesQuery.data;
  const connectedAgentsByWorkspace = agentsQuery.data.reduce<Record<string, number>>((counts, session) => {
    const key = workspaceRowKey(session.databaseId, session.workspaceId);
    counts[key] = (counts[key] ?? 0) + 1;
    return counts;
  }, {});
  const starterWorkspace = workspaces.length === 1 && isGettingStartedWorkspace(workspaces[0].name)
    ? workspaces[0]
    : null;
  const showStarterConnectPanel = starterWorkspace != null &&
    (connectedAgentsByWorkspace[workspaceRowKey(starterWorkspace.databaseId, starterWorkspace.id)] ?? 0) === 0;

  // Free-tier quota: the starter database caps each user at 3 workspaces.
  const onboardingDb = databases.find((db) => db.purpose === "onboarding") ?? null;
  const freeTierUsed = onboardingDb
    ? workspaces.filter((ws) => ws.databaseId === onboardingDb.id).length
    : 0;
  const onlyHasOnboardingDb = databases.length === 1 && onboardingDb != null;
  const freeTierExhausted = onlyHasOnboardingDb && freeTierUsed >= FREE_TIER_WORKSPACE_LIMIT;

  if (workspacesQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const isEditing = dialogMode === "edit" && editingWorkspaceId != null;
  const formBusy =
    updateWorkspace.isPending ||
    (isEditing && editingWorkspaceQuery.isLoading);

  function closeDialog() {
    setDialogMode(null);
    setEditingWorkspaceId(null);
    setEditingWorkspaceDatabaseId(null);
    setFormError(null);
    setForm(createInitialFormState(preferredDatabase(databases)));
  }

  function openCreateDialog() {
    if (freeTierExhausted) {
      setFreeTierDialogOpen(true);
      return;
    }
    if (eligibleDatabases.length === 0) {
      void navigate({ to: "/databases" });
      return;
    }
    setDialogMode("create");
  }

  function openEditDialog(workspace: AFSWorkspaceSummary) {
    setDialogMode("edit");
    setEditingWorkspaceId(workspace.id);
    setEditingWorkspaceDatabaseId(workspace.databaseId);
    setFormError(null);
  }

  function updateForm<TKey extends keyof WorkspaceFormState>(
    key: TKey,
    value: WorkspaceFormState[TKey],
  ) {
    setFormError(null);
    setForm((current) => ({ ...current, [key]: value }));
  }

  function mutationErrorMessage(error: unknown, fallback: string) {
    if (error instanceof Error && error.message.trim() !== "") {
      return error.message;
    }
    return fallback;
  }

  function openWorkspace(workspace: AFSWorkspaceSummary) {
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: workspace.id },
      search: { databaseId: workspace.databaseId },
    });
  }

  function previewWorkspace(workspace: AFSWorkspaceSummary) {
    void router.preloadRoute({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: workspace.id },
      search: { databaseId: workspace.databaseId },
    });
  }

  function openWorkspaceTab(workspace: AFSWorkspaceSummary, tab: "browse" | "checkpoints" | "activity" | "settings") {
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: workspace.id },
      search: {
        databaseId: workspace.databaseId,
        ...(tab === "browse" ? {} : { tab }),
      },
    });
  }

  function deleteSelectedWorkspace(workspace: AFSWorkspaceSummary) {
    setDeleteError(null);
    setWorkspaceToDelete(workspace);
  }

  function closeDeleteDialog() {
    if (deleteWorkspace.isPending) {
      return;
    }
    setWorkspaceToDelete(null);
    setDeleteError(null);
  }

  function confirmDeleteWorkspace() {
    if (workspaceToDelete == null) {
      return;
    }
    const target = workspaceToDelete;
    setDeleteError(null);
    deleteWorkspace.mutate(
      {
        databaseId: target.databaseId,
        workspaceId: target.id,
      },
      {
        onSuccess: () => {
          if (editingWorkspaceId === target.id) {
            closeDialog();
          }
          setWorkspaceToDelete(null);
        },
        onError: (error) => {
          setDeleteError(mutationErrorMessage(error, "Unable to remove the workspace."));
        },
      },
    );
  }

  if (location.pathname !== "/workspaces") {
    return <Outlet />;
  }

  return (
    <PageStack>
      {showStarterConnectPanel ? (
        <GettingStartedPanel>
          <GettingStartedPanelCopy>
            <GettingStartedPanelEyebrow>Getting Started</GettingStartedPanelEyebrow>
            <GettingStartedPanelTitle>Connect your agents to your first workspace</GettingStartedPanelTitle>
            <GettingStartedPanelBody>
              Your starter workspace is ready with sample files. Connect an agent to sync it locally or access it through MCP.
            </GettingStartedPanelBody>
          </GettingStartedPanelCopy>
          <GettingStartedPanelActions>
            <Button size="medium" onClick={() => setOnboardingWorkspace(starterWorkspace)}>
              Connect my first agent
            </Button>
            <Button
              size="medium"
              variant="secondary-fill"
              onClick={() => openWorkspace(starterWorkspace)}
            >
              Open workspace
            </Button>
          </GettingStartedPanelActions>
        </GettingStartedPanel>
      ) : null}

      <WorkspaceTable
        rows={workspaces}
        loading={workspacesQuery.isLoading}
        error={workspacesQuery.isError}
        connectedAgentsByWorkspace={connectedAgentsByWorkspace}
        deletingWorkspaceKey={
          deleteWorkspace.isPending && deleteWorkspace.variables != null
            ? workspaceRowKey(deleteWorkspace.variables.databaseId, deleteWorkspace.variables.workspaceId)
            : null
        }
        toolbarAction={(
          <ToolbarActions>
            {onboardingDb != null && (
              <FreeTierChip $exhausted={freeTierExhausted} title="Free tier: 3 workspaces on AFS Cloud">
                {freeTierUsed} / {FREE_TIER_WORKSPACE_LIMIT} free
              </FreeTierChip>
            )}
            <Button size="medium" onClick={openCreateDialog}>
              Add workspace
            </Button>
          </ToolbarActions>
        )}
        onOpenWorkspace={openWorkspace}
        onPreviewWorkspace={previewWorkspace}
        onOpenWorkspaceTab={openWorkspaceTab}
        onEditWorkspace={openEditDialog}
        onDeleteWorkspace={deleteSelectedWorkspace}
      />

      {isEditing ? (
        <DialogOverlay
          onClick={(event) => {
            if (event.target === event.currentTarget) {
              closeDialog();
            }
          }}
        >
          <DialogCard>
            <DialogHeader>
              <div>
                <DialogTitle>Edit workspace</DialogTitle>
                <DialogBody>
                  Update the workspace name and metadata.
                </DialogBody>
              </div>
              <DialogCloseButton type="button" aria-label="Close" onClick={closeDialog}>
                &times;
              </DialogCloseButton>
            </DialogHeader>

            <FormGrid
              onSubmit={(event) => {
                event.preventDefault();
                if (form.name.trim() === "") {
                  return;
                }
                updateWorkspace.mutate(
                  {
                    databaseId: editingWorkspaceDatabaseId ?? undefined,
                    workspaceId: editingWorkspaceId!,
                    name: form.name,
                    description: form.description,
                    cloudAccount: form.cloudAccount,
                    databaseName: form.databaseName,
                    region: form.region,
                  },
                  {
                    onSuccess: () => {
                      closeDialog();
                    },
                    onError: (error) => {
                      setFormError(mutationErrorMessage(error, "Unable to update the workspace."));
                    },
                  },
                );
              }}
            >
              <Field>
                Name
                <TextInput
                  autoFocus
                  value={form.name}
                  onChange={(event) => updateForm("name", event.target.value)}
                  placeholder="customer-portal"
                />
                <FieldHint>Renaming keeps the same stable workspace ID.</FieldHint>
              </Field>

              <Field>
                Description
                <TextInput
                  value={form.description}
                  onChange={(event) => updateForm("description", event.target.value)}
                  placeholder="What this workspace is for, who owns it, and why it exists."
                />
              </Field>

              {formError ? <DialogError role="alert">{formError}</DialogError> : null}

              <DialogActions>
                <Button
                  disabled={formBusy}
                  size="medium"
                  type="button"
                  variant="secondary-fill"
                  onClick={closeDialog}
                >
                  Cancel
                </Button>
                <Button disabled={formBusy} size="medium" type="submit">
                  {updateWorkspace.isPending ? "Saving\u2026" : "Save changes"}
                </Button>
              </DialogActions>
            </FormGrid>
          </DialogCard>
        </DialogOverlay>
      ) : null}

      <CreateWorkspaceDialog
        open={dialogMode === "create"}
        onClose={closeDialog}
        onFreeTierLimitHit={() => {
          closeDialog();
          setFreeTierDialogOpen(true);
        }}
      />

      <ImportDialog open={importOpen} onClose={() => setImportOpen(false)} databases={databases} />

      {onboardingWorkspace ? (
        <GettingStartedOnboardingDialog
          open
          workspaceId={onboardingWorkspace.id}
          workspaceName={onboardingWorkspace.name}
          databaseName={onboardingWorkspace.databaseName}
          fileCount={onboardingWorkspace.fileCount}
          folderCount={onboardingWorkspace.folderCount}
          initialStage="connect"
          onClose={() => setOnboardingWorkspace(null)}
        />
      ) : null}

      <FreeTierLimitDialog
        open={freeTierDialogOpen}
        used={freeTierUsed}
        limit={FREE_TIER_WORKSPACE_LIMIT}
        onClose={() => setFreeTierDialogOpen(false)}
      />

      {workspaceToDelete ? (
        <DialogOverlay
          role="dialog"
          aria-modal="true"
          aria-labelledby="remove-workspace-dialog-title"
          onClick={(event) => {
            if (event.target === event.currentTarget) {
              closeDeleteDialog();
            }
          }}
        >
          <ConfirmCard onClick={(event) => event.stopPropagation()}>
            <DialogHeader>
              <div>
                <DialogTitle id="remove-workspace-dialog-title">
                  Remove this workspace?
                </DialogTitle>
                <DialogBody>
                  Remove <strong>{workspaceToDelete.name}</strong> from the workspace registry
                  and permanently delete its files, folders, and checkpoints from Redis.
                  This action cannot be undone.
                </DialogBody>
              </div>
              <DialogCloseButton
                type="button"
                aria-label="Close"
                onClick={closeDeleteDialog}
              >
                &times;
              </DialogCloseButton>
            </DialogHeader>

            {deleteError ? <DialogError role="alert">{deleteError}</DialogError> : null}

            <DialogActions style={{ justifyContent: "flex-end", marginTop: 20 }}>
              <Button
                variant="secondary-fill"
                size="medium"
                onClick={closeDeleteDialog}
                disabled={deleteWorkspace.isPending}
              >
                Cancel
              </Button>
              <DeleteConfirmButton
                size="medium"
                onClick={confirmDeleteWorkspace}
                disabled={deleteWorkspace.isPending}
              >
                {deleteWorkspace.isPending ? "Removing..." : "Remove workspace"}
              </DeleteConfirmButton>
            </DialogActions>
          </ConfirmCard>
        </DialogOverlay>
      ) : null}
    </PageStack>
  );
}

function isGettingStartedWorkspace(name: string) {
  const trimmed = name.trim().toLowerCase();
  return trimmed === "getting-started" || trimmed.startsWith("getting-started-");
}

const FieldHint = styled.span`
  font-size: 12px;
  color: var(--afs-muted, #71717a);
  line-height: 1.5;
`;

const ToolbarActions = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 12px;
`;

const FreeTierChip = styled.span<{ $exhausted?: boolean }>`
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.02em;
  background: ${(p) =>
    p.$exhausted
      ? "#fef2f2"
      : "color-mix(in srgb, var(--afs-accent) 10%, transparent)"};
  color: ${(p) => (p.$exhausted ? "#b91c1c" : "var(--afs-accent, #2563eb)")};
  white-space: nowrap;
`;

const GettingStartedPanel = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 22px 24px;
  border: 1px solid color-mix(in srgb, var(--afs-accent) 18%, var(--afs-line));
  border-radius: 20px;
  background:
    radial-gradient(circle at top right, color-mix(in srgb, var(--afs-accent) 12%, transparent), transparent 32%),
    linear-gradient(180deg, var(--afs-panel-strong), color-mix(in srgb, var(--afs-bg-soft) 52%, white));

  @media (max-width: 900px) {
    flex-direction: column;
    align-items: flex-start;
  }
`;

const GettingStartedPanelCopy = styled.div`
  display: flex;
  flex-direction: column;
  gap: 8px;
`;

const GettingStartedPanelEyebrow = styled.div`
  color: var(--afs-accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const GettingStartedPanelTitle = styled.h2`
  margin: 0;
  color: var(--afs-ink);
  font-size: 22px;
  line-height: 1.15;
`;

const GettingStartedPanelBody = styled.p`
  margin: 0;
  max-width: 58ch;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const GettingStartedPanelActions = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
`;

const ConfirmCard = styled(DialogCard)`
  max-width: 540px;
`;

const DeleteConfirmButton = styled(Button)`
  && {
    background: ${({ theme }) => theme.semantic.color.background.danger500};
    color: ${({ theme }) => theme.semantic.color.text.inverse};
    box-shadow: none;
  }

  &&:hover:not(:disabled),
  &&:focus-visible:not(:disabled) {
    background: ${({ theme }) => theme.semantic.color.background.danger600};
    color: ${({ theme }) => theme.semantic.color.text.inverse};
    box-shadow: none;
  }
`;
