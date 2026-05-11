import { Button, Select } from "@redis-ui/components";
import { useNavigate } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type { FormEvent } from "react";
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
  TextInput,
} from "../../../components/afs-kit";
import { formatBytes } from "../../../foundation/api/afs";
import {
  useDatabaseScope,
  useScopedWorkspaceSummaries,
} from "../../../foundation/database-scope";
import {
  useCreateWorkspaceCompositionMutation,
  useCreateWorkspaceMutation,
  useWorkspaceCompositions,
} from "../../../foundation/hooks/use-afs";
import { WorkspaceCompositionTable } from "../../../foundation/tables/workspace-composition-table";
import type { AFSWorkspaceCompositionSummary } from "../../../foundation/types/afs";
import type { MountMode, WorkspaceOption } from "./types";

export function AgentProfilesTab() {
  const navigate = useNavigate();
  const workspacesQuery = useWorkspaceCompositions();
  const [createDialogOpen, setCreateDialogOpen] = useState(false);

  function openWorkspace(workspace: AFSWorkspaceCompositionSummary) {
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: workspace.id },
    });
  }

  return (
    <>
      <WorkspaceCompositionTable
        rows={workspacesQuery.data ?? []}
        loading={workspacesQuery.isLoading}
        error={workspacesQuery.isError}
        onOpenWorkspace={openWorkspace}
        toolbarAction={
          <Button size="medium" onClick={() => setCreateDialogOpen(true)}>
            <Plus size={16} strokeWidth={2} aria-hidden="true" />
            &nbsp;Add Agent Workspace
          </Button>
        }
      />
      <CreateAgentWorkspaceDialog
        open={createDialogOpen}
        onClose={() => setCreateDialogOpen(false)}
      />
    </>
  );
}

type CreateDialogProps = {
  open: boolean;
  onClose: () => void;
};

function mountPathForVolume(name: string) {
  return "/" + name.trim().replace(/^\/+/, "").replace(/\s+/g, "-");
}

