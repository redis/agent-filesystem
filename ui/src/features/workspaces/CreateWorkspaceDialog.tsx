import { Button, Select } from "@redis-ui/components";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { useNavigate } from "@tanstack/react-router";
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
  type AFSDatabaseScopeRecord,
  useDatabaseScope,
} from "../../foundation/database-scope";
import {
  useCreateMCPAccessTokenMutation,
  useCreateWorkspaceMutation,
  useImportLocalMutation,
} from "../../foundation/hooks/use-afs";
import {
  findTemplate,
  type Template,
  type TemplateSeedFile,
} from "../templates/templates-data";

type SeedMode = "blank" | "import";
type View = "chooser" | "template-form";

type Props = {
  open: boolean;
  onClose: () => void;
  onFreeTierLimitHit?: () => void;
  initialTemplateId?: string;
};

function eligibleDatabases(databases: AFSDatabaseScopeRecord[]) {
  return databases.filter((database) => database.canCreateWorkspaces);
}

function preferredDatabase(databases: AFSDatabaseScopeRecord[]) {
  const list = eligibleDatabases(databases);
  return list.find((database) => database.isDefault) ?? list[0] ?? null;
}

function isFreeTierLimitError(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  return error.message.toLowerCase().includes("free tier workspace limit");
}

function slugFromPath(path: string) {
  const trimmed = path.trim().replace(/\/+$/, "");
  const last = trimmed.split("/").pop() ?? "";
  return last.toLowerCase().replace(/[^a-z0-9-]+/g, "-").replace(/^-+|-+$/g, "");
}

async function seedTemplateFiles(
  mcpUrl: string,
  token: string,
  files: readonly TemplateSeedFile[],
  onProgress?: (done: number, total: number) => void,
): Promise<number> {
  let done = 0;
  for (let index = 0; index < files.length; index++) {
    const file = files[index];
    const absolutePath = "/" + file.path.replace(/^\/+/, "");
    const response = await fetch(mcpUrl, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({
        jsonrpc: "2.0",
        id: index + 1,
        method: "tools/call",
        params: {
          name: "file_write",
          arguments: { path: absolutePath, content: file.content },
        },
      }),
    });

    if (!response.ok) {
      const text = await response.text();
      throw new Error(
        `Seeding ${file.path} failed: HTTP ${response.status} ${text.slice(0, 160)}`,
      );
    }
    const body = (await response.json()) as {
      error?: { message?: string };
      result?: { isError?: boolean; content?: Array<{ text?: string }> };
    };
    if (body.error) {
      throw new Error(
        `Seeding ${file.path} failed: ${body.error.message ?? "unknown error"}`,
      );
    }
    if (body.result?.isError) {
      const detail = body.result.content?.[0]?.text ?? "unknown error";
      throw new Error(`Seeding ${file.path} failed: ${detail}`);
    }

    done += 1;
    onProgress?.(done, files.length);
  }
  return done;
}

