import { Button, Typography } from "@redislabsdev/redis-ui-components";
import { useEffect, useMemo, useState } from "react";
import styled from "styled-components";
import {
  CardHeader,
  EditorArea,
  EditorPanel,
  Field,
  FileButton,
  FileList,
  FileStudio,
  InlineActions,
  MetaRow,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
  Select,
  Tag,
} from "../../components/afs-kit";
import { formatBytes } from "../../foundation/api/afs";
import {
  useUpdateWorkspaceFileMutation,
  useWorkspaceFileContent,
  useWorkspaceTree,
} from "../../foundation/hooks/use-afs";
import type { AFSWorkspaceDetail, AFSWorkspaceView } from "../../foundation/types/afs";

type Props = {
  workspace: AFSWorkspaceDetail;
  databaseId: string;
  browserView: AFSWorkspaceView;
  onBrowserViewChange: (view: AFSWorkspaceView) => void;
};

export function FilesTab({ workspace, databaseId, browserView, onBrowserViewChange }: Props) {
  const updateFile = useUpdateWorkspaceFileMutation();

  const [currentPath, setCurrentPath] = useState("/");
  const [selectedPath, setSelectedPath] = useState("");
  const [draftContent, setDraftContent] = useState("");

  useEffect(() => {
    setCurrentPath("/");
    setSelectedPath("");
  }, [browserView]);

  const treeQuery = useWorkspaceTree(
    {
      databaseId,
      workspaceId: workspace.id,
      view: browserView,
      path: currentPath,
      depth: 1,
    },
    true,
  );

  const selectedFileQuery = useWorkspaceFileContent(
    {
      databaseId,
      workspaceId: workspace.id,
      view: browserView,
      path: selectedPath,
    },
    selectedPath !== "",
  );

  useEffect(() => {
    const file = selectedFileQuery.data;
    setDraftContent(file?.content ?? file?.target ?? "");
  }, [
    selectedFileQuery.data?.content,
    selectedFileQuery.data?.revision,
    selectedFileQuery.data?.target,
  ]);

  const browserItems = treeQuery.data?.items ?? [];
  const selectedFile = selectedFileQuery.data;
  const editable =
    workspace.capabilities.editWorkingCopy === true &&
    browserView === "working-copy" &&
    selectedFile?.kind === "file";
  const currentViewLabel = useMemo(() => viewLabel(browserView, workspace), [browserView, workspace]);

  return (
    <SectionGrid>
      <SectionCard $span={12}>
        <SectionHeader>
          <SectionTitle title="Filesystem browser" />
        </SectionHeader>

        <BrowserBanner>
          <BrowserMetric>
            <BrowserMetricLabel>Current view</BrowserMetricLabel>
            <BrowserMetricValue>{currentViewLabel}</BrowserMetricValue>
          </BrowserMetric>
          <BrowserMetric>
            <BrowserMetricLabel>Current directory</BrowserMetricLabel>
            <BrowserMetricValue>{currentPath}</BrowserMetricValue>
          </BrowserMetric>
        </BrowserBanner>

        <FileStudio>
          <BrowserPanel>
            <CardHeader>
              <div>
                <Typography.Heading component="h3" size="S">
                  Directory slice
                </Typography.Heading>
                <Typography.Body color="secondary" component="p">
                  {currentPath}
                </Typography.Body>
              </div>
              <InlineActions>
                <Button
                  size="medium"
                  variant="secondary-fill"
                  disabled={currentPath === "/"}
                  onClick={() => {
                    setCurrentPath(parentPath(currentPath));
                    setSelectedPath("");
                  }}
                >
                  Up
                </Button>
              </InlineActions>
            </CardHeader>

            <Field>
              View
              <Select
                value={browserView}
                onChange={(event) => {
                  onBrowserViewChange(event.target.value as AFSWorkspaceView);
                  setCurrentPath("/");
                  setSelectedPath("");
                }}
              >
                {browserViewOptions(workspace).map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </Select>
            </Field>

            {treeQuery.isLoading ? (
              <PanelNote>Loading directory contents...</PanelNote>
            ) : null}

            {treeQuery.isError ? <PanelNote>Unable to load this directory.</PanelNote> : null}

            <FileList style={{ marginTop: 14 }}>
              {browserItems.map((item) => (
                <FileButton
                  key={item.path}
                  $active={item.path === selectedPath}
                  onClick={() => {
                    if (item.kind === "dir") {
                      setCurrentPath(item.path);
                      setSelectedPath("");
                      return;
                    }
                    setSelectedPath(item.path);
                  }}
                >
                  <FileButtonHeader>
                    <Typography.Body component="strong">{item.name}</Typography.Body>
                    <FileKind>{item.kind}</FileKind>
                  </FileButtonHeader>
                  <Typography.Body color="secondary" component="p">
                    {item.kind !== "dir" ? `${formatItemSize(item.size)} · ` : ""}
                    {item.modifiedAt
                      ? new Date(item.modifiedAt).toLocaleString()
                      : "No modification timestamp"}
                  </Typography.Body>
                </FileButton>
              ))}
              {!treeQuery.isLoading && browserItems.length === 0 ? (
                <PanelNote>This directory is empty.</PanelNote>
              ) : null}
            </FileList>
          </BrowserPanel>

          <EditorPanel>
            {selectedPath === "" ? (
              <PanelNote>Select a file to inspect its content.</PanelNote>
            ) : selectedFileQuery.isLoading ? (
              <PanelNote>Loading file content...</PanelNote>
            ) : selectedFile == null ? (
              <PanelNote>Select a file to inspect its content.</PanelNote>
            ) : selectedFile.binary ? (
              <EditorStack>
                <CardHeader>
                  <div>
                    <Typography.Heading component="h3" size="S">
                      {selectedFile.path}
                    </Typography.Heading>
                    <Typography.Body color="secondary" component="p">
                      Binary asset
                    </Typography.Body>
                  </div>
                </CardHeader>
                <Typography.Body color="secondary" component="p">
                  This item looks binary, so the studio is showing metadata instead of raw
                  content.
                </Typography.Body>
                <MetaRow>
                  <Tag>{selectedFile.language}</Tag>
                  <Tag>{formatItemSize(selectedFile.size)}</Tag>
                  <Tag>{selectedFile.kind}</Tag>
                </MetaRow>
              </EditorStack>
            ) : editable ? (
              <form
                onSubmit={(event) => {
                  event.preventDefault();
                  updateFile.mutate({
                    databaseId,
                    workspaceId: workspace.id,
                    path: selectedFile.path,
                    content: draftContent,
                  });
                }}
              >
                <EditorStack>
                  <CardHeader>
                    <div>
                      <Typography.Heading component="h3" size="S">
                        {selectedFile.path}
                      </Typography.Heading>
                      <Typography.Body color="secondary" component="p">
                        {selectedFile.language}
                      </Typography.Body>
                    </div>
                  </CardHeader>
                  <EditorArea
                    value={draftContent}
                    onChange={(event) => setDraftContent(event.target.value)}
                  />
                  <InlineActions>
                    <Button size="medium" type="submit" disabled={updateFile.isPending}>
                      Save file
                    </Button>
                  </InlineActions>
                </EditorStack>
              </form>
            ) : (
              <EditorStack>
                <CardHeader>
                  <div>
                    <Typography.Heading component="h3" size="S">
                      {selectedFile.path}
                    </Typography.Heading>
                    <Typography.Body color="secondary" component="p">
                      {selectedFile.language}
                    </Typography.Body>
                  </div>
                  <MetaRow>
                    <Tag>{selectedFile.kind}</Tag>
                    <Tag>{formatItemSize(selectedFile.size)}</Tag>
                  </MetaRow>
                </CardHeader>
                <EditorArea
                  readOnly
                  value={selectedFile.content ?? selectedFile.target ?? ""}
                />
              </EditorStack>
            )}
          </EditorPanel>
        </FileStudio>
      </SectionCard>
    </SectionGrid>
  );
}

function browserViewOptions(workspace: AFSWorkspaceDetail) {
  const options: Array<{ value: AFSWorkspaceView; label: string }> = [];

  if (workspace.capabilities.browseWorkingCopy) {
    options.push({ value: "working-copy", label: "Working copy" });
  }
  if (workspace.capabilities.browseHead) {
    options.push({ value: "head", label: "Saved head" });
  }
  if (workspace.capabilities.browseCheckpoints) {
    for (const savepoint of workspace.savepoints) {
      options.push({
        value: `checkpoint:${savepoint.id}`,
        label: `Checkpoint: ${savepoint.name}`,
      });
    }
  }

  return options;
}

function viewLabel(view: AFSWorkspaceView, workspace: AFSWorkspaceDetail) {
  if (view === "working-copy") {
    return "Working copy";
  }
  if (view === "head") {
    return "Saved head";
  }
  const checkpointId = view.replace(/^checkpoint:/, "");
  const checkpoint = workspace.savepoints.find((savepoint) => savepoint.id === checkpointId);
  return checkpoint == null ? "Checkpoint" : `Checkpoint: ${checkpoint.name}`;
}

function parentPath(value: string) {
  if (value === "/" || value === "") {
    return "/";
  }
  const parts = value.split("/").filter(Boolean);
  parts.pop();
  return parts.length === 0 ? "/" : `/${parts.join("/")}`;
}

function formatItemSize(size: number) {
  if (size === 0) {
    return "0 KB";
  }
  return formatBytes(size);
}

const BrowserBanner = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  margin-bottom: 18px;

  @media (max-width: 860px) {
    grid-template-columns: 1fr;
  }
`;

const BrowserMetric = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 18px;
  padding: 14px 16px;
  background: rgba(255, 255, 255, 0.72);
`;

const BrowserMetricLabel = styled.div`
  color: var(--afs-muted);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const BrowserMetricValue = styled.div`
  margin-top: 7px;
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
  line-height: 1.45;
`;

const BrowserPanel = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 24px;
  padding: 18px;
  background: #fff;
`;

const PanelNote = styled.p`
  margin: 14px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const FileButtonHeader = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: center;
`;

const FileKind = styled.span`
  display: inline-flex;
  align-items: center;
  padding: 4px 8px;
  border-radius: 999px;
  background: rgba(8, 6, 13, 0.08);
  color: var(--afs-muted);
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const EditorStack = styled.div`
  display: grid;
  gap: 16px;
`;
