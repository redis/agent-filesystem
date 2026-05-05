import { Button, Select } from "@redis-ui/components";
import { useEffect, useState } from "react";
import styled from "styled-components";
import {
  DialogActions,
  DialogError,
  Field,
  FormGrid,
  TextArea,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
  TextInput,
} from "../../components/afs-kit";
import { SurfaceCard } from "../../components/card-shell";
import {
  useUpdateWorkspaceVersioningPolicyMutation,
  useWorkspaceVersioningPolicy,
} from "../../foundation/hooks/use-afs";
import type {
  AFSMCPToken,
  AFSWorkspaceContentStorage,
  AFSWorkspaceDetail,
  AFSWorkspaceSearchIndex,
} from "../../foundation/types/afs";

type Props = {
  workspace: AFSWorkspaceDetail;
  onSave: (input: {
    name: string;
    description: string;
  }) => void | Promise<void>;
  isSaving: boolean;
  saveError?: string | null;
  onDelete: () => void;
  isDeleting: boolean;
  mcpTokens: AFSMCPToken[];
  onOpenMCPConsole: () => void;
};

export function SettingsTab({
  workspace,
  onSave,
  isSaving,
  saveError,
  onDelete,
  isDeleting,
  mcpTokens,
  onOpenMCPConsole,
}: Props) {
  const activeTokens = mcpTokens.filter(
    (token) => token.revokedAt == null || token.revokedAt === "",
  );
  const activeToken = activeTokens.at(0);
  const tokenCount = activeTokens.length;
  const versioningQuery = useWorkspaceVersioningPolicy({
    databaseId: workspace.databaseId,
    workspaceId: workspace.id,
  });
  const updateVersioning = useUpdateWorkspaceVersioningPolicyMutation();
  const [versioningMode, setVersioningMode] = useState<"off" | "all" | "paths">(
    "off",
  );
  const [includeGlobsText, setIncludeGlobsText] = useState("");
  const [excludeGlobsText, setExcludeGlobsText] = useState("");
  const [maxVersionsPerFile, setMaxVersionsPerFile] = useState("0");
  const [maxAgeDays, setMaxAgeDays] = useState("0");
  const [maxTotalBytes, setMaxTotalBytes] = useState("0");
  const [largeFileCutoffBytes, setLargeFileCutoffBytes] = useState("0");
  const [versioningError, setVersioningError] = useState<string | null>(null);
  const [versioningNotice, setVersioningNotice] = useState<string | null>(null);

  useEffect(() => {
    if (!versioningQuery.data) {
      return;
    }
    setVersioningMode(versioningQuery.data.mode);
    setIncludeGlobsText(versioningQuery.data.includeGlobs.join("\n"));
    setExcludeGlobsText(versioningQuery.data.excludeGlobs.join("\n"));
    setMaxVersionsPerFile(String(versioningQuery.data.maxVersionsPerFile));
    setMaxAgeDays(String(versioningQuery.data.maxAgeDays));
    setMaxTotalBytes(String(versioningQuery.data.maxTotalBytes));
    setLargeFileCutoffBytes(String(versioningQuery.data.largeFileCutoffBytes));
  }, [versioningQuery.data]);

  return (
    <SectionGrid>
      <SectionCard $span={12}>
        <SectionHeader>
          <SectionTitle title="Workspace details" />
        </SectionHeader>

        <FormGrid
          onSubmit={(event) => {
            event.preventDefault();
            const form = new FormData(event.currentTarget);
            const name = String(form.get("name") ?? "").trim();
            const description = String(form.get("description") ?? "").trim();
            void onSave({ name, description });
          }}
        >
          <Field>
            Workspace name
            <TextInput
              name="name"
              defaultValue={workspace.name}
              placeholder="customer-portal"
            />
          </Field>

          <Field>
            Description
            <TextInput
              name="description"
              defaultValue={workspace.description}
              placeholder="What this workspace is for, who owns it, and why it exists."
            />
          </Field>

          {saveError ? (
            <DialogError role="alert">{saveError}</DialogError>
          ) : null}

          <DialogActions style={{ justifyContent: "flex-end" }}>
            <Button size="medium" type="submit" disabled={isSaving}>
              {isSaving ? "Saving..." : "Save changes"}
            </Button>
          </DialogActions>
        </FormGrid>

        <MetaTable>
          <tbody>
            <MetaRow>
              <MetaLabel>Workspace ID</MetaLabel>
              <MetaValue>
                <MonoValue>{workspace.id}</MonoValue>
              </MetaValue>
            </MetaRow>
            <MetaRow>
              <MetaLabel>Database</MetaLabel>
              <MetaValue>{workspace.databaseName}</MetaValue>
            </MetaRow>
            {workspace.contentStorage ? (
              <MetaRow>
                <MetaLabel>File storage</MetaLabel>
                <MetaValue>
                  <StorageSummary>
                    <StorageBadge $profile={workspace.contentStorage.profile}>
                      {storageProfileLabel(workspace.contentStorage)}
                    </StorageBadge>
                    <StorageText>
                      {storageProfileDescription(workspace.contentStorage)}
                    </StorageText>
                  </StorageSummary>
                </MetaValue>
              </MetaRow>
            ) : null}
            {workspace.searchIndex ? (
              <MetaRow>
                <MetaLabel>Search index</MetaLabel>
                <MetaValue>
                  <StorageSummary>
                    <StatusBadge
                      $tone={searchIndexTone(workspace.searchIndex)}
                    >
                      {searchIndexLabel(workspace.searchIndex)}
                    </StatusBadge>
                    <StorageText>
                      {searchIndexDescription(workspace.searchIndex)}
                    </StorageText>
                    {workspace.searchIndex.present ? (
                      <MonoValue>{workspace.searchIndex.name}</MonoValue>
                    ) : null}
                  </StorageSummary>
                </MetaValue>
              </MetaRow>
            ) : null}
            {workspace.mountedPath ? (
              <MetaRow>
                <MetaLabel>Mounted path</MetaLabel>
                <MetaValue>{workspace.mountedPath}</MetaValue>
              </MetaRow>
            ) : null}
          </tbody>
        </MetaTable>
      </SectionCard>

      <SectionCard $span={12}>
        <SectionHeader>
          <SectionTitle title="Transparent file versioning" />
        </SectionHeader>

        <VersioningCopy>
          The live file tree still shows only the latest workspace state. This
          policy controls which paths get immutable per-file history behind the
          scenes and how aggressively old versions are retained.
        </VersioningCopy>

        <FormGrid
          onSubmit={(event) => {
            event.preventDefault();
            try {
              setVersioningError(null);
              setVersioningNotice(null);
              const nextPolicy = {
                mode: versioningMode,
                includeGlobs: splitGlobList(includeGlobsText),
                excludeGlobs: splitGlobList(excludeGlobsText),
                maxVersionsPerFile: parseWholeNumber(
                  maxVersionsPerFile,
                  "Max versions per file",
                ),
                maxAgeDays: parseWholeNumber(maxAgeDays, "Max age (days)"),
                maxTotalBytes: parseWholeNumber(
                  maxTotalBytes,
                  "Workspace budget (bytes)",
                ),
                largeFileCutoffBytes: parseWholeNumber(
                  largeFileCutoffBytes,
                  "Large file cutoff (bytes)",
                ),
              } as const;
              void updateVersioning
                .mutateAsync({
                  databaseId: workspace.databaseId,
                  workspaceId: workspace.id,
                  policy: nextPolicy,
                })
                .then(() => {
                  setVersioningNotice("Versioning policy saved.");
                })
                .catch((error) => {
                  setVersioningError(
                    error instanceof Error
                      ? error.message
                      : "Unable to save versioning policy.",
                  );
                });
            } catch (error) {
              setVersioningError(
                error instanceof Error
                  ? error.message
                  : "Unable to parse versioning policy.",
              );
            }
          }}
        >
          <ToggleRow>
            <ToggleText>
              <strong>Enable file versioning</strong>
              <span>
                Turning this off keeps the working copy unchanged but stops
                automatic version capture for future writes.
              </span>
            </ToggleText>
            <ToggleSwitchLabel>
              <ToggleSwitch>
                <ToggleCheckbox
                  type="checkbox"
                  checked={versioningMode !== "off"}
                  onChange={(event) => {
                    setVersioningNotice(null);
                    setVersioningError(null);
                    setVersioningMode(
                      event.currentTarget.checked
                        ? versioningMode === "off"
                          ? "all"
                          : versioningMode
                        : "off",
                    );
                  }}
                />
                <ToggleTrack />
              </ToggleSwitch>
              <ToggleState>
                {versioningMode === "off" ? "Off" : "On"}
              </ToggleState>
            </ToggleSwitchLabel>
          </ToggleRow>

          <TwoFieldGrid>
            <Field>
              Tracking mode
              <SelectFieldWrap>
                <Select
                  aria-label="Tracking mode"
                  id="workspace-versioning-mode"
                  options={VERSIONING_MODE_OPTIONS}
                  value={versioningMode}
                  onChange={(next) => {
                    setVersioningNotice(null);
                    setVersioningMode(next as "off" | "all" | "paths");
                  }}
                  placeholder="Tracking mode"
                />
              </SelectFieldWrap>
            </Field>

            <Field>
              Current scope
              <ScopeSummary>
                {versioningMode === "off"
                  ? "No automatic file history will be recorded."
                  : versioningMode === "all"
                    ? "Every tracked file path is versioned unless excluded below."
                    : "Only paths that match the include globs are versioned."}
              </ScopeSummary>
            </Field>
          </TwoFieldGrid>

          <TwoFieldGrid>
            <Field>
              Include globs
              <TextArea
                value={includeGlobsText}
                onChange={(event) => {
                  setVersioningNotice(null);
                  setIncludeGlobsText(event.currentTarget.value);
                }}
                placeholder={"src/**\napp/**/*.ts"}
              />
            </Field>

            <Field>
              Exclude globs
              <TextArea
                value={excludeGlobsText}
                onChange={(event) => {
                  setVersioningNotice(null);
                  setExcludeGlobsText(event.currentTarget.value);
                }}
                placeholder={"**/*.log\nnode_modules/**"}
              />
            </Field>
          </TwoFieldGrid>

          <RetentionGrid>
            <Field>
              Max versions per file
              <TextInput
                value={maxVersionsPerFile}
                onChange={(event) =>
                  setMaxVersionsPerFile(event.currentTarget.value)
                }
                inputMode="numeric"
              />
            </Field>

            <Field>
              Max age (days)
              <TextInput
                value={maxAgeDays}
                onChange={(event) => setMaxAgeDays(event.currentTarget.value)}
                inputMode="numeric"
              />
            </Field>

            <Field>
              Workspace budget (bytes)
              <TextInput
                value={maxTotalBytes}
                onChange={(event) =>
                  setMaxTotalBytes(event.currentTarget.value)
                }
                inputMode="numeric"
              />
            </Field>

            <Field>
              Large file cutoff (bytes)
              <TextInput
                value={largeFileCutoffBytes}
                onChange={(event) =>
                  setLargeFileCutoffBytes(event.currentTarget.value)
                }
                inputMode="numeric"
              />
            </Field>
          </RetentionGrid>

          {versioningQuery.isLoading ? (
            <VersioningStatus>
              Loading current versioning policy…
            </VersioningStatus>
          ) : null}
          {versioningQuery.isError ? (
            <DialogError role="alert">
              {versioningQuery.error instanceof Error
                ? versioningQuery.error.message
                : "Unable to load versioning policy."}
            </DialogError>
          ) : null}
          {versioningError ? (
            <DialogError role="alert">{versioningError}</DialogError>
          ) : null}
          {versioningNotice ? (
            <VersioningNotice role="status">
              {versioningNotice}
            </VersioningNotice>
          ) : null}

          <DialogActions style={{ justifyContent: "flex-end" }}>
            <Button
              size="medium"
              type="submit"
              disabled={updateVersioning.isPending}
            >
              {updateVersioning.isPending
                ? "Saving..."
                : "Save versioning policy"}
            </Button>
          </DialogActions>
        </FormGrid>
      </SectionCard>

      <SectionCard $span={12}>
        <SectionHeader>
          <SectionTitle title="Agent access" />
          <Button size="medium" onClick={onOpenMCPConsole}>
            Open MCP console
          </Button>
        </SectionHeader>

        <AccessCopy>
          MCP setup now lives on the Agents page so you can manage all
          workspace-scoped access tokens and config snippets in one place. This
          panel stays focused on the current workspace and shows whether it
          already has authorized MCP access.
        </AccessCopy>

        <MetaTable>
          <tbody>
            <MetaRow>
              <MetaLabel>Authorized tokens</MetaLabel>
              <MetaValue>
                {tokenCount === 0
                  ? "None yet"
                  : `${tokenCount} active token${tokenCount === 1 ? "" : "s"}`}
              </MetaValue>
            </MetaRow>
            <MetaRow>
              <MetaLabel>Workspace scope</MetaLabel>
              <MetaValue>
                All MCP tokens created from this workspace stay locked to{" "}
                {workspace.name}.
              </MetaValue>
            </MetaRow>
            <MetaRow>
              <MetaLabel>Admin tools</MetaLabel>
              <MetaValue>
                Workspace settings no longer mint admin access tokens. Use the
                access token console for explicit elevated flows.
              </MetaValue>
            </MetaRow>
            {activeToken ? (
              <>
                <MetaRow>
                  <MetaLabel>Latest token</MetaLabel>
                  <MetaValue>
                    {activeToken.name?.trim() || activeToken.id}
                  </MetaValue>
                </MetaRow>
                <MetaRow>
                  <MetaLabel>Last used</MetaLabel>
                  <MetaValue>
                    {activeToken.lastUsedAt
                      ? formatTimestamp(activeToken.lastUsedAt)
                      : "Never"}
                  </MetaValue>
                </MetaRow>
              </>
            ) : null}
          </tbody>
        </MetaTable>

        {activeTokens.length > 0 ? (
          <TokenTable>
            <thead>
              <tr>
                <TokenHead>Name</TokenHead>
                <TokenHead>Profile</TokenHead>
                <TokenHead>Last used</TokenHead>
                <TokenHead>Expires</TokenHead>
              </tr>
            </thead>
            <tbody>
              {activeTokens.slice(0, 5).map((token) => (
                <TokenRow key={token.id}>
                  <TokenCell>
                    <TokenName>{token.name?.trim() || token.id}</TokenName>
                    <TokenSubtle>{token.id}</TokenSubtle>
                  </TokenCell>
                  <TokenCell>{formatProfile(token.profile)}</TokenCell>
                  <TokenCell>
                    {token.lastUsedAt
                      ? formatTimestamp(token.lastUsedAt)
                      : "Never"}
                  </TokenCell>
                  <TokenCell>
                    {token.expiresAt
                      ? formatTimestamp(token.expiresAt)
                      : "Never"}
                  </TokenCell>
                </TokenRow>
              ))}
            </tbody>
          </TokenTable>
        ) : null}
      </SectionCard>

      <DangerZoneCard>
        <DangerZoneHeader>
          <DangerZoneTitle>Delete workspace</DangerZoneTitle>
          <DangerZoneDesc>
            Permanently remove <strong>{workspace.name}</strong> from the
            workspace registry.
          </DangerZoneDesc>
        </DangerZoneHeader>
        <DangerZoneActions>
          <DeleteWorkspaceButton
            size="large"
            disabled={isDeleting}
            onClick={onDelete}
          >
            {isDeleting ? "Deleting..." : "Delete workspace"}
          </DeleteWorkspaceButton>
        </DangerZoneActions>
      </DangerZoneCard>
    </SectionGrid>
  );
}

