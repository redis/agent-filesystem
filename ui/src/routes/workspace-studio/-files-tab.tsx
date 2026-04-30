import { Button } from "@redis-ui/components";
import { Check, ChevronDown, GitFork, History, Search, X } from "lucide-react";
import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import styled, { keyframes } from "styled-components";
import {
  EditorArea,
  InlineActions,
  MetaRow,
  Tag,
} from "../../components/afs-kit";
import { formatBytes } from "../../foundation/api/afs";
import {
  useEvents,
  useUpdateWorkspaceFileMutation,
  useWorkspaceFileContent,
  useWorkspaceTree,
} from "../../foundation/hooks/use-afs";
import { PathHistoryPanel } from "../../foundation/tables/path-history-panel";
import {
  getActiveWorkspaceView,
  getWorkspaceBrowserViewOptions,
} from "../../foundation/workspace-browser-views";
import type { AFSWorkspaceDetail, AFSWorkspaceView } from "../../foundation/types/afs";
import { displayWorkspaceName } from "../../foundation/workspace-display";

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

/* ─── Props ──────────────────────────────────────────────────────────── */

type Props = {
  workspace: AFSWorkspaceDetail;
  browserView: AFSWorkspaceView;
  onBrowserViewChange: (view: AFSWorkspaceView) => void;
  onViewAllCheckpoints: () => void;
};

/* ─── Component ──────────────────────────────────────────────────────── */

