import { createFileRoute, Outlet, useLocation, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redislabsdev/redis-ui-components";
import { useEffect, useState } from "react";
import styled from "styled-components";
import {
  Field,
  FormGrid,
  PageStack,
  SectionTitle,
  TextArea,
  TextInput,
} from "../components/afs-kit";
import {
  useCreateWorkspaceMutation,
  useDeleteWorkspaceMutation,
  useUpdateWorkspaceMutation,
  useWorkspace,
} from "../foundation/hooks/use-afs";
import {
  type AFSDatabaseScopeRecord,
  useDatabaseScope,
  useScopedWorkspaceSummaries,
} from "../foundation/database-scope";
import { WorkspaceTable } from "../foundation/tables/workspace-table";
import type { AFSWorkspaceSource } from "../foundation/types/afs";

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
  source: AFSWorkspaceSource;
};

type DialogMode = "create" | "edit" | null;

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
    source: "blank",
  };
}

function WorkspacesPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const workspacesQuery = useScopedWorkspaceSummaries();
  const { selectedDatabase, selectedDatabaseId } = useDatabaseScope();
  const createWorkspace = useCreateWorkspaceMutation();
  const updateWorkspace = useUpdateWorkspaceMutation();
  const deleteWorkspace = useDeleteWorkspaceMutation();

  const [dialogMode, setDialogMode] = useState<DialogMode>(null);
  const [editingWorkspaceId, setEditingWorkspaceId] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [form, setForm] = useState<WorkspaceFormState>(() =>
    createInitialFormState(selectedDatabase),
  );

  const editingWorkspaceQuery = useWorkspace(
    selectedDatabaseId,
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
      source: workspace.source,
    });
  }, [dialogMode, editingWorkspaceQuery.data]);

  useEffect(() => {
    if (dialogMode !== "create" || selectedDatabase == null) {
      return;
    }

    const defaults = createWorkspaceDefaults(selectedDatabase);
    setForm((current) => ({ ...current, ...defaults }));
  }, [dialogMode, selectedDatabase]);

  const workspaces = workspacesQuery.data;

  if (workspacesQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const isDialogOpen = dialogMode != null;
  const isEditing = dialogMode === "edit" && editingWorkspaceId != null;
  const formBusy =
    createWorkspace.isPending ||
    updateWorkspace.isPending ||
    (isEditing && editingWorkspaceQuery.isLoading);

  function closeDialog() {
    setDialogMode(null);
    setEditingWorkspaceId(null);
    setFormError(null);
    setForm(createInitialFormState(selectedDatabase));
  }

  function openCreateDialog() {
    if (selectedDatabase == null) {
      void navigate({ to: "/databases" });
      return;
    }

    setDialogMode("create");
    setEditingWorkspaceId(null);
    setFormError(null);
    setForm(createInitialFormState(selectedDatabase));
  }

  function openEditDialog(workspaceId: string) {
    setDialogMode("edit");
    setEditingWorkspaceId(workspaceId);
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

  function openWorkspace(workspaceId: string) {
    if (selectedDatabaseId == null) {
      void navigate({ to: "/databases" });
      return;
    }

    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId },
    });
  }

  function deleteSelectedWorkspace(workspaceId: string) {
    const workspace = workspaces.find((item) => item.id === workspaceId);
    const confirmed = window.confirm(
      `Delete workspace "${workspace?.name ?? workspaceId}"? This removes its registry entry from the catalog.`,
    );

    if (!confirmed) {
      return;
    }

    deleteWorkspace.mutate({
      databaseId: selectedDatabaseId ?? "",
      workspaceId,
    }, {
      onSuccess: () => {
        if (editingWorkspaceId === workspaceId) {
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
      <PageActionsRow>
        <Button size="medium" onClick={openCreateDialog}>
          Add workspace
        </Button>
      </PageActionsRow>

      <WorkspaceTable
        rows={workspaces}
        loading={workspacesQuery.isLoading}
        error={workspacesQuery.isError}
        deletingWorkspaceId={
          deleteWorkspace.isPending ? deleteWorkspace.variables?.workspaceId ?? null : null
        }
        onOpenWorkspace={openWorkspace}
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
              <SectionTitle
                eyebrow={isEditing ? "Edit Workspace" : "Add Workspace"}
                title={isEditing ? "Update workspace details" : "Create a workspace"}
                body={
                  isEditing
                    ? "Update the workspace metadata without leaving the registry."
                    : `Create a workspace inside ${selectedDatabase?.displayName ?? "the current database"}.`
                }
              />
              <Button size="medium" variant="secondary-fill" onClick={closeDialog}>
                Close
              </Button>
            </DialogHeader>

            <SourcePicker>
              <SourceOption
                $active={form.source === "blank"}
                disabled={isEditing}
                type="button"
                onClick={() => updateForm("source", "blank")}
              >
                <SourceTitle>Blank workspace</SourceTitle>
                <SourceBody>Start from an empty filesystem and shape the workspace from scratch.</SourceBody>
              </SourceOption>
              <SourceOption
                $active={form.source === "git-import"}
                disabled={isEditing}
                type="button"
                onClick={() => updateForm("source", "git-import")}
              >
                <SourceTitle>Git import</SourceTitle>
                <SourceBody>Bring in code or configuration that still needs browser-side edits.</SourceBody>
              </SourceOption>
              <SourceOption
                $active={form.source === "cloud-import"}
                disabled={isEditing}
                type="button"
                onClick={() => updateForm("source", "cloud-import")}
              >
                <SourceTitle>Redis Cloud import</SourceTitle>
                <SourceBody>Register an existing managed workspace and operate on it from the studio.</SourceBody>
              </SourceOption>
            </SourcePicker>

            <FormGrid
              id="workspace-dialog-form"
              onSubmit={(event) => {
                event.preventDefault();
                if (form.name.trim() === "") {
                  return;
                }

                if (editingWorkspaceId != null) {
                  updateWorkspace.mutate(
                    {
                      databaseId: form.databaseId,
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

                createWorkspace.mutate(
                  {
                    databaseId: form.databaseId,
                    name: form.name,
                    description: form.description,
                    cloudAccount: form.cloudAccount,
                    databaseName: form.databaseName,
                    region: form.region,
                    source: form.source,
                  },
                  {
                    onSuccess: () => {
                      closeDialog();
                    },
                    onError: (error) => {
                      setFormError(
                        mutationErrorMessage(
                          error,
                          "Unable to create the workspace in the selected database.",
                        ),
                      );
                    },
                  },
                );
              }}
            >
              <Field>
                Workspace name
                <TextInput
                  disabled={isEditing}
                  value={form.name}
                  onChange={(event) => updateForm("name", event.target.value)}
                  placeholder="customer-portal"
                />
              </Field>

              <Field>
                Description
                <TextArea
                  value={form.description}
                  onChange={(event) => updateForm("description", event.target.value)}
                  placeholder="What this workspace is for, who owns it, and why it exists."
                />
              </Field>
            </FormGrid>

            {formError ? <DialogError role="alert">{formError}</DialogError> : null}

            <DialogFooter>
              <DialogActions>
                <Button disabled={formBusy} size="medium" type="submit" form="workspace-dialog-form">
                  {isEditing
                    ? updateWorkspace.isPending
                      ? "Saving..."
                      : "Save changes"
                    : createWorkspace.isPending
                      ? "Creating..."
                      : "Create workspace"}
                </Button>
                <Button
                  disabled={formBusy}
                  size="medium"
                  type="button"
                  variant="secondary-fill"
                  onClick={closeDialog}
                >
                  Cancel
                </Button>
              </DialogActions>
            </DialogFooter>
          </DialogCard>
        </DialogOverlay>
      ) : null}
    </PageStack>
  );
}

const DialogOverlay = styled.div`
  position: fixed;
  inset: 0;
  z-index: 30;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(8, 6, 13, 0.36);
`;

const PageActionsRow = styled.div`
  display: flex;
  justify-content: flex-end;
  width: 100%;
`;

const DialogCard = styled.div`
  width: min(760px, 100%);
  max-height: min(88vh, 860px);
  overflow: auto;
  border: 1px solid var(--afs-line);
  border-radius: 24px;
  padding: 24px 24px 0;
  background: #fff;
  box-shadow: 0 18px 40px rgba(8, 6, 13, 0.12);
`;

const DialogHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  margin-bottom: 18px;

  @media (max-width: 720px) {
    flex-direction: column;
  }
`;

const SourcePicker = styled.div`
  display: grid;
  gap: 10px;
  margin-bottom: 18px;
`;

const SourceOption = styled.button<{ $active: boolean }>`
  border: 1px solid
    ${({ $active }) => ($active ? "var(--afs-line-strong)" : "var(--afs-line)")};
  border-radius: 18px;
  padding: 14px 15px;
  background: ${({ $active }) =>
    $active ? "rgba(8, 6, 13, 0.04)" : "#fff"};
  text-align: left;
  cursor: pointer;
  transition:
    border-color 160ms ease,
    background 160ms ease,
    opacity 160ms ease;

  &:hover:enabled {
    border-color: var(--afs-line-strong);
  }

  &:disabled {
    cursor: default;
    opacity: 0.6;
  }
`;

const SourceTitle = styled.div`
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
`;

const SourceBody = styled.p`
  margin: 6px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const DialogFooter = styled.div`
  position: sticky;
  bottom: 0;
  display: flex;
  justify-content: flex-end;
  gap: 16px;
  align-items: flex-end;
  margin: 20px -24px 0;
  padding: 18px 24px 24px;
  border-top: 1px solid var(--afs-line);
  background: #fff;

  @media (max-width: 720px) {
    flex-direction: column;
    align-items: stretch;
  }
`;

const DialogError = styled.p`
  margin: 16px 0 0;
  color: #c2364a;
  font-size: 14px;
  line-height: 1.5;
`;

const DialogActions = styled.div`
  display: flex;
  flex-wrap: nowrap;
  gap: 10px;
  align-items: center;
  justify-content: flex-end;
  overflow-x: auto;
`;
