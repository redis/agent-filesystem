export type AFSWorkspaceSource = "blank" | "git-import" | "cloud-import";
export type AFSClientMode = "demo" | "http";
export type AFSAuthMode = "none" | "trusted-header" | "clerk" | "oidc" | string;
export type AFSWorkspaceView = "head" | `checkpoint:${string}` | "working-copy";
export type AFSTreeItemKind = "file" | "dir" | "symlink";

export type AFSAuthUser = {
  subject: string;
  name?: string;
  email?: string;
  groups?: string[];
};

export type AFSProductMode = "cloud" | "self-hosted";

export type AFSAuthConfig = {
  mode: AFSAuthMode;
  enabled: boolean;
  provider: string;
  signInRequired: boolean;
  authenticated: boolean;
  productMode: AFSProductMode;
  clerkPublishableKey?: string;
  user?: AFSAuthUser;
};

export type AFSServerVersion = {
  version: string;
  commit?: string;
  buildDate?: string;
};

export type AFSAccount = {
  subject?: string;
  provider: string;
  canDeleteIdentity: boolean;
  canResetData: boolean;
  ownedDatabaseCount: number;
  ownedWorkspaceCount: number;
  deletedDatabaseCount?: number;
  deletedWorkspaceCount?: number;
  identityDeleted?: boolean;
};

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

export type AFSChangelogEntry = {
  id: string;
  occurredAt?: string;
  workspaceId?: string;
  workspaceName?: string;
  databaseId?: string;
  databaseName?: string;
  sessionId?: string;
  agentId?: string;
  user?: string;
  label?: string;
  agentVersion?: string;
  op: string;
  path: string;
  prevPath?: string;
  sizeBytes?: number;
  deltaBytes?: number;
  contentHash?: string;
  prevHash?: string;
  mode?: number;
  checkpointId?: string;
  source?: string;
};

export type AFSChangelogResponse = {
  entries: AFSChangelogEntry[];
  nextCursor?: string;
};

export type AFSAgentSession = {
  sessionId: string;
  workspaceId: string;
  workspaceName: string;
  databaseId?: string;
  databaseName?: string;
  agentId?: string;
  clientKind: string;
  afsVersion: string;
  hostname: string;
  operatingSystem: string;
  localPath: string;
  label?: string;
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
  ownerSubject?: string;
  ownerLabel?: string;
  databaseManagementType?: string;
  databaseCanEdit?: boolean;
  databaseCanDelete?: boolean;
  redisKey: string;
  region: string;
  mountedPath?: string;
  source: AFSWorkspaceSource;
  templateSlug?: string;
  createdAt: string;
  updatedAt: string;
  draftState: string;
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
  ownerSubject?: string;
  ownerLabel?: string;
  databaseManagementType?: string;
  databaseCanEdit?: boolean;
  databaseCanDelete?: boolean;
  redisKey: string;
  fileCount: number;
  folderCount: number;
  totalBytes: number;
  checkpointCount: number;
  lastCheckpointAt: string;
  updatedAt: string;
  region: string;
  source: AFSWorkspaceSource;
  templateSlug?: string;
};

export type AFSWorkspaceListResponse = {
  items: AFSWorkspaceSummary[];
};

export type AFSActivityListResponse = {
  items: AFSActivityEvent[];
};

export type AFSWorkspaceDetail = AFSWorkspace;

export type AFSRedisStats = {
  usedMemoryBytes: number;
  maxMemoryBytes: number; // 0 = no limit
  fragmentationRatio: number;
  keyCount: number;
  opsPerSec: number;
  cacheHitRate: number; // 0..1 (0 if no hits/misses sampled yet)
  connectedClients: number;
  sampledAt?: string;
};

export type AFSDatabase = {
  id: string;
  name: string;
  description: string;
  ownerSubject?: string;
  ownerLabel?: string;
  managementType?: string;
  purpose?: string;
  canEdit: boolean;
  canDelete: boolean;
  canCreateWorkspaces: boolean;
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
  // AFS-specific footprint across all workspaces in this database
  afsTotalBytes: number;
  afsFileCount: number;
  // Redis server stats snapshot (undefined while the poller warms up or the
  // database is unreachable)
  stats?: AFSRedisStats;
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
  templateSlug?: string;
};

export type UpdateWorkspaceInput = {
  databaseId?: string;
  workspaceId: string;
  name: string;
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

export type OnboardingTokenResponse = {
  token: string;
  databaseId: string;
  workspaceId: string;
  workspaceName: string;
  expiresAt: string;
};

export type AFSMCPProfile =
  | "workspace-ro"
  | "workspace-rw"
  | "workspace-rw-checkpoint"
  | "admin-ro"
  | "admin-rw";

/**
 * Scope of an access token. `control-plane` = user-scoped, no workspace
 * binding; agents use it for workspace management + on-demand issuance of
 * workspace tokens. `workspace:<workspaceId>` = bound to a single workspace;
 * agents use it for file operations + checkpoints.
 */
export type AFSMCPScope = string;

export const AFS_MCP_SCOPE_CONTROL_PLANE = "control-plane";

export function isControlPlaneScope(scope?: string): boolean {
  return typeof scope === "string" && scope.trim() === AFS_MCP_SCOPE_CONTROL_PLANE;
}

export type AFSMCPToken = {
  id: string;
  name?: string;
  scope?: AFSMCPScope;
  databaseId: string;
  workspaceId: string;
  workspaceName?: string;
  profile: AFSMCPProfile;
  readonly: boolean;
  token?: string;
  createdAt: string;
  lastUsedAt?: string;
  expiresAt?: string;
  revokedAt?: string;
  templateSlug?: string;
};

export type CreateMCPTokenInput = {
  databaseId?: string;
  workspaceId: string;
  name?: string;
  profile: AFSMCPProfile;
  expiresAt?: string;
  templateSlug?: string;
};

export type CreateControlPlaneTokenInput = {
  name?: string;
  expiresAt?: string;
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