function CreateAgentWorkspaceDialog({ open, onClose }: CreateDialogProps) {
  const { databases } = useDatabaseScope();
  const volumesQuery = useScopedWorkspaceSummaries();
  const createWorkspace = useCreateWorkspaceCompositionMutation();
  const createVolume = useCreateWorkspaceMutation();

  const volumeOptions = useMemo<WorkspaceOption[]>(
    () =>
      volumesQuery.data.map((volume) => ({
        id: volume.id,
        name: volume.name,
        files: volume.fileCount,
        size: volume.totalBytes === 0 ? "0 KB" : formatBytes(volume.totalBytes),
      })),
    [volumesQuery.data],
  );

  const eligibleDatabases = useMemo(
    () => databases.filter((database) => database.canCreateWorkspaces),
    [databases],
  );
  const defaultDatabase = eligibleDatabases.find((database) => database.isDefault);
  const defaultDatabaseId =
    defaultDatabase != null
      ? defaultDatabase.id
      : eligibleDatabases.length > 0
        ? eligibleDatabases[0].id
        : "";

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [databaseId, setDatabaseId] = useState("");
  const [selectedVolumeIds, setSelectedVolumeIds] = useState<string[]>([]);
  // Per-volume mount mode; absent keys default to read-only.
  const [mountModes, setMountModes] = useState<Record<string, MountMode>>({});
  const [formError, setFormError] = useState<string | null>(null);
  // Inline new-volume form state.
  const [newVolumeName, setNewVolumeName] = useState("");
  const [newVolumeError, setNewVolumeError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setName("");
    setDescription("");
    setSelectedVolumeIds([]);
    setMountModes({});
    setFormError(null);
    setNewVolumeName("");
    setNewVolumeError(null);
    setDatabaseId(defaultDatabaseId);
  }, [defaultDatabaseId, open]);

  if (!open) return null;

  const busy = createWorkspace.isPending;

  function closeDialog() {
    if (busy) return;
    onClose();
  }

  function toggleVolume(volumeId: string) {
    setSelectedVolumeIds((current) =>
      current.includes(volumeId)
        ? current.filter((id) => id !== volumeId)
        : [...current, volumeId],
    );
  }

  function setVolumeMode(volumeId: string, mode: MountMode) {
    setMountModes((current) => ({ ...current, [volumeId]: mode }));
  }

  async function addNewVolume() {
    const trimmed = newVolumeName.trim();
    if (trimmed === "" || createVolume.isPending) return;
    setNewVolumeError(null);
    try {
      const volumeDb = databaseId || defaultDatabaseId || undefined;
      const databaseName =
        eligibleDatabases.find((database) => database.id === volumeDb)
          ?.databaseName ?? "";
      const result = await createVolume.mutateAsync({
        databaseId: volumeDb,
        name: trimmed,
        description: "",
        cloudAccount: "Direct Redis",
        databaseName,
        region: "",
        source: "blank",
      });
      setSelectedVolumeIds((current) =>
        current.includes(result.id) ? current : [...current, result.id],
      );
      setMountModes((current) => ({ ...current, [result.id]: "r" }));
      setNewVolumeName("");
    } catch (error) {
      setNewVolumeError(
        error instanceof Error ? error.message : "Unable to create the volume.",
      );
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;

    const trimmedName = name.trim();
    if (trimmedName === "") {
      setFormError("Agent Workspace name is required.");
      return;
    }

    setFormError(null);
    try {
      await createWorkspace.mutateAsync({
        name: trimmedName,
        description: description.trim() || undefined,
        databaseId: databaseId || undefined,
        mounts: selectedVolumeIds.flatMap((volumeId) => {
          const volume = volumeOptions.find((item) => item.id === volumeId);
          if (volume == null) return [];
          return [
            {
              volumeId: volume.id,
              volumeName: volume.name,
              mountPath: mountPathForVolume(volume.name),
              readonly: (mountModes[volume.id] ?? "r") === "r",
            },
          ];
        }),
      });
      onClose();
    } catch (error) {
      setFormError(
        error instanceof Error
          ? error.message
          : "Unable to create the Agent Workspace.",
      );
    }
  }

  const submitLabel =
    selectedVolumeIds.length === 0
      ? "Create Agent Workspace"
      : selectedVolumeIds.length === 1
        ? "Create with 1 volume"
        : `Create with ${selectedVolumeIds.length} volumes`;

  return (
    <DialogOverlay
      role="dialog"
      aria-modal="true"
      aria-labelledby="create-agent-workspace-title"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) closeDialog();
      }}
    >
      <DialogCard>
        <DialogHeader>
          <div>
            <DialogTitle id="create-agent-workspace-title">
              Add Agent Workspace
            </DialogTitle>
            <DialogBody>
              Name the workspace, then optionally mount existing volumes at
              creation.
            </DialogBody>
          </div>
          <DialogCloseButton
            type="button"
            aria-label="Close"
            onClick={closeDialog}
          >
            &times;
          </DialogCloseButton>
        </DialogHeader>

        <FormGrid onSubmit={submit}>
          <Field>
            Name
            <TextInput
              autoFocus
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="coding-agent"
            />
          </Field>

          <Field>
            Description
            <TextInput
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              placeholder="What this agent can read and write. (optional)"
            />
          </Field>

          {eligibleDatabases.length > 1 ? (
            <Field>
              Database
              <Select
                options={eligibleDatabases.map((database) => ({
                  value: database.id,
                  label: `${database.displayName || database.databaseName}${database.isDefault ? " (default)" : ""}`,
                }))}
                value={databaseId}
                onChange={(next) => setDatabaseId(next)}
              />
            </Field>
          ) : null}

          <Field>
            Initial volumes
            <NewVolumeRow>
              <NewVolumeInput
                value={newVolumeName}
                placeholder="new-volume-name"
                onChange={(event) => setNewVolumeName(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    void addNewVolume();
                  }
                }}
                disabled={createVolume.isPending}
              />
              <Button
                size="medium"
                type="button"
                variant="secondary-fill"
                onClick={() => void addNewVolume()}
                disabled={
                  createVolume.isPending || newVolumeName.trim() === ""
                }
              >
                {createVolume.isPending ? "Creating..." : "+ Create volume"}
              </Button>
            </NewVolumeRow>
            {newVolumeError ? (
              <DialogError role="alert">{newVolumeError}</DialogError>
            ) : null}
            {volumeOptions.length === 0 ? (
              <EmptyVolumes>
                No volumes are available yet. Create one with the field above
                or save this Agent Workspace and add volumes later.
              </EmptyVolumes>
            ) : (
              <VolumeList>
                {volumeOptions.map((volume) => {
                  const selected = selectedVolumeIds.includes(volume.id);
                  const mode = mountModes[volume.id] ?? "r";
                  return (
                    <VolumeOption key={volume.id} $selected={selected}>
                      <input
                        type="checkbox"
                        checked={selected}
                        aria-label={`Select ${volume.name}`}
                        onChange={() => toggleVolume(volume.id)}
                      />
                      <VolumeOptionMain>
                        <VolumeName>/{volume.name}</VolumeName>
                        <VolumeMeta>
                          {volume.name} &middot; {volume.id}
                        </VolumeMeta>
                      </VolumeOptionMain>
                      <VolumeStats>
                        {volume.files.toLocaleString()} files &middot; {volume.size}
                      </VolumeStats>
                      <VolumeModeSelect
                        onClick={(event) => event.preventDefault()}
                      >
                        <Select
                          aria-label={`Permission for ${volume.name}`}
                          options={[
                            { value: "r", label: "Read only" },
                            { value: "rw", label: "Read / write" },
                          ]}
                          value={mode}
                          onChange={(value) =>
                            setVolumeMode(volume.id, value as MountMode)
                          }
                        />
                      </VolumeModeSelect>
                    </VolumeOption>
                  );
                })}
              </VolumeList>
            )}
          </Field>

          {formError ? <DialogError role="alert">{formError}</DialogError> : null}

          <DialogActions style={{ justifyContent: "flex-end" }}>
            <Button
              size="medium"
              type="button"
              variant="secondary-fill"
              onClick={closeDialog}
              disabled={busy}
            >
              Cancel
            </Button>
            <Button size="medium" type="submit" disabled={busy}>
              {busy ? "Creating..." : submitLabel}
            </Button>
          </DialogActions>
        </FormGrid>
      </DialogCard>
    </DialogOverlay>
  );
}

