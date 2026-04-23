import { Button, Select } from "@redis-ui/components";
import { useEffect, useMemo, useState } from "react";
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
  FormGrid,
} from "../../components/afs-kit";
import type { AFSMCPProfile, AFSWorkspaceSummary } from "../../foundation/types/afs";

type Props = {
  isOpen: boolean;
  onClose: () => void;
  workspaces: AFSWorkspaceSummary[];
  initialWorkspaceId?: string;
  initialDatabaseId?: string;
};

export function LocalMCPAccessDialog({
  isOpen,
  onClose,
  workspaces,
  initialWorkspaceId,
  initialDatabaseId,
}: Props) {
  const [workspaceKey, setWorkspaceKey] = useState("");
  const [profile, setProfile] = useState<AFSMCPProfile>("workspace-rw");
  const [copied, setCopied] = useState(false);

  const options = useMemo(
    () =>
      workspaces
        .slice()
        .sort((left, right) => left.name.localeCompare(right.name))
        .map((workspace) => ({ key: keyFor(workspace.databaseId, workspace.id), workspace })),
    [workspaces],
  );

  useEffect(() => {
    if (!isOpen) return;
    setCopied(false);
    const requestedKey =
      initialWorkspaceId && initialDatabaseId
        ? keyFor(initialDatabaseId, initialWorkspaceId)
        : "";
    const fallback = options[0]?.key ?? "";
    const match = options.find((option) => option.key === requestedKey)?.key;
    setWorkspaceKey(match ?? fallback);
  }, [isOpen, initialDatabaseId, initialWorkspaceId, options]);

  const selected = options.find((option) => option.key === workspaceKey)?.workspace ?? null;
  const snippet = selected ? buildLocalMCPSnippet(selected.name, profile) : null;

  function copy() {
    if (!snippet) return;
    void navigator.clipboard.writeText(snippet).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    });
  }

  if (!isOpen) return null;

  return (
    <DialogOverlay
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <DialogCard>
        <DialogHeader>
          <div>
            <DialogTitle>Local MCP access</DialogTitle>
            <DialogBody>
              For agents running on a machine with the AFS CLI installed. No token required
              &mdash; authentication uses the local CLI session.
            </DialogBody>
          </div>
          <DialogCloseButton onClick={onClose}>&times;</DialogCloseButton>
        </DialogHeader>

        <FormGrid onSubmit={(event) => event.preventDefault()}>
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

          <SnippetBlock>
            <FieldLabel>MCP config snippet</FieldLabel>
            {snippet ? (
              <>
                <CodeBlock>{snippet}</CodeBlock>
                <Hint>
                  Paste into <code>claude_desktop_config.json</code> (or your client's MCP config).
                  The CLI must be installed and signed in.
                </Hint>
              </>
            ) : (
              <EmptyHint>Select a workspace to generate a snippet.</EmptyHint>
            )}
          </SnippetBlock>

          <DialogActions style={{ justifyContent: "flex-end" }}>
            <Button size="medium" type="button" variant="secondary-fill" onClick={onClose}>
              Close
            </Button>
            <Button size="medium" type="button" onClick={copy} disabled={snippet == null}>
              {copied ? "Copied!" : "Copy snippet"}
            </Button>
          </DialogActions>
        </FormGrid>
      </DialogCard>
    </DialogOverlay>
  );
}

function keyFor(databaseId: string, workspaceId: string) {
  return `${databaseId}::${workspaceId}`;
}

function buildLocalMCPSnippet(workspaceName: string, profile: AFSMCPProfile) {
  return JSON.stringify(
    {
      mcpServers: {
        [`afs-${workspaceName}`]: {
          command: "afs",
          args: ["mcp", "--workspace", workspaceName, "--profile", profile],
        },
      },
    },
    null,
    2,
  );
}

const SnippetBlock = styled.div`
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
  max-height: 280px;
`;

const Hint = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 12px;
  line-height: 1.5;

  code {
    font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    background: var(--afs-panel);
    padding: 1px 5px;
    border-radius: 6px;
    border: 1px solid var(--afs-line);
    font-size: 11px;
  }
`;

const EmptyHint = styled.div`
  padding: 14px;
  border-radius: 14px;
  background: var(--afs-panel);
  border: 1px solid var(--afs-line);
  color: var(--afs-muted);
  font-size: 13px;
`;