export function CreateWorkspaceDialog({
  open,
  onClose,
  onFreeTierLimitHit,
  initialTemplateId,
}: Props) {
  const { databases } = useDatabaseScope();
  const eligible = useMemo(() => eligibleDatabases(databases), [databases]);
  const createWorkspace = useCreateWorkspaceMutation();
  const createToken = useCreateMCPAccessTokenMutation();
  const importLocal = useImportLocalMutation();
  const navigate = useNavigate();

  const [view, setView] = useState<View>("chooser");
  const [mode, setMode] = useState<SeedMode>("blank");
  const [selectedTemplateId, setSelectedTemplateId] = useState<string | null>(
    null,
  );

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [databaseId, setDatabaseId] = useState("");
  const [importPath, setImportPath] = useState("");
  const [importFileCount, setImportFileCount] = useState(0);
  const [formError, setFormError] = useState<string | null>(null);
  const [nameEdited, setNameEdited] = useState(false);
  const [seedingProgress, setSeedingProgress] = useState<{
    done: number;
    total: number;
  } | null>(null);

  const fileInputRef = useRef<HTMLInputElement>(null);

  const selectedTemplate: Template | null = useMemo(
    () => (selectedTemplateId ? findTemplate(selectedTemplateId) ?? null : null),
    [selectedTemplateId],
  );

  // Initialize on open.
  useEffect(() => {
    if (!open) return;
    const startTemplate = initialTemplateId
      ? findTemplate(initialTemplateId) ?? null
      : null;
    const startedFromTemplate =
      startTemplate != null && startTemplate.id !== "blank";

    if (startedFromTemplate && startTemplate) {
      setView("template-form");
      setSelectedTemplateId(startTemplate.id);
      setName(startTemplate.slug);
      setDescription(startTemplate.tagline);
    } else {
      setView("chooser");
      setSelectedTemplateId(null);
      setName("");
      setDescription("");
    }
    setMode("blank");
    setImportPath("");
    setImportFileCount(0);
    setFormError(null);
    setNameEdited(false);
    setSeedingProgress(null);
    const defaultDb = preferredDatabase(databases);
    setDatabaseId(defaultDb?.id ?? "");
  }, [open, initialTemplateId, databases]);

  const busy =
    createWorkspace.isPending ||
    createToken.isPending ||
    importLocal.isPending ||
    seedingProgress != null;

  if (!open) return null;

  function handleClose() {
    if (busy) return;
    onClose();
  }

  function selectMode(next: SeedMode) {
    setFormError(null);
    setMode(next);
  }

  function handleFolderPicked(files: FileList | null) {
    if (!files || files.length === 0) return;
    const path = files[0].webkitRelativePath?.split("/")[0] ?? "";
    if (!path) return;
    setImportPath(path);
    setImportFileCount(files.length);
    if (!nameEdited || name.trim() === "") {
      setName(slugFromPath(path) || path);
    }
  }

  async function submitChooser(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;

    const trimmedName = name.trim();
    if (trimmedName === "") {
      setFormError("Workspace name is required.");
      return;
    }
    setFormError(null);

    const database = eligible.find((item) => item.id === databaseId) ?? null;
    const databaseName = database?.databaseName ?? "";

    if (mode === "import") {
      if (importPath.trim() === "") {
        setFormError("Pick a local folder to import, or switch to Blank.");
        return;
      }
      try {
        const result = await importLocal.mutateAsync({
          databaseId: databaseId || undefined,
          name: trimmedName,
          path: importPath.trim(),
          description,
        });
        onClose();
        void navigate({
          to: "/workspaces/$workspaceId",
          params: { workspaceId: result.workspaceId },
          search: { databaseId: result.databaseId, tab: "browse" },
        });
      } catch (error) {
        setFormError(
          error instanceof Error ? error.message : "Unable to import files.",
        );
      }
      return;
    }

    // mode === "blank"
    try {
      await createWorkspace.mutateAsync({
        databaseId: databaseId || undefined,
        name: trimmedName,
        description,
        cloudAccount: "Direct Redis",
        databaseName,
        region: "",
        source: "blank",
      });
      onClose();
    } catch (error) {
      if (isFreeTierLimitError(error)) {
        onClose();
        onFreeTierLimitHit?.();
        return;
      }
      setFormError(
        error instanceof Error
          ? error.message
          : "Unable to create the workspace.",
      );
    }
  }

  async function submitTemplateForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy || !selectedTemplate) return;

    const trimmedName = name.trim();
    if (trimmedName === "") {
      setFormError("Workspace name is required.");
      return;
    }
    setFormError(null);

    const database = eligible.find((item) => item.id === databaseId) ?? null;
    const databaseName = database?.databaseName ?? "";

    try {
      const workspace = await createWorkspace.mutateAsync({
        databaseId: databaseId || undefined,
        name: trimmedName,
        description,
        cloudAccount: "Direct Redis",
        databaseName,
        region: "",
        source: "blank",
        templateSlug: selectedTemplate.id,
      });

      const token = await createToken.mutateAsync({
        databaseId: workspace.databaseId || databaseId || undefined,
        workspaceId: workspace.id,
        name: `${selectedTemplate.title} setup`,
        profile: selectedTemplate.profile,
        templateSlug: selectedTemplate.id,
      });

      const tokenValue = token.token ?? "";
      if (tokenValue === "") {
        throw new Error(
          "Workspace and token were created, but the token value was not returned; cannot seed files.",
        );
      }

      const mcpUrl = `${getControlPlaneURL().replace(/\/+$/, "")}/mcp`;
      setSeedingProgress({ done: 0, total: selectedTemplate.seedFiles.length });
      const seeded = await seedTemplateFiles(
        mcpUrl,
        tokenValue,
        selectedTemplate.seedFiles,
        (done, total) => setSeedingProgress({ done, total }),
      );
      setSeedingProgress(null);

      onClose();
      void navigate({
        to: "/templates/installed/$workspaceId",
        params: { workspaceId: workspace.id },
        search: {
          seeded,
          fresh: true,
          ...(workspace.databaseId ? { databaseId: workspace.databaseId } : {}),
        },
      });
    } catch (error) {
      setSeedingProgress(null);
      if (isFreeTierLimitError(error)) {
        onClose();
        onFreeTierLimitHit?.();
        return;
      }
      setFormError(
        error instanceof Error
          ? error.message
          : "Unable to create the workspace from this template.",
      );
    }
  }

  const header = renderHeader();
  const body = renderBody();

  return (
    <DialogOverlay
      onClick={(event) => {
        if (event.target === event.currentTarget) handleClose();
      }}
    >
      <DialogCard>
        {header}
        {body}
      </DialogCard>
    </DialogOverlay>
  );

  function renderHeader() {
    if (view === "template-form" && selectedTemplate) {
      return (
        <DialogHeader>
          <div>
            <DialogTitle>Install &ldquo;{selectedTemplate.title}&rdquo;</DialogTitle>
            <DialogBody>{selectedTemplate.tagline}</DialogBody>
          </div>
          <DialogCloseButton
            type="button"
            aria-label="Close"
            onClick={handleClose}
          >
            &times;
          </DialogCloseButton>
        </DialogHeader>
      );
    }

    return (
      <DialogHeader>
        <div>
          <DialogTitle>Create workspace</DialogTitle>
          <DialogBody>
            Choose how you want to start. You can switch any time before you
            create.
          </DialogBody>
        </div>
        <DialogCloseButton
          type="button"
          aria-label="Close"
          onClick={handleClose}
        >
          &times;
        </DialogCloseButton>
      </DialogHeader>
    );
  }

  function renderBody() {
    if (view === "template-form" && selectedTemplate) {
      return (
        <FormGrid onSubmit={submitTemplateForm}>
          <TemplateSummary $accent={selectedTemplate.accent}>
            <TemplateSummaryIcon $accent={selectedTemplate.accent}>
              <selectedTemplate.icon size="M" />
            </TemplateSummaryIcon>
            <TemplateSummaryBody>
              <TemplateSummaryTitle>
                {selectedTemplate.title}
              </TemplateSummaryTitle>
              <TemplateSummaryText>
                {selectedTemplate.tagline}
              </TemplateSummaryText>
            </TemplateSummaryBody>
            <TemplateSummaryBadge>
              {selectedTemplate.profileLabel}
            </TemplateSummaryBadge>
          </TemplateSummary>

          <Field>
            Workspace name
            <TextInput
              autoFocus
              value={name}
              onChange={(event) => {
                setName(event.target.value);
                setNameEdited(true);
              }}
              placeholder={selectedTemplate.slug}
            />
          </Field>

          {eligible.length > 1 ? (
            <Field>
              Database
              <Select
                options={eligible.map((database) => ({
                  value: database.id,
                  label: `${database.displayName || database.databaseName}${database.isDefault ? " (default)" : ""}`,
                }))}
                value={databaseId}
                onChange={(next) => setDatabaseId(next as string)}
              />
            </Field>
          ) : null}

          <Field>
            Description
            <TextInput
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              placeholder={`${selectedTemplate.tagline} (optional)`}
            />
          </Field>

          {formError ? <DialogError role="alert">{formError}</DialogError> : null}

          <DialogActions style={{ justifyContent: "flex-end" }}>
            <Button
              size="medium"
              type="button"
              variant="secondary-fill"
              onClick={handleClose}
              disabled={busy}
            >
              Cancel
            </Button>
            <Button size="medium" type="submit" disabled={busy}>
              {seedingProgress
                ? `Seeding files… ${seedingProgress.done}/${seedingProgress.total}`
                : busy
                  ? "Installing…"
                  : "Install template"}
            </Button>
          </DialogActions>
        </FormGrid>
      );
    }

    // view === "chooser"
    return (
      <FormGrid onSubmit={submitChooser}>
        <SectionLabel>Starting point</SectionLabel>
        <ModeStrip>
          <ModeCard
            type="button"
            $active={mode === "blank"}
            onClick={() => selectMode("blank")}
          >
            <ModeTitle>Blank</ModeTitle>
            <ModeHint>Empty workspace, add files later.</ModeHint>
          </ModeCard>
          <ModeCard
            type="button"
            $active={mode === "import"}
            onClick={() => selectMode("import")}
          >
            <ModeTitle>Import folder</ModeTitle>
            <ModeHint>Copy files from a local directory.</ModeHint>
          </ModeCard>
        </ModeStrip>

        {mode === "import" ? (
          <ImportSlot>
            {importPath === "" ? (
              <Button
                size="medium"
                variant="secondary-fill"
                type="button"
                onClick={() => fileInputRef.current?.click()}
              >
                Pick a folder&hellip;
              </Button>
            ) : (
              <SelectedFolder>
                <SelectedFolderInfo>
                  <SelectedFolderName>{importPath}</SelectedFolderName>
                  <SelectedFolderMeta>
                    {importFileCount} file
                    {importFileCount === 1 ? "" : "s"} ready to import
                  </SelectedFolderMeta>
                </SelectedFolderInfo>
                <Button
                  size="small"
                  variant="secondary-fill"
                  type="button"
                  onClick={() => {
                    setImportPath("");
                    setImportFileCount(0);
                    if (fileInputRef.current) {
                      fileInputRef.current.value = "";
                    }
                  }}
                >
                  Remove
                </Button>
              </SelectedFolder>
            )}
            <input
              ref={fileInputRef}
              type="file"
              /* @ts-expect-error webkitdirectory is non-standard */
              webkitdirectory=""
              directory=""
              style={{ display: "none" }}
              onChange={(event) => handleFolderPicked(event.target.files)}
            />
          </ImportSlot>
        ) : null}

        <Field>
          Workspace name
          <TextInput
            autoFocus
            value={name}
            onChange={(event) => {
              setName(event.target.value);
              setNameEdited(true);
            }}
            placeholder="customer-portal"
          />
        </Field>

        {eligible.length > 1 ? (
          <Field>
            Database
            <Select
              options={eligible.map((database) => ({
                value: database.id,
                label: `${database.displayName || database.databaseName}${database.isDefault ? " (default)" : ""}`,
              }))}
              value={databaseId}
              onChange={(next) => setDatabaseId(next as string)}
            />
          </Field>
        ) : null}

        <Field>
          Description
          <TextInput
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            placeholder="What this workspace is for, who owns it, and why it exists. (optional)"
          />
        </Field>

        {formError ? <DialogError role="alert">{formError}</DialogError> : null}

        <DialogActions style={{ justifyContent: "flex-end" }}>
          <Button
            size="medium"
            type="button"
            variant="secondary-fill"
            onClick={handleClose}
            disabled={busy}
          >
            Cancel
          </Button>
          <Button size="medium" type="submit" disabled={busy}>
            {busy
              ? "Creating..."
              : mode === "import"
                ? "Create and import"
                : "Create workspace"}
          </Button>
        </DialogActions>
      </FormGrid>
    );
  }
}

