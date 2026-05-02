import { Button } from "@redis-ui/components";
import { useEffect, useMemo, useState } from "react";
import styled, { keyframes } from "styled-components";
import {
  EditorArea,
  InlineActions,
  MetaRow,
  Tag,
} from "../../components/afs-kit";
import { formatBytes } from "../../foundation/api/afs";
import {
  useUpdateWorkspaceFileMutation,
  useWorkspaceFileContent,
  useWorkspaceTree,
} from "../../foundation/hooks/use-afs";
import { getWorkspaceBrowserViewOptions } from "../../foundation/workspace-browser-views";
import type { AFSWorkspaceDetail, AFSWorkspaceView } from "../../foundation/types/afs";
import { displayWorkspaceName } from "../../foundation/workspace-display";
import { FileHistoryDrawer } from "./-file-history-drawer";

/* ─── Icons ─────────────────────────────────────────────────────────── */

function FolderIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" style={{ color: "#9ca3af" }}>
      <path d="M1.75 1A1.75 1.75 0 0 0 0 2.75v10.5C0 14.216.784 15 1.75 15h12.5A1.75 1.75 0 0 0 16 13.25v-8.5A1.75 1.75 0 0 0 14.25 3H7.5a.25.25 0 0 1-.2-.1l-.9-1.2c-.33-.44-.85-.7-1.4-.7H1.75z" />
    </svg>
  );
}

function FileIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" style={{ color: "var(--afs-muted)" }}>
      <path d="M2 1.75C2 .784 2.784 0 3.75 0h6.586c.464 0 .909.184 1.237.513l2.914 2.914c.329.328.513.773.513 1.237v9.586A1.75 1.75 0 0 1 13.25 16h-9.5A1.75 1.75 0 0 1 2 14.25Zm1.75-.25a.25.25 0 0 0-.25.25v12.5c0 .138.112.25.25.25h9.5a.25.25 0 0 0 .25-.25V6h-2.75A1.75 1.75 0 0 1 9 4.25V1.5Zm6.75.062V4.25c0 .138.112.25.25.25h2.688l-.011-.013-2.914-2.914-.013-.011Z" />
    </svg>
  );
}

function ChevronIcon() {
  return (
    <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor" style={{ color: "var(--afs-muted)" }}>
      <path d="M4.427 7.427l3.396 3.396a.25.25 0 0 0 .354 0l3.396-3.396A.25.25 0 0 0 11.396 7H4.604a.25.25 0 0 0-.177.427z" />
    </svg>
  );
}

/* ─── Props ──────────────────────────────────────────────────────────── */

type Props = {
  workspace: AFSWorkspaceDetail;
  browserView: AFSWorkspaceView;
  onBrowserViewChange: (view: AFSWorkspaceView) => void;
};

/* ─── Component ──────────────────────────────────────────────────────── */

