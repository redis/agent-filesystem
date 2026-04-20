import { createFileRoute, Outlet, useLocation, useNavigate, useRouter } from "@tanstack/react-router";
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
import { GettingStartedOnboardingDialog } from "../components/getting-started-onboarding-dialog";
import {
  agentsQueryOptions,
  useCreateWorkspaceMutation,
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
                    search: { tab: "browse" },
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

function workspaceRowKey(workspaceId: string) {
  return workspaceId;
}

function WorkspacesPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const router = useRouter();
  const workspacesQuery = useScopedWorkspaceSummaries();
  const agentsQuery = useScopedAgents();
  const { databases } = useDatabaseScope();
  const eligibleDatabases = workspaceEligibleDatabases(databases);
  const createWorkspace = useCreateWorkspaceMutation();
  const updateWorkspace = useUpdateWorkspaceMutation();
  const deleteWorkspace = useDeleteWorkspaceMutation();
  const importLocal = useImportLocalMutation();

  const [importOpen, setImportOpen] = useState(false);
  const [importPath, setImportPath] = useState("");
  const [importPathAuto, setImportPathAuto] = useState(true);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [dialogMode, setDialogMode] = useState<DialogMode>(null);
  const [editingWorkspaceId, setEditingWorkspaceId] = useState<string | null>(null);
  const [onboardingWorkspace, setOnboardingWorkspace] = useState<AFSWorkspaceSummary | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [form, setForm] = useState<WorkspaceFormState>(() =>
    createInitialFormState(preferredDatabase(databases)),
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
      const currentDatabase = databases.find((item) => item.id === current.databaseId) ?? preferredDatabase(databases);
      if (currentDatabase != null && !currentDatabase.canCreateWorkspaces) {
        return { ...current, ...createWorkspaceDefaults(preferredDatabase(databases)) };
      }
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
  const starterWorkspace = workspaces.length === 1 && isGettingStartedWorkspace(workspaces[0].name)
    ? workspaces[0]
    : null;
  const showStarterConnectPanel = starterWorkspace != null &&
    (connectedAgentsByWorkspace[workspaceRowKey(starterWorkspace.id)] ?? 0) === 0;

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
    setImportPathAuto(true);
    setForm(createInitialFormState(preferredDatabase(databases)));
  }

  function openCreateDialog() {
    if (eligibleDatabases.length === 0) {
      void navigate({ to: "/databases" });
      return;
    }

    setDialogMode("create");
    setEditingWorkspaceId(null);
    setFormError(null);
    setImportPath("");
    setImportPathAuto(true);
    setForm(createInitialFormState(preferredDatabase(databases)));
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
    if (key === "name" && importPathAuto && dialogMode === "create") {
      const trimmed = typeof value === "string" ? value.trim() : "";
      setImportPath(trimmed === "" ? "" : `~/${trimmed}`);
    }
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

  function previewWorkspace(workspace: AFSWorkspaceSummary) {
    void router.preloadRoute({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: workspace.id },
    });
  }

  function openWorkspaceTab(workspace: AFSWorkspaceSummary, tab: "browse" | "checkpoints" | "activity" | "settings") {
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: workspace.id },
      search: tab === "browse" ? {} : { tab },
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
            ? workspaceRowKey(deleteWorkspace.variables.workspaceId)
            : null
        }
        toolbarAction={(
          <Button size="medium" onClick={openCreateDialog} disabled={eligibleDatabases.length === 0}>
            Add workspace
          </Button>
        )}
        onOpenWorkspace={openWorkspace}
        onPreviewWorkspace={previewWorkspace}
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
                    ? "Update workspace metadata. Rename is not supported yet."
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
                          search: { tab: "browse" },
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
                {isEditing ? (
                  <FieldHint>
                    Workspace names are immutable today. Use the workspace ID and database name below to
                    disambiguate duplicates.
                  </FieldHint>
                ) : null}
              </Field>

              {!isEditing ? (
                <Field>
                  Database
                  <Select
                    value={form.databaseId}
                    onChange={(event) => {
                    const nextDatabase = eligibleDatabases.find((item) => item.id === event.target.value);
                      updateForm("databaseId", event.target.value);
                      updateForm("databaseName", nextDatabase?.databaseName ?? nextDatabase?.displayName ?? "");
                    }}
                  >
                    {eligibleDatabases.map((database) => (
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
                        onChange={(event) => {
                          setImportPath(event.target.value);
                          setImportPathAuto(false);
                        }}
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
                            if (path) {
                              setImportPath(path);
                              setImportPathAuto(false);
                            }
                          }
                        }}
                      />
                    </div>
                    {importPathAuto && importPath !== "" ? (
                      <FieldHint>
                        Auto-filled from workspace name. Edit to customize.
                      </FieldHint>
                    ) : null}
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
    </PageStack>
  );
}

function isGettingStartedWorkspace(name: string) {
  const trimmed = name.trim().toLowerCase();
  return trimmed === "getting-started" || trimmed.startsWith("getting-started-");
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

const FieldHint = styled.span`
  font-size: 12px;
  color: var(--afs-muted, #71717a);
  line-height: 1.5;
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