const MetaTable = styled.table`
  width: 100%;
  border-collapse: collapse;
  margin-top: 8px;
`;

const MetaRow = styled.tr`
  border-top: 1px solid var(--afs-line);

  &:first-child {
    border-top: none;
  }
`;

const MetaLabel = styled.th`
  width: 220px;
  padding: 14px 0;
  color: var(--afs-muted);
  font-size: 13px;
  font-weight: 600;
  text-align: left;
  vertical-align: top;
`;

const MetaValue = styled.td`
  padding: 14px 0;
  color: var(--afs-ink);
  font-size: 14px;
  line-height: 1.5;
  text-align: left;
`;

const MonoValue = styled.code`
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 13px;
`;

const StorageSummary = styled.div`
  display: grid;
  gap: 8px;
`;

const StorageBadge = styled.span<{
  $profile: AFSWorkspaceContentStorage["profile"];
}>`
  display: inline-flex;
  width: fit-content;
  align-items: center;
  border-radius: 999px;
  padding: 6px 11px;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.01em;
  border: 1px solid
    ${({ $profile }) =>
      $profile === "array"
        ? "rgba(22, 163, 74, 0.24)"
        : $profile === "mixed"
          ? "rgba(217, 119, 6, 0.24)"
          : "var(--afs-line)"};
  background: ${({ $profile }) =>
    $profile === "array"
      ? "rgba(34, 197, 94, 0.08)"
      : $profile === "mixed"
        ? "rgba(245, 158, 11, 0.08)"
        : "var(--afs-panel)"};
  color: ${({ $profile }) =>
    $profile === "array"
      ? "#15803d"
      : $profile === "mixed"
        ? "#b45309"
        : "var(--afs-ink-soft)"};
`;

