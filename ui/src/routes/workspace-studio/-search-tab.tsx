import { Button, Select } from "@redis-ui/components";
import { useEffect, useState } from "react";
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
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
  TextInput,
} from "../../components/afs-kit";
import {
  useQueryWorkspaceMutation,
  useRebuildWorkspaceQueryIndexMutation,
  useUpdateWorkspaceConfigMutation,
  useWorkspaceConfig,
  useWorkspaceQueryIndexStatus,
} from "../../foundation/hooks/use-afs";
import type {
  AFSFileQueryMode,
  AFSFileQueryResponse,
  AFSWorkspaceConfig,
  AFSWorkspaceDetail,
  AFSWorkspaceQueryIndexStatus,
} from "../../foundation/types/afs";

type Props = {
  workspace: AFSWorkspaceDetail;
};

export function SearchTab({ workspace }: Props) {
  const [queryText, setQueryText] = useState("");
  const [mode, setMode] = useState<AFSFileQueryMode>("query");
  const [limit, setLimit] = useState("10");
  const [explain, setExplain] = useState(true);
  const [rebuildDialogOpen, setRebuildDialogOpen] = useState(false);
  const [rebuildPath, setRebuildPath] = useState("/");
  const [forceRebuild, setForceRebuild] = useState(false);
  const [embeddingsDialogOpen, setEmbeddingsDialogOpen] = useState(false);
  const [settingsNotice, setSettingsNotice] = useState<string | null>(null);
  const [settingsError, setSettingsError] = useState<string | null>(null);
  const [embeddingsEnabled, setEmbeddingsEnabled] = useState(false);
  const [embeddingsModel, setEmbeddingsModel] = useState("");
  const [chunkStrategy, setChunkStrategy] = useState<"auto" | "regex">("auto");
  const [draftEmbeddingsModel, setDraftEmbeddingsModel] = useState("");
  const [draftChunkStrategy, setDraftChunkStrategy] = useState<"auto" | "regex">("auto");

  const statusQuery = useWorkspaceQueryIndexStatus({
    databaseId: workspace.databaseId,
    workspaceId: workspace.id,
    path: "/",
  });
  const configQuery = useWorkspaceConfig({
    databaseId: workspace.databaseId,
    workspaceId: workspace.id,
  });
  const rebuildIndex = useRebuildWorkspaceQueryIndexMutation();
  const updateConfig = useUpdateWorkspaceConfigMutation();
  const queryWorkspace = useQueryWorkspaceMutation();

  useEffect(() => {
    const config = configQuery.data;
    if (config == null) {
      return;
    }
    setEmbeddingsEnabled(config.query.embeddings.enabled);
    setEmbeddingsModel(config.query.embeddings.model);
    setChunkStrategy(config.query.embeddings.chunkStrategy === "regex" ? "regex" : "auto");
    setDraftEmbeddingsModel(config.query.embeddings.model);
    setDraftChunkStrategy(config.query.embeddings.chunkStrategy === "regex" ? "regex" : "auto");
  }, [configQuery.data]);

  const status = statusQuery.data;
  const progress = status ? queryProgress(status) : 0;
  const queryResult = queryWorkspace.data;

  function saveQuerySettings(input: {
    enabled: boolean;
    model: string;
    chunkStrategy: "auto" | "regex";
    closeDialog?: boolean;
  }) {
    const current = configQuery.data ?? defaultWorkspaceConfig();
    const next: AFSWorkspaceConfig = {
      ...current,
      query: {
        embeddings: {
          enabled: input.enabled,
          model: input.model.trim(),
          chunkStrategy: input.chunkStrategy,
        },
      },
    };
    setSettingsError(null);
    setSettingsNotice(null);
    void updateConfig
      .mutateAsync({
        databaseId: workspace.databaseId,
        workspaceId: workspace.id,
        config: next,
      })
      .then(() => {
        setEmbeddingsEnabled(input.enabled);
        setEmbeddingsModel(input.model.trim());
        setChunkStrategy(input.chunkStrategy);
        setSettingsNotice("Settings saved.");
        if (input.closeDialog) {
          setEmbeddingsDialogOpen(false);
        }
      })
      .catch((error) => {
        setSettingsError(
          error instanceof Error ? error.message : "Unable to save query settings.",
        );
      });
  }

  function openEmbeddingsDialog() {
    setDraftEmbeddingsModel(embeddingsModel);
    setDraftChunkStrategy(chunkStrategy);
    setEmbeddingsDialogOpen(true);
  }

  function runQuery() {
    const normalizedLimit = parsePositiveInt(limit, 10);
    void queryWorkspace.mutateAsync({
      databaseId: workspace.databaseId,
      workspaceId: workspace.id,
      request: {
        path: "/",
        mode,
        query: queryText,
        limit: normalizedLimit,
        explain,
      },
    });
  }

  function rebuildQueryIndex() {
    void rebuildIndex
      .mutateAsync({
        databaseId: workspace.databaseId,
        workspaceId: workspace.id,
        path: normalizeWorkspacePath(rebuildPath),
        force: forceRebuild,
        wait: false,
      })
      .then(() => {
        setRebuildDialogOpen(false);
        void statusQuery.refetch();
      });
  }

  return (
    <>
      <SectionGrid>
        <SectionCard $span={6}>
          <SectionHeader>
            <SectionTitle
              title="Search Index Status"
              body="BM25 keyword index state for this workspace."
            />
            <Button
              size="medium"
              type="button"
              variant="secondary-fill"
              onClick={() => {
                setRebuildPath("/");
                setForceRebuild(false);
                setRebuildDialogOpen(true);
              }}
            >
              Rebuild index
            </Button>
          </SectionHeader>

          {isIndexing(status) ? (
            <ProgressBlock>
              <ProgressTopline>
                <strong>{Math.round(progress * 100)}%</strong>
                <span>{status?.keyword.ready ?? 0} ready</span>
              </ProgressTopline>
              <ProgressTrack>
                <ProgressBar style={{ width: `${Math.round(progress * 100)}%` }} />
              </ProgressTrack>
              <IndexFacts>
                <span>{status?.keyword.pending ?? 0} pending</span>
                <span>{status?.keyword.stale ?? 0} stale</span>
                <span>{status?.keyword.unindexed ?? 0} unindexed</span>
                <span>{status?.keyword.errors ?? 0} errors</span>
              </IndexFacts>
            </ProgressBlock>
          ) : (
            <IndexStatusBlock>
              <SearchBadge $tone={statusTone(status?.state)}>
                {statusLabel(status)}
              </SearchBadge>
              <IndexStatusText>{indexStatusText(status)}</IndexStatusText>
            </IndexStatusBlock>
          )}

          {statusQuery.isError ? (
            <DialogError role="alert">
              {statusQuery.error instanceof Error
                ? statusQuery.error.message
                : "Unable to load query index status."}
            </DialogError>
          ) : null}
          {rebuildIndex.isError ? (
            <DialogError role="alert">
              {rebuildIndex.error instanceof Error
                ? rebuildIndex.error.message
                : "Unable to rebuild query index."}
            </DialogError>
          ) : null}
        </SectionCard>

        <SectionCard $span={6}>
          <SectionHeader>
            <SectionTitle
              title="Settings"
              body="Workspace search options."
            />
            <Button
              size="medium"
              type="button"
              variant="secondary-fill"
              onClick={openEmbeddingsDialog}
            >
              More &gt;
            </Button>
          </SectionHeader>
          <SettingsControlRow>
            <SwitchControl
              checked={embeddingsEnabled}
              disabled={updateConfig.isPending || configQuery.isLoading}
              label="Semantic Embeddings"
              onChange={(nextEnabled) => {
                setEmbeddingsEnabled(nextEnabled);
                saveQuerySettings({
                  enabled: nextEnabled,
                  model: embeddingsModel,
                  chunkStrategy,
                });
              }}
            />
            <ToggleStateText>{embeddingsEnabled ? "On" : "Off"}</ToggleStateText>
          </SettingsControlRow>
          {settingsError ? <DialogError role="alert">{settingsError}</DialogError> : null}
          {settingsNotice ? <Notice role="status">{settingsNotice}</Notice> : null}
        </SectionCard>

        <SectionCard $span={12}>
          <SectionHeader>
            <SectionTitle
              title="Search Workspace"
              body="Run the same ranked retrieval that agents get through file_query."
            />
          </SectionHeader>

          <QueryForm
            onSubmit={(event) => {
              event.preventDefault();
              runQuery();
            }}
          >
            <Field>
              Query
              <TextInput
                value={queryText}
                onChange={(event) => setQueryText(event.currentTarget.value)}
                placeholder="where is auth token configuration handled?"
              />
            </Field>
            <QueryControls>
              <Field>
                Mode
                <SelectWrap>
                  <Select
                    aria-label="Search mode"
                    options={[
                      { value: "query", label: "Hybrid" },
                      { value: "keyword", label: "BM25 keyword" },
                      { value: "semantic", label: "Semantic only" },
                    ]}
                    value={mode}
                    onChange={(next) => setMode(next as AFSFileQueryMode)}
                  />
                </SelectWrap>
              </Field>
              <SearchOptionsRow>
                <LimitField>
                  Limit
                  <TextInput
                    value={limit}
                    onChange={(event) => setLimit(event.currentTarget.value)}
                    inputMode="numeric"
                  />
                </LimitField>
                <SwitchControl
                  checked={explain}
                  label="Show explanation"
                  onChange={setExplain}
                />
              </SearchOptionsRow>
            </QueryControls>
            <DialogActions style={{ justifyContent: "flex-end" }}>
              <Button
                size="medium"
                type="submit"
                disabled={queryWorkspace.isPending || queryText.trim() === ""}
              >
                {queryWorkspace.isPending ? "Searching..." : "Run query"}
              </Button>
            </DialogActions>
          </QueryForm>

          {queryWorkspace.isError ? (
            <DialogError role="alert">
              {queryWorkspace.error instanceof Error
                ? queryWorkspace.error.message
                : "Workspace query failed."}
            </DialogError>
          ) : null}
          {queryResult ? <QueryResults response={queryResult} /> : null}
        </SectionCard>
      </SectionGrid>

      {rebuildDialogOpen ? (
        <DialogOverlay
          role="dialog"
          aria-modal="true"
          aria-labelledby="rebuild-query-index-title"
          onClick={() => {
            if (!rebuildIndex.isPending) {
              setRebuildDialogOpen(false);
            }
          }}
        >
          <DialogCard onClick={(event) => event.stopPropagation()}>
            <DialogHeader>
              <div>
                <DialogTitle id="rebuild-query-index-title">
                  Rebuild index
                </DialogTitle>
                <DialogBody>
                  Rebuild BM25 query chunks for the selected workspace path.
                </DialogBody>
              </div>
              <DialogCloseButton
                type="button"
                aria-label="Close"
                onClick={() => setRebuildDialogOpen(false)}
                disabled={rebuildIndex.isPending}
              >
                ×
              </DialogCloseButton>
            </DialogHeader>
            <FormGrid
              onSubmit={(event) => {
                event.preventDefault();
                rebuildQueryIndex();
              }}
            >
              <Field>
                Path
                <TextInput
                  autoFocus
                  value={rebuildPath}
                  onChange={(event) => setRebuildPath(event.currentTarget.value)}
                  placeholder="/"
                />
              </Field>
              <SwitchControl
                checked={forceRebuild}
                label="Force rebuild existing chunks"
                onChange={setForceRebuild}
              />
              <DialogActions style={{ justifyContent: "flex-end" }}>
                <Button
                  size="medium"
                  type="button"
                  variant="secondary-fill"
                  onClick={() => setRebuildDialogOpen(false)}
                  disabled={rebuildIndex.isPending}
                >
                  Cancel
                </Button>
                <Button size="medium" type="submit" disabled={rebuildIndex.isPending}>
                  {rebuildIndex.isPending ? "Rebuilding..." : "Rebuild index"}
                </Button>
              </DialogActions>
            </FormGrid>
          </DialogCard>
        </DialogOverlay>
      ) : null}

      {embeddingsDialogOpen ? (
        <DialogOverlay
          role="dialog"
          aria-modal="true"
          aria-labelledby="semantic-embeddings-title"
          onClick={() => {
            if (!updateConfig.isPending) {
              setEmbeddingsDialogOpen(false);
            }
          }}
        >
          <DialogCard onClick={(event) => event.stopPropagation()}>
            <DialogHeader>
              <div>
                <DialogTitle id="semantic-embeddings-title">
                  Semantic embeddings
                </DialogTitle>
                <DialogBody>
                  Configure the semantic-only query settings for this workspace.
                </DialogBody>
              </div>
              <DialogCloseButton
                type="button"
                aria-label="Close"
                onClick={() => setEmbeddingsDialogOpen(false)}
                disabled={updateConfig.isPending}
              >
                ×
              </DialogCloseButton>
            </DialogHeader>
            <FormGrid
              onSubmit={(event) => {
                event.preventDefault();
                saveQuerySettings({
                  enabled: embeddingsEnabled,
                  model: draftEmbeddingsModel,
                  chunkStrategy: draftChunkStrategy,
                  closeDialog: true,
                });
              }}
            >
              <Field>
                Embedding model
                <TextInput
                  value={draftEmbeddingsModel}
                  onChange={(event) => setDraftEmbeddingsModel(event.currentTarget.value)}
                  placeholder="embeddinggemma"
                />
              </Field>
              <Field>
                Chunk strategy
                <SelectWrap>
                  <Select
                    aria-label="Chunk strategy"
                    options={[
                      { value: "auto", label: "Auto" },
                      { value: "regex", label: "Regex" },
                    ]}
                    value={draftChunkStrategy}
                    onChange={(next) =>
                      setDraftChunkStrategy(next === "regex" ? "regex" : "auto")
                    }
                  />
                </SelectWrap>
              </Field>
              {settingsError ? <DialogError role="alert">{settingsError}</DialogError> : null}
              <DialogActions style={{ justifyContent: "flex-end" }}>
                <Button
                  size="medium"
                  type="button"
                  variant="secondary-fill"
                  onClick={() => setEmbeddingsDialogOpen(false)}
                  disabled={updateConfig.isPending}
                >
                  Cancel
                </Button>
                <Button size="medium" type="submit" disabled={updateConfig.isPending}>
                  {updateConfig.isPending ? "Saving..." : "Save settings"}
                </Button>
              </DialogActions>
            </FormGrid>
          </DialogCard>
        </DialogOverlay>
      ) : null}
    </>
  );
}