export function FilesTab({ workspace, browserView, onBrowserViewChange }: Props) {
  const updateFile = useUpdateWorkspaceFileMutation();

  const [currentPath, setCurrentPath] = useState("/");
  const [selectedPath, setSelectedPath] = useState("");
  const [historyPath, setHistoryPath] = useState("");
  const [draftContent, setDraftContent] = useState("");

  useEffect(() => {
    setCurrentPath("/");
    setSelectedPath("");
    setHistoryPath("");
  }, [browserView]);

  useEffect(() => {
    if (selectedPath === "") return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setSelectedPath("");
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [selectedPath]);

  const treeQuery = useWorkspaceTree(
    {
      workspaceId: workspace.id,
      view: browserView,
      path: currentPath,
      depth: 1,
    },
    true,
  );

  const selectedFileQuery = useWorkspaceFileContent(
    {
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

  const browserItems = useMemo(() => {
    const items = treeQuery.data?.items ?? [];
    // Sort: directories first, then files, alphabetically within each group
    return [...items].sort((a, b) => {
      if (a.kind === "dir" && b.kind !== "dir") return -1;
      if (a.kind !== "dir" && b.kind === "dir") return 1;
      return a.name.localeCompare(b.name);
    });
  }, [treeQuery.data?.items]);

  const selectedFile = selectedFileQuery.data;
  const editable =
    workspace.capabilities.editWorkingCopy === true &&
    browserView === "working-copy" &&
    selectedFile?.kind === "file";

  const pathSegments = useMemo(() => {
    if (currentPath === "/") return [];
    return currentPath.split("/").filter(Boolean);
  }, [currentPath]);

  const viewOptions = useMemo(() => getWorkspaceBrowserViewOptions(workspace), [workspace]);

  return (
    <RepoContainer>
      {/* ─── Toolbar: checkpoint selector + breadcrumb ─── */}
      <RepoToolbar>
        <ToolbarLeft>
          <BranchDropdown
            value={browserView}
            onChange={(e) => {
              onBrowserViewChange(e.target.value as AFSWorkspaceView);
              setCurrentPath("/");
              setSelectedPath("");
            }}
          >
            {viewOptions.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </BranchDropdown>

          <Breadcrumb>
            <BreadcrumbLink
              onClick={() => {
                setCurrentPath("/");
                setSelectedPath("");
              }}
              $isRoot
            >
              {displayWorkspaceName(workspace.name)}
            </BreadcrumbLink>
            {pathSegments.map((segment, i) => {
              const fullPath = "/" + pathSegments.slice(0, i + 1).join("/");
              const isLast = i === pathSegments.length - 1;
              return (
                <span key={fullPath}>
                  <BreadcrumbSep>/</BreadcrumbSep>
                  {isLast ? (
                    <BreadcrumbCurrent>{segment}</BreadcrumbCurrent>
                  ) : (
                    <BreadcrumbLink onClick={() => {
                      setCurrentPath(fullPath);
                      setSelectedPath("");
                    }}>
                      {segment}
                    </BreadcrumbLink>
                  )}
                </span>
              );
            })}
          </Breadcrumb>
        </ToolbarLeft>
      </RepoToolbar>

      {/* ─── File table ─── */}
      <FileTableContainer>
        {treeQuery.isLoading ? (
          <TableMessage>Loading...</TableMessage>
        ) : treeQuery.isError ? (
          <TableMessage>Unable to load this directory.</TableMessage>
        ) : browserItems.length === 0 ? (
          <TableMessage>This directory is empty.</TableMessage>
        ) : (
          <FileTable>
            <thead>
              <tr>
                <FileTableHeader $name>Name</FileTableHeader>
                <FileTableHeader $size>Size</FileTableHeader>
                <FileTableHeader $time>Last updated</FileTableHeader>
              </tr>
            </thead>
            <tbody>
              {currentPath !== "/" && (
                <FileRow
                  onClick={() => {
                    setCurrentPath(parentPath(currentPath));
                    setSelectedPath("");
                  }}
                >
                  <FileCell $name>
                    <FileNameContent>
                      <IconWrap><FolderIcon /></IconWrap>
                      <FileName>..</FileName>
                    </FileNameContent>
                  </FileCell>
                  <FileCell $message />
                  <FileCell $time />
                </FileRow>
              )}
              {browserItems.map((item) => (
                <FileRow
                  key={item.path}
                  $active={item.path === selectedPath}
                  onClick={() => {
                    if (item.kind === "dir") {
                      setCurrentPath(item.path);
                      setSelectedPath("");
                    } else {
                      setSelectedPath(item.path);
                    }
                  }}
                >
                  <FileCell $name>
                    <FileNameContent>
                      <IconWrap>
                        {item.kind === "dir" ? <FolderIcon /> : <FileIcon />}
                      </IconWrap>
                      <FileName $isDir={item.kind === "dir"}>{item.name}</FileName>
                    </FileNameContent>
                  </FileCell>
                  <FileCell $message>
                    {item.kind !== "dir" ? formatItemSize(item.size) : ""}
                  </FileCell>
                  <FileCell $time>
                    {item.modifiedAt ? formatRelativeTime(item.modifiedAt) : ""}
                  </FileCell>
                </FileRow>
              ))}
            </tbody>
          </FileTable>
        )}
      </FileTableContainer>

      {/* ─── File content viewer (slide-over drawer) ─── */}
      {selectedPath !== "" && (
        <DrawerOverlay onClick={() => setSelectedPath("")}>
          <DrawerPanel onClick={(e) => e.stopPropagation()} role="dialog" aria-modal="true">
            {selectedFileQuery.isLoading ? (
              <>
                <DrawerHeader>
                  <DrawerTitleWrap>
                    <ViewerTitle>{selectedPath.split("/").pop()}</ViewerTitle>
                  </DrawerTitleWrap>
                  <DrawerCloseButton onClick={() => setSelectedPath("")} aria-label="Close">×</DrawerCloseButton>
                </DrawerHeader>
                <ViewerMessage>Loading file content...</ViewerMessage>
              </>
            ) : selectedFile == null ? (
              <>
                <DrawerHeader>
                  <DrawerTitleWrap>
                    <ViewerTitle>{selectedPath.split("/").pop()}</ViewerTitle>
                  </DrawerTitleWrap>
                  <DrawerCloseButton onClick={() => setSelectedPath("")} aria-label="Close">×</DrawerCloseButton>
                </DrawerHeader>
                <ViewerMessage>Could not load file.</ViewerMessage>
              </>
            ) : selectedFile.binary ? (
              <>
                <DrawerHeader>
                  <DrawerTitleWrap>
                    <ViewerTitle>{selectedFile.path.split("/").pop()}</ViewerTitle>
                    <MetaRow style={{ margin: 0 }}>
                      <Tag>{selectedFile.language}</Tag>
                      <Tag>{formatItemSize(selectedFile.size)}</Tag>
                      <Tag>binary</Tag>
                    </MetaRow>
                  </DrawerTitleWrap>
                  <InlineActions>
                    <Button size="medium" variant="secondary-fill" onClick={() => setHistoryPath(selectedFile.path)}>
                      History
                    </Button>
                    <DrawerCloseButton onClick={() => setSelectedPath("")} aria-label="Close">×</DrawerCloseButton>
                  </InlineActions>
                </DrawerHeader>
                <ViewerMessage>Binary file — content not shown.</ViewerMessage>
              </>
            ) : editable ? (
              <DrawerForm
                onSubmit={(e) => {
                  e.preventDefault();
                  updateFile.mutate({
                    workspaceId: workspace.id,
                    path: selectedFile.path,
                    content: draftContent,
                  });
                }}
              >
                <DrawerHeader>
                  <DrawerTitleWrap>
                    <ViewerTitle>{selectedFile.path.split("/").pop()}</ViewerTitle>
                    <ViewerMeta>
                      {selectedFile.language} · {formatItemSize(selectedFile.size)}
                    </ViewerMeta>
                  </DrawerTitleWrap>
                  <InlineActions>
                    <Button
                      size="medium"
                      variant="secondary-fill"
                      type="button"
                      onClick={() => setHistoryPath(selectedFile.path)}
                    >
                      History
                    </Button>
                    <Button size="medium" type="submit" disabled={updateFile.isPending}>
                      Save
                    </Button>
                    <DrawerCloseButton type="button" onClick={() => setSelectedPath("")} aria-label="Close">×</DrawerCloseButton>
                  </InlineActions>
                </DrawerHeader>
                <DrawerCodeArea
                  value={draftContent}
                  onChange={(e) => setDraftContent(e.target.value)}
                />
              </DrawerForm>
            ) : (
              <>
                <DrawerHeader>
                  <DrawerTitleWrap>
                    <ViewerTitle>{selectedFile.path.split("/").pop()}</ViewerTitle>
                    <ViewerMeta>
                      {selectedFile.language} · {formatItemSize(selectedFile.size)}
                    </ViewerMeta>
                  </DrawerTitleWrap>
                  <InlineActions>
                    <Button size="medium" variant="secondary-fill" onClick={() => setHistoryPath(selectedFile.path)}>
                      History
                    </Button>
                    <DrawerCloseButton onClick={() => setSelectedPath("")} aria-label="Close">×</DrawerCloseButton>
                  </InlineActions>
                </DrawerHeader>
                <DrawerCodeArea
                  readOnly
                  value={selectedFile.content ?? selectedFile.target ?? ""}
                />
              </>
            )}
          </DrawerPanel>
        </DrawerOverlay>
      )}

      {historyPath !== "" ? (
        <FileHistoryDrawer
          databaseId={workspace.databaseId}
          workspaceId={workspace.id}
          path={historyPath}
          editable={workspace.capabilities.editWorkingCopy === true}
          onClose={() => setHistoryPath("")}
        />
      ) : null}
    </RepoContainer>
  );
}

/* ─── Helpers ───────────────────────────────────────────────────────── */

function parentPath(value: string) {
  if (value === "/" || value === "") return "/";
  const parts = value.split("/").filter(Boolean);
  parts.pop();
  return parts.length === 0 ? "/" : `/${parts.join("/")}`;
}

function formatItemSize(size: number) {
  return size === 0 ? "0 KB" : formatBytes(size);
}

function formatRelativeTime(iso: string): string {
  const now = Date.now();
  const then = new Date(iso).getTime();
  const diffMs = now - then;
  const diffSec = Math.floor(diffMs / 1000);

  if (diffSec < 60) return "just now";
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin} min ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `${diffHr} hour${diffHr > 1 ? "s" : ""} ago`;
  const diffDay = Math.floor(diffHr / 24);
  if (diffDay < 30) return `${diffDay} day${diffDay > 1 ? "s" : ""} ago`;
  const diffMon = Math.floor(diffDay / 30);
  if (diffMon < 12) return `${diffMon} month${diffMon > 1 ? "s" : ""} ago`;
  return new Date(iso).toLocaleDateString();
}

/* ─── Styled components ─────────────────────────────────────────────── */

const RepoContainer = styled.div`
  display: flex;
  flex-direction: column;
  gap: 0;
  width: 100%;
`;

const RepoToolbar = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 0;
  flex-wrap: wrap;
`;

const ToolbarLeft = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  min-width: 0;
`;

const BranchDropdown = styled.select`
  appearance: none;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 6px 28px 6px 12px;
  border: 1px solid var(--afs-line-strong);
  border-radius: 6px;
  background: var(--afs-panel);
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  outline: none;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 16 16'%3E%3Cpath fill='%23626b78' d='M4.427 7.427l3.396 3.396a.25.25 0 0 0 .354 0l3.396-3.396A.25.25 0 0 0 11.396 7H4.604a.25.25 0 0 0-.177.427z'/%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 8px center;

  &:hover {
    background-color: var(--afs-panel-strong);
    border-color: var(--afs-line-strong);
  }

  &:focus-visible {
    border-color: var(--afs-accent);
    box-shadow: 0 0 0 2px var(--afs-accent-soft);
  }
`;

const Breadcrumb = styled.div`
  display: flex;
  align-items: center;
  gap: 2px;
  font-size: 14px;
  min-width: 0;
  flex-wrap: wrap;
`;

const BreadcrumbLink = styled.button<{ $isRoot?: boolean }>`
  border: none;
  background: none;
  padding: 2px 4px;
  margin: 0;
  color: var(--afs-accent);
  font: inherit;
  font-size: 14px;
  font-weight: ${({ $isRoot }) => ($isRoot ? 700 : 400)};
  cursor: pointer;
  border-radius: 4px;

  &:hover {
    text-decoration: underline;
  }
`;

const BreadcrumbSep = styled.span`
  color: var(--afs-muted);
  margin: 0 1px;
  font-size: 14px;
`;

const BreadcrumbCurrent = styled.span`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 600;
  padding: 2px 4px;
`;

/* ─── File table ─── */

const FileTableContainer = styled.div`
  border: 1px solid var(--afs-line-strong);
  border-radius: 8px;
  overflow: hidden;
`;

const FileTable = styled.table`
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
`;

const FileTableHeader = styled.th<{ $name?: boolean; $size?: boolean; $time?: boolean }>`
  padding: 10px 16px;
  background: var(--afs-panel);
  border-bottom: 1px solid var(--afs-line);
  font-size: 12px;
  font-weight: 400;
  color: var(--afs-muted);
  text-align: left;

  ${({ $name }) => $name && `width: 40%;`}

  ${({ $size }) =>
    $size &&
    `
    width: 35%;
    text-align: left;
    @media (max-width: 640px) { display: none; }
  `}

  ${({ $time }) =>
    $time &&
    `
    width: 25%;
    text-align: right;
    @media (max-width: 480px) { display: none; }
  `}
`;

const FileRow = styled.tr<{ $active?: boolean }>`
  cursor: pointer;
  background: ${({ $active }) => ($active ? "var(--afs-accent-soft)" : "var(--afs-panel-strong)")};

  &:hover {
    background: ${({ $active }) => ($active ? "var(--afs-accent-soft)" : "var(--afs-panel)")};
  }

  &:not(:last-child) > td {
    border-bottom: 1px solid var(--afs-line);
  }
`;

const FileCell = styled.td<{ $name?: boolean; $message?: boolean; $time?: boolean }>`
  padding: 8px 16px;
  font-size: 13px;
  color: var(--afs-ink-soft);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  vertical-align: middle;

  ${({ $name }) =>
    $name &&
    `
    width: 40%;
  `}

  ${({ $message }) =>
    $message &&
    `
    width: 35%;
    color: var(--afs-muted);
    text-align: left;
    @media (max-width: 640px) {
      display: none;
    }
  `}

  ${({ $time }) =>
    $time &&
    `
    width: 25%;
    text-align: right;
    color: var(--afs-muted);
    @media (max-width: 480px) {
      display: none;
    }
  `}
`;

const FileNameContent = styled.div`
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
`;

const IconWrap = styled.span`
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  width: 16px;
  height: 16px;
`;

const FileName = styled.span<{ $isDir?: boolean }>`
  color: var(--afs-ink);
  font-weight: ${({ $isDir }) => ($isDir ? 600 : 400)};
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  &:hover {
    text-decoration: underline;
  }
`;

const TableMessage = styled.div`
  padding: 32px 16px;
  text-align: center;
  color: var(--afs-muted);
  font-size: 14px;
  background: var(--afs-panel-strong);
`;

/* ─── File viewer (slide-over drawer) ─── */

const fadeIn = keyframes`
  from { opacity: 0; }
  to { opacity: 1; }
`;

const slideIn = keyframes`
  from { transform: translateX(100%); }
  to { transform: translateX(0); }
`;

const DrawerOverlay = styled.div`
  position: fixed;
  inset: 0;
  z-index: 40;
  display: flex;
  justify-content: flex-end;
  background: rgba(8, 6, 13, 0.36);
  animation: ${fadeIn} 150ms ease-out;
`;

const DrawerPanel = styled.div`
  width: min(960px, 60vw);
  max-width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--afs-panel-strong);
  border-left: 1px solid var(--afs-line-strong);
  box-shadow: -18px 0 40px rgba(8, 6, 13, 0.2);
  animation: ${slideIn} 180ms ease-out both;

  @media (max-width: 768px) {
    width: 100%;
  }
`;

const DrawerForm = styled.form`
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
`;

const DrawerHeader = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 16px;
  background: var(--afs-panel);
  border-bottom: 1px solid var(--afs-line);
  flex-wrap: wrap;
`;

const DrawerTitleWrap = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  flex: 1;
`;

const DrawerCloseButton = styled.button`
  border: none;
  background: none;
  color: var(--afs-muted);
  font-size: 22px;
  line-height: 1;
  padding: 4px 8px;
  cursor: pointer;
  border-radius: 4px;

  &:hover {
    background: var(--afs-panel-strong);
    color: var(--afs-ink);
  }
`;

const DrawerCodeArea = styled.textarea`
  flex: 1;
  width: 100%;
  min-height: 0;
  border: none;
  border-radius: 0;
  padding: 16px;
  background: var(--afs-panel-strong);
  color: var(--afs-ink);
  font-family: var(--afs-mono);
  font-size: 13px;
  line-height: 1.6;
  resize: none;
  outline: none;
  box-sizing: border-box;
`;

const ViewerTitle = styled.span`
  font-size: 13px;
  font-weight: 600;
  color: var(--afs-ink);
`;

const ViewerMeta = styled.span`
  font-size: 12px;
  color: var(--afs-muted);
`;

const ViewerMessage = styled.div`
  padding: 32px 16px;
  text-align: center;
  color: var(--afs-muted);
  font-size: 14px;
  background: var(--afs-panel-strong);
`;
