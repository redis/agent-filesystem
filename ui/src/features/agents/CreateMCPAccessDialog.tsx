import { Button, Select } from "@redis-ui/components";
import { useEffect, useMemo, useState, type FormEvent } from "react";
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
} from "../../components/afs-kit";
import { getControlPlaneURL } from "../../foundation/api/afs";
import { useCreateMCPAccessTokenMutation } from "../../foundation/hooks/use-afs";
import type {
  AFSMCPProfile,
  AFSMCPToken,
  AFSWorkspaceSummary,
} from "../../foundation/types/afs";

type WorkspaceOption = { key: string; workspace: AFSWorkspaceSummary };

type Props = {
  isOpen: boolean;
  onClose: () => void;
  workspaces: AFSWorkspaceSummary[];
  initialWorkspaceId?: string;
  initialDatabaseId?: string;
};

export function CreateMCPAccessDialog({
  isOpen,
  onClose,
  workspaces,
  initialWorkspaceId,
  initialDatabaseId,
}: Props) {
  const createMCPAccessToken = useCreateMCPAccessTokenMutation();

  const [workspaceKey, setWorkspaceKey] = useState("");
  const [name, setName] = useState("");
  const [profile, setProfile] = useState<AFSMCPProfile>("workspace-rw");
  const [expiry, setExpiry] = useState("7d");
  const [createdToken, setCreatedToken] = useState<AFSMCPToken | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const options: WorkspaceOption[] = useMemo(
    () =>
      workspaces
        .slice()
        .sort((left, right) => left.name.localeCompare(right.name))
        .map((workspace) => ({
          key: keyFor(workspace.databaseId, workspace.id),
          workspace,
        })),
    [workspaces],
  );

  useEffect(() => {
    if (!isOpen) return;
    setCreatedToken(null);
    setFormError(null);
    setName("");
    setProfile("workspace-rw");
    setExpiry("7d");
    const requestedKey =
      initialWorkspaceId && initialDatabaseId
        ? keyFor(initialDatabaseId, initialWorkspaceId)
        : "";
    const fallback = options[0]?.key ?? "";
    const match = options.find((option) => option.key === requestedKey)?.key;
    setWorkspaceKey(match ?? fallback);
  }, [isOpen, initialDatabaseId, initialWorkspaceId, options]);

  const selected = options.find((option) => option.key === workspaceKey)?.workspace ?? null;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (selected == null || createMCPAccessToken.isPending) return;
    try {
      setFormError(null);
      const token = await createMCPAccessToken.mutateAsync({
        databaseId: selected.databaseId,
        workspaceId: selected.id,
        name: name.trim() || undefined,
        profile,
        expiresAt: expiryValueToTimestamp(expiry),
      });
      setCreatedToken(token);
    } catch (error) {
      setFormError(
        error instanceof Error ? error.message : "Unable to create MCP access.",
      );
    }
  }

  function handleClose() {
    if (createMCPAccessToken.isPending) return;
    onClose();
  }

  function copy(text: string, label: string) {
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(label);
      window.setTimeout(() => setCopied(null), 2000);
    });
  }

  if (!isOpen) return null;

  const hostedSnippet =
    createdToken && selected
      ? buildHostedMCPSnippet(selected.name, getControlPlaneURL(), createdToken.token ?? "")
      : null;

  return (
    <DialogOverlay
      onClick={(event) => {
        if (event.target === event.currentTarget) handleClose();
      }}
    >
      <DialogCard>
        <DialogHeader>
          <div>
            <DialogTitle>
              {createdToken ? "MCP server created" : "Add MCP server"}
            </DialogTitle>
            <DialogBody>
              {createdToken
                ? "Copy the config below into your remote MCP client. The token is shown once \u2014 store it safely."
                : "Issue a workspace-scoped bearer token that hosted MCP clients can use to reach this workspace."}
            </DialogBody>
          </div>
          <DialogCloseButton onClick={handleClose}>&times;</DialogCloseButton>
        </DialogHeader>

        {createdToken ? (
          <CreatedPanel>
            <FieldBlock>
              <FieldLabel>Token</FieldLabel>
              <CodeBlock>{createdToken.token ?? "(not returned)"}</CodeBlock>
              <InlineActionsRight>
                <Button
                  size="small"
                  variant="secondary-fill"
                  onClick={() => createdToken.token && copy(createdToken.token, "token")}
                >
                  {copied === "token" ? "Copied!" : "Copy token"}
                </Button>
              </InlineActionsRight>
            </FieldBlock>

            {hostedSnippet ? (
              <FieldBlock>
                <FieldLabel>Hosted MCP config</FieldLabel>
                <CodeBlock>{hostedSnippet}</CodeBlock>
                <InlineActionsRight>
                  <Button
                    size="small"
                    variant="secondary-fill"
                    onClick={() => copy(hostedSnippet, "snippet")}
                  >
                    {copied === "snippet" ? "Copied!" : "Copy config"}
                  </Button>
                </InlineActionsRight>
              </FieldBlock>
            ) : null}

            <DialogActions style={{ justifyContent: "flex-end", marginTop: 8 }}>
              <Button size="medium" onClick={handleClose}>
                Done
              </Button>
            </DialogActions>
          </CreatedPanel>
        ) : (
          <FormGrid onSubmit={submit}>
            <Field>
              Workspace
              <Select
                options={
                  options.length === 0
                    ? [{ value: "", label: "No workspaces available" }]
                    : options.map((option) => ({
                        value: option.key,
                        label: option.workspace.name,
                      }))
                }
                value={workspaceKey}
                onChange={(next) => setWorkspaceKey(next as string)}
                disabled={options.length === 0}
              />
            </Field>

            <Field>
              Name
              <TextInput
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="Claude Desktop on Rowan's Mac"
              />
            </Field>

            <Field>
              Access profile
              <Select
                options={[
                  { value: "workspace-ro", label: "Read only" },
                  { value: "workspace-rw", label: "Read / write" },
                  { value: "workspace-rw-checkpoint", label: "Read / write + checkpoints" },
                ]}
                value={profile}
                onChange={(next) => setProfile(next as AFSMCPProfile)}
              />
            </Field>

            <Field>
              Expiry
              <Select
                options={[
                  { value: "24h", label: "24 hours" },
                  { value: "7d", label: "7 days" },
                  { value: "30d", label: "30 days" },
                  { value: "never", label: "No expiry" },
                ]}
                value={expiry}
                onChange={(next) => setExpiry(next as string)}
              />
            </Field>

            <ToolPreview>
              {toolListForProfile(profile).map((tool) => (
                <ToolChip key={tool}>{tool}</ToolChip>
              ))}
            </ToolPreview>

            {formError ? <DialogError role="alert">{formError}</DialogError> : null}

            <DialogActions style={{ justifyContent: "flex-end" }}>
              <Button
                size="medium"
                type="button"
                variant="secondary-fill"
                onClick={handleClose}
                disabled={createMCPAccessToken.isPending}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                size="medium"
                disabled={selected == null || createMCPAccessToken.isPending}
              >
                {createMCPAccessToken.isPending ? "Creating..." : "Create MCP server"}
              </Button>
            </DialogActions>
          </FormGrid>
        )}
      </DialogCard>
    </DialogOverlay>
  );
}

