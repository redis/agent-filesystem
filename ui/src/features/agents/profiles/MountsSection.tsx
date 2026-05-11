import { Button, Select } from "@redis-ui/components";
import { useNavigate } from "@tanstack/react-router";
import { Folder, Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import styled from "styled-components";
import {
  DialogActions,
  DialogBody,
  DialogCard,
  DialogCloseButton,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
  Field,
  TextInput,
  Tag,
} from "../../../components/afs-kit";
import type {
  Filesystem,
  Mount,
  MountMode,
  MountPersistence,
  WorkspaceOption,
} from "./types";

type Props = {
  fs: Filesystem;
  volumes: WorkspaceOption[];
  onAddVolumes: (mounts: Mount[]) => void;
  onUpdateRow: (
    kind: MountPersistence,
    idx: number,
    patch: { wsId?: string; mount?: string; mode?: MountMode },
  ) => void;
  onRemoveRow: (kind: MountPersistence, idx: number) => void;
};

type RowWithMeta = Mount & { kind: MountPersistence; idx: number };

function bytesFromSize(s: string | undefined): number {
  if (!s) return 0;
  const m = s.match(/([\d.]+)\s*(B|KB|MB|GB)?/i);
  if (!m) return 0;
  const n = parseFloat(m[1]);
  const u = (m[2] || "B").toUpperCase();
  const factors: Record<string, number> = {
    B: 1,
    KB: 1024,
    MB: 1024 ** 2,
    GB: 1024 ** 3,
  };
  return n * (factors[u] || 1);
}

function formatSize(b: number): string {
  if (b < 1024) return `${b} B`;
  if (b < 1024 ** 2)
    return `${(b / 1024).toFixed(b < 10240 ? 1 : 0)} KB`;
  if (b < 1024 ** 3) return `${(b / 1024 ** 2).toFixed(1)} MB`;
  return `${(b / 1024 ** 3).toFixed(2)} GB`;
}

export function MountsSection({
  fs,
  volumes,
  onAddVolumes,
  onUpdateRow,
  onRemoveRow,
}: Props) {
  const navigate = useNavigate();
  const [editing, setEditing] = useState<{
    kind: MountPersistence;
    idx: number;
  } | null>(null);
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [selectedVolumeIds, setSelectedVolumeIds] = useState<string[]>([]);
  // Per-volume mode in the Add Volumes dialog. Keys are volume ids; missing
  // entries default to read-only.
  const [addModes, setAddModes] = useState<Record<string, MountMode>>({});

  const rows: RowWithMeta[] = useMemo(() => {
    const out: RowWithMeta[] = [];
    fs.shared.forEach((r, i) => out.push({ ...r, kind: "shared", idx: i }));
    fs.perRun.forEach((r, i) => out.push({ ...r, kind: "perRun", idx: i }));
    return out;
  }, [fs]);

  const knownVolumes = useMemo(() => {
    const merged = new Map<string, WorkspaceOption>();
    for (const volume of volumes) {
      merged.set(volume.id, volume);
    }
    return Array.from(merged.values());
  }, [volumes]);

  const usedIds = new Set([...fs.shared, ...fs.perRun].map((m) => m.wsId));
  const addableVolumes = volumes.filter((volume) => !usedIds.has(volume.id));

  function availableFor(currentId: string) {
    return knownVolumes.filter(
      (w) => !usedIds.has(w.id) || w.id === currentId,
    );
  }

  const totalFiles = rows.reduce(
    (s, r) =>
      s + (knownVolumes.find((w) => w.id === r.wsId)?.files ?? 0),
    0,
  );
  const totalBytes = rows.reduce(
    (s, r) =>
      s +
      bytesFromSize(knownVolumes.find((w) => w.id === r.wsId)?.size),
    0,
  );

  function openAddDialog() {
    setSelectedVolumeIds([]);
    setAddModes({});
    setAddDialogOpen(true);
  }

  function closeAddDialog() {
    setAddDialogOpen(false);
  }

  function toggleVolume(volumeId: string) {
    setSelectedVolumeIds((current) =>
      current.includes(volumeId)
        ? current.filter((id) => id !== volumeId)
        : [...current, volumeId],
    );
  }

  function setVolumeMode(volumeId: string, mode: MountMode) {
    setAddModes((current) => ({ ...current, [volumeId]: mode }));
  }

  function addSelectedVolumes() {
    const mounts = selectedVolumeIds.flatMap((volumeId) => {
      const volume = addableVolumes.find((item) => item.id === volumeId);
      if (volume == null) return [];
      return [
        {
          wsId: volume.id,
          mount: "/" + volume.name.replace(/^\/+/, ""),
          mode: addModes[volume.id] ?? "r",
        },
      ];
    });

    if (mounts.length === 0) return;
    onAddVolumes(mounts);
    closeAddDialog();
  }

  const addButtonLabel =
    selectedVolumeIds.length === 0
      ? "Add volumes"
      : selectedVolumeIds.length === 1
        ? "Add volume"
        : `Add ${selectedVolumeIds.length} volumes`;

  return (
    <Stack>
      <SectionToolbar>
        <SectionTitle>Volumes</SectionTitle>
        <Button size="medium" onClick={openAddDialog}>
          <Plus size={16} strokeWidth={2} aria-hidden="true" />
          &nbsp;Add Volume
        </Button>
      </SectionToolbar>

      <RowList>
        <RootRow>
          <Folder size={16} strokeWidth={1.75} aria-hidden="true" />
          <span>
            <strong>/</strong> &mdash; filesystem root
          </span>
          <RootMeta>
            {rows.length} {rows.length === 1 ? "volume" : "volumes"} ·{" "}
            {totalFiles.toLocaleString()}{" "}
            {totalFiles === 1 ? "file" : "files"} ·{" "}
            {formatSize(totalBytes)}
          </RootMeta>
        </RootRow>

        {rows.length === 0 ? (
          <EmptyHint>
            No volumes yet. Add a volume to start building this agent's filesystem.
          </EmptyHint>
        ) : null}

        {rows.map((row, index) => {
          const ws = knownVolumes.find((w) => w.id === row.wsId);
          const isEditing =
            editing != null &&
            editing.kind === row.kind &&
            editing.idx === row.idx;
          const openVolume = () => {
            if (!ws || isEditing) return;
            void navigate({
              to: "/volumes/$volumeId",
              params: { volumeId: ws.id },
            });
          };
          return (
            <MountRow
              key={`${row.kind}-${row.idx}`}
              $editing={isEditing}
              $isLast={index === rows.length - 1}
              $clickable={ws != null && !isEditing}
              role={ws != null && !isEditing ? "button" : undefined}
              tabIndex={ws != null && !isEditing ? 0 : undefined}
              aria-label={
                ws != null && !isEditing
                  ? `Open volume ${ws.name}`
                  : undefined
              }
              onClick={openVolume}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  openVolume();
                }
              }}
            >
              <RowMain>
                <Folder size={18} strokeWidth={1.75} aria-hidden="true" />
                <RowTitle>
                  <RowPath>{row.mount}</RowPath>
                  <RowSub>
                    {ws ? (
                      <VolumeLinkButton
                        type="button"
                        title={`Open volume ${ws.name}`}
                        onClick={(event) => {
                          event.stopPropagation();
                          void navigate({
                            to: "/volumes/$volumeId",
                            params: { volumeId: ws.id },
                          });
                        }}
                      >
                        {ws.name}
                      </VolumeLinkButton>
                    ) : (
                      <strong>{ws?.name}</strong>
                    )}
                    {" · "}
                    <span>{ws?.id}</span>
                  </RowSub>
                </RowTitle>
              </RowMain>

              <RowFiles>
                <RowFilesValue>
                  {ws?.files.toLocaleString() ?? "—"}
                </RowFilesValue>
                <RowFilesLabel>
                  files · {ws?.size ?? "—"}
                </RowFilesLabel>
              </RowFiles>

              <RowBadges>
                <Tag>{row.mode === "rw" ? "read / write" : "Read-Only"}</Tag>
              </RowBadges>

              <RowActions onClick={(event) => event.stopPropagation()}>
                <IconButton
                  type="button"
                  aria-label="Edit mount"
                  onClick={(event) => {
                    event.stopPropagation();
                    setEditing(
                      isEditing ? null : { kind: row.kind, idx: row.idx },
                    );
                  }}
                >
                  <Pencil size={16} strokeWidth={1.75} aria-hidden="true" />
                </IconButton>
                <IconButton
                  type="button"
                  aria-label="Remove mount"
                  onClick={(event) => {
                    event.stopPropagation();
                    onRemoveRow(row.kind, row.idx);
                    if (isEditing) setEditing(null);
                  }}
                >
                  <Trash2 size={16} strokeWidth={1.75} aria-hidden="true" />
                </IconButton>
              </RowActions>

              {isEditing ? (
                <EditPanel>
                  <Field>
                    Volume
                    <Select
                      options={availableFor(row.wsId).map((w) => ({
                        value: w.id,
                        label: w.name,
                      }))}
                      value={row.wsId}
                      onChange={(value) => {
                        const next = knownVolumes.find((w) => w.id === value);
                        onUpdateRow(row.kind, row.idx, {
                          wsId: value,
                          mount: "/" + (next?.name ?? "mount"),
                        });
                      }}
                    />
                  </Field>
                  <Field>
                    Mount path
                    <TextInput
                      value={row.mount}
                      onChange={(event) =>
                        onUpdateRow(row.kind, row.idx, {
                          mount: event.target.value,
                        })
                      }
                    />
                  </Field>
                  <Field>
                    Permissions
                    <Select
                      options={[
                        { value: "r", label: "Read only" },
                        { value: "rw", label: "Read / write" },
                      ]}
                      value={row.mode}
                      onChange={(value) =>
                        onUpdateRow(row.kind, row.idx, {
                          mode: value as MountMode,
                        })
                      }
                    />
                  </Field>
                </EditPanel>
              ) : null}
            </MountRow>
          );
        })}
      </RowList>

      {addDialogOpen ? (
        <DialogOverlay
          role="dialog"
          aria-modal="true"
          aria-labelledby="add-volumes-title"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) {
              closeAddDialog();
            }
          }}
        >
          <DialogCard>
            <DialogHeader>
              <div>
                <DialogTitle id="add-volumes-title">Add volumes</DialogTitle>
                <DialogBody>
                  Pick the volumes to mount and set the permission for each.
                  Read-only mounts block writes for every agent and key bound
                  to this workspace.
                </DialogBody>
              </div>
              <DialogCloseButton type="button" onClick={closeAddDialog}>
                &times;
              </DialogCloseButton>
            </DialogHeader>

            <WizardBody>
              <Field>
                Volumes
                {addableVolumes.length === 0 ? (
                  <EmptyVolumes>All available volumes are already mounted.</EmptyVolumes>
                ) : (
                  <VolumeList>
                    {addableVolumes.map((volume) => {
                      const selected = selectedVolumeIds.includes(volume.id);
                      const mode = addModes[volume.id] ?? "r";
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
                              {volume.name} · {volume.id}
                            </VolumeMeta>
                          </VolumeOptionMain>
                          <VolumeStats>
                            {volume.files.toLocaleString()} files · {volume.size}
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
            </WizardBody>

            <DialogActions>
              <Button
                size="medium"
                type="button"
                variant="secondary-fill"
                onClick={closeAddDialog}
              >
                Cancel
              </Button>
              <Button
                size="medium"
                type="button"
                disabled={selectedVolumeIds.length === 0}
                onClick={addSelectedVolumes}
              >
                {addButtonLabel}
              </Button>
            </DialogActions>
          </DialogCard>
        </DialogOverlay>
      ) : null}

    </Stack>
  );
}

const Stack = styled.div`
  display: flex;
  flex-direction: column;
  gap: 16px;
`;

const SectionToolbar = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;

  @media (max-width: 560px) {
    align-items: stretch;
    flex-direction: column;
  }
`;

const SectionTitle = styled.h2`
  margin: 0;
  color: var(--afs-ink);
  font-size: 16px;
  font-weight: 700;
  line-height: 1.2;
  letter-spacing: 0;
`;

const RowList = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 12px;
  overflow: hidden;
  background: var(--afs-panel);
`;

const RootRow = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--afs-line);
  color: var(--afs-muted);
  font-size: 13px;

  strong {
    color: var(--afs-ink);
  }
