import { createFileRoute, Outlet, useLocation, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redis-ui/components";
import { useEffect, useRef, useState } from "react";
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
  Select,
  TextInput,
} from "../components/afs-kit";
import {
  useCreateWorkspaceMutation,
  useDeleteWorkspaceMutation,
  useUpdateWorkspaceMutation,
  useImportLocalMutation,
  useWorkspace,
} from "../foundation/hooks/use-afs";
import {
  type AFSDatabaseScopeRecord,
  useDatabaseScope,
  useScopedAgents,
  useScopedWorkspaceSummaries,
} from "../foundation/database-scope";
import { WorkspaceTable } from "../foundation/tables/workspace-table";
import type { AFSWorkspaceSummary } from "../foundation/types/afs";

export const Route = createFileRoute("/workspaces")({
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
    databaseId: databases[0]?.id ?? "",
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
                    search: { tab: "files" },
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
                value={form.databaseId}
                onChange={(e) => update("databaseId", e.target.value)}
              >
                {databases.map((db) => (
                  <option key={db.id} value={db.id}>
                    {db.displayName || db.databaseName}
                  </option>
                ))}
              </Select>
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

function workspaceRowKey(workspaceId: string) {
  return workspaceId;
}

function WorkspacesPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const workspacesQuery = useScopedWorkspaceSummaries();
  const agentsQuery = useScopedAgents();
  const { databases } = useDatabaseScope();
  const createWorkspace = useCreateWorkspaceMutation();
  const updateWorkspace = useUpdateWorkspaceMutation();
  const deleteWorkspace = useDeleteWorkspaceMutation();
  const importLocal = useImportLocalMutation();

  const [importOpen, setImportOpen] = useState(false);
  const [importPath, setImportPath] = useState("");
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [dialogMode, setDialogMode] = useState<DialogMode>(null);
  const [editingWorkspaceId, setEditingWorkspaceId] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [form, setForm] = useState<WorkspaceFormState>(() =>
    createInitialFormState(databases[0] ?? null),
  );

  const editingWorkspaceQuery = useWorkspace(
    null,
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

  useEffect(() => {
    if (dialogMode !== "create") {
      return;
    }

    setForm((current) => {
      const currentDatabase = databases.find((item) => item.id === current.databaseId) ?? databases[0] ?? null;
      const defaults = createWorkspaceDefaults(currentDatabase);
      return { ...current, ...defaults };
    });
  }, [databases, dialogMode]);

  const workspaces = workspacesQuery.data;
  const connectedAgentsByWorkspace = agentsQuery.data.reduce<Record<string, number>>((counts, session) => {
    const key = workspaceRowKey(session.workspaceId);
    counts[key] = (counts[key] ?? 0) + 1;
    return counts;
  }, {});

  if (workspacesQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const isDialogOpen = dialogMode != null;
  const isEditing = dialogMode === "edit" && editingWorkspaceId != null;
  const formBusy =
    createWorkspace.isPending ||
    updateWorkspace.isPending ||
    importLocal.isPending ||
    (isEditing && editingWorkspaceQuery.isLoading);

  function closeDialog() {
    setDialogMode(null);
    setEditingWorkspaceId(null);
    setFormError(null);
    setImportPath("");
    setForm(createInitialFormState(databases[0] ?? null));
  }

  function openCreateDialog() {
    if (databases.length === 0) {
      void navigate({ to: "/databases" });
      return;
    }

    setDialogMode("create");
    setEditingWorkspaceId(null);
    setFormError(null);
    setForm(createInitialFormState(databases[0] ?? null));
  }

  function openEditDialog(workspace: AFSWorkspaceSummary) {
    setDialogMode("edit");
    setEditingWorkspaceId(workspace.id);
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
    });
  }

  function openWorkspaceTab(workspace: AFSWorkspaceSummary, tab: "overview" | "files" | "checkpoints" | "activity") {
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: workspace.id },
      search: tab === "overview" ? {} : { tab },
    });
  }

  function deleteSelectedWorkspace(workspace: AFSWorkspaceSummary) {
    const confirmed = window.confirm(
      `Delete workspace "${workspace.name}"? This removes its registry entry from the catalog.`,
    );

    if (!confirmed) {
      return;
    }

    deleteWorkspace.mutate({
      workspaceId: workspace.id,
    }, {
      onSuccess: () => {
        if (editingWorkspaceId === workspace.id) {
          closeDialog();
        }
      },
    });
  }

  if (location.pathname !== "/workspaces") {
    return <Outlet />;
  }

  return (
    <PageStack>
      <WorkspaceTable
        rows={workspaces}
        loading={workspacesQuery.isLoading}
        error={workspacesQuery.isError}
        connectedAgentsByWorkspace={connectedAgentsByWorkspace}
        deletingWorkspaceKey={
          deleteWorkspace.isPending && deleteWorkspace.variables != null
            ? workspaceRowKey(deleteWorkspace.variables.workspaceId)
            : null
        }
        toolbarAction={(
          <Button size="medium" onClick={openCreateDialog}>
            Add workspace
          </Button>
        )}
        onOpenWorkspace={openWorkspace}
        onOpenWorkspaceTab={openWorkspaceTab}
        onEditWorkspace={openEditDialog}
        onDeleteWorkspace={deleteSelectedWorkspace}
      />

      {isDialogOpen ? (
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
                <DialogTitle>
                  {isEditing ? "Edit workspace" : "Add workspace"}
                </DialogTitle>
                <DialogBody>
                  {isEditing
                    ? "Update the workspace name and description."
                    : "Add a new workspace and choose which database will host it."}
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

                if (editingWorkspaceId != null) {
                  updateWorkspace.mutate(
                    {
                      workspaceId: editingWorkspaceId,
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
                  return;
                }

                if (importPath.trim() !== "") {
                  importLocal.mutate(
                    {
                      databaseId: form.databaseId,
                      name: form.name,
                      path: importPath.trim(),
                      description: form.description,
                    },
                    {
                      onSuccess: (result) => {
                        closeDialog();
                        void navigate({
                          to: "/workspaces/$workspaceId",
                          params: { workspaceId: result.workspaceId },
                          search: { tab: "files" },
                        });
                      },
                      onError: (error) => {
                        setFormError(
                          mutationErrorMessage(error, "Unable to import files."),
                        );
                      },
                    },
                  );
                } else {
                  createWorkspace.mutate(
                    {
                      databaseId: form.databaseId,
                      name: form.name,
                      description: form.description,
                      cloudAccount: form.cloudAccount,
                      databaseName: form.databaseName,
                      region: form.region,
                      source: "blank",
                    },
                    {
                      onSuccess: () => {
                        closeDialog();
                      },
                      onError: (error) => {
                        setFormError(
                          mutationErrorMessage(
                            error,
                            "Unable to create the workspace.",
                          ),
                        );
                      },
                    },
                  );
                }
              }}
            >
              <Field>
                Name
                <TextInput
                  autoFocus
                  disabled={isEditing}
                  value={form.name}
                  onChange={(event) => updateForm("name", event.target.value)}
                  placeholder="customer-portal"
                />
              </Field>

              {!isEditing ? (
                <Field>
                  Database
                  <Select
                    value={form.databaseId}
                    onChange={(event) => {
                      const nextDatabase = databases.find((item) => item.id === event.target.value);
                      updateForm("databaseId", event.target.value);
                      updateForm("databaseName", nextDatabase?.databaseName ?? nextDatabase?.displayName ?? "");
                    }}
                  >
                    {databases.map((database) => (
                      <option key={database.id} value={database.id}>
                        {database.displayName || database.databaseName}
                      </option>
                    ))}
                  </Select>
                </Field>
              ) : null}

              <Field>
                Description
                <TextInput
                  value={form.description}
                  onChange={(event) => updateForm("description", event.target.value)}
                  placeholder="What this workspace is for, who owns it, and why it exists."
                />
              </Field>

              {!isEditing ? (
                <ImportFilesSection>
                  <ImportFilesHeader>Import Files</ImportFilesHeader>
                  <ImportFilesDescription>
                    Optionally import files from a local directory. The contents will be scanned
                    and stored in the workspace.
                  </ImportFilesDescription>
                  <Field>
                    Local directory path
                    <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
                      <TextInput
                        value={importPath}
                        onChange={(event) => setImportPath(event.target.value)}
                        placeholder="~/code/my-project"
                        style={{ flex: 1 }}
                      />
                      <Button
                        size="medium"
                        variant="secondary-fill"
                        type="button"
                        onClick={() => fileInputRef.current?.click()}
                      >
                        Browse
                      </Button>
                      <input
                        ref={fileInputRef}
                        type="file"
                        /* @ts-expect-error webkitdirectory is non-standard */
                        webkitdirectory=""
                        directory=""
                        style={{ display: "none" }}
                        onChange={(event) => {
                          const files = event.target.files;
                          if (files && files.length > 0) {
                            // Extract the common directory path from the first file
                            const path = files[0].webkitRelativePath?.split("/")[0] ?? "";
                            if (path) setImportPath(path);
                          }
                        }}
                      />
                    </div>
                  </Field>
                </ImportFilesSection>
              ) : null}

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
                  {isEditing
                    ? updateWorkspace.isPending
                      ? "Saving..."
                      : "Save changes"
                    : importLocal.isPending
                      ? "Importing..."
                      : createWorkspace.isPending
                        ? "Adding..."
                        : importPath.trim() !== ""
                          ? "Import Files"
                          : "Add workspace"}
                </Button>
              </DialogActions>
            </FormGrid>
          </DialogCard>
        </DialogOverlay>
      ) : null}

      <ImportDialog open={importOpen} onClose={() => setImportOpen(false)} databases={databases} />
    </PageStack>
  );
}

/* ---- Import Files section styled components ---- */
const ImportFilesSection = styled.div`
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px;
  border: 1px solid var(--afs-line, #e4e4e7);
  border-radius: 12px;
  background: var(--afs-panel, #fafafa);
`;

const ImportFilesHeader = styled.span`
  font-size: 14px;
  font-weight: 700;
  color: var(--afs-ink, #18181b);
`;

const ImportFilesDescription = styled.span`
  font-size: 12px;
  color: var(--afs-muted, #71717a);
  line-height: 1.5;
`;
