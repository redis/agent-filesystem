export type AFSWorkspaceSource = "blank" | "git-import" | "cloud-import";
export type AFSClientMode = "demo" | "http";
export type AFSWorkspaceView = "head" | `checkpoint:${string}` | "working-copy";
export type AFSTreeItemKind = "file" | "dir" | "symlink";

export type AFSFile = {
  language: string;
  modifiedAt: string;
  path: string;
  content: string;
};

export type AFSWorkspaceCapabilities = {
  browseHead: boolean;
  browseCheckpoints: boolean;
  browseWorkingCopy: boolean;
  editWorkingCopy: boolean;
  createCheckpoint: boolean;
  restoreCheckpoint: boolean;
};

export type AFSSavepoint = {
  id: string;
  name: string;
  author: string;
  createdAt: string;
  note: string;
  fileCount: number;
  folderCount: number;
  totalBytes: number;
  sizeLabel: string;
  filesSnapshot: AFSFile[];
  isHead?: boolean;
};

export type AFSActivityEvent = {
  id: string;
  workspaceId?: string;
  workspaceName?: string;
  databaseId?: string;
  databaseName?: string;
  actor: string;
  createdAt: string;
  detail: string;
  kind: string;
  scope: string;
  title: string;
};

export type AFSAgentSession = {
  sessionId: string;
  workspaceId: string;
  workspaceName: string;
  databaseId?: string;
  databaseName?: string;
  clientKind: string;
  afsVersion: string;
  hostname: string;
  operatingSystem: string;
  localPath: string;
  readonly: boolean;
  state: string;
  startedAt: string;
  lastSeenAt: string;
  leaseExpiresAt: string;
};

export type AFSTreeItem = {
  path: string;
  name: string;
  kind: AFSTreeItemKind;
  size: number;
  modifiedAt?: string;
  target?: string;
};

export type AFSTreeResponse = {
  workspaceId: string;
  view: AFSWorkspaceView;
  path: string;
  items: AFSTreeItem[];
};

export type AFSFileContent = {
  workspaceId: string;
  view: AFSWorkspaceView;
  path: string;
  kind: Exclude<AFSTreeItemKind, "dir">;
  revision: string;
  language: string;
  encoding: string;
  contentType: string;
  size: number;
  modifiedAt?: string;
  binary: boolean;
  content?: string;
  target?: string;
};

export type AFSWorkspace = {
  id: string;
  name: string;
  description: string;
  cloudAccount: string;
  databaseId: string;
  databaseName: string;
  redisKey: string;
  region: string;
  mountedPath?: string;
  source: AFSWorkspaceSource;
  createdAt: string;
  updatedAt: string;
  headSavepointId: string;
  tags: string[];
  fileCount: number;
  folderCount: number;
  totalBytes: number;
  checkpointCount: number;
  files: AFSFile[];
  savepoints: AFSSavepoint[];
  activity: AFSActivityEvent[];
  agents: AFSAgentSession[];
  capabilities: AFSWorkspaceCapabilities;
};

export type AFSWorkspaceSummary = {
  id: string;
  name: string;
  cloudAccount: string;
  databaseId: string;
  databaseName: string;
  redisKey: string;
  fileCount: number;
  folderCount: number;
  totalBytes: number;
  checkpointCount: number;
  lastCheckpointAt: string;
  updatedAt: string;
  region: string;
  source: AFSWorkspaceSource;
};

export type AFSWorkspaceListResponse = {
  items: AFSWorkspaceSummary[];
};

export type AFSActivityListResponse = {
  items: AFSActivityEvent[];
};

export type AFSWorkspaceDetail = AFSWorkspace;

export type AFSDatabase = {
  id: string;
  name: string;
  description: string;
  redisAddr: string;
  redisUsername: string;
  redisPassword: string;
  redisDB: number;
  redisTLS: boolean;
  isDefault: boolean;
  workspaceCount: number;
  activeSessionCount: number;
  connectionError?: string;
  lastWorkspaceRefreshAt?: string;
  lastWorkspaceRefreshError?: string;
  lastSessionReconcileAt?: string;
  lastSessionReconcileError?: string;
};

export type AFSDatabaseListResponse = {
  items: AFSDatabase[];
};

export type AFSState = {
  workspaces: AFSWorkspace[];
};

export type CreateWorkspaceInput = {
  databaseId?: string;
  name: string;
  description: string;
  cloudAccount?: string;
  databaseName?: string;
  region?: string;
  source: AFSWorkspaceSource;
};

export type UpdateWorkspaceInput = {
  databaseId?: string;
  workspaceId: string;
  description: string;
  cloudAccount?: string;
  databaseName?: string;
  region?: string;
};

export type UpdateWorkspaceFileInput = {
  databaseId?: string;
  workspaceId: string;
  path: string;
  content: string;
  expectedRevision?: string;
};

export type CreateSavepointInput = {
  databaseId?: string;
  workspaceId: string;
  name: string;
  note: string;
};

export type RestoreSavepointInput = {
  databaseId?: string;
  workspaceId: string;
  savepointId: string;
};

export type GetWorkspaceTreeInput = {
  databaseId?: string;
  workspaceId: string;
  view: AFSWorkspaceView;
  path: string;
  depth?: number;
};

export type GetWorkspaceFileContentInput = {
  databaseId?: string;
  workspaceId: string;
  view: AFSWorkspaceView;
  path: string;
};

export type SaveDatabaseInput = {
  id?: string;
  name: string;
  description: string;
  redisAddr: string;
  redisUsername: string;
  redisPassword: string;
  redisDB: number;
  redisTLS: boolean;
};

export type QuickstartInput = {
  redisAddr?: string;
  redisPassword?: string;
  redisUsername?: string;
  redisDB?: number;
  redisTLS?: boolean;
};

export type QuickstartResponse = {
  databaseId: string;
  workspaceId: string;
  workspace: AFSWorkspaceDetail;
};

export type ImportLocalInput = {
  databaseId?: string;
  name: string;
  path: string;
  description?: string;
};

export type ImportLocalResponse = {
  workspaceId: string;
  workspace: AFSWorkspaceDetail;
  fileCount: number;
  dirCount: number;
  totalBytes: number;
};