`;

const RootMeta = styled.span`
  margin-left: auto;
  font-size: 12px;
  color: var(--afs-muted);
`;

const EmptyHint = styled.div`
  padding: 28px 16px;
  text-align: center;
  color: var(--afs-muted);
  font-size: 13px;
`;

const MountRow = styled.div<{
  $editing: boolean;
  $isLast: boolean;
  $clickable?: boolean;
}>`
  position: relative;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto auto;
  gap: 16px;
  align-items: center;
  padding: 12px 14px 12px 54px;
  border-bottom: 1px solid var(--afs-line);
  background: ${({ $editing }) =>
    $editing ? "var(--afs-bg-soft)" : "transparent"};
  cursor: ${({ $clickable }) => ($clickable ? "pointer" : "default")};
  transition: background 120ms ease;

  ${({ $clickable, $editing }) =>
    $clickable && !$editing
      ? `&:hover { background: var(--afs-bg-soft); }
         &:focus-visible {
           outline: 2px solid var(--afs-selection-border);
           outline-offset: -2px;
         }`
      : ""}

  &::before,
  &::after {
    content: "";
    position: absolute;
    background: var(--afs-line-strong);
    pointer-events: none;
  }

  &::before {
    left: 28px;
    top: 0;
    bottom: ${({ $isLast }) => ($isLast ? "calc(100% - 28px)" : "0")};
    width: 1px;
  }

  &::after {
    left: 28px;
    top: 28px;
    width: 18px;
    height: 1px;
  }

  &:last-child {
    border-bottom: 0;
  }

  @media (max-width: 760px) {
    grid-template-columns: minmax(0, 1fr) auto;
    padding-left: 46px;
    row-gap: 8px;

    &::before {
      left: 22px;
    }

    &::after {
      left: 22px;
      width: 16px;
    }
  }
