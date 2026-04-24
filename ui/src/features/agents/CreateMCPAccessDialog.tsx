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
import {
  useCreateControlPlaneTokenMutation,
  useCreateMCPAccessTokenMutation,
} from "../../foundation/hooks/use-afs";
import type {
  AFSMCPProfile,
  AFSMCPToken,
  AFSWorkspaceSummary,
} from "../../foundation/types/afs";

type WorkspaceOption = { key: string; workspace: AFSWorkspaceSummary };

type TokenScopeMode = "control-plane" | "workspace";

type Props = {
  isOpen: boolean;
  onClose: () => void;
  workspaces: AFSWorkspaceSummary[];
  initialWorkspaceId?: string;
  initialDatabaseId?: string;
  initialScope?: TokenScopeMode;
};

export function CreateMCPAccessDialog({
  isOpen,
  onClose,
  workspaces,
  initialWorkspaceId,
  initialDatabaseId,
  initialScope,
}: Props) {
  const createWorkspaceToken = useCreateMCPAccessTokenMutation();
  const createControlPlaneToken = useCreateControlPlaneTokenMutation();

  const [scopeMode, setScopeMode] = useState<TokenScopeMode>(initialScope ?? "workspace");
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
    setScopeMode(initialScope ?? "workspace");
    const requestedKey =
      initialWorkspaceId && initialDatabaseId
        ? keyFor(initialDatabaseId, initialWorkspaceId)
        : "";
    const fallback = options[0]?.key ?? "";
    const match = options.find((option) => option.key === requestedKey)?.key;
    setWorkspaceKey(match ?? fallback);
  }, [isOpen, initialDatabaseId, initialWorkspaceId, initialScope, options]);

  const selected = options.find((option) => option.key === workspaceKey)?.workspace ?? null;
  const pending = createWorkspaceToken.isPending || createControlPlaneToken.isPending;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;
    setFormError(null);
    try {
      if (scopeMode === "control-plane") {
        const token = await createControlPlaneToken.mutateAsync({
          name: name.trim() || undefined,
          expiresAt: expiryValueToTimestamp(expiry),
        });
        setCreatedToken(token);
        return;
      }
      if (selected == null) return;
      const token = await createWorkspaceToken.mutateAsync({
        databaseId: selected.databaseId,
        workspaceId: selected.id,
        name: name.trim() || undefined,
        profile,
        expiresAt: expiryValueToTimestamp(expiry),
      });
      setCreatedToken(token);
    } catch (error) {
      setFormError(
        error instanceof Error ? error.message : "Unable to create access token.",
      );
    }
  }

  function handleClose() {
    if (pending) return;
    onClose();
  }

  function copy(text: string, label: string) {
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(label);
      window.setTimeout(() => setCopied(null), 2000);
    });
  }

  if (!isOpen) return null;

  const mcpEndpoint = `${getControlPlaneURL().replace(/\/+$/, "")}/mcp`;
  const createdSnippet = createdToken
    ? buildSnippet({
        token: createdToken.token ?? "",
        url: mcpEndpoint,
        serverName:
          createdToken.scope === "control-plane"
            ? "agent-filesystem"
            : `afs-${(createdToken.workspaceName ?? selected?.name ?? "workspace").trim()}`,
      })
    : null;

  const submitLabel = (() => {
    if (createdToken) return null;
    if (pending) {
      return scopeMode === "control-plane" ? "Issuing..." : "Creating...";
    }
    return scopeMode === "control-plane" ? "Issue control-plane token" : "Create workspace token";
  })();

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
              {createdToken ? "Access token created" : "Create access token"}
            </DialogTitle>
            <DialogBody>
              {createdToken
                ? "Copy the config below into your MCP client. The token is shown once \u2014 store it safely."
                : "Issue a bearer token an MCP client can use. Pick a scope to decide what it's allowed to do."}
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

            {createdSnippet ? (
              <FieldBlock>
                <FieldLabel>MCP client config</FieldLabel>
                <CodeBlock>{createdSnippet}</CodeBlock>
                <InlineActionsRight>
                  <Button
                    size="small"
                    variant="secondary-fill"
                    onClick={() => copy(createdSnippet, "snippet")}
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
            <ScopeField>
              <FieldLabel>Scope</FieldLabel>
              <ScopeRow>
                <ScopeOption $selected={scopeMode === "workspace"}>
                  <input
                    type="radio"
                    checked={scopeMode === "workspace"}
                    onChange={() => setScopeMode("workspace")}
                  />
                  <ScopeLabel>
                    <ScopeName>Workspace</ScopeName>
                    <ScopeHint>
                      File tools scoped to one workspace.
                    </ScopeHint>
                  </ScopeLabel>
                </ScopeOption>
                <ScopeOption $selected={scopeMode === "control-plane"}>
                  <input
                    type="radio"
                    checked={scopeMode === "control-plane"}
                    onChange={() => setScopeMode("control-plane")}
                  />
                  <ScopeLabel>
                    <ScopeName>Control plane</ScopeName>
                    <ScopeHint>
                      Manage workspaces + mint workspace tokens on demand.
                    </ScopeHint>
                  </ScopeLabel>
                </ScopeOption>
              </ScopeRow>
            </ScopeField>

            {scopeMode === "workspace" ? (
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
            ) : null}

            <Field>
              Name
              <TextInput
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder={
                  scopeMode === "control-plane"
                    ? "e.g. dev laptop, staging agent"
                    : "Claude Desktop on Rowan's Mac"
                }
              />
            </Field>

            {scopeMode === "workspace" ? (
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
            ) : null}

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
              {toolListForScope(scopeMode, profile).map((tool) => (
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
                disabled={pending}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                size="medium"
                disabled={
                  pending || (scopeMode === "workspace" && selected == null)
                }
              >
                {submitLabel}
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

function buildSnippet({
  token,
  url,
  serverName,
}: {
  token: string;
  url: string;
  serverName: string;
}) {
  return JSON.stringify(
    {
      mcpServers: {
        [serverName]: {
          url,
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

function toolListForScope(scope: TokenScopeMode, profile: AFSMCPProfile) {
  if (scope === "control-plane") {
    return [
      "workspace_list",
      "workspace_get",
      "workspace_create",
      "workspace_fork",
      "workspace_delete",
      "checkpoint_list",
      "checkpoint_create",
      "checkpoint_restore",
      "mcp_token_issue",
      "mcp_token_revoke",
    ];
  }
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

const ScopeField = styled.div`
  display: grid;
  gap: 10px;
`;

const ScopeRow = styled.div`
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;

  @media (max-width: 640px) {
    grid-template-columns: 1fr;
  }
`;

const ScopeOption = styled.label<{ $selected: boolean }>`
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 14px;
  border-radius: 12px;
  border: 1px solid ${({ $selected }) => ($selected ? "var(--afs-accent, #2563eb)" : "var(--afs-line)")};
  background: ${({ $selected }) =>
    $selected
      ? "color-mix(in srgb, var(--afs-accent, #2563eb) 8%, var(--afs-panel))"
      : "var(--afs-panel)"};
  cursor: pointer;
  transition: border-color 120ms ease, background 120ms ease;

  input[type="radio"] {
    margin-top: 3px;
  }
`;

const ScopeLabel = styled.div`
  display: flex;
  flex-direction: column;
  gap: 2px;
`;

const ScopeName = styled.div`
  color: var(--afs-ink);
  font-size: 13.5px;
  font-weight: 700;
`;

const ScopeHint = styled.div`
  color: var(--afs-muted);
  font-size: 12px;
  line-height: 1.45;
`;
