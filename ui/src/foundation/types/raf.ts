export type RAFWorkspaceStatus = "healthy" | "syncing" | "attention";
export type RAFSessionStatus = "clean" | "dirty";
export type RAFSessionKind = "main" | "branch" | "imported";
export type RAFWorkspaceSource = "blank" | "git-import" | "cloud-import";
export type RAFClientMode = "demo" | "http";

export type RAFFile = {
  language: string;
  modifiedAt: string;
  path: string;
  content: string;
};

export type RAFSavepoint = {
  id: string;
  name: string;
  author: string;
  createdAt: string;
  note: string;
  fileCount: number;
  sizeLabel: string;
  filesSnapshot: RAFFile[];
};

export type RAFSession = {
  id: string;
  name: string;
  description: string;
  author: string;
  createdAt: string;
  updatedAt: string;
  lastRunAt: string;
  status: RAFSessionStatus;
  kind: RAFSessionKind;
  headSavepointId: string;
  files: RAFFile[];
  savepoints: RAFSavepoint[];
};

export type RAFActivityEvent = {
  id: string;
  actor: string;
  createdAt: string;
  detail: string;
  kind: string;
  scope: string;
  title: string;
};

export type RAFWorkspace = {
  id: string;
  name: string;
  description: string;
  cloudAccount: string;
  databaseId: string;
  databaseName: string;
  redisKey: string;
  region: string;
  mountedPath: string;
  status: RAFWorkspaceStatus;
  source: RAFWorkspaceSource;
  createdAt: string;
  updatedAt: string;
  defaultSessionId: string;
  tags: string[];
  sessions: RAFSession[];
  activity: RAFActivityEvent[];
};

export type RAFWorkspaceSummary = {
  id: string;
  name: string;
  databaseId: string;
  databaseName: string;
  redisKey: string;
  status: RAFWorkspaceStatus;
  fileCount: number;
  folderCount: number;
  totalBytes: number;
  sessionCount: number;
  forkCount: number;
  checkpointCount: number;
  dirtySessionCount: number;
  defaultSessionId: string;
  lastCheckpointAt: string;
  updatedAt: string;
  region: string;
  source: RAFWorkspaceSource;
};

export type RAFWorkspaceListResponse = {
  items: RAFWorkspaceSummary[];
};

export type RAFWorkspaceDetail = RAFWorkspace;

export type RAFState = {
  workspaces: RAFWorkspace[];
};

export type CreateWorkspaceInput = {
  name: string;
  description: string;
  cloudAccount: string;
  databaseName: string;
  region: string;
  source: RAFWorkspaceSource;
};

export type CreateSessionInput = {
  workspaceId: string;
  name: string;
  description: string;
  mode: RAFSessionKind;
  baseSessionId?: string;
};

export type UpdateSessionFileInput = {
  workspaceId: string;
  sessionId: string;
  path: string;
  content: string;
  expectedRevision?: string;
};

export type CreateSavepointInput = {
  workspaceId: string;
  sessionId: string;
  name: string;
  note: string;
};

export type RollbackSessionInput = {
  workspaceId: string;
  sessionId: string;
  savepointId: string;
};
