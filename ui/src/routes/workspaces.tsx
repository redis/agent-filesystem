import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader, Typography } from "@redislabsdev/redis-ui-components";
import { useEffect, useState } from "react";
import styled from "styled-components";
import {
  CardHeader,
  EditorArea,
  EditorPanel,
  Field,
  FileButton,
  FileList,
  FormGrid,
  PageStack,
  SectionTitle,
  Select,
  TextArea,
  TextInput,
  TwoColumnFields,
} from "../components/afs-kit";
import {
  useCreateWorkspaceMutation,
  useDeleteWorkspaceMutation,
  useWorkspaceFileContent,
  useWorkspaceTree,
  useUpdateWorkspaceMutation,
  useWorkspace,
  useWorkspaceSummaries,
} from "../foundation/hooks/use-afs";
import { WorkspaceTable } from "../foundation/tables/workspace-table";
import type { AFSWorkspaceSource, AFSWorkspaceView } from "../foundation/types/afs";

export const Route = createFileRoute("/workspaces")({
  component: WorkspacesPage,
});

type WorkspaceFormState = {
  name: string;
  description: string;
  cloudAccount: string;
  databaseName: string;
  region: string;
  source: AFSWorkspaceSource;
};

type DialogMode = "create" | "edit" | null;

function createInitialFormState(): WorkspaceFormState {
  return {
    name: "",
    description: "",
    cloudAccount: "Redis Cloud / Product",
    databaseName: "agentfs-dev-us-east-1",
    region: "us-east-1",
    source: "blank",
  };
}