export function FilesTab({
  workspace,
  browserView,
  onBrowserViewChange,
  onViewAllCheckpoints,
}: Props) {
  const updateFile = useUpdateWorkspaceFileMutation();

  const [currentPath, setCurrentPath] = useState("/");
  const [selectedPath, setSelectedPath] = useState("");
  const [draftContent, setDraftContent] = useState("");
  const [pathHistoryOpen, setPathHistoryOpen] = useState(false);
  const [checkpointMenuOpen, setCheckpointMenuOpen] = useState(false);
  const [checkpointSearch, setCheckpointSearch] = useState("");
  const checkpointMenuRef = useRef<HTMLDivElement | null>(null);
  const checkpointSearchRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    setCurrentPath("/");
    closeSelectedFile();
  }, [browserView]);

  useEffect(() => {
    if (selectedPath === "") return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") closeSelectedFile();
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

  const pathHistoryQuery = useEvents(
    {
      databaseId: workspace.databaseId,
      workspaceId: workspace.id,
      path: selectedPath,
      limit: 25,
      direction: "desc",
    },
    selectedPath !== "" && pathHistoryOpen,
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
  const activeBrowserView = getActiveWorkspaceView(workspace);
  const editable =
    workspace.capabilities.editWorkingCopy === true &&
    browserView === activeBrowserView &&
    selectedFile?.kind === "file";

  const pathSegments = useMemo(() => {
    if (currentPath === "/") return [];
    return currentPath.split("/").filter(Boolean);
  }, [currentPath]);
  const currentFolderName = pathSegments.length === 0
    ? displayWorkspaceName(workspace.name)
    : pathSegments[pathSegments.length - 1];

  const viewOptions = useMemo(() => getWorkspaceBrowserViewOptions(workspace), [workspace]);
  const checkpointOptions = useMemo(
    () =>
      viewOptions.map((option) => ({
        ...option,
      })),
    [viewOptions],
  );
  const currentCheckpoint = checkpointOptions.find((option) => option.value === browserView)
    ?? checkpointOptions[0];
  const filteredCheckpointOptions = useMemo(() => {
    const query = checkpointSearch.trim().toLowerCase();
    if (query === "") {
      return checkpointOptions;
    }

    return checkpointOptions.filter((option) =>
      [option.label, option.value].some((value) =>
        value.toLowerCase().includes(query),
      ),
    );
  }, [checkpointOptions, checkpointSearch]);

  useEffect(() => {
    if (!checkpointMenuOpen) {
      return;
    }

    const focusId = window.setTimeout(() => {
      checkpointSearchRef.current?.focus();
    }, 0);

    function onPointerDown(event: PointerEvent) {
      const target = event.target;
      if (
        target instanceof Node &&
        checkpointMenuRef.current != null &&
        !checkpointMenuRef.current.contains(target)
      ) {
        setCheckpointMenuOpen(false);
      }
    }

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setCheckpointMenuOpen(false);
      }
    }

    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      window.clearTimeout(focusId);
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [checkpointMenuOpen]);

  function switchCheckpoint(nextView: AFSWorkspaceView) {
    onBrowserViewChange(nextView);
    setCurrentPath("/");
    closeSelectedFile();
    setCheckpointMenuOpen(false);
    setCheckpointSearch("");
  }

  function openAllCheckpoints() {
    setCheckpointMenuOpen(false);
    setCheckpointSearch("");
    onViewAllCheckpoints();
  }

  function closeSelectedFile() {
    setSelectedPath("");
    setPathHistoryOpen(false);
  }

  function openPathHistoryEvent(event: { kind: string; checkpointId?: string }) {
    if (event.kind === "checkpoint" || event.checkpointId) {
      closeSelectedFile();
      onViewAllCheckpoints();
    }
  }

  function renderPathHistoryToggle() {
    return (
      <PathHistoryToggleButton
        type="button"
        aria-pressed={pathHistoryOpen}
        $active={pathHistoryOpen}
        onClick={() => setPathHistoryOpen((open) => !open)}
      >
        <History size={14} strokeWidth={1.8} aria-hidden="true" />
        <span>History</span>
      </PathHistoryToggleButton>
    );
  }

  function renderPathHistoryPanel() {
    if (!pathHistoryOpen || selectedPath === "") {
      return null;
    }

    return (
      <PathHistoryPanel
        path={selectedPath}
        rows={pathHistoryQuery.data?.items ?? []}
        loading={pathHistoryQuery.isLoading}
        error={pathHistoryQuery.isError}
        errorMessage={
          pathHistoryQuery.error instanceof Error ? pathHistoryQuery.error.message : undefined
        }
        onOpenEvent={openPathHistoryEvent}
      />
    );
  }

  return (
    <RepoContainer>
      {/* ─── Toolbar: checkpoint selector + breadcrumb ─── */}
      <RepoToolbar>
        <ToolbarLeft>
          <CheckpointDropdown ref={checkpointMenuRef}>
            <CheckpointTrigger
              type="button"
              aria-haspopup="menu"
              aria-expanded={checkpointMenuOpen}
              onClick={() => setCheckpointMenuOpen((open) => !open)}
            >
              <GitFork size={15} strokeWidth={1.8} aria-hidden="true" />
              <CheckpointTriggerText>{currentCheckpoint.label}</CheckpointTriggerText>
              <ChevronDown size={14} strokeWidth={1.8} aria-hidden="true" />
            </CheckpointTrigger>
            {checkpointMenuOpen ? (
              <CheckpointMenu role="menu" aria-label="Switch browser view">
                <CheckpointMenuHeader>
                  <CheckpointMenuTitle>Switch View</CheckpointMenuTitle>
                  <CheckpointCloseButton
                    type="button"
                    aria-label="Close checkpoint switcher"
                    onClick={() => setCheckpointMenuOpen(false)}
                  >
                    <X size={14} strokeWidth={1.8} aria-hidden="true" />
                  </CheckpointCloseButton>
                </CheckpointMenuHeader>
                <CheckpointSearchWrap>
                  <Search size={14} strokeWidth={1.8} aria-hidden="true" />
                  <CheckpointSearchInput
                    ref={checkpointSearchRef}
                    value={checkpointSearch}
                    onChange={(event) => setCheckpointSearch(event.target.value)}
                    placeholder="Find a checkpoint"
                  />
                </CheckpointSearchWrap>
                <CheckpointMenuList>
                  {filteredCheckpointOptions.length === 0 ? (
                    <CheckpointEmpty>No checkpoints match.</CheckpointEmpty>
                  ) : (
                    filteredCheckpointOptions.map((option) => {
                      const selected = option.value === browserView;
                      return (
                        <Fragment key={option.value}>
                          <CheckpointMenuItem
                            type="button"
                            role="menuitem"
                            $selected={selected}
                            onClick={() => switchCheckpoint(option.value)}
                          >
                            <CheckpointMenuItemName>{option.label}</CheckpointMenuItemName>
                            <CheckpointSelectedIcon $visible={selected}>
                              <Check size={14} strokeWidth={1.9} aria-hidden="true" />
                            </CheckpointSelectedIcon>
                          </CheckpointMenuItem>
                          {option.value === activeBrowserView && filteredCheckpointOptions.length > 1 ? (
                            <CheckpointMenuDivider role="separator" />
                          ) : null}
                        </Fragment>
                      );
                    })
                  )}
                </CheckpointMenuList>
                <ViewAllCheckpointsButton type="button" role="menuitem" onClick={openAllCheckpoints}>
                  View all checkpoints
                </ViewAllCheckpointsButton>
              </CheckpointMenu>
            ) : null}
          </CheckpointDropdown>

          <Breadcrumb>
            <BreadcrumbLink
              onClick={() => {
                setCurrentPath("/");
                closeSelectedFile();
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
                      closeSelectedFile();
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
          <>
            <FileStatsBar>
              <FileStatsFolderTitle>{currentFolderName}</FileStatsFolderTitle>
              <FileStatsLine>
                <FileStatsValue>{workspace.fileCount.toLocaleString()}</FileStatsValue>
                <span>{pluralize(workspace.fileCount, "file")}</span>
                <FileStatsSeparator>·</FileStatsSeparator>
                <FileStatsValue>{workspace.folderCount.toLocaleString()}</FileStatsValue>
                <span>{pluralize(workspace.folderCount, "folder")}</span>
                <FileStatsSeparator>·</FileStatsSeparator>
                <FileStatsValue>{formatBytes(workspace.totalBytes)}</FileStatsValue>
              </FileStatsLine>
            </FileStatsBar>
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
                      closeSelectedFile();
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
                        closeSelectedFile();
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
          </>
        )}
      </FileTableContainer>

      {/* ─── File content viewer (slide-over drawer) ─── */}
      {selectedPath !== "" && (
        <DrawerOverlay onClick={closeSelectedFile}>
          <DrawerPanel onClick={(e) => e.stopPropagation()} role="dialog" aria-modal="true">
            {selectedFileQuery.isLoading ? (
              <>
                <DrawerHeader>
                  <DrawerTitleWrap>
                    <ViewerTitle>{selectedPath.split("/").pop()}</ViewerTitle>
                  </DrawerTitleWrap>
                  <DrawerHeaderActions>
                    {renderPathHistoryToggle()}
                    <DrawerCloseButton onClick={closeSelectedFile} aria-label="Close">×</DrawerCloseButton>
                  </DrawerHeaderActions>
                </DrawerHeader>
                <ViewerMessage>Loading file content...</ViewerMessage>
                {renderPathHistoryPanel()}
              </>
            ) : selectedFile == null ? (
              <>
                <DrawerHeader>
                  <DrawerTitleWrap>
                    <ViewerTitle>{selectedPath.split("/").pop()}</ViewerTitle>
                  </DrawerTitleWrap>
                  <DrawerHeaderActions>
                    {renderPathHistoryToggle()}
                    <DrawerCloseButton onClick={closeSelectedFile} aria-label="Close">×</DrawerCloseButton>
                  </DrawerHeaderActions>
                </DrawerHeader>
                <ViewerMessage>Could not load file.</ViewerMessage>
                {renderPathHistoryPanel()}
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
                  <DrawerHeaderActions>
                    {renderPathHistoryToggle()}
                    <DrawerCloseButton onClick={closeSelectedFile} aria-label="Close">×</DrawerCloseButton>
                  </DrawerHeaderActions>
                </DrawerHeader>
                <ViewerMessage>Binary file - content not shown.</ViewerMessage>
                {renderPathHistoryPanel()}
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
                    {renderPathHistoryToggle()}
                    <Button size="medium" type="submit" disabled={updateFile.isPending}>
                      Save
                    </Button>
                    <DrawerCloseButton type="button" onClick={closeSelectedFile} aria-label="Close">×</DrawerCloseButton>
                  </InlineActions>
                </DrawerHeader>
                <DrawerCodeArea
                  value={draftContent}
                  onChange={(e) => setDraftContent(e.target.value)}
                />
                {renderPathHistoryPanel()}
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
                  <DrawerHeaderActions>
                    {renderPathHistoryToggle()}
                    <DrawerCloseButton onClick={closeSelectedFile} aria-label="Close">×</DrawerCloseButton>
                  </DrawerHeaderActions>
                </DrawerHeader>
                <DrawerCodeArea
                  readOnly
                  value={selectedFile.content ?? selectedFile.target ?? ""}
                />
                {renderPathHistoryPanel()}
              </>
            )}
          </DrawerPanel>
        </DrawerOverlay>
      )}
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

function pluralize(value: number, singular: string) {
  return value === 1 ? singular : `${singular}s`;
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
  padding: 0 0 12px;
  flex-wrap: wrap;
`;

const ToolbarLeft = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  min-width: 0;
`;

const CheckpointDropdown = styled.div`
  position: relative;
  flex: 0 0 auto;
`;

const CheckpointTrigger = styled.button`
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  max-width: 260px;
  height: 34px;
  padding: 0 10px;
  border: 1px solid var(--afs-line-strong);
  border-radius: 7px;
  background: var(--afs-panel);
  color: var(--afs-ink);
  font: inherit;
  font-size: 13px;
  font-weight: 650;
  cursor: pointer;
  box-shadow: none;
  transition:
    background 120ms ease,
    border-color 120ms ease;

  &:hover {
    background: var(--afs-panel-strong);
  }

  &:focus-visible {
    outline: none;
    border-color: var(--afs-accent);
    box-shadow: 0 0 0 2px var(--afs-accent-soft);
  }
`;

const CheckpointTriggerText = styled.span`
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const CheckpointMenu = styled.div`
  position: absolute;
  top: calc(100% + 8px);
  left: 0;
  z-index: 30;
  width: min(360px, calc(100vw - 32px));
  overflow: hidden;
  border: 1px solid var(--afs-line-strong);
  border-radius: 8px;
  background: var(--afs-panel-strong);
  box-shadow: 0 18px 48px rgba(8, 6, 13, 0.14);
`;

const CheckpointMenuHeader = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 13px 14px 11px;
  border-bottom: 1px solid var(--afs-line);
`;

const CheckpointMenuTitle = styled.div`
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 700;
`;

const CheckpointCloseButton = styled.button`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--afs-muted);
  cursor: pointer;

  &:hover {
    background: var(--afs-panel);
    color: var(--afs-ink);
  }
`;

const CheckpointSearchWrap = styled.label`
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 12px 14px;
  padding: 0 10px;
  height: 34px;
  border: 1px solid var(--afs-line);
  border-radius: 7px;
  background: var(--afs-panel);
  color: var(--afs-muted);

  &:focus-within {
    border-color: var(--afs-accent);
    box-shadow: 0 0 0 2px var(--afs-accent-soft);
  }
`;

const CheckpointSearchInput = styled.input`
  width: 100%;
  min-width: 0;
  border: none;
  outline: none;
  background: transparent;
  color: var(--afs-ink);
  font: inherit;
  font-size: 13px;

  &::placeholder {
    color: var(--afs-muted);
  }
`;

const CheckpointMenuList = styled.div`
  max-height: 240px;
  overflow-y: auto;
  padding: 2px 6px 6px;
`;

const CheckpointMenuItem = styled.button<{ $selected?: boolean }>`
  display: grid;
  grid-template-columns: minmax(0, 1fr) 20px;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 34px;
  padding: 7px 8px;
  border: none;
  border-radius: 6px;
  background: ${({ $selected }) => ($selected ? "var(--afs-accent-soft)" : "transparent")};
  color: var(--afs-ink);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;

  &:hover {
    background: var(--afs-panel);
  }
`;

const CheckpointMenuItemName = styled.span`
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const CheckpointMenuDivider = styled.div`
  height: 1px;
  margin: 6px 2px;
  background: var(--afs-line);
`;

const CheckpointSelectedIcon = styled.span<{ $visible?: boolean }>`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--afs-accent);
  opacity: ${({ $visible }) => ($visible ? 1 : 0)};
`;

const CheckpointEmpty = styled.div`
  padding: 12px 8px;
  color: var(--afs-muted);
  font-size: 13px;
`;

const ViewAllCheckpointsButton = styled.button`
  display: flex;
  align-items: center;
  width: 100%;
  min-height: 38px;
  padding: 10px 14px;
  border: none;
  border-top: 1px solid var(--afs-line);
  background: transparent;
  color: var(--afs-accent);
  font: inherit;
  font-size: 13px;
  font-weight: 700;
  text-align: left;
  cursor: pointer;

  &:hover {
    background: var(--afs-panel);
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

const FileStatsBar = styled.div`
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
  min-width: 0;
  padding: 13px 16px;
  background: var(--afs-panel-strong);
  border-bottom: 1px solid var(--afs-line);

  @media (max-width: 720px) {
    align-items: flex-start;
    flex-direction: column;
    gap: 6px;
  }
`;

const FileStatsFolderTitle = styled.div`
  min-width: 0;
  overflow: hidden;
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 700;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const FileStatsLine = styled.div`
  display: flex;
  align-items: baseline;
  justify-content: flex-end;
  gap: 7px;
  min-width: 0;
  max-width: 100%;
  overflow-x: auto;
  text-align: right;
  color: var(--afs-muted);
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
  font-size: 12px;
  line-height: 1.25;
  white-space: nowrap;

  @media (max-width: 720px) {
    justify-content: flex-start;
    text-align: left;
  }
`;

const FileStatsValue = styled.strong`
  color: var(--afs-ink);
  font-weight: 700;
`;

const FileStatsSeparator = styled.span`
  color: var(--afs-muted);
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

const DrawerHeaderActions = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex: 0 0 auto;
`;

const PathHistoryToggleButton = styled.button<{ $active?: boolean }>`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 32px;
  padding: 0 10px;
  border: 1px solid ${({ $active }) => ($active ? "var(--afs-accent)" : "var(--afs-line)")};
  border-radius: 7px;
  background: ${({ $active }) => ($active ? "var(--afs-accent-soft)" : "var(--afs-panel-strong)")};
  color: ${({ $active }) => ($active ? "var(--afs-accent)" : "var(--afs-ink-soft)")};
  font: inherit;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;

  &:hover {
    border-color: var(--afs-line-strong);
    color: var(--afs-ink);
  }

  &:focus-visible {
    outline: none;
    border-color: var(--afs-focus);
    box-shadow: 0 0 0 3px var(--afs-focus-soft);
  }
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