const SectionLabel = styled.h4`
  margin: 0;
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.02em;
`;

const ModeStrip = styled.div`
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;

  @media (max-width: 560px) {
    grid-template-columns: 1fr;
  }
`;

const ModeCard = styled.button<{ $active?: boolean }>`
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px;
  border-radius: 14px;
  border: 1.5px solid
    ${(p) => (p.$active ? "var(--afs-accent, #2563eb)" : "var(--afs-line)")};
  background: ${(p) =>
    p.$active
      ? "color-mix(in srgb, var(--afs-accent, #2563eb) 8%, transparent)"
      : "var(--afs-panel)"};
  text-align: left;
  cursor: pointer;
  transition:
    border-color 120ms ease,
    background 120ms ease;

  &:hover {
    border-color: var(--afs-accent, #2563eb);
  }
`;

const ModeTitle = styled.span`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 700;
`;

const ModeHint = styled.span`
  color: var(--afs-muted);
  font-size: 12px;
  line-height: 1.45;
`;

const ImportSlot = styled.div`
  display: flex;
  flex-direction: column;
  gap: 8px;
`;

const SelectedFolder = styled.div`
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--afs-line);
  border-radius: 12px;
  background: var(--afs-panel);
`;

const SelectedFolderInfo = styled.div`
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
`;