function keyFor(databaseId: string, workspaceId: string) {
  return `${databaseId}::${workspaceId}`;
}

function expiryValueToTimestamp(value: string) {
  if (value === "never") return undefined;
  const now = Date.now();
  switch (value) {
    case "24h":
      return new Date(now + 24 * 60 * 60 * 1000).toISOString();
    case "30d":
      return new Date(now + 30 * 24 * 60 * 60 * 1000).toISOString();
    case "7d":
    default:
      return new Date(now + 7 * 24 * 60 * 60 * 1000).toISOString();
  }
}

function buildHostedMCPSnippet(workspaceName: string, controlPlaneURL: string, token: string) {
  return JSON.stringify(
    {
      mcpServers: {
        [`afs-${workspaceName}`]: {
          url: `${controlPlaneURL.replace(/\/+$/, "")}/mcp`,
          headers: {
            Authorization: `Bearer ${token}`,
          },
        },
      },
    },
    null,
    2,
  );
}

function toolListForProfile(profile: AFSMCPProfile) {
  switch (profile) {
    case "workspace-ro":
      return ["file_read", "file_lines", "file_list", "file_glob", "file_grep"];
    case "workspace-rw":
      return [
        "file_read",
        "file_lines",
        "file_list",
        "file_glob",
        "file_grep",
        "file_write",
        "file_replace",
        "file_insert",
        "file_delete_lines",
        "file_patch",
      ];
    case "workspace-rw-checkpoint":
      return [
        "file_read",
        "file_lines",
        "file_list",
        "file_glob",
        "file_grep",
        "file_write",
        "file_replace",
        "file_insert",
        "file_delete_lines",
        "file_patch",
        "checkpoint_list",
        "checkpoint_create",
        "checkpoint_restore",
      ];
    default:
      return ["profile-specific admin tools"];
  }
}

const CreatedPanel = styled.div`
  display: grid;
  gap: 16px;
`;

const FieldBlock = styled.div`
  display: grid;
  gap: 8px;
`;

const FieldLabel = styled.span`
  color: var(--afs-ink-soft);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
`;

const CodeBlock = styled.pre`
  margin: 0;
  padding: 14px;
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.94);
  color: #e2e8f0;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  overflow: auto;
  max-height: 260px;
`;

const InlineActionsRight = styled.div`
  display: flex;
  justify-content: flex-end;
`;

const ToolPreview = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
`;

const ToolChip = styled.span`
  display: inline-flex;
  align-items: center;
  padding: 6px 10px;
  border-radius: 999px;
  border: 1px solid var(--afs-line);
  background: var(--afs-panel);
  color: var(--afs-ink-soft);
  font-size: 11px;
  font-weight: 700;
`;