function SwitchControl({
  checked,
  disabled = false,
  label,
  onChange,
}: {
  checked: boolean;
  disabled?: boolean;
  label: string;
  onChange: (checked: boolean) => void;
}) {
  return (
    <SwitchButton
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={() => onChange(!checked)}
    >
      <SwitchTrack $on={checked}>
        <SwitchThumb $on={checked} />
      </SwitchTrack>
      <SwitchLabel>{label}</SwitchLabel>
    </SwitchButton>
  );
}

function QueryResults({ response }: { response: AFSFileQueryResponse }) {
  return (
    <ResultsBlock>
      {response.warnings.length > 0 ? (
        <WarningList>
          {response.warnings.map((warning) => (
            <li key={warning}>{warning}</li>
          ))}
        </WarningList>
      ) : null}
      {response.explain.length > 0 ? (
        <ExplainRow>
          {response.explain.map((entry) => (
            <SearchBadge key={`${entry.stage}:${entry.message}`} $tone="neutral">
              {entry.stage}: {entry.message}
            </SearchBadge>
          ))}
        </ExplainRow>
      ) : null}
      {response.results.length === 0 ? (
        <EmptyResult>No ranked results for this query.</EmptyResult>
      ) : (
        <ResultList>
          {response.results.map((result) => (
            <ResultItem key={`${result.path}:${result.chunkId ?? result.startLine ?? 0}`}>
              <ResultHeader>
                <ResultPath>{result.path}</ResultPath>
                <SearchBadge $tone="good">score {result.score.toFixed(2)}</SearchBadge>
              </ResultHeader>
              <ResultMeta>
                {lineRange(result)} {result.searchTypes.length > 0 ? `· ${result.searchTypes.join(", ")}` : ""}
              </ResultMeta>
              <ResultSnippet>{result.snippet}</ResultSnippet>
            </ResultItem>
          ))}
        </ResultList>
      )}
    </ResultsBlock>
  );
}