function WorkspacesPage() {
  const navigate = useNavigate();
  const workspacesQuery = useWorkspaceSummaries();
  const createWorkspace = useCreateWorkspaceMutation();
  const updateWorkspace = useUpdateWorkspaceMutation();
  const deleteWorkspace = useDeleteWorkspaceMutation();

  const [dialogMode, setDialogMode] = useState<DialogMode>(null);
  const [editingWorkspaceId, setEditingWorkspaceId] = useState<string | null>(null);
  const [previewWorkspaceId, setPreviewWorkspaceId] = useState<string | null>(null);
  const [previewPath, setPreviewPath] = useState("/");
  const [previewSelectedPath, setPreviewSelectedPath] = useState("");
  const [form, setForm] = useState<WorkspaceFormState>(createInitialFormState);

  const editingWorkspaceQuery = useWorkspace(
    editingWorkspaceId ?? "",
    dialogMode === "edit" && editingWorkspaceId != null,
  );
  const previewWorkspaceQuery = useWorkspace(
    previewWorkspaceId ?? "",
    previewWorkspaceId != null,
  );
  const previewTreeQuery = useWorkspaceTree(
    {
      workspaceId: previewWorkspaceId ?? "",
      view: "head" as AFSWorkspaceView,
      path: previewPath,
      depth: 1,
    },
    previewWorkspaceId != null,
  );
  const previewFileQuery = useWorkspaceFileContent(
    {
      workspaceId: previewWorkspaceId ?? "",
      view: "head" as AFSWorkspaceView,
      path: previewSelectedPath,
    },
    previewWorkspaceId != null && previewSelectedPath !== "",
  );

  useEffect(() => {
    const workspace = editingWorkspaceQuery.data;
    if (workspace == null || dialogMode !== "edit") {
      return;
    }

    setForm({
      name: workspace.name,
      description: workspace.description,
      cloudAccount: workspace.cloudAccount,
      databaseName: workspace.databaseName,
      region: workspace.region,
      source: workspace.source,
    });
  }, [dialogMode, editingWorkspaceQuery.data]);

  useEffect(() => {
    setPreviewPath("/");
    setPreviewSelectedPath("");
  }, [previewWorkspaceId]);

  if (workspacesQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const workspaces = workspacesQuery.data ?? [];
  const isDialogOpen = dialogMode != null;
  const isEditing = dialogMode === "edit" && editingWorkspaceId != null;
  const formBusy =
    createWorkspace.isPending ||
    updateWorkspace.isPending ||
    (isEditing && editingWorkspaceQuery.isLoading);

  function closeDialog() {
    setDialogMode(null);
    setEditingWorkspaceId(null);
    setForm(createInitialFormState());
  }

  function openCreateDialog() {
    setDialogMode("create");
    setEditingWorkspaceId(null);
    setForm(createInitialFormState());
  }

  function openEditDialog(workspaceId: string) {
    setPreviewWorkspaceId(null);
    setDialogMode("edit");
    setEditingWorkspaceId(workspaceId);
  }

  function updateForm<TKey extends keyof WorkspaceFormState>(
    key: TKey,
    value: WorkspaceFormState[TKey],
  ) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  function openWorkspace(workspaceId: string) {
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId },
    });
  }

  function openWorkspacePreview(workspaceId: string) {
    setPreviewWorkspaceId(workspaceId);
  }

  function deleteSelectedWorkspace(workspaceId: string) {
    const workspace = workspaces.find((item) => item.id === workspaceId);
    const confirmed = window.confirm(
      `Delete workspace "${workspace?.name ?? workspaceId}"? This removes its registry entry from the catalog.`,
    );

    if (!confirmed) {
      return;
    }

    deleteWorkspace.mutate(workspaceId, {
      onSuccess: () => {
        if (editingWorkspaceId === workspaceId) {
          closeDialog();
        }
        if (previewWorkspaceId === workspaceId) {
          setPreviewWorkspaceId(null);
        }
      },
    });
  }

  const previewWorkspace = previewWorkspaceQuery.data;
  const previewItems = previewTreeQuery.data?.items ?? [];
  const previewFile = previewFileQuery.data;

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
          deleteWorkspace.isPending ? deleteWorkspace.variables : null
        }
        onPreviewWorkspace={openWorkspacePreview}
        onOpenWorkspace={openWorkspace}
        onEditWorkspace={openEditDialog}
        onDeleteWorkspace={deleteSelectedWorkspace}
      />

      {previewWorkspaceId != null ? (
        <DetailPanelOverlay
          onClick={(event) => {
            if (event.target === event.currentTarget) {
              setPreviewWorkspaceId(null);
            }
          }}
        >
          <DetailPanel>
            <DialogHeader>
              <SectionTitle
                eyebrow="Workspace Details"
                title={previewWorkspace?.name ?? previewWorkspaceId}
                body="Browse the saved contents of this workspace here, or jump directly to its checkpoint history."
              />
              <InlineButtonRow>
                <Button
                  size="medium"
                  onClick={() => {
                    void navigate({
                      to: "/workspaces/$workspaceId",
                      params: { workspaceId: previewWorkspaceId },
                      search: { tab: "checkpoints" },
                    });
                  }}
                >
                  Checkpoints
                </Button>
                <Button
                  size="medium"
                  variant="secondary-fill"
                  onClick={() => setPreviewWorkspaceId(null)}
                >
                  Close
                </Button>
              </InlineButtonRow>
            </DialogHeader>

            <DetailBrowserGrid>
              <BrowserCard>
                <CardHeader>
                  <div>
                    <Typography.Heading component="h3" size="S">
                      File browser
                    </Typography.Heading>
                    <Typography.Body color="secondary" component="p">
                      {previewPath}
                    </Typography.Body>
                  </div>
                  <Button
                    size="medium"
                    variant="secondary-fill"
                    disabled={previewPath === "/"}
                    onClick={() => {
                      setPreviewPath(parentPath(previewPath));
                      setPreviewSelectedPath("");
                    }}
                  >
                    Up
                  </Button>
                </CardHeader>

                {previewTreeQuery.isLoading ? <PanelNote>Loading directory contents...</PanelNote> : null}
                {previewTreeQuery.isError ? <PanelNote>Unable to load this directory.</PanelNote> : null}

                <FileList style={{ marginTop: 14 }}>
                  {previewItems.map((item) => (
                    <FileButton
                      key={item.path}
                      $active={item.path === previewSelectedPath}
                      onClick={() => {
                        if (item.kind === "dir") {
                          setPreviewPath(item.path);
                          setPreviewSelectedPath("");
                          return;
                        }
                        setPreviewSelectedPath(item.path);
                      }}
                    >
                      <FileButtonHeader>
                        <Typography.Body component="strong">{item.name}</Typography.Body>
                        <FileKind>{item.kind}</FileKind>
                      </FileButtonHeader>
                      <Typography.Body color="secondary" component="p">
                        {item.kind !== "dir" ? `${formatItemSize(item.size)} · ` : ""}
                        {item.modifiedAt
                          ? new Date(item.modifiedAt).toLocaleString()
                          : "No modification timestamp"}
                      </Typography.Body>
                    </FileButton>
                  ))}
                  {!previewTreeQuery.isLoading && previewItems.length === 0 ? (
                    <PanelNote>This directory is empty.</PanelNote>
                  ) : null}
                </FileList>
              </BrowserCard>

              <EditorPanel>
                {previewSelectedPath === "" ? (
                  <PanelNote>Select a file to inspect its contents.</PanelNote>
                ) : previewFileQuery.isLoading ? (
                  <PanelNote>Loading file content...</PanelNote>
                ) : previewFile == null ? (
                  <PanelNote>Select a file to inspect its contents.</PanelNote>
                ) : previewFile.binary ? (
                  <DetailStack>
                    <CardHeader>
                      <div>
                        <Typography.Heading component="h3" size="S">
                          {previewFile.path}
                        </Typography.Heading>
                        <Typography.Body color="secondary" component="p">
                          Binary asset
                        </Typography.Body>
                      </div>
                    </CardHeader>
                    <Typography.Body color="secondary" component="p">
                      This item looks binary, so the details panel is showing metadata instead of raw content.
                    </Typography.Body>
                  </DetailStack>
                ) : (
                  <DetailStack>
                    <CardHeader>
                      <div>
                        <Typography.Heading component="h3" size="S">
                          {previewFile.path}
                        </Typography.Heading>
                        <Typography.Body color="secondary" component="p">
                          {previewFile.language}
                        </Typography.Body>
                      </div>
                    </CardHeader>
                    <EditorArea readOnly value={previewFile.content ?? previewFile.target ?? ""} />
                  </DetailStack>
                )}
              </EditorPanel>
            </DetailBrowserGrid>
          </DetailPanel>
        </DetailPanelOverlay>
      ) : null}

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
                    : "Create a workspace and choose how it should enter the registry."
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
                    },
                  );
                  return;
                }

                createWorkspace.mutate(
                  {
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
                  },
                );
              }}
            >
              <TwoColumnFields>
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
                  Database hosting
                  <TextInput
                    value={form.databaseName}
                    onChange={(event) => updateForm("databaseName", event.target.value)}
                    placeholder="agentfs-dev-us-east-1"
                  />
                </Field>
              </TwoColumnFields>

              <Field>
                Description
                <TextArea
                  value={form.description}
                  onChange={(event) => updateForm("description", event.target.value)}
                  placeholder="What this workspace is for, who owns it, and why it exists."
                />
              </Field>

              <TwoColumnFields>
                <Field>
                  Cloud account
                  <TextInput
                    value={form.cloudAccount}
                    onChange={(event) => updateForm("cloudAccount", event.target.value)}
                  />
                </Field>
                <Field>
                  Region
                  <TextInput
                    value={form.region}
                    onChange={(event) => updateForm("region", event.target.value)}
                  />
                </Field>
              </TwoColumnFields>

              <Field>
                Source
                <Select
                  disabled={isEditing}
                  value={form.source}
                  onChange={(event) => updateForm("source", event.target.value as AFSWorkspaceSource)}
                >
                  <option value="blank">Blank workspace</option>
                  <option value="git-import">Git import</option>
                  <option value="cloud-import">Redis Cloud import</option>
                </Select>
              </Field>

              <DialogActions>
                <Button disabled={formBusy} size="medium" type="submit">
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
            </FormGrid>
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
  padding: 24px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(249, 251, 255, 0.94)),
    var(--afs-panel);
  box-shadow: var(--afs-shadow);
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
    $active ? "var(--afs-accent-soft)" : "rgba(255, 255, 255, 0.74)"};
  text-align: left;
  cursor: pointer;
  transition:
    transform 160ms ease,
    border-color 160ms ease,
    background 160ms ease,
    opacity 160ms ease;

  &:hover:enabled {
    transform: translateY(-1px);
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

const DialogActions = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
`;

const InlineButtonRow = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
`;

const DetailPanelOverlay = styled(DialogOverlay)`
  justify-content: flex-end;
  padding: 24px;
`;

const DetailPanel = styled(DialogCard)`
  width: min(920px, 100%);
`;

const DetailBrowserGrid = styled.div`
  display: grid;
  gap: 18px;
  grid-template-columns: minmax(280px, 320px) minmax(0, 1fr);

  @media (max-width: 1100px) {
    grid-template-columns: 1fr;
  }
`;

const BrowserCard = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 24px;
  padding: 18px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.84), rgba(243, 246, 251, 0.92)),
    rgba(255, 255, 255, 0.8);
`;

const PanelNote = styled.p`
  margin: 14px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const FileButtonHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: center;
`;

const FileKind = styled.span`
  display: inline-flex;
  align-items: center;
  padding: 4px 8px;
  border-radius: 999px;
  background: rgba(8, 6, 13, 0.08);
  color: var(--afs-muted);
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const DetailStack = styled.div`
  display: grid;
  gap: 16px;
`;

function parentPath(value: string) {
  if (value === "/" || value === "") {
    return "/";
  }
  const parts = value.split("/").filter(Boolean);
  parts.pop();
  return parts.length === 0 ? "/" : `/${parts.join("/")}`;
}

function formatItemSize(size: number) {
  if (size === 0) {
    return "0 KB";
  }
  return new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 1,
  }).format(size / 1024) + " KB";
}