const NewVolumeRow = styled.div`
  display: flex;
  gap: 8px;
  align-items: stretch;
`;

const NewVolumeInput = styled.input`
  flex: 1 1 auto;
  min-width: 0;
  padding: 8px 12px;
  border: 1px solid var(--afs-line);
  border-radius: 8px;
  background: var(--afs-panel);
  color: var(--afs-ink);
  font: inherit;
  font-size: 13px;

  &:focus-visible {
    outline: 2px solid var(--afs-selection-border);
    outline-offset: 1px;
  }

  &::placeholder {
    color: var(--afs-muted);
  }

  &:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
`;

const VolumeList = styled.div`
  display: flex;
  flex-direction: column;
  gap: 8px;
  /* Cap to ~3–4 rows so very long volume lists scroll instead of expanding
     the dialog past the viewport. */
  max-height: 280px;
  overflow-y: auto;
  padding-right: 4px;
`;

const VolumeOption = styled.label<{ $selected: boolean }>`
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto auto;
  gap: 12px;
  align-items: center;
  padding: 12px;
  border: 1px solid
    ${({ $selected }) =>
      $selected ? "var(--afs-selection-border)" : "var(--afs-line)"};
  border-radius: 8px;
  background: ${({ $selected }) =>
    $selected ? "var(--afs-selection-bg)" : "var(--afs-panel)"};
  color: ${({ $selected }) => ($selected ? "var(--afs-selection-text)" : "var(--afs-ink)")};
  cursor: pointer;
  transition: background 140ms ease, border-color 140ms ease, color 140ms ease;

  &:hover {
    border-color: var(--afs-selection-border);
    background: ${({ $selected }) => ($selected ? "var(--afs-selection-bg)" : "var(--afs-selection-hover-bg)")};
    color: ${({ $selected }) => ($selected ? "var(--afs-selection-text)" : "var(--afs-selection-hover-ink)")};
  }

  input[type="checkbox"] {
    accent-color: var(--afs-accent);
  }

  @media (max-width: 560px) {
    grid-template-columns: auto minmax(0, 1fr);
  }
`;

const VolumeModeSelect = styled.div`
  min-width: 168px;

  > * {
    width: 100%;
  }

  @media (max-width: 560px) {
    grid-column: 2;
    min-width: 0;
  }
`;

const VolumeOptionMain = styled.span`
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
`;

const VolumeName = styled.span`
  color: var(--afs-ink);
  font-family: var(--afs-mono, monospace);
  font-size: 13.5px;
  font-weight: 700;
`;

const VolumeMeta = styled.span`
  min-width: 0;
  color: var(--afs-muted);
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const VolumeStats = styled.span`
  color: var(--afs-muted);
  font-size: 12px;
  white-space: nowrap;

  @media (max-width: 560px) {
    grid-column: 2;
    white-space: normal;
  }
`;

const EmptyVolumes = styled.div`
  padding: 18px 14px;
  border: 1px solid var(--afs-line);
  border-radius: 8px;
  color: var(--afs-muted);
  background: var(--afs-panel);
  font-size: 13px;
`;