const StatusBadge = styled.span<{
  $tone: "good" | "warn" | "neutral" | "bad";
}>`
  display: inline-flex;
  width: fit-content;
  align-items: center;
  border-radius: 999px;
  padding: 6px 11px;
  font-size: 12px;
  font-weight: 700;
  border: 1px solid
    ${({ $tone }) =>
      $tone === "good"
        ? "rgba(22, 163, 74, 0.24)"
        : $tone === "warn"
          ? "rgba(217, 119, 6, 0.24)"
          : $tone === "bad"
            ? "rgba(220, 38, 38, 0.24)"
            : "var(--afs-line)"};
  background: ${({ $tone }) =>
    $tone === "good"
      ? "rgba(34, 197, 94, 0.08)"
      : $tone === "warn"
        ? "rgba(245, 158, 11, 0.08)"
        : $tone === "bad"
          ? "rgba(239, 68, 68, 0.08)"
          : "var(--afs-panel)"};
  color: ${({ $tone }) =>
    $tone === "good"
      ? "#15803d"
      : $tone === "warn"
        ? "#b45309"
        : $tone === "bad"
          ? "#b91c1c"
          : "var(--afs-ink-soft)"};
`;

const StorageText = styled.div`
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.55;
`;

const AccessCopy = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const VersioningCopy = styled.p`
  margin: 0 0 16px;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.7;
`;