function defaultWorkspaceConfig(): AFSWorkspaceConfig {
  return {
    versioning: {
      mode: "off",
      includeGlobs: [],
      excludeGlobs: [],
      maxVersionsPerFile: 0,
      maxAgeDays: 0,
      maxTotalBytes: 0,
      largeFileCutoffBytes: 0,
    },
    query: {
      embeddings: {
        enabled: false,
        model: "",
        chunkStrategy: "auto",
      },
    },
  };
}

function normalizeWorkspacePath(value: string) {
  const trimmed = value.trim();
  if (trimmed === "" || trimmed === "/") {
    return "/";
  }
  return `/${trimmed.replace(/^\/+/, "")}`;
}

function parsePositiveInt(value: string, fallback: number) {
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return fallback;
  }
  return parsed;
}

function queryProgress(status: AFSWorkspaceQueryIndexStatus) {
  const total = Math.max(1, status.keyword.files);
  return Math.min(1, (status.keyword.ready + status.keyword.skipped) / total);
}

function isIndexing(status?: AFSWorkspaceQueryIndexStatus) {
  return (
    status?.state === "indexing" ||
    (status?.keyword.pending ?? 0) > 0 ||
    (status?.keyword.stale ?? 0) > 0
  );
}

function indexStatusText(status?: AFSWorkspaceQueryIndexStatus) {
  if (!status) {
    return "Checking index status.";
  }
  switch (status.state) {
    case "ready":
      return "The index is up to date.";
    case "needs_rebuild":
      return "The index needs a rebuild.";
    case "unavailable":
      return "RedisSearch is unavailable; queries use fallback ranking.";
    case "error":
      return "The index has errors.";
    default:
      return status.message || "Index status is unknown.";
  }
}

