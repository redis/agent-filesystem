import { Button } from "@redis-ui/components";
import styled from "styled-components";
import {
  DialogActions,
  DialogError,
  Field,
  FormGrid,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
  TextInput,
} from "../../components/afs-kit";
import type { AFSMCPToken, AFSWorkspaceDetail } from "../../foundation/types/afs";

type Props = {
  workspace: AFSWorkspaceDetail;
  onSave: (input: { name: string; description: string }) => void | Promise<void>;
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
  const activeTokens = mcpTokens.filter((token) => token.revokedAt == null || token.revokedAt === "");
  const [activeToken] = activeTokens;
  const tokenCount = activeTokens.length;

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
            <TextInput name="name" defaultValue={workspace.name} placeholder="customer-portal" />
          </Field>

          <Field>
            Description
            <TextInput
              name="description"
              defaultValue={workspace.description}
              placeholder="What this workspace is for, who owns it, and why it exists."
            />
          </Field>

          {saveError ? <DialogError role="alert">{saveError}</DialogError> : null}

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
            <MetaRow>
              <MetaLabel>Redis key</MetaLabel>
              <MetaValue>
                <MonoValue>{workspace.redisKey}</MonoValue>
              </MetaValue>
            </MetaRow>
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
          <SectionTitle title="Agent access" />
          <Button size="medium" onClick={onOpenMCPConsole}>
            Open MCP console
          </Button>
        </SectionHeader>

        <AccessCopy>
          MCP setup now lives on the Agents page so you can manage all workspace-scoped access tokens and config snippets in one place. This panel stays focused on the current workspace and shows whether it already has authorized MCP access.
        </AccessCopy>

        <MetaTable>
          <tbody>
            <MetaRow>
              <MetaLabel>Authorized tokens</MetaLabel>
              <MetaValue>{tokenCount === 0 ? "None yet" : `${tokenCount} active token${tokenCount === 1 ? "" : "s"}`}</MetaValue>
            </MetaRow>
            <MetaRow>
              <MetaLabel>Workspace scope</MetaLabel>
              <MetaValue>All MCP tokens created from this workspace stay locked to {workspace.name}.</MetaValue>
            </MetaRow>
            <MetaRow>
              <MetaLabel>Admin tools</MetaLabel>
              <MetaValue>Workspace settings no longer mint admin access tokens. Use the access token console for explicit elevated flows.</MetaValue>
            </MetaRow>
            {activeToken ? (
              <>
                <MetaRow>
                  <MetaLabel>Latest token</MetaLabel>
                  <MetaValue>{activeToken.name?.trim() || activeToken.id}</MetaValue>
                </MetaRow>
                <MetaRow>
                  <MetaLabel>Last used</MetaLabel>
                  <MetaValue>{activeToken.lastUsedAt ? formatTimestamp(activeToken.lastUsedAt) : "Never"}</MetaValue>
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
                  <TokenCell>{token.lastUsedAt ? formatTimestamp(token.lastUsedAt) : "Never"}</TokenCell>
                  <TokenCell>{token.expiresAt ? formatTimestamp(token.expiresAt) : "Never"}</TokenCell>
                </TokenRow>
              ))}
            </tbody>
          </TokenTable>
        ) : null}
      </SectionCard>

      <DangerZoneCard>
        <DangerZoneHeader>
          <DangerZoneTitle>Danger zone</DangerZoneTitle>
          <DangerZoneDesc>
            Permanently delete this workspace and remove it from the registry.
          </DangerZoneDesc>
        </DangerZoneHeader>
        <DeleteWorkspaceButton size="large" disabled={isDeleting} onClick={onDelete}>
          {isDeleting ? "Deleting..." : "Delete workspace"}
        </DeleteWorkspaceButton>
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

const AccessCopy = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
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

const TokenName = styled.div`
  font-weight: 700;
`;

const TokenSubtle = styled.div`
  margin-top: 4px;
  color: var(--afs-muted);
  font-size: 12px;
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
`;

const DangerZoneCard = styled.div`
  grid-column: span 12;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 20px 24px;
  border: 1px solid rgba(220, 38, 38, 0.2);
  border-radius: 16px;
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

function formatTimestamp(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}