const ToggleRow = styled.div`
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background: var(--afs-panel);
  padding: 14px 16px;

  @media (max-width: 820px) {
    flex-direction: column;
    align-items: flex-start;
  }
`;

const ToggleText = styled.div`
  display: grid;
  gap: 4px;

  strong {
    color: var(--afs-ink);
    font-size: 14px;
  }

  span {
    color: var(--afs-muted);
    font-size: 13px;
    line-height: 1.6;
  }
`;

const ToggleSwitchLabel = styled.label`
  display: inline-flex;
  align-items: center;
  gap: 10px;
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 700;
`;

const ToggleSwitch = styled.span`
  position: relative;
  display: inline-flex;
  width: 38px;
  height: 22px;
  flex-shrink: 0;
`;

const ToggleCheckbox = styled.input`
  position: absolute;
  opacity: 0;
  width: 0;
  height: 0;
`;

const ToggleTrack = styled.span`
  position: absolute;
  inset: 0;
  border-radius: 999px;
  background: var(--afs-line-strong, #cbd5e1);
  transition: background 160ms ease;
  cursor: pointer;

  &::after {
    content: "";
    position: absolute;
    top: 2px;
    left: 2px;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: white;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.16);
    transition: transform 160ms ease;
  }

  ${ToggleCheckbox}:checked + & {
    background: var(--afs-focus, #2563eb);
  }

  ${ToggleCheckbox}:checked + &::after {
    transform: translateX(16px);
  }

  ${ToggleCheckbox}:focus-visible + & {
    box-shadow: 0 0 0 3px var(--afs-focus-soft);
  }
`;