function statusTone(state?: string): "good" | "warn" | "neutral" | "bad" {
  switch (state) {
    case "ready":
      return "good";
    case "indexing":
    case "needs_rebuild":
      return "warn";
    case "error":
      return "bad";
    default:
      return "neutral";
  }
}

function statusLabel(status?: AFSWorkspaceQueryIndexStatus) {
  if (!status) {
    return "Loading";
  }
  switch (status.state) {
    case "ready":
      return "On";
    case "indexing":
      return "Indexing";
    case "needs_rebuild":
      return "Needs rebuild";
    case "unavailable":
      return "Fallback";
    case "error":
      return "Error";
    default:
      return status.state || "Unknown";
  }
}

function lineRange(result: { startLine?: number; endLine?: number }) {
  if (!result.startLine) {
    return "Workspace chunk";
  }
  if (!result.endLine || result.endLine === result.startLine) {
    return `Line ${result.startLine}`;
  }
  return `Lines ${result.startLine}-${result.endLine}`;
}

const SearchBadge = styled.span<{ $tone: "good" | "warn" | "neutral" | "bad" }>`
  display: inline-flex;
  align-items: center;
  min-height: 26px;
  border-radius: 6px;
  padding: 4px 9px;
  border: 1px solid
    ${({ $tone }) =>
      $tone === "good"
        ? "rgba(22, 163, 74, 0.32)"
        : $tone === "warn"
          ? "rgba(217, 119, 6, 0.35)"
          : $tone === "bad"
            ? "rgba(220, 38, 38, 0.32)"
            : "var(--afs-line)"};
  background:
    ${({ $tone }) =>
      $tone === "good"
        ? "rgba(22, 163, 74, 0.08)"
        : $tone === "warn"
          ? "rgba(217, 119, 6, 0.1)"
          : $tone === "bad"
            ? "rgba(220, 38, 38, 0.08)"
            : "var(--afs-panel)"};
  color:
    ${({ $tone }) =>
      $tone === "good"
        ? "#15803d"
        : $tone === "warn"
          ? "#a16207"
          : $tone === "bad"
            ? "#b91c1c"
            : "var(--afs-ink-soft)"};
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
`;

