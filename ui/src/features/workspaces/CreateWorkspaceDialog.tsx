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
import type {
  AFSMCPToken,
  AFSWorkspaceDetail,
} from "../../foundation/types/afs";
import {
  findTemplate,
  templates,
  type Template,
  type TemplateSeedFile,
} from "../templates/templates-data";

type SeedMode = "blank" | "import";
type View =
  | "chooser"
  | "gallery"
  | "template-form"
  | "template-success";

type Props = {
  open: boolean;
  onClose: () => void;
  onFreeTierLimitHit?: () => void;
  initialTemplateId?: string;
};

type CreatedState = {
  workspace: AFSWorkspaceDetail;
  token: AFSMCPToken;
  template: Template;
  seededCount: number;
};

const GALLERY_TEMPLATES = templates.filter(
  (template) => template.id !== "blank",
);

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

function buildHostedMCPConfig(
  workspaceName: string,
  controlPlaneUrl: string,
  token: string,
) {
  return JSON.stringify(
    {
      mcpServers: {
        [`afs-${workspaceName}`]: {
          url: `${controlPlaneUrl.replace(/\/+$/, "")}/mcp`,
          headers: {
            Authorization: `Bearer ${token || "<token-not-returned>"}`,
          },
        },
      },
    },
    null,
    2,
  );
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
  const [entryWasTemplate, setEntryWasTemplate] = useState(false);

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [databaseId, setDatabaseId] = useState("");
  const [importPath, setImportPath] = useState("");
  const [importFileCount, setImportFileCount] = useState(0);
  const [formError, setFormError] = useState<string | null>(null);
  const [nameEdited, setNameEdited] = useState(false);
  const [created, setCreated] = useState<CreatedState | null>(null);
  const [seedingProgress, setSeedingProgress] = useState<{
    done: number;
    total: number;
  } | null>(null);
  const [copied, setCopied] = useState<string | null>(null);

  const fileInputRef = useRef<HTMLInputElement>(null);

  const selectedTemplate = useMemo(
    () => (selectedTemplateId ? findTemplate(selectedTemplateId) : null),
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

    setEntryWasTemplate(startedFromTemplate);
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
    setCreated(null);
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

  function copy(text: string, label: string) {
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(label);
      window.setTimeout(() => setCopied(null), 2000);
    });
  }

  function goToWorkspace() {
    if (!created) return;
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: created.workspace.id },
    });
    onClose();
  }

  function selectMode(next: SeedMode | "templates") {
    setFormError(null);
    if (next === "templates") {
      setView("gallery");
      return;
    }
    setMode(next);
  }

  function pickTemplate(id: string) {
    const tpl = findTemplate(id);
    if (!tpl) return;
    setSelectedTemplateId(id);
    setName(tpl.slug);
    setDescription(tpl.tagline);
    setNameEdited(false);
    setFormError(null);
    setView("template-form");
  }

  function handleBackFromGallery() {
    setView("chooser");
    setFormError(null);
  }

  function handleBackFromTemplateForm() {
    if (entryWasTemplate) {
      // Came in via /templates page; back = close.
      onClose();
      return;
    }
    setSelectedTemplateId(null);
    setName("");
    setDescription("");
    setNameEdited(false);
    setFormError(null);
    setView("gallery");
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
      });

      const token = await createToken.mutateAsync({
        databaseId: workspace.databaseId || databaseId || undefined,
        workspaceId: workspace.id,
        name: `${selectedTemplate.title} setup`,
        profile: selectedTemplate.profile,
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
      setCreated({
        workspace,
        token,
        template: selectedTemplate,
        seededCount: seeded,
      });
      setView("template-success");
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

  /* ────────────────────── view renderers ────────────────────── */

  function renderHeader() {
    if (view === "template-success" && created) {
      return (
        <DialogHeader>
          <div>
            <DialogTitle>Workspace ready: {created.workspace.name}</DialogTitle>
            <DialogBody>
              Files are in place. Connect your agent and it will pick up the
              protocol from AGENTS.md automatically.
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

    if (view === "template-form" && selectedTemplate) {
      return (
        <DialogHeader>
          <div>
            <DialogTitle>Create &ldquo;{selectedTemplate.title}&rdquo;</DialogTitle>
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

    if (view === "gallery") {
      return (
        <DialogHeader>
          <div>
            <DialogTitle>Choose a template</DialogTitle>
            <DialogBody>
              Pre-shaped workspaces for teams of agents working together.
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
    if (view === "template-success" && created) {
      const mcpConfig = buildHostedMCPConfig(
        created.workspace.name,
        getControlPlaneURL(),
        created.token.token ?? "",
      );
      return (
        <SuccessPanel>
          <SeededBanner>
            <SeededDot aria-hidden>&#10003;</SeededDot>
            <SeededText>
              Seeded <strong>{created.seededCount}</strong> file
              {created.seededCount === 1 ? "" : "s"} into{" "}
              <code>{created.workspace.name}</code>. The workspace layout is
              ready.
            </SeededText>
          </SeededBanner>

          <SuccessSection>
            <SectionLabel>Point your agent at this workspace</SectionLabel>
            <SectionHint>
              Add the block below to your MCP client config. The token is
              shown once — save it somewhere safe now.
            </SectionHint>
            <CodeBlock>{mcpConfig}</CodeBlock>
            <InlineActionsRight>
              <Button
                size="small"
                variant="secondary-fill"
                onClick={() => copy(mcpConfig, "config")}
              >
                {copied === "config" ? "Copied!" : "Copy MCP config"}
              </Button>
            </InlineActionsRight>
            <ClientHints>
              <ClientHint>
                <strong>Claude Code.</strong> Paste into{" "}
                <code>.mcp.json</code> at your project root or into{" "}
                <code>~/.claude.json</code> under <code>mcpServers</code>.
              </ClientHint>
              <ClientHint>
                <strong>Codex.</strong> Add to{" "}
                <code>~/.codex/config.toml</code> as{" "}
                <code>[mcp_servers.afs-{created.workspace.name}]</code> with{" "}
                <code>url</code> and <code>bearer_token</code>.
              </ClientHint>
              <ClientHint>
                <strong>Prefer the CLI?</strong> Run{" "}
                <code>
                  afs mcp --workspace {created.workspace.name} --profile{" "}
                  {created.template.profile}
                </code>{" "}
                after <code>afs login</code> with the token above.
              </ClientHint>
            </ClientHints>
          </SuccessSection>

          <SuccessSection>
            <SectionLabel>Then ask your agent</SectionLabel>
            <FirstPrompt>
              &ldquo;{created.template.firstPrompt}&rdquo;
            </FirstPrompt>
          </SuccessSection>

          <DialogActions style={{ justifyContent: "flex-end" }}>
            <Button
              size="medium"
              variant="secondary-fill"
              onClick={handleClose}
            >
              Close
            </Button>
            <Button size="medium" onClick={goToWorkspace}>
              Open workspace
            </Button>
          </DialogActions>
        </SuccessPanel>
      );
    }

    if (view === "gallery") {
      return (
        <GalleryBody>
          <BackRow>
            <BackButton type="button" onClick={handleBackFromGallery}>
              <BackArrow aria-hidden>&larr;</BackArrow>
              Back
            </BackButton>
          </BackRow>
          <GalleryGrid>
            {GALLERY_TEMPLATES.map((template) => (
              <GalleryCard
                key={template.id}
                type="button"
                onClick={() => pickTemplate(template.id)}
                aria-label={`Use the ${template.title} template`}
              >
                <GalleryCardHead>
                  <GalleryIconSlot $accent={template.accent}>
                    <template.icon size="M" />
                  </GalleryIconSlot>
                  <GalleryAddFab aria-hidden>+</GalleryAddFab>
                </GalleryCardHead>
                <GalleryCardTitle>{template.title}</GalleryCardTitle>
                <GalleryCardBody>{template.tagline}</GalleryCardBody>
                <GalleryProfileBadge $profile={template.profile}>
                  {template.profileLabel}
                </GalleryProfileBadge>
              </GalleryCard>
            ))}
          </GalleryGrid>
        </GalleryBody>
      );
    }

    if (view === "template-form" && selectedTemplate) {
      return (
        <FormGrid onSubmit={submitTemplateForm}>
          <BackRow>
            <BackButton type="button" onClick={handleBackFromTemplateForm}>
              <BackArrow aria-hidden>&larr;</BackArrow>
              {entryWasTemplate ? "Back" : "Back to templates"}
            </BackButton>
          </BackRow>

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
                  ? "Creating…"
                  : "Create workspace"}
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
            $active={mode === "blank" && view === "chooser"}
            onClick={() => selectMode("blank")}
          >
            <ModeTitle>Blank</ModeTitle>
            <ModeHint>Empty workspace, add files later.</ModeHint>
          </ModeCard>
          <ModeCard
            type="button"
            onClick={() => selectMode("templates")}
          >
            <ModeTitle>Templates</ModeTitle>
            <ModeHint>Pre-shaped for a multi-agent workflow.</ModeHint>
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

/* ── Styled components ── */

const SectionLabel = styled.h4`
  margin: 0;
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.02em;
`;

const SectionHint = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.55;
`;

const ModeStrip = styled.div`
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
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

/* Back button */

const BackRow = styled.div`
  display: flex;
  margin: -4px 0 4px;
`;

const BackButton = styled.button`
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: none;
  background: transparent;
  color: var(--afs-muted);
  font-size: 13px;
  font-weight: 600;
  padding: 4px 6px;
  border-radius: 8px;
  cursor: pointer;
  transition: color 120ms ease, background 120ms ease;

  &:hover {
    color: var(--afs-accent, #2563eb);
    background: color-mix(in srgb, var(--afs-muted) 8%, transparent);
  }
`;

const BackArrow = styled.span`
  font-size: 14px;
  line-height: 1;
`;

/* Gallery (inside dialog) */

const GalleryBody = styled.div`
  display: flex;
  flex-direction: column;
  gap: 12px;
`;

const GalleryGrid = styled.div`
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;

  @media (max-width: 600px) {
    grid-template-columns: 1fr;
  }
`;

const GalleryCard = styled.button`
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px;
  border: 1px solid var(--afs-line);
  border-radius: 14px;
  background: var(--afs-panel);
  text-align: left;
  cursor: pointer;
  transition:
    transform 140ms ease,
    border-color 140ms ease,
    box-shadow 140ms ease;

  &:hover {
    border-color: var(--afs-accent, #2563eb);
    transform: translateY(-1px);
    box-shadow: 0 8px 20px rgba(8, 6, 13, 0.08);
  }

  &:hover [data-fab] {
    background: var(--afs-accent, #2563eb);
    color: #fff;
  }
`;

const GalleryCardHead = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
`;

const GalleryIconSlot = styled.div<{ $accent: string }>`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: ${({ $accent }) =>
    `color-mix(in srgb, ${$accent} 18%, transparent)`};
  color: ${({ $accent }) => $accent};
`;

const GalleryAddFab = styled.span.attrs({ "data-fab": true })`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  border: 1px solid var(--afs-line);
  background: var(--afs-panel-strong);
  color: var(--afs-muted);
  font-size: 16px;
  font-weight: 600;
  line-height: 1;
  transition: background 140ms ease, color 140ms ease, border-color 140ms ease;
`;

const GalleryCardTitle = styled.h3`
  margin: 0;
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 700;
`;

const GalleryCardBody = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 12.5px;
  line-height: 1.5;
`;

const GalleryProfileBadge = styled.span<{ $profile: string }>`
  align-self: flex-start;
  margin-top: 2px;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  background: ${({ $profile }) => profileBackground($profile)};
  color: ${({ $profile }) => profileForeground($profile)};
`;

function profileBackground(profile: string) {
  switch (profile) {
    case "workspace-ro":
      return "color-mix(in srgb, #2563eb 14%, transparent)";
    case "workspace-rw-checkpoint":
      return "color-mix(in srgb, #22c55e 18%, transparent)";
    case "workspace-rw":
    default:
      return "color-mix(in srgb, #f59e0b 16%, transparent)";
  }
}

function profileForeground(profile: string) {
  switch (profile) {
    case "workspace-ro":
      return "#2563eb";
    case "workspace-rw-checkpoint":
      return "#16a34a";
    case "workspace-rw":
    default:
      return "#b45309";
  }
}

/* Template summary pill on template-form screen */

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
`;

const TemplateSummaryBadge = styled.span`
  flex-shrink: 0;
  color: var(--afs-muted);
  font-size: 10.5px;
  font-weight: 800;
  letter-spacing: 0.06em;
  text-transform: uppercase;
`;

/* Success panel */

const SuccessPanel = styled.div`
  display: grid;
  gap: 20px;
`;

const SuccessSection = styled.div`
  display: grid;
  gap: 8px;

  & + & {
    padding-top: 4px;
    border-top: 1px solid var(--afs-line);
  }
`;

const SeededBanner = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  border-radius: 12px;
  background: color-mix(in srgb, #22c55e 14%, transparent);
  border: 1px solid color-mix(in srgb, #22c55e 40%, transparent);
`;

const SeededDot = styled.span`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: #16a34a;
  color: #fff;
  font-size: 13px;
  font-weight: 800;
`;

const SeededText = styled.span`
  color: var(--afs-ink);
  font-size: 13px;
  line-height: 1.5;

  strong {
    font-weight: 700;
  }

  code {
    font-family: "SF Mono", "Fira Code", "Consolas", monospace;
    font-size: 12px;
    padding: 1px 6px;
    border-radius: 4px;
    background: color-mix(in srgb, #22c55e 16%, transparent);
    color: var(--afs-ink);
  }
`;

const CodeBlock = styled.pre`
  margin: 0;
  padding: 14px 16px;
  border: 1px solid var(--afs-line);
  border-radius: 12px;
  background: rgba(15, 23, 42, 0.94);
  color: #e2e8f0;
  font-family: "SF Mono", "Fira Code", "Consolas", monospace;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
`;

const InlineActionsRight = styled.div`
  display: flex;
  justify-content: flex-end;
`;

const ClientHints = styled.div`
  display: grid;
  gap: 8px;
  margin-top: 4px;
`;

const ClientHint = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 12.5px;
  line-height: 1.55;

  code {
    font-family: "SF Mono", "Fira Code", "Consolas", monospace;
    font-size: 11.5px;
    padding: 1px 6px;
    margin: 0 2px;
    border-radius: 4px;
    background: color-mix(in srgb, var(--afs-line) 60%, transparent);
    color: var(--afs-ink);
  }
`;

const FirstPrompt = styled.blockquote`
  margin: 0;
  padding: 10px 14px;
  border-left: 3px solid var(--afs-accent, #2563eb);
  background: color-mix(in srgb, var(--afs-accent, #2563eb) 8%, transparent);
  color: var(--afs-ink);
  font-size: 14px;
  line-height: 1.55;
  font-style: italic;
`;