const ToggleState = styled.span`
  min-width: 24px;
`;

const TwoFieldGrid = styled.div`
  display: grid;
  gap: 14px;
  grid-template-columns: repeat(2, minmax(0, 1fr));

  @media (max-width: 900px) {
    grid-template-columns: 1fr;
  }
`;

const RetentionGrid = styled.div`
  display: grid;
  gap: 14px;
  grid-template-columns: repeat(4, minmax(0, 1fr));

  @media (max-width: 1100px) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  @media (max-width: 720px) {
    grid-template-columns: 1fr;
  }
`;

const SelectFieldWrap = styled.div`
  width: 100%;

  > * {
    width: 100%;
  }
`;

const ScopeSummary = styled.div`
  min-height: 48px;
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background: var(--afs-panel);
  color: var(--afs-muted);
  padding: 12px 14px;
  font-size: 13px;
  line-height: 1.6;
`;

const VersioningStatus = styled.div`
  color: var(--afs-muted);
  font-size: 13px;
`;

const VersioningNotice = styled.div`
  color: #166534;
  font-size: 13px;
  font-weight: 600;
`;

const TokenTable = styled.table`
  width: 100%;
  border-collapse: collapse;
  margin-top: 18px;
`;

const TokenHead = styled.th`
  padding: 0 0 10px;
  color: var(--afs-muted);
  font-size: 12px;
  font-weight: 700;
  text-align: left;
  border-bottom: 1px solid var(--afs-line);
`;