const IndexStatusBlock = styled.div`
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
`;

const IndexStatusText = styled.div`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 700;
`;

const ProgressBlock = styled.div`
  display: grid;
  gap: 8px;
`;

const ProgressTopline = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 12px;
  color: var(--afs-ink-soft);
  font-size: 13px;
`;

const ProgressTrack = styled.div`
  height: 8px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--afs-line);
`;

const ProgressBar = styled.div`
  height: 100%;
  border-radius: inherit;
  background: var(--afs-accent);
`;

const IndexFacts = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 8px 12px;
  color: var(--afs-muted);
  font-size: 12px;
`;

const SettingsControlRow = styled.div`
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  border: 1px solid var(--afs-line);
  border-radius: 8px;
  padding: 14px;
  background: var(--afs-panel);
`;

const ToggleStateText = styled.span`
  color: var(--afs-muted);
  font-size: 13px;
  font-weight: 700;
`;

const SelectWrap = styled.div`
  min-width: 0;
`;

const Notice = styled.div`
  color: #15803d;
  font-size: 13px;
  font-weight: 700;
`;

const QueryForm = styled.form`
  display: grid;
  gap: 14px;
`;

const QueryControls = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: minmax(180px, 1fr) minmax(260px, 1.4fr);
  align-items: end;

  @media (max-width: 860px) {
    grid-template-columns: 1fr;
  }