const SelectedFolderName = styled.span`
  font-size: 13px;
  font-weight: 700;
  color: var(--afs-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const SelectedFolderMeta = styled.span`
  font-size: 12px;
  color: var(--afs-muted);
`;

const TemplateSummary = styled.div<{ $accent: string }>`
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid
    ${(p) => `color-mix(in srgb, ${p.$accent} 40%, var(--afs-line))`};
  border-radius: 12px;
  background: ${(p) =>
    `color-mix(in srgb, ${p.$accent} 6%, var(--afs-panel))`};
`;

const TemplateSummaryIcon = styled.div<{ $accent: string }>`
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: ${({ $accent }) =>
    `color-mix(in srgb, ${$accent} 16%, transparent)`};
  color: ${({ $accent }) => $accent};
`;

const TemplateSummaryBody = styled.div`
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
`;

const TemplateSummaryTitle = styled.span`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 700;
`;

const TemplateSummaryText = styled.span`
  color: var(--afs-muted);
  font-size: 12.5px;
  line-height: 1.45;
  overflow-wrap: anywhere;
`;

const TemplateSummaryBadge = styled.span`
  flex-shrink: 0;
  color: var(--afs-muted);
  font-size: 10.5px;
  font-weight: 800;
  letter-spacing: 0.06em;
  text-transform: uppercase;
`;