const TokenRow = styled.tr`
  border-bottom: 1px solid var(--afs-line);
`;

const TokenCell = styled.td`
  padding: 14px 0;
  color: var(--afs-ink);
  font-size: 13px;
  vertical-align: top;
`;

function splitGlobList(raw: string) {
  return raw
    .split(/\r?\n|,/)
    .map((value) => value.trim())
    .filter(Boolean);
}

function parseWholeNumber(raw: string, label: string) {
  const trimmed = raw.trim();
  if (trimmed === "") {
    return 0;
  }
  const parsed = Number.parseInt(trimmed, 10);
  if (!Number.isFinite(parsed) || parsed < 0) {
    throw new Error(`${label} must be a non-negative integer.`);
  }
  return parsed;
}

const TokenName = styled.div`
  font-weight: 700;
`;

const TokenSubtle = styled.div`
  margin-top: 4px;
  color: var(--afs-muted);
  font-size: 12px;
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
`;

const DangerZoneCard = styled(SurfaceCard)`
  grid-column: span 12;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 20px 24px;
  border: 1px solid rgba(220, 38, 38, 0.2);
  background: rgba(220, 38, 38, 0.03);

  @media (max-width: 720px) {
    flex-direction: column;
    align-items: flex-start;
  }
`;

const DangerZoneHeader = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
`;

const DangerZoneActions = styled.div`
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 12px;

  @media (max-width: 720px) {
    width: 100%;
    align-items: flex-start;
  }