`;

const RowMain = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  color: var(--afs-muted);
`;

const RowTitle = styled.div`
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
`;

const RowPath = styled.span`
  color: var(--afs-ink);
  font-weight: 700;
  font-family: var(--afs-mono, monospace);
  font-size: 13.5px;
`;

const RowSub = styled.span`
  color: var(--afs-muted);
  font-size: 12px;

  strong {
    color: var(--afs-ink-soft);
    font-weight: 600;
  }
`;

const VolumeLinkButton = styled.button`
  appearance: none;
  border: 0;
  background: transparent;
  padding: 0;
  color: var(--afs-accent, #2563eb);
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;

  &:hover,
  &:focus-visible {
    text-decoration: underline;
    outline: none;
  }
`;

const RowFiles = styled.div`
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
  color: var(--afs-ink-soft);

  @media (max-width: 760px) {
    align-items: flex-start;
  }
`;

const RowFilesValue = styled.span`
  font-family: var(--afs-mono, monospace);
  font-size: 13px;
`;

const RowFilesLabel = styled.span`
  color: var(--afs-muted);
  font-size: 11px;
`;

const WizardBody = styled.div`
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-bottom: 20px;
`;

const VolumeList = styled.div`
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 300px;
  overflow: auto;
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

const RowBadges = styled.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
`;

const RowActions = styled.div`
  display: flex;
  gap: 4px;

  @media (max-width: 760px) {
    grid-column: 1 / -1;
    justify-content: flex-end;
  }
`;

const IconButton = styled.button`
  width: 32px;
  height: 32px;
  border-radius: 8px;
  border: 1px solid var(--afs-line);
  background: var(--afs-panel);
  color: var(--afs-ink-soft);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background 140ms ease, border-color 140ms ease;

  &:hover {
    background: var(--afs-bg-soft);
    border-color: var(--afs-line-strong);
  }
`;

const EditPanel = styled.div`
  grid-column: 1 / -1;
  margin-top: 6px;
  padding: 14px;
  border-top: 1px solid var(--afs-line);
  background: var(--afs-bg-soft);
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;

  @media (max-width: 760px) {
    grid-template-columns: 1fr;
  }
`;