`;

const SearchOptionsRow = styled.div`
  display: flex;
  align-items: end;
  gap: 14px;

  @media (max-width: 560px) {
    align-items: stretch;
    flex-direction: column;
  }
`;

const LimitField = styled(Field)`
  max-width: 92px;
`;

const SwitchButton = styled.button`
  display: inline-flex;
  align-items: center;
  gap: 10px;
  min-height: 38px;
  border: 0;
  background: transparent;
  color: var(--afs-ink);
  padding: 0;
  font: inherit;
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;

  &:disabled {
    cursor: not-allowed;
    opacity: 0.62;
  }
`;

const SwitchTrack = styled.span<{ $on: boolean }>`
  position: relative;
  display: inline-flex;
  width: 38px;
  height: 22px;
  flex-shrink: 0;
  border-radius: 999px;
  background: ${({ $on }) => ($on ? "var(--afs-focus, #2563eb)" : "var(--afs-line-strong, #cbd5e1)")};
  transition: background 160ms ease;

  ${SwitchButton}:focus-visible & {
    box-shadow: 0 0 0 3px var(--afs-focus-soft);
  }
`;

const SwitchThumb = styled.span<{ $on: boolean }>`
  position: absolute;
  top: 2px;
  left: 2px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: white;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.16);
  transform: translateX(${({ $on }) => ($on ? "16px" : "0")});
  transition: transform 160ms ease;
`;

const SwitchLabel = styled.span`
  white-space: nowrap;
`;

const ResultsBlock = styled.div`
  display: grid;
  gap: 12px;
  margin-top: 20px;
`;

const WarningList = styled.ul`
  margin: 0;
  padding-left: 20px;
  color: #a16207;
  font-size: 13px;
`;

const ExplainRow = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
`;

const EmptyResult = styled.div`
  border: 1px dashed var(--afs-line-strong);
  border-radius: 8px;
  padding: 18px;
  color: var(--afs-muted);
  text-align: center;
`;

const ResultList = styled.div`
  display: grid;
  gap: 10px;
`;

const ResultItem = styled.article`
  border: 1px solid var(--afs-line);
  border-radius: 8px;
  padding: 14px;
  background: var(--afs-panel);
`;

const ResultHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
`;

const ResultPath = styled.div`
  color: var(--afs-ink);
  font-family: var(--afs-mono);
  font-size: 13px;
  font-weight: 700;
  overflow-wrap: anywhere;
`;

const ResultMeta = styled.div`
  margin-top: 5px;
  color: var(--afs-muted);
  font-size: 12px;
`;

const ResultSnippet = styled.p`
  margin: 10px 0 0;
  color: var(--afs-ink-soft);
  font-size: 13px;
  line-height: 1.55;
  overflow-wrap: anywhere;
`;