`;

const DangerZoneTitle = styled.h3`
  margin: 0;
  color: #dc2626;
  font-size: 15px;
  font-weight: 700;
`;

const DangerZoneDesc = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.5;
`;

const DeleteWorkspaceButton = styled(Button)`
  && {
    white-space: nowrap;
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

function formatProfile(profile: AFSMCPToken["profile"]) {
  switch (profile) {
    case "workspace-ro":
      return "Read only";
    case "workspace-rw":
      return "Read/write";
    case "workspace-rw-checkpoint":
      return "Read/write + checkpoints";
    case "admin-ro":
      return "Admin read only";
    case "admin-rw":
      return "Admin read/write";
    default:
      return profile;
  }
}

const VERSIONING_MODE_OPTIONS = [
  { value: "off", label: "Off" },
  { value: "all", label: "All paths" },
  { value: "paths", label: "Matching paths only" },
];

function storageProfileLabel(storage: AFSWorkspaceContentStorage) {
  switch (storage.profile) {
    case "array":
      return "Redis Array";
    case "mixed":
      return "Mixed backends";
    case "legacy":
      return "Legacy strings";
    default:
      return "No file content yet";
  }
}

function storageProfileDescription(storage: AFSWorkspaceContentStorage) {
  switch (storage.profile) {
    case "array":
      return `All ${storage.fileCount} file${storage.fileCount === 1 ? "" : "s"} use Redis Array content keys.`;
    case "mixed":
      return `${storage.arrayFileCount} file${storage.arrayFileCount === 1 ? "" : "s"} use Redis Array and ${storage.legacyFileCount} still use legacy Redis string keys.`;
    case "legacy":
      return `All ${storage.fileCount} file${storage.fileCount === 1 ? "" : "s"} use legacy Redis string content keys.`;
    default:
      return "This workspace does not have any file content stored yet.";
  }
}

function searchIndexTone(
  index: AFSWorkspaceSearchIndex,
): "good" | "warn" | "neutral" | "bad" {
  switch (index.status) {
    case "ready":
      return "good";
    case "building":
      return "warn";
    case "error":
      return "bad";
    default:
      return "neutral";
  }
}

function searchIndexLabel(index: AFSWorkspaceSearchIndex) {
  switch (index.status) {
    case "ready":
      return "Present";
    case "building":
      return "Building";
    case "missing":
      return "Not created";
    case "unavailable":
      return "Unavailable";
    case "error":
      return "Error";
    default:
      return "Unknown";
  }
}

function searchIndexDescription(index: AFSWorkspaceSearchIndex) {
  switch (index.status) {
    case "ready":
      return `Search index is ready with ${index.documentCount} indexed document${index.documentCount === 1 ? "" : "s"}.`;
    case "building":
      return `Search index exists and is ${formatPercent(index.percentIndexed)} indexed.`;
    case "missing":
      return "No RediSearch index exists for this workspace yet.";
    case "unavailable":
      return "RediSearch is not available on this Redis database.";
    case "error":
      return index.error
        ? `AFS could not inspect this workspace index: ${index.error}`
        : "AFS could not inspect this workspace index.";
    default:
      return "AFS could not determine search index state.";
  }
}

function formatPercent(value: number) {
  if (!Number.isFinite(value) || value <= 0) {
    return "0%";
  }
  return `${Math.min(100, Math.round(value * 100))}%`;
}

function formatTimestamp(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}
