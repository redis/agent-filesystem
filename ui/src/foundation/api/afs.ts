import { cloneInitialAFSState } from "../mocks/afs";
import type {
  AFSAdminOverview,
  AFSAdminUser,
  AFSChangelogEntry,
  AFSChangelogResponse,
  AFSDatabase,
  AFSDatabaseListResponse,
  AFSAgentSession,
  AFSDiffEntry,
  AFSEventEntry,
  AFSEventListResponse,
  AFSMCPToken,
  AFSTextDiff,
  CreateSavepointInput,
  CreateMCPTokenInput,
  CreateControlPlaneTokenInput,
  CreateWorkspaceInput,
  GetWorkspaceFileContentInput,
  GetWorkspaceDiffInput,
  GetWorkspaceTreeInput,
  AFSActivityEvent,
  AFSActivityListResponse,
  AFSClientMode,
  AFSFile,
  AFSFileContent,
  AFSFileHistoryResponse,
  AFSFileVersionContent,
  AFSFileVersionDiff,
  AFSFileVersionRestoreResponse,
  AFSFileVersionUndeleteResponse,
  AFSSavepoint,
  AFSState,
  AFSTreeItem,
  AFSTreeResponse,
  AFSWorkspace,
  AFSWorkspaceCapabilities,
  AFSWorkspaceDetail,
  AFSWorkspaceDiffResponse,
  AFSWorkspaceSource,
  AFSWorkspaceSummary,
  AFSWorkspaceVersioningPolicy,
  AFSWorkspaceView,
  DiffFileVersionsInput,
  GetFileHistoryInput,
  GetFileVersionContentInput,
  GetWorkspaceVersioningPolicyInput,
  RestoreSavepointInput,
  SaveDatabaseInput,
  RestoreFileVersionInput,
  UndeleteFileVersionInput,
  UpdateWorkspaceInput,
  UpdateWorkspaceFileInput,
  UpdateWorkspaceVersioningPolicyInput,
  QuickstartInput,
  QuickstartResponse,
  OnboardingTokenResponse,
  ImportLocalInput,
  ImportLocalResponse,
  AFSAuthConfig,
  AFSAccount,
  AFSServerVersion,
} from "../types/afs";

const STORAGE_KEY = "afs-ui-demo-state-v1";
const DATABASE_STORAGE_KEY = "afs-ui-demo-databases-v1";
const VERSIONING_POLICY_STORAGE_KEY = "afs-ui-demo-versioning-policies-v1";
const DEMO_DELAY_MS = 120;
const CLIENT_MODE_OVERRIDE = String(import.meta.env.VITE_AFS_CLIENT_MODE ?? "").trim().toLowerCase();
const HTTP_REQUEST_TIMEOUT_MS = 8000;

type AFSClient = {
  mode: AFSClientMode;
  listDatabases: () => Promise<AFSDatabase[]>;
  reconcileCatalog: () => Promise<void>;
  saveDatabase: (input: SaveDatabaseInput) => Promise<AFSDatabase>;
  setDefaultDatabase: (databaseId: string) => Promise<AFSDatabase>;
  deleteDatabase: (databaseId: string) => Promise<void>;
  listWorkspaceSummaries: (databaseId?: string) => Promise<AFSWorkspaceSummary[]>;
  getWorkspace: (databaseId: string | undefined, workspaceId: string) => Promise<AFSWorkspaceDetail | null>;
  listAgents: (databaseId?: string) => Promise<AFSAgentSession[]>;
  createWorkspace: (input: CreateWorkspaceInput) => Promise<AFSWorkspaceDetail>;
  deleteWorkspace: (databaseId: string, workspaceId: string) => Promise<void>;
  updateWorkspace: (input: UpdateWorkspaceInput) => Promise<AFSWorkspaceDetail | null>;
  updateWorkspaceFile: (input: UpdateWorkspaceFileInput) => Promise<AFSWorkspaceDetail | null>;
  getWorkspaceVersioningPolicy: (input: GetWorkspaceVersioningPolicyInput) => Promise<AFSWorkspaceVersioningPolicy>;
  updateWorkspaceVersioningPolicy: (input: UpdateWorkspaceVersioningPolicyInput) => Promise<AFSWorkspaceVersioningPolicy>;
  getFileHistory: (input: GetFileHistoryInput) => Promise<AFSFileHistoryResponse>;
  getFileVersionContent: (input: GetFileVersionContentInput) => Promise<AFSFileVersionContent | null>;
  diffFileVersions: (input: DiffFileVersionsInput) => Promise<AFSFileVersionDiff>;
  restoreFileVersion: (input: RestoreFileVersionInput) => Promise<AFSFileVersionRestoreResponse>;
  undeleteFileVersion: (input: UndeleteFileVersionInput) => Promise<AFSFileVersionUndeleteResponse>;
  createSavepoint: (input: CreateSavepointInput) => Promise<AFSWorkspaceDetail | null>;
  restoreSavepoint: (input: RestoreSavepointInput) => Promise<AFSWorkspaceDetail | null>;
  listActivity: (databaseId?: string, limit?: number) => Promise<AFSActivityEvent[]>;
  listActivityPage: (input: ListActivityInput) => Promise<AFSActivityListResponse>;
  listEvents: (input: ListEventsInput) => Promise<AFSEventListResponse>;
  listChangelog: (input: ListChangelogInput) => Promise<AFSChangelogResponse>;
  getWorkspaceTree: (input: GetWorkspaceTreeInput) => Promise<AFSTreeResponse>;
  getWorkspaceFileContent: (input: GetWorkspaceFileContentInput) => Promise<AFSFileContent | null>;
  getWorkspaceDiff: (input: GetWorkspaceDiffInput) => Promise<AFSWorkspaceDiffResponse>;
  quickstart: (input: QuickstartInput) => Promise<QuickstartResponse>;
  createOnboardingToken: (databaseId: string | undefined, workspaceId: string) => Promise<OnboardingTokenResponse>;
  listAllMCPAccessTokens: () => Promise<AFSMCPToken[]>;
  listMCPAccessTokens: (databaseId: string | undefined, workspaceId: string) => Promise<AFSMCPToken[]>;
  createMCPAccessToken: (input: CreateMCPTokenInput) => Promise<AFSMCPToken>;
  revokeMCPAccessToken: (databaseId: string | undefined, workspaceId: string, tokenId: string) => Promise<void>;
  listControlPlaneTokens: () => Promise<AFSMCPToken[]>;
  createControlPlaneToken: (input: CreateControlPlaneTokenInput) => Promise<AFSMCPToken>;
  revokeControlPlaneToken: (tokenId: string) => Promise<void>;
  importLocal: (input: ImportLocalInput) => Promise<ImportLocalResponse>;
  getAuthConfig: () => Promise<AFSAuthConfig>;
  getServerVersion: () => Promise<AFSServerVersion>;
  getAccount: () => Promise<AFSAccount>;
  resetAccountData: () => Promise<AFSAccount>;
  deleteAccount: () => Promise<AFSAccount>;
  getAdminOverview: () => Promise<AFSAdminOverview>;
  listAdminUsers: () => Promise<AFSAdminUser[]>;
  listAdminDatabases: () => Promise<AFSDatabase[]>;
  listAdminWorkspaceSummaries: () => Promise<AFSWorkspaceSummary[]>;
  listAdminAgents: () => Promise<AFSAgentSession[]>;
  resetDemo: () => AFSState;
};

export type ListChangelogInput = {
  databaseId?: string;
  workspaceId?: string;
  sessionId?: string;
  path?: string;
  since?: string;
  until?: string;
  limit?: number;
  direction?: "asc" | "desc";
};

export type ListActivityInput = {
  databaseId?: string;
  workspaceId?: string;
  limit?: number;
  until?: string;
};

export type ListEventsInput = {
  databaseId?: string;
  workspaceId?: string;
  kind?: string;
  sessionId?: string;
  path?: string;
  since?: string;
  until?: string;
  limit?: number;
  direction?: "asc" | "desc";
};

type HTTPChangelogEntry = {
  id: string;
  occurred_at?: string;
  workspace_id?: string;
  workspace_name?: string;
  database_id?: string;
  database_name?: string;
  session_id?: string;
  agent_id?: string;
  user?: string;
  label?: string;
  agent_version?: string;
  op: string;
  path: string;
  prev_path?: string;
  size_bytes?: number;
  delta_bytes?: number;
  content_hash?: string;
  prev_hash?: string;
  mode?: number;
  checkpoint_id?: string;
  source?: string;
  file_id?: string;
  version_id?: string;
};

type HTTPChangelogResponse = {
  entries: HTTPChangelogEntry[];
  next_cursor?: string;
};

type HTTPRedisStats = {
  used_memory_bytes?: number;
  max_memory_bytes?: number;
  fragmentation_ratio?: number;
  key_count?: number;
  ops_per_sec?: number;
  cache_hit_rate?: number;
  connected_clients?: number;
  sampled_at?: string;
};

type HTTPDatabase = {
  id: string;
  name: string;
  description?: string;
  owner_subject?: string;
  owner_label?: string;
  management_type?: string;
  purpose?: string;
  can_edit?: boolean;
  can_delete?: boolean;
  can_create_workspaces?: boolean;
  redis_addr: string;
  redis_username?: string;
  redis_db: number;
  redis_tls: boolean;
  is_default: boolean;
  workspace_count: number;
  active_session_count?: number;
  connection_error?: string;
  last_workspace_refresh_at?: string;
  last_workspace_refresh_error?: string;
  last_session_reconcile_at?: string;
  last_session_reconcile_error?: string;
  afs_total_bytes?: number;
  afs_file_count?: number;
  stats?: HTTPRedisStats;
};

type HTTPWorkspaceSummary = {
  id: string;
  name: string;
  cloud_account: string;
  database_id: string;
  database_name: string;
  owner_subject?: string;
  owner_label?: string;
  database_management_type?: string;
  database_can_edit?: boolean;
  database_can_delete?: boolean;
  redis_key: string;
  file_count: number;
  folder_count: number;
  total_bytes: number;
  checkpoint_count: number;
  last_checkpoint_at: string;
  updated_at: string;
  region: string;
  source: AFSWorkspaceSource;
  template_slug?: string;
};

type HTTPCheckpoint = {
  id: string;
  name: string;
  author?: string;
  note?: string;
  created_at: string;
  file_count: number;
  folder_count: number;
  total_bytes: number;
  is_head?: boolean;
};

type HTTPActivity = {
  id: string;
  workspace_id?: string;
  workspace_name?: string;
  database_id?: string;
  database_name?: string;
  actor: string;
  created_at: string;
  detail: string;
  kind: string;
  path?: string;
  scope: string;
  title: string;
};

type HTTPWorkspaceCapabilities = {
  browse_head: boolean;
  browse_checkpoints: boolean;
  browse_working_copy: boolean;
  edit_working_copy: boolean;
  create_checkpoint: boolean;
  restore_checkpoint: boolean;
};

type HTTPWorkspaceDetail = {
  id: string;
  name: string;
  description?: string;
  cloud_account: string;
  database_id: string;
  database_name: string;
  owner_subject?: string;
  owner_label?: string;
  database_management_type?: string;
  database_can_edit?: boolean;
  database_can_delete?: boolean;
  redis_key: string;
  region: string;
  source: AFSWorkspaceSource;
  template_slug?: string;
  created_at: string;
  updated_at: string;
  draft_state: string;
  head_checkpoint_id: string;
  tags?: string[];
  file_count: number;
  folder_count: number;
  total_bytes: number;
  checkpoint_count: number;
  checkpoints: HTTPCheckpoint[];
  activity: HTTPActivity[];
  capabilities: HTTPWorkspaceCapabilities;
};

type HTTPActivityList = {
  items: HTTPActivity[];
  next_cursor?: string;
};

type HTTPEventEntry = {
  id: string;
  workspace_id?: string;
  workspace_name?: string;
  database_id?: string;
  database_name?: string;
  created_at?: string;
  kind: string;
  op: string;
  source?: string;
  actor?: string;
  session_id?: string;
  user?: string;
  label?: string;
  agent_version?: string;
  hostname?: string;
  path?: string;
  prev_path?: string;
  size_bytes?: number;
  delta_bytes?: number;
  content_hash?: string;
  prev_hash?: string;
  mode?: number;
  checkpoint_id?: string;
  extras?: Record<string, string>;
};

type HTTPEventList = {
  items: HTTPEventEntry[];
  next_cursor?: string;
};

type HTTPWorkspaceSessionInfo = {
  session_id: string;
  workspace: string;
  workspace_id?: string;
  workspace_name?: string;
  database_id?: string;
  database_name?: string;
  agent_id?: string;
  client_kind?: string;
  afs_version?: string;
  hostname?: string;
  os?: string;
  local_path?: string;
  label?: string;
  readonly?: boolean;
  state: string;
  started_at: string;
  last_seen_at: string;
  lease_expires_at: string;
};

type HTTPWorkspaceSessionList = {
  items: HTTPWorkspaceSessionInfo[];
};

type HTTPTreeItem = {
  path: string;
  name: string;
  kind: AFSTreeItem["kind"];
  size: number;
  modified_at?: string;
  target?: string;
};

type HTTPTreeResponse = {
  workspace_id: string;
  view: AFSWorkspaceView;
  path: string;
  items: HTTPTreeItem[];
};

type HTTPFileContent = {
  workspace_id: string;
  view: AFSWorkspaceView;
  path: string;
  kind: AFSFileContent["kind"];
  revision: string;
  language: string;
  encoding: string;
  content_type: string;
  size: number;
  modified_at?: string;
  binary: boolean;
  content?: string;
  target?: string;
};

type HTTPDiffState = {
  view: AFSWorkspaceView;
  checkpoint_id?: string;
  manifest_hash?: string;
  file_count: number;
  folder_count: number;
  total_bytes: number;
};

type HTTPDiffSummary = {
  total: number;
  created: number;
  updated: number;
  deleted: number;
  renamed: number;
  metadata_changed: number;
  bytes_added: number;
  bytes_removed: number;
};

type HTTPTextDiffLine = {
  kind: "context" | "delete" | "insert";
  old_line?: number;
  new_line?: number;
  text: string;
};

type HTTPTextDiffHunk = {
  old_start: number;
  old_lines: number;
  new_start: number;
  new_lines: number;
  lines: HTTPTextDiffLine[];
};

type HTTPDiffEntry = {
  op: AFSWorkspaceDiffResponse["entries"][number]["op"];
  path: string;
  previous_path?: string;
  kind?: AFSWorkspaceDiffResponse["entries"][number]["kind"];
  previous_kind?: AFSWorkspaceDiffResponse["entries"][number]["previousKind"];
  size_bytes?: number;
  previous_size_bytes?: number;
  delta_bytes?: number;
  text_diff?: {
    language?: string;
    previous_exists: boolean;
    next_exists: boolean;
    hunks?: HTTPTextDiffHunk[];
  };
};

type HTTPWorkspaceDiffResponse = {
  workspace_id: string;
  workspace_name: string;
  base: HTTPDiffState;
  head: HTTPDiffState;
  summary: HTTPDiffSummary;
  entries: HTTPDiffEntry[];
};

type HTTPWorkspaceVersioningPolicy = {
  mode?: AFSWorkspaceVersioningPolicy["mode"];
  include_globs?: string[];
  exclude_globs?: string[];
  max_versions_per_file?: number;
  max_age_days?: number;
  max_total_bytes?: number;
  large_file_cutoff_bytes?: number;
};

type HTTPFileVersion = {
  version_id: string;
  file_id: string;
  ordinal: number;
  path: string;
  prev_path?: string;
  op: string;
  kind: "file" | "symlink" | "tombstone";
  blob_id?: string;
  content_hash?: string;
  prev_hash?: string;
  size_bytes?: number;
  delta_bytes?: number;
  mode?: number;
  target?: string;
  source?: string;
  session_id?: string;
  agent_id?: string;
  user?: string;
  checkpoint_ids?: string[];
  created_at: string;
};

type HTTPFileHistoryLineage = {
  file_id: string;
  state: string;
  current_path: string;
  versions: HTTPFileVersion[];
};

type HTTPFileHistoryResponse = {
  workspace_id: string;
  path: string;
  order: "asc" | "desc";
  lineages: HTTPFileHistoryLineage[];
  next_cursor?: string;
};

type HTTPFileVersionContent = {
  workspace_id: string;
  file_id: string;
  version_id: string;
  ordinal: number;
  path: string;
  kind: "file" | "symlink" | "tombstone";
  source?: string;
  content?: string;
  target?: string;
  binary?: boolean;
  encoding?: string;
  content_type?: string;
  language?: string;
  size: number;
  created_at: string;
};

type HTTPFileVersionSelector = {
  ref?: "head" | "working-copy";
  version_id?: string;
  file_id?: string;
  ordinal?: number;
};

type HTTPFileVersionDiff = {
  workspace_id: string;
  path: string;
  from: string;
  to: string;
  binary: boolean;
  diff?: string;
};

type HTTPFileVersionRestoreResponse = {
  workspace_id: string;
  path: string;
  dirty: boolean;
  file_id?: string;
  version_id?: string;
  restored_from_version_id?: string;
  restored_from_file_id?: string;
  restored_from_ordinal?: number;
};

type HTTPFileVersionUndeleteResponse = {
  workspace_id: string;
  path: string;
  dirty: boolean;
  file_id?: string;
  version_id?: string;
  undeleted_from_version_id?: string;
  undeleted_from_file_id?: string;
  undeleted_from_ordinal?: number;
};

type HTTPOnboardingTokenResponse = {
  token: string;
  database_id: string;
  workspace_id: string;
  workspace_name: string;
  expires_at: string;
};

type HTTPMCPToken = {
  id: string;
  name?: string;
  scope?: string;
  database_id?: string;
  workspace_id?: string;
  workspace_name?: string;
  profile?: string;
  readonly?: boolean;
  token?: string;
  created_at: string;
  last_used_at?: string;
  expires_at?: string;
  revoked_at?: string;
  template_slug?: string;
};

function mapHTTPMCPToken(
  item: HTTPMCPToken,
  opts?: { profileFallback?: AFSMCPToken["profile"] },
): AFSMCPToken {
  const profileFallback = opts?.profileFallback ?? "workspace-rw";
  return {
    id: item.id,
    name: item.name,
    scope: item.scope,
    databaseId: item.database_id ?? "",
    workspaceId: item.workspace_id ?? "",
    workspaceName: item.workspace_name,
    profile: (item.profile?.trim() || profileFallback) as AFSMCPToken["profile"],
    readonly: Boolean(item.readonly),
    token: item.token,
    createdAt: item.created_at,
    lastUsedAt: item.last_used_at,
    expiresAt: item.expires_at,
    revokedAt: item.revoked_at,
    templateSlug: item.template_slug,
  };
}

type HTTPAuthConfig = {
  mode: string;
  enabled: boolean;
  provider: string;
  sign_in_required: boolean;
  authenticated: boolean;
  product_mode?: string;
  clerk_publishable_key?: string;
  user?: {
    subject: string;
    name?: string;
    email?: string;
    groups?: string[];
  };
};

type HTTPAccount = {
  subject?: string;
  provider: string;
  can_delete_identity: boolean;
  can_reset_data: boolean;
  owned_database_count: number;
  owned_workspace_count: number;
  deleted_database_count?: number;
  deleted_workspace_count?: number;
  identity_deleted?: boolean;
};

type HTTPAdminOverview = {
  user_count: number;
  database_count: number;
  workspace_count: number;
  agent_count: number;
  active_agent_count: number;
  stale_agent_count: number;
  unavailable_database_count: number;
  total_bytes: number;
  file_count: number;
};

type HTTPAdminUser = {
  subject: string;
  label?: string;
  database_count: number;
  workspace_count: number;
  mcp_token_count: number;
  agent_session_count: number;
  last_seen_at?: string;
  sources?: string[];
};

class HTTPError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "HTTPError";
    this.status = status;
  }
}

function inferLocalHTTPBaseURL() {
  if (typeof window === "undefined") {
    return "";
  }
  const hostname = window.location.hostname.trim().toLowerCase();
  if (hostname !== "127.0.0.1" && hostname !== "localhost") {
    return "";
  }
  return `${window.location.protocol}//${hostname}:8091`;
}

const HTTP_BASE_URL = (
  import.meta.env.VITE_AFS_API_BASE_URL?.replace(/\/+$/, "") ??
  inferLocalHTTPBaseURL()
).trim();

function clone<T>(value: T) {
  return JSON.parse(JSON.stringify(value)) as T;
}

function wait() {
  return new Promise((resolve) => window.setTimeout(resolve, DEMO_DELAY_MS));
}

function nowISO() {
  return new Date().toISOString();
}

function makeId(prefix: string) {
  return `${prefix}-${Math.random().toString(36).slice(2, 8)}-${Date.now().toString(36)}`;
}

function slugify(value: string) {
  return value.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-");
}

function bytesCount(files: AFSFile[]) {
  return files.reduce(
    (sum, file) => sum + new TextEncoder().encode(file.content).length,
    0,
  );
}

function bytesLabel(files: AFSFile[]) {
  return formatBytes(bytesCount(files));
}

function bytesLabelForValue(value: number) {
  if (value >= 1024 * 1024) {
    return `${(value / (1024 * 1024)).toFixed(1)} MB`;
  }

  return `${Math.max(1, Math.round(value / 1024))} KB`;
}

function folderCount(files: AFSFile[]) {
  const folders = new Set<string>();

  for (const file of files) {
    const parts = file.path.split("/").slice(0, -1);
    let prefix = "";
    for (const part of parts) {
      prefix = prefix === "" ? part : `${prefix}/${part}`;
      folders.add(prefix);
    }
  }

  return folders.size;
}

function lastCheckpointAt(workspace: AFSWorkspace) {
  const values = workspace.savepoints.map((savepoint) => savepoint.createdAt);
  return values.sort((left, right) => right.localeCompare(left))[0] ?? workspace.updatedAt;
}

function demoCapabilities(): AFSWorkspaceCapabilities {
  return {
    browseHead: true,
    browseCheckpoints: true,
    browseWorkingCopy: true,
    editWorkingCopy: true,
    createCheckpoint: true,
    restoreCheckpoint: true,
  };
}

function normalizeWorkspace(workspace: AFSWorkspace): AFSWorkspace {
  const normalized: AFSWorkspace = clone(workspace);
  normalized.draftState = workspace.draftState || "clean";
  normalized.fileCount = workspace.fileCount;
  normalized.folderCount = workspace.folderCount;
  normalized.totalBytes = workspace.totalBytes;
  normalized.checkpointCount = workspace.checkpointCount;
  normalized.capabilities = workspace.capabilities;
  normalized.savepoints = workspace.savepoints.map((savepoint) => ({
    ...savepoint,
    folderCount: savepoint.folderCount,
    totalBytes: savepoint.totalBytes,
    sizeLabel: savepoint.sizeLabel || bytesLabel(savepoint.filesSnapshot),
  }));
  normalized.activity = workspace.activity.map((event) => ({
    ...event,
    workspaceId: event.workspaceId ?? workspace.id,
    workspaceName: event.workspaceName ?? workspace.name,
    databaseId: event.databaseId ?? workspace.databaseId,
    databaseName: event.databaseName ?? workspace.databaseName,
  }));
  normalized.agents = workspace.agents.map((agent) => ({
    ...agent,
    workspaceId: agent.workspaceId || workspace.id,
    workspaceName: agent.workspaceName || workspace.name,
    databaseId: agent.databaseId || workspace.databaseId,
    databaseName: agent.databaseName || workspace.databaseName,
  }));
  return normalized;
}

function createActivity(
  title: string,
  detail: string,
  actor: string,
  kind: string,
  scope: string,
  workspaceId: string,
  workspaceName: string,
  activityPath?: string,
): AFSActivityEvent {
  return {
    id: makeId("evt"),
    actor,
    createdAt: nowISO(),
    detail,
    kind,
    path: activityPath,
    scope,
    title,
    workspaceId,
    workspaceName,
  };
}

function createSavepointRecord(
  name: string,
  note: string,
  author: string,
  files: AFSFile[],
): AFSSavepoint {
  return {
    id: makeId("sp"),
    name,
    author,
    createdAt: nowISO(),
    note,
    fileCount: files.length,
    folderCount: folderCount(files),
    totalBytes: bytesCount(files),
    sizeLabel: bytesLabel(files),
    filesSnapshot: clone(files),
  };
}

function sourceLabel(source: AFSWorkspaceSource) {
  if (source === "git-import") return "Git import";
  if (source === "cloud-import") return "Redis Cloud import";
  return "Blank workspace";
}

function workspaceToSummary(workspace: AFSWorkspace): AFSWorkspaceSummary {
  const normalized = normalizeWorkspace(workspace);
  return {
    id: normalized.id,
    name: normalized.name,
    cloudAccount: normalized.cloudAccount,
    databaseId: normalized.databaseId,
    databaseName: normalized.databaseName,
    redisKey: normalized.redisKey,
    fileCount: normalized.fileCount,
    folderCount: normalized.folderCount,
    totalBytes: normalized.totalBytes,
    checkpointCount: normalized.checkpointCount,
    lastCheckpointAt: lastCheckpointAt(normalized),
    updatedAt: normalized.updatedAt,
    region: normalized.region,
    source: normalized.source,
  };
}

function loadState(): AFSState {
  const raw = window.localStorage.getItem(STORAGE_KEY);

  if (raw == null) {
    const seeded = cloneInitialAFSState();
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(seeded));
    return seeded;
  }

  try {
    return JSON.parse(raw) as AFSState;
  } catch {
    const reset = cloneInitialAFSState();
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(reset));
    return reset;
  }
}

function saveState(state: AFSState) {
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
}

function loadDatabaseState(): AFSDatabase[] {
  const raw = window.localStorage.getItem(DATABASE_STORAGE_KEY);
  if (raw == null) {
    return [];
  }

  try {
    return JSON.parse(raw) as AFSDatabase[];
  } catch {
    window.localStorage.removeItem(DATABASE_STORAGE_KEY);
    return [];
  }
}

function saveDatabaseState(databases: AFSDatabase[]) {
  window.localStorage.setItem(DATABASE_STORAGE_KEY, JSON.stringify(databases));
}

function updateState(mutator: (draft: AFSState) => void) {
  const state = loadState();
  const draft = clone(state);
  mutator(draft);
  saveState(draft);
  return draft;
}

function requireWorkspace(state: AFSState, workspaceId: string) {
  const workspace = state.workspaces.find((item) => item.id === workspaceId);
  if (workspace == null) {
    throw new Error(`Workspace ${workspaceId} was not found.`);
  }
  return workspace;
}

function requireSavepoint(workspace: AFSWorkspace, savepointId: string) {
  const savepoint = workspace.savepoints.find((item) => item.id === savepointId);
  if (savepoint == null) {
    throw new Error(`Savepoint ${savepointId} was not found.`);
  }
  return savepoint;
}

function matchesOptionalDatabase(databaseId: string | undefined, workspace: AFSWorkspace) {
  const resolved = databaseId?.trim() ?? "";
  return resolved === "" || workspace.databaseId === resolved;
}

function touchWorkspace(workspace: AFSWorkspace) {
  workspace.updatedAt = nowISO();
  workspace.fileCount = workspace.files.length;
  workspace.folderCount = folderCount(workspace.files);
  workspace.totalBytes = bytesCount(workspace.files);
  workspace.checkpointCount = workspace.savepoints.length;
}

function sortWorkspaces(items: AFSWorkspace[]) {
  return [...items].sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));
}

function deriveDemoDatabases(state: AFSState) {
  const grouped = new Map<string, AFSDatabase>();

  for (const workspace of state.workspaces) {
    const existing = grouped.get(workspace.databaseId);
    if (existing == null) {
      grouped.set(workspace.databaseId, {
        id: workspace.databaseId,
        name: workspace.databaseName,
        description: "",
        purpose: "general",
        canEdit: true,
        canDelete: true,
        canCreateWorkspaces: true,
        redisAddr: workspace.databaseName,
        redisUsername: "",
        redisPassword: "",
        redisDB: 0,
        redisTLS: false,
        isDefault: false,
        workspaceCount: 1,
        activeSessionCount: 0,
      });
      continue;
    }
    existing.workspaceCount += 1;
  }

  const saved = loadDatabaseState();
  for (const database of saved) {
    const existing = grouped.get(database.id);
    grouped.set(database.id, {
      ...database,
      workspaceCount: existing?.workspaceCount ?? database.workspaceCount,
      activeSessionCount: existing?.activeSessionCount ?? database.activeSessionCount,
    });
  }

  const items = [...grouped.values()].sort((left, right) => left.name.localeCompare(right.name));
  if (items.length > 0 && !items.some((item) => item.isDefault)) {
    items[0] = { ...items[0], isDefault: true };
  }
  return items;
}

function requireDemoDatabase(databaseId?: string) {
  const databases = deriveDemoDatabases(loadState());
  const resolvedID = databaseId?.trim() ?? "";
  const database = resolvedID === ""
    ? databases.find((item) => item.isDefault) ?? databases[0]
    : databases.find((item) => item.id === resolvedID);
  if (database == null) {
    throw new Error(resolvedID === "" ? "No database was found." : `Database ${resolvedID} was not found.`);
  }
  return database;
}

function normalizeFilePath(value: string) {
  const trimmed = value.trim();
  if (trimmed === "" || trimmed === "/") {
    return "/";
  }
  return `/${trimmed.replace(/^\/+/, "")}`;
}

function demoFilesForView(workspace: AFSWorkspace, view: AFSWorkspaceView): AFSFile[] {
  if (view === "working-copy") {
    return workspace.files.map((file) => ({
      ...file,
      path: normalizeFilePath(file.path),
    }));
  }

  const checkpointId =
    view === "head" ? workspace.headSavepointId : view.replace(/^checkpoint:/, "");
  const savepoint = requireSavepoint(workspace, checkpointId);
  return savepoint.filesSnapshot.map((file) => ({
    ...file,
    path: normalizeFilePath(file.path),
  }));
}

function parentPath(value: string) {
  const normalized = normalizeFilePath(value);
  if (normalized === "/") {
    return "/";
  }
  const parts = normalized.split("/").filter(Boolean);
  parts.pop();
  return parts.length === 0 ? "/" : `/${parts.join("/")}`;
}

function baseName(value: string) {
  const normalized = normalizeFilePath(value);
  if (normalized === "/") {
    return "/";
  }
  return normalized.split("/").filter(Boolean).at(-1) ?? normalized;
}

function languageForPath(path: string) {
  const lower = path.toLowerCase();
  if (lower.endsWith(".md")) return "markdown";
  if (lower.endsWith(".go")) return "go";
  if (lower.endsWith(".ts") || lower.endsWith(".tsx")) return "typescript";
  if (lower.endsWith(".js") || lower.endsWith(".jsx")) return "javascript";
  if (lower.endsWith(".json")) return "json";
  if (lower.endsWith(".yaml") || lower.endsWith(".yml")) return "yaml";
  if (lower.endsWith(".sh")) return "shell";
  if (lower.endsWith(".py")) return "python";
  return "text";
}

function contentTypeForPath(path: string) {
  const lower = path.toLowerCase();
  if (lower.endsWith(".md")) return "text/markdown";
  if (lower.endsWith(".json")) return "application/json";
  if (lower.endsWith(".yaml") || lower.endsWith(".yml")) return "application/yaml";
  return "text/plain";
}

function treeItemsForFiles(files: AFSFile[], currentPath: string): AFSTreeItem[] {
  const normalizedPath = normalizeFilePath(currentPath);
  const items = new Map<string, AFSTreeItem>();

  for (const file of files) {
    const filePath = normalizeFilePath(file.path);
    const parent = parentPath(filePath);

    if (parent === normalizedPath) {
      items.set(filePath, {
        path: filePath,
        name: baseName(filePath),
        kind: "file",
        size: new TextEncoder().encode(file.content).length,
        modifiedAt: file.modifiedAt,
      });
    }

    const segments = filePath.split("/").filter(Boolean);
    let prefix = "";
    for (let index = 0; index < segments.length - 1; index += 1) {
      prefix = `${prefix}/${segments[index]}`;
      const parentDir = parentPath(prefix);
      if (parentDir === normalizedPath && !items.has(prefix)) {
        items.set(prefix, {
          path: prefix,
          name: baseName(prefix),
          kind: "dir",
          size: 0,
        });
      }
    }
  }

  return [...items.values()].sort((left, right) => {
    if (left.kind !== right.kind) {
      return left.kind === "dir" ? -1 : 1;
    }
    return left.path.localeCompare(right.path);
  });
}

function allActivityForState(state: AFSState) {
  return state.workspaces
    .flatMap((workspace) =>
      normalizeWorkspace(workspace).activity.map((event) => ({
        ...event,
        workspaceId: event.workspaceId ?? workspace.id,
        workspaceName: event.workspaceName ?? workspace.name,
        databaseId: event.databaseId ?? workspace.databaseId,
        databaseName: event.databaseName ?? workspace.databaseName,
      })),
    )
    .sort((left, right) => right.createdAt.localeCompare(left.createdAt));
}

function allEventsForState(state: AFSState): AFSEventEntry[] {
  return allActivityForState(state).map((event) => ({
    id: event.id,
    workspaceId: event.workspaceId,
    workspaceName: event.workspaceName,
    databaseId: event.databaseId,
    databaseName: event.databaseName,
    createdAt: event.createdAt,
    kind: event.scope || event.kind,
    op: event.kind,
    actor: event.actor,
    path: event.path,
  }));
}

function activityForState(state: AFSState, limit: number) {
  return allActivityForState(state).slice(0, limit);
}

function fileSizeBytes(file: AFSFile) {
  return new TextEncoder().encode(file.content).length;
}

function demoTextDiff(previous: AFSFile | undefined, next: AFSFile | undefined): AFSTextDiff {
  const previousLines = previous?.content.split("\n") ?? [];
  const nextLines = next?.content.split("\n") ?? [];
  return {
    language: next?.language ?? previous?.language,
    previousExists: previous != null,
    nextExists: next != null,
    hunks: [
      {
        oldStart: 1,
        oldLines: previousLines.length,
        newStart: 1,
        newLines: nextLines.length,
        lines: [
          ...previousLines.map((text, index) => ({ kind: "delete" as const, oldLine: index + 1, text })),
          ...nextLines.map((text, index) => ({ kind: "insert" as const, newLine: index + 1, text })),
        ],
      },
    ],
  };
}

function demoDiffState(workspace: AFSWorkspace, view: AFSWorkspaceView, files: AFSFile[]) {
  const checkpointId = view === "head" ? workspace.headSavepointId : view.startsWith("checkpoint:") ? view.slice("checkpoint:".length) : undefined;
  return {
    view,
    checkpointId,
    manifestHash: undefined,
    fileCount: files.length,
    folderCount: folderCount(files),
    totalBytes: bytesCount(files),
  };
}

function summarizeDiffEntries(entries: AFSDiffEntry[]) {
  return entries.reduce(
    (summary, entry) => {
      summary.total += 1;
      if (entry.op === "create") summary.created += 1;
      if (entry.op === "update") summary.updated += 1;
      if (entry.op === "delete") summary.deleted += 1;
      if (entry.op === "rename") summary.renamed += 1;
      if (entry.op === "metadata") summary.metadataChanged += 1;
      if ((entry.deltaBytes ?? 0) > 0) summary.bytesAdded += entry.deltaBytes ?? 0;
      if ((entry.deltaBytes ?? 0) < 0) summary.bytesRemoved += Math.abs(entry.deltaBytes ?? 0);
      return summary;
    },
    {
      total: 0,
      created: 0,
      updated: 0,
      deleted: 0,
      renamed: 0,
      metadataChanged: 0,
      bytesAdded: 0,
      bytesRemoved: 0,
    },
  );
}

function compareDemoFiles(
  workspace: AFSWorkspace,
  baseView: AFSWorkspaceView,
  headView: AFSWorkspaceView,
): AFSWorkspaceDiffResponse {
  const baseFiles = demoFilesForView(workspace, baseView);
  const headFiles = demoFilesForView(workspace, headView);
  const baseByPath = new Map(baseFiles.map((file) => [normalizeFilePath(file.path), file]));
  const headByPath = new Map(headFiles.map((file) => [normalizeFilePath(file.path), file]));
  const entries: AFSDiffEntry[] = [];
  const allPaths = new Set([...baseByPath.keys(), ...headByPath.keys()]);

  for (const path of [...allPaths].sort()) {
    const previous = baseByPath.get(path);
    const next = headByPath.get(path);
    if (previous == null && next != null) {
      const sizeBytes = fileSizeBytes(next);
      entries.push({ op: "create", path, kind: "file", sizeBytes, deltaBytes: sizeBytes, textDiff: demoTextDiff(undefined, next) });
      continue;
    }
    if (previous != null && next == null) {
      const previousSizeBytes = fileSizeBytes(previous);
      entries.push({ op: "delete", path, previousPath: path, previousKind: "file", previousSizeBytes, deltaBytes: previousSizeBytes * -1, textDiff: demoTextDiff(previous, undefined) });
      continue;
    }
    if (previous != null && next != null && previous.content !== next.content) {
      const previousSizeBytes = fileSizeBytes(previous);
      const sizeBytes = fileSizeBytes(next);
      entries.push({ op: "update", path, kind: "file", previousKind: "file", sizeBytes, previousSizeBytes, deltaBytes: sizeBytes - previousSizeBytes, textDiff: demoTextDiff(previous, next) });
    }
  }

  return {
    workspaceId: workspace.id,
    workspaceName: workspace.name,
    base: demoDiffState(workspace, baseView, baseFiles),
    head: demoDiffState(workspace, headView, headFiles),
    summary: summarizeDiffEntries(entries),
    entries,
  };
}

const demoAFSClient: AFSClient = {
  mode: "demo",

  async listDatabases() {
    await wait();
    return deriveDemoDatabases(loadState());
  },

  async reconcileCatalog() {
    await wait();
  },

  async saveDatabase(input: SaveDatabaseInput) {
    await wait();
    const current = loadDatabaseState();
    const id = input.id?.trim() || slugify(`${input.name}-${input.redisAddr}-${input.redisDB}`);
    const wasDefault = current.find((item) => item.id === id)?.isDefault ?? false;
    const nextRecord: AFSDatabase = {
      id,
      name: input.name.trim(),
      description: input.description.trim(),
      purpose: "general",
      canEdit: true,
      canDelete: true,
      canCreateWorkspaces: true,
      redisAddr: input.redisAddr.trim(),
      redisUsername: input.redisUsername.trim(),
      redisPassword: input.redisPassword,
      redisDB: input.redisDB,
      redisTLS: input.redisTLS,
      isDefault: wasDefault || current.length === 0,
      workspaceCount: deriveDemoDatabases(loadState()).find((item) => item.id === id)?.workspaceCount ?? 0,
      activeSessionCount: 0,
    };
    const next = current
      .filter((item) => item.id !== id)
      .map((item) => ({ ...item, isDefault: nextRecord.isDefault ? false : item.isDefault }));
    next.unshift(nextRecord);
    saveDatabaseState(next);
    return nextRecord;
  },

  async setDefaultDatabase(databaseId: string) {
    await wait();
    const current = deriveDemoDatabases(loadState());
    const next = current.map((item) => ({ ...item, isDefault: item.id === databaseId }));
    saveDatabaseState(next);
    const updated = next.find((item) => item.id === databaseId);
    if (updated == null) {
      throw new Error(`Database ${databaseId} was not found.`);
    }
    return updated;
  },

  async deleteDatabase(databaseId: string) {
    await wait();
    const next = loadDatabaseState().filter((item) => item.id !== databaseId);
    if (next.length > 0 && !next.some((item) => item.isDefault)) {
      next[0] = { ...next[0], isDefault: true };
    }
    saveDatabaseState(next);
  },

  async listWorkspaceSummaries(databaseId = "") {
    await wait();
    const state = loadState();
    const workspaces = state.workspaces
      .map(normalizeWorkspace)
      .filter((workspace) => databaseId === "" || workspace.databaseId === databaseId);

    return sortWorkspaces(workspaces).map(workspaceToSummary);
  },

  async getWorkspace(databaseId = "", workspaceId: string) {
    await wait();
    const state = loadState();
    const workspace = state.workspaces.find(
      (item) => item.id === workspaceId && (databaseId === "" || item.databaseId === databaseId),
    );
    return workspace == null ? null : normalizeWorkspace(workspace);
  },

  async listAgents(databaseId = "") {
    await wait();
    const state = loadState();
    return sortWorkspaces(state.workspaces)
      .filter((workspace) => databaseId === "" || workspace.databaseId === databaseId)
      .flatMap((workspace) => normalizeWorkspace(workspace).agents)
      .sort((left, right) => right.lastSeenAt.localeCompare(left.lastSeenAt));
  },

  async createWorkspace(input: CreateWorkspaceInput) {
    await wait();
    const database = requireDemoDatabase(input.databaseId);
    const state = updateState((draft) => {
      const id = slugify(input.name);
      const createdAt = nowISO();
      const baseFiles: AFSFile[] = [
        {
          path: "README.md",
          language: "markdown",
          modifiedAt: createdAt,
          content: `# ${input.name}

This workspace was created from the AFS Web UI.

- account: ${input.cloudAccount}
- database: ${input.databaseName}
- region: ${input.region}
- source: ${sourceLabel(input.source)}`,
        },
      ];
      const initialSavepoint = createSavepointRecord(
        "initial",
        "Workspace created from the Web UI.",
        "webui",
        baseFiles,
      );

      const workspace = normalizeWorkspace({
        id,
        name: input.name.trim(),
        description: input.description.trim(),
        cloudAccount: input.cloudAccount?.trim() || "Direct Redis",
        databaseId: database.id,
        databaseName: input.databaseName?.trim() || database.name,
        redisKey: `afs:${id}`,
        region: input.region?.trim() || "",
        mountedPath: `~/.afs/workspaces/${id}`,
        source: input.source,
        createdAt,
        updatedAt: createdAt,
        draftState: "clean",
        headSavepointId: initialSavepoint.id,
        tags: [(input.region?.trim() || ""), sourceLabel(input.source)],
        files: baseFiles,
        savepoints: [initialSavepoint],
        activity: [
          createActivity(
            `Created ${input.name.trim()}`,
            "Workspace provisioned from the catalog page.",
            "webui",
            "workspace.created",
            "workspace",
            id,
            input.name.trim(),
          ),
        ],
        agents: [],
        capabilities: demoCapabilities(),
        fileCount: baseFiles.length,
        folderCount: folderCount(baseFiles),
        totalBytes: bytesCount(baseFiles),
        checkpointCount: 1,
      });

      draft.workspaces.unshift(workspace);
    });

    return normalizeWorkspace(state.workspaces[0]);
  },

  async deleteWorkspace(databaseId: string, workspaceId: string) {
    await wait();
    updateState((draft) => {
      draft.workspaces = draft.workspaces.filter(
        (workspace) => !(workspace.id === workspaceId && matchesOptionalDatabase(databaseId, workspace)),
      );
    });
  },

  async updateWorkspace(input: UpdateWorkspaceInput) {
    await wait();
    const state = updateState((draft) => {
      const workspace = requireWorkspace(draft, input.workspaceId);
      if (!matchesOptionalDatabase(input.databaseId, workspace)) {
        throw new Error(`Workspace ${input.workspaceId} was not found in database ${input.databaseId}.`);
      }
      if (input.name !== undefined) {
        workspace.name = input.name.trim();
      }
      if (input.description !== undefined) {
        workspace.description = input.description.trim();
      }
      workspace.cloudAccount = input.cloudAccount?.trim() || workspace.cloudAccount;
      workspace.databaseName = input.databaseName?.trim() || workspace.databaseName;
      workspace.region = input.region?.trim() || workspace.region;
      workspace.tags = [workspace.region, sourceLabel(workspace.source)].filter(Boolean);
      touchWorkspace(workspace);
      workspace.activity.unshift(
        createActivity(
          `Updated ${workspace.name}`,
          "Workspace details were updated from the Web UI.",
          "webui",
          "workspace.updated",
          "workspace",
          workspace.id,
          workspace.name,
        ),
      );
    });

    return normalizeWorkspace(requireWorkspace(state, input.workspaceId));
  },

  async updateWorkspaceFile(input: UpdateWorkspaceFileInput) {
    await wait();
    const state = updateState((draft) => {
      const workspace = requireWorkspace(draft, input.workspaceId);
      if (!matchesOptionalDatabase(input.databaseId, workspace)) {
        throw new Error(`Workspace ${input.workspaceId} was not found in database ${input.databaseId}.`);
      }
      const modifiedAt = nowISO();
      const normalizedPath = normalizeFilePath(input.path).replace(/^\//, "");
      const file = workspace.files.find(
        (item) => normalizeFilePath(item.path).replace(/^\//, "") === normalizedPath,
      );
      if (file == null) {
        workspace.files.unshift({
          path: normalizedPath,
          language: languageForPath(normalizedPath),
          modifiedAt,
          content: input.content,
        });
      } else {
        file.content = input.content;
        file.modifiedAt = modifiedAt;
      }
      workspace.draftState = "dirty";
      workspace.capabilities = demoCapabilities();
      touchWorkspace(workspace);
      workspace.activity.unshift(
        createActivity(
          `Edited ${normalizedPath}`,
          "Updated from the Web UI editor.",
          "webui",
          "file.updated",
          "file",
          workspace.id,
          workspace.name,
        ),
      );
    });

    return normalizeWorkspace(requireWorkspace(state, input.workspaceId));
  },

  async getWorkspaceVersioningPolicy(input: GetWorkspaceVersioningPolicyInput) {
    await wait();
    const policies = loadDemoVersioningPolicies();
    return policies[input.workspaceId] ?? {
      mode: "off",
      includeGlobs: [],
      excludeGlobs: [],
      maxVersionsPerFile: 0,
      maxAgeDays: 0,
      maxTotalBytes: 0,
      largeFileCutoffBytes: 0,
    };
  },

  async updateWorkspaceVersioningPolicy(input: UpdateWorkspaceVersioningPolicyInput) {
    await wait();
    const policies = loadDemoVersioningPolicies();
    policies[input.workspaceId] = input.policy;
    saveDemoVersioningPolicies(policies);
    return input.policy;
  },

  async getFileHistory(input: GetFileHistoryInput) {
    await wait();
    return {
      workspaceId: input.workspaceId,
      path: normalizeFilePath(input.path),
      order: input.direction ?? "desc",
      lineages: [],
    };
  },

  async getFileVersionContent() {
    await wait();
    return null;
  },

  async diffFileVersions(input: DiffFileVersionsInput) {
    await wait();
    return {
      path: normalizeFilePath(input.path),
      from: "unavailable",
      to: "unavailable",
      binary: false,
      diff: "",
    };
  },

  async restoreFileVersion() {
    throw new Error("File version restore is not available in demo mode.");
  },

  async undeleteFileVersion() {
    throw new Error("File version undelete is not available in demo mode.");
  },

  async createSavepoint(input: CreateSavepointInput) {
    await wait();
    const state = updateState((draft) => {
      const workspace = requireWorkspace(draft, input.workspaceId);
      if (!matchesOptionalDatabase(input.databaseId, workspace)) {
        throw new Error(`Workspace ${input.workspaceId} was not found in database ${input.databaseId}.`);
      }
      const savepoint = createSavepointRecord(
        input.name.trim(),
        input.note.trim(),
        "webui",
        workspace.files,
      );
      workspace.savepoints.unshift(savepoint);
      workspace.draftState = "clean";
      workspace.headSavepointId = savepoint.id;
      workspace.updatedAt = savepoint.createdAt;
      workspace.checkpointCount = workspace.savepoints.length;
      workspace.activity.unshift(
        createActivity(
          `Created savepoint ${savepoint.name}`,
          "Checkpoint captured from the Web UI.",
          "webui",
          "savepoint.created",
          "savepoint",
          workspace.id,
          workspace.name,
        ),
      );
    });

    return normalizeWorkspace(requireWorkspace(state, input.workspaceId));
  },

  async restoreSavepoint(input: RestoreSavepointInput) {
    await wait();
    const state = updateState((draft) => {
      const workspace = requireWorkspace(draft, input.workspaceId);
      if (!matchesOptionalDatabase(input.databaseId, workspace)) {
        throw new Error(`Workspace ${input.workspaceId} was not found in database ${input.databaseId}.`);
      }
      const savepoint = requireSavepoint(workspace, input.savepointId);
      workspace.files = clone(savepoint.filesSnapshot);
      workspace.draftState = "clean";
      workspace.headSavepointId = savepoint.id;
      touchWorkspace(workspace);
      workspace.activity.unshift(
        createActivity(
          `Restored ${savepoint.name}`,
          "Workspace files rolled back to a saved checkpoint.",
          "webui",
          "savepoint.restored",
          "savepoint",
          workspace.id,
          workspace.name,
        ),
      );
    });

    return normalizeWorkspace(requireWorkspace(state, input.workspaceId));
  },

  async listActivity(databaseId = "", limit = 50) {
    await wait();
    const state = loadState();
    return allActivityForState(state).filter((event) =>
      databaseId === "" || event.databaseId === databaseId,
    ).slice(0, limit);
  },

  async listActivityPage(input: ListActivityInput) {
    await wait();
    const state = loadState();
    const limit = input.limit != null && input.limit > 0 ? input.limit : 50;
    const events = allActivityForState(state)
      .filter((event) => input.databaseId == null || input.databaseId === "" || event.databaseId === input.databaseId)
      .filter((event) => input.until == null || input.until === "" || event.id < input.until);
    const page = events.slice(0, limit);
    return {
      items: page,
      nextCursor: page.length > 0 ? page[page.length - 1].id : undefined,
    };
  },

  async listEvents(input: ListEventsInput) {
    await wait();
    const state = loadState();
    const limit = input.limit != null && input.limit > 0 ? input.limit : 100;
    const events = allEventsForState(state)
      .filter((event) => input.databaseId == null || input.databaseId === "" || event.databaseId === input.databaseId)
      .filter((event) => input.workspaceId == null || input.workspaceId === "" || event.workspaceId === input.workspaceId)
      .filter((event) => input.kind == null || input.kind === "" || event.kind === input.kind)
      .filter((event) => input.sessionId == null || input.sessionId === "" || event.sessionId === input.sessionId)
      .filter((event) => input.path == null || input.path === "" || event.path === input.path)
      .filter((event) => input.until == null || input.until === "" || event.id < input.until);
    const page = events.slice(0, limit);
    return {
      items: page,
      nextCursor: page.length > 0 ? page[page.length - 1].id : undefined,
    };
  },

  async listChangelog(_input: ListChangelogInput): Promise<AFSChangelogResponse> {
    await wait();
    return { entries: [] };
  },

  async getWorkspaceTree(input: GetWorkspaceTreeInput) {
    await wait();
    const state = loadState();
    const workspace = normalizeWorkspace(requireWorkspace(state, input.workspaceId));
    if (!matchesOptionalDatabase(input.databaseId, workspace)) {
      throw new Error(`Workspace ${input.workspaceId} was not found in database ${input.databaseId}.`);
    }
    const files = demoFilesForView(workspace, input.view);

    return {
      workspaceId: workspace.id,
      view: input.view,
      path: normalizeFilePath(input.path),
      items: treeItemsForFiles(files, input.path),
    };
  },

  async getWorkspaceFileContent(input: GetWorkspaceFileContentInput) {
    await wait();
    const state = loadState();
    const workspace = normalizeWorkspace(requireWorkspace(state, input.workspaceId));
    if (!matchesOptionalDatabase(input.databaseId, workspace)) {
      throw new Error(`Workspace ${input.workspaceId} was not found in database ${input.databaseId}.`);
    }
    const files = demoFilesForView(workspace, input.view);
    const normalizedPath = normalizeFilePath(input.path);
    const file = files.find((item) => normalizeFilePath(item.path) === normalizedPath);
    if (file == null) {
      return null;
    }

    return {
      workspaceId: workspace.id,
      view: input.view,
      path: normalizedPath,
      kind: "file",
      revision: `${workspace.headSavepointId}:${normalizedPath}:${file.modifiedAt}`,
      language: file.language || languageForPath(file.path),
      encoding: "utf-8",
      contentType: contentTypeForPath(file.path),
      size: new TextEncoder().encode(file.content).length,
      modifiedAt: file.modifiedAt,
      binary: false,
      content: file.content,
    };
  },

  async getWorkspaceDiff(input: GetWorkspaceDiffInput) {
    await wait();
    const state = loadState();
    const workspace = normalizeWorkspace(requireWorkspace(state, input.workspaceId));
    if (!matchesOptionalDatabase(input.databaseId, workspace)) {
      throw new Error(`Workspace ${input.workspaceId} was not found in database ${input.databaseId}.`);
    }
    return compareDemoFiles(workspace, input.base, input.head);
  },

  async quickstart() {
    throw new Error("Quickstart is not available in demo mode.");
  },

  async createOnboardingToken(_databaseId: string | undefined, _workspaceId: string) {
    throw new Error("Onboarding tokens are not available in demo mode.");
  },

  async listMCPAccessTokens() {
    throw new Error("MCP tokens are not available in demo mode.");
  },

  async listAllMCPAccessTokens() {
    throw new Error("MCP tokens are not available in demo mode.");
  },

  async createMCPAccessToken() {
    throw new Error("MCP tokens are not available in demo mode.");
  },

  async revokeMCPAccessToken() {
    throw new Error("MCP tokens are not available in demo mode.");
  },

  async listControlPlaneTokens() {
    return [] as AFSMCPToken[];
  },

  async createControlPlaneToken() {
    throw new Error("MCP tokens are not available in demo mode.");
  },

  async revokeControlPlaneToken() {
    throw new Error("MCP tokens are not available in demo mode.");
  },

  async importLocal() {
    throw new Error("Import is not available in demo mode.");
  },

  async getAuthConfig() {
    return {
      mode: "none",
      enabled: false,
      provider: "none",
      signInRequired: false,
      authenticated: true,
      productMode: "self-hosted",
    };
  },

  async getServerVersion() {
    return { version: "demo" } satisfies AFSServerVersion;
  },

  async getAccount() {
    return {
      provider: "none",
      canDeleteIdentity: false,
      canResetData: false,
      ownedDatabaseCount: 0,
      ownedWorkspaceCount: 0,
    };
  },

  async resetAccountData() {
    throw new Error("Account reset is not available in demo mode.");
  },

  async deleteAccount() {
    throw new Error("Account deletion is not available in demo mode.");
  },

  async getAdminOverview() {
    const databases = deriveDemoDatabases(loadState());
    const state = loadState();
    const agents = sortWorkspaces(state.workspaces).flatMap((workspace) => normalizeWorkspace(workspace).agents);
    return {
      userCount: 0,
      databaseCount: databases.length,
      workspaceCount: state.workspaces.length,
      agentCount: agents.length,
      activeAgentCount: agents.filter((agent) => agent.state === "active").length,
      staleAgentCount: agents.filter((agent) => agent.state !== "active").length,
      unavailableDatabaseCount: databases.filter((database) => Boolean(database.connectionError)).length,
      totalBytes: state.workspaces.reduce((sum, workspace) => sum + workspace.totalBytes, 0),
      fileCount: state.workspaces.reduce((sum, workspace) => sum + workspace.fileCount, 0),
    };
  },

  async listAdminUsers() {
    return [] as AFSAdminUser[];
  },

  async listAdminDatabases() {
    return demoAFSClient.listDatabases();
  },

  async listAdminWorkspaceSummaries() {
    return demoAFSClient.listWorkspaceSummaries();
  },

  async listAdminAgents() {
    return demoAFSClient.listAgents();
  },

  resetDemo() {
    const seeded = cloneInitialAFSState();
    saveState(seeded);
    return seeded;
  },
};

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const url = `${HTTP_BASE_URL}/v1${path}`;
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), HTTP_REQUEST_TIMEOUT_MS);
  let response: Response;
  try {
    response = await fetch(url, {
      ...init,
      signal: controller.signal,
      headers: {
        "Content-Type": "application/json",
        ...(init?.headers ?? {}),
      },
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") {
      throw new Error(`Request to ${path} timed out after ${HTTP_REQUEST_TIMEOUT_MS / 1000}s.`);
    }
    throw error;
  } finally {
    window.clearTimeout(timeout);
  }

  const rawBody = response.status === 204 ? "" : await response.text();

  if (!response.ok) {
    let message = `Request failed with status ${response.status}`;
    try {
      const payload = JSON.parse(rawBody) as { error?: string };
      if (payload.error) {
        message = payload.error;
      }
    } catch {
      if (rawBody.trim() !== "") {
        message = rawBody;
      }
    }
    throw new HTTPError(response.status, message);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  try {
    return JSON.parse(rawBody) as T;
  } catch (error) {
    const contentType = response.headers.get("content-type") ?? "unknown";
    const preview = rawBody.slice(0, 160).replace(/\s+/g, " ").trim();
    throw new Error(
      `Expected JSON from ${url}, but received ${contentType} (status ${response.status}). Body preview: ${preview || "<empty>"}`,
      { cause: error },
    );
  }
}

function hasHTTPBackend() {
  return HTTP_BASE_URL !== "";
}

function workspaceBasePath(databaseId: string | undefined, workspaceId: string) {
  const resolvedDatabaseID = databaseId?.trim() ?? "";
  if (resolvedDatabaseID === "") {
    return `/workspaces/${workspaceId}`;
  }
  return `/databases/${resolvedDatabaseID}/workspaces/${workspaceId}`;
}

function loadDemoVersioningPolicies(): Record<string, AFSWorkspaceVersioningPolicy> {
  if (typeof window === "undefined") {
    return {};
  }
  try {
    const raw = window.localStorage.getItem(VERSIONING_POLICY_STORAGE_KEY);
    if (!raw) {
      return {};
    }
    return JSON.parse(raw) as Record<string, AFSWorkspaceVersioningPolicy>;
  } catch {
    return {};
  }
}

function saveDemoVersioningPolicies(policies: Record<string, AFSWorkspaceVersioningPolicy>) {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem(VERSIONING_POLICY_STORAGE_KEY, JSON.stringify(policies));
}

function resolveAFSClient() {
  if (CLIENT_MODE_OVERRIDE === "demo") {
    return demoAFSClient;
  }
  return httpAFSClient;
}

function mapCapabilities(input: HTTPWorkspaceCapabilities): AFSWorkspaceCapabilities {
  return {
    browseHead: input.browse_head,
    browseCheckpoints: input.browse_checkpoints,
    browseWorkingCopy: input.browse_working_copy,
    editWorkingCopy: input.edit_working_copy,
    createCheckpoint: input.create_checkpoint,
    restoreCheckpoint: input.restore_checkpoint,
  };
}

function mapDatabase(input: HTTPDatabase): AFSDatabase {
  return {
    id: input.id,
    name: input.name,
    description: input.description ?? "",
    ownerSubject: input.owner_subject,
    ownerLabel: input.owner_label,
    managementType: input.management_type,
    purpose: input.purpose,
    canEdit: input.can_edit ?? true,
    canDelete: input.can_delete ?? true,
    canCreateWorkspaces: input.can_create_workspaces ?? true,
    redisAddr: input.redis_addr,
    redisUsername: input.redis_username ?? "",
    redisPassword: "",
    redisDB: input.redis_db,
    redisTLS: input.redis_tls,
    isDefault: input.is_default,
    workspaceCount: input.workspace_count,
    activeSessionCount: input.active_session_count ?? 0,
    connectionError: input.connection_error,
    lastWorkspaceRefreshAt: input.last_workspace_refresh_at,
    lastWorkspaceRefreshError: input.last_workspace_refresh_error,
    lastSessionReconcileAt: input.last_session_reconcile_at,
    lastSessionReconcileError: input.last_session_reconcile_error,
    afsTotalBytes: input.afs_total_bytes ?? 0,
    afsFileCount: input.afs_file_count ?? 0,
    stats: input.stats
      ? {
          usedMemoryBytes: input.stats.used_memory_bytes ?? 0,
          maxMemoryBytes: input.stats.max_memory_bytes ?? 0,
          fragmentationRatio: input.stats.fragmentation_ratio ?? 0,
          keyCount: input.stats.key_count ?? 0,
          opsPerSec: input.stats.ops_per_sec ?? 0,
          cacheHitRate: input.stats.cache_hit_rate ?? 0,
          connectedClients: input.stats.connected_clients ?? 0,
          sampledAt: input.stats.sampled_at,
        }
      : undefined,
  };
}

function mapActivity(
  input: HTTPActivity,
  database?: { databaseId?: string; databaseName?: string },
): AFSActivityEvent {
  return {
    id: input.id,
    workspaceId: input.workspace_id,
    workspaceName: input.workspace_name,
    databaseId: input.database_id ?? database?.databaseId,
    databaseName: input.database_name ?? database?.databaseName,
    actor: input.actor,
    createdAt: input.created_at,
    detail: input.detail,
    kind: input.kind,
    path: input.path,
    scope: input.scope,
    title: input.title,
  };
}

function mapActivityList(
  response: HTTPActivityList,
  database?: { databaseId?: string; databaseName?: string },
): AFSActivityListResponse {
  return {
    items: response.items.map((item) => mapActivity(item, database)),
    nextCursor: response.next_cursor,
  };
}

function mapEventEntry(
  input: HTTPEventEntry,
  scope?: {
    workspaceId?: string;
    workspaceName?: string;
    databaseId?: string;
    databaseName?: string;
  },
): AFSEventEntry {
  return {
    id: input.id,
    workspaceId: input.workspace_id ?? scope?.workspaceId,
    workspaceName: input.workspace_name ?? scope?.workspaceName,
    databaseId: input.database_id ?? scope?.databaseId,
    databaseName: input.database_name ?? scope?.databaseName,
    createdAt: input.created_at,
    kind: input.kind,
    op: input.op,
    source: input.source,
    actor: input.actor,
    sessionId: input.session_id,
    user: input.user,
    label: input.label,
    agentVersion: input.agent_version,
    hostname: input.hostname,
    path: input.path,
    prevPath: input.prev_path,
    sizeBytes: input.size_bytes,
    deltaBytes: input.delta_bytes,
    contentHash: input.content_hash,
    prevHash: input.prev_hash,
    mode: input.mode,
    checkpointId: input.checkpoint_id,
    extras: input.extras,
  };
}

function mapEventList(
  response: HTTPEventList,
  scope?: {
    workspaceId?: string;
    workspaceName?: string;
    databaseId?: string;
    databaseName?: string;
  },
): AFSEventListResponse {
  return {
    items: response.items.map((item) => mapEventEntry(item, scope)),
    nextCursor: response.next_cursor,
  };
}

function mapCheckpoint(input: HTTPCheckpoint): AFSSavepoint {
  return {
    id: input.id,
    name: input.name,
    author: input.author ?? "afs",
    createdAt: input.created_at,
    note: input.note ?? "",
    fileCount: input.file_count,
    folderCount: input.folder_count,
    totalBytes: input.total_bytes,
    sizeLabel: bytesLabelForValue(input.total_bytes),
    filesSnapshot: [],
    isHead: input.is_head,
  };
}

function mapChangelogEntry(
  input: HTTPChangelogEntry,
  scope?: {
    workspaceId?: string;
    workspaceName?: string;
    databaseId?: string;
    databaseName?: string;
  },
): AFSChangelogEntry {
  return {
    id: input.id,
    occurredAt: input.occurred_at,
    workspaceId: input.workspace_id ?? scope?.workspaceId,
    workspaceName: input.workspace_name ?? scope?.workspaceName,
    databaseId: input.database_id ?? scope?.databaseId,
    databaseName: input.database_name ?? scope?.databaseName,
    sessionId: input.session_id,
    agentId: input.agent_id,
    user: input.user,
    label: input.label,
    agentVersion: input.agent_version,
    op: input.op,
    path: input.path,
    prevPath: input.prev_path,
    sizeBytes: input.size_bytes,
    deltaBytes: input.delta_bytes,
    contentHash: input.content_hash,
    prevHash: input.prev_hash,
    mode: input.mode,
    checkpointId: input.checkpoint_id,
    source: input.source,
    fileId: input.file_id,
    versionId: input.version_id,
  };
}

function changelogSearchParams(input: ListChangelogInput): URLSearchParams {
  const params = new URLSearchParams();
  if (input.limit != null && input.limit > 0) {
    params.set("limit", String(input.limit));
  }
  if (input.sessionId) {
    params.set("session_id", input.sessionId);
  }
  if (input.path) {
    params.set("path", input.path);
  }
  if (input.since) {
    params.set("since", input.since);
  }
  if (input.until) {
    params.set("until", input.until);
  }
  if (input.direction) {
    params.set("direction", input.direction);
  }
  return params;
}

function eventSearchParams(input: ListEventsInput): URLSearchParams {
  const params = new URLSearchParams();
  if (input.limit != null && input.limit > 0) {
    params.set("limit", String(input.limit));
  }
  if (input.kind) {
    params.set("kind", input.kind);
  }
  if (input.sessionId) {
    params.set("session_id", input.sessionId);
  }
  if (input.path) {
    params.set("path", input.path);
  }
  if (input.since) {
    params.set("since", input.since);
  }
  if (input.until) {
    params.set("until", input.until);
  }
  if (input.direction) {
    params.set("direction", input.direction);
  }
  return params;
}

function mapAgentSession(
  input: HTTPWorkspaceSessionInfo,
  workspaceId: string,
  workspaceName: string,
  databaseId?: string,
  databaseName?: string,
): AFSAgentSession {
  return {
    sessionId: input.session_id,
    workspaceId: input.workspace_id ?? workspaceId,
    workspaceName: input.workspace_name ?? workspaceName,
    databaseId: input.database_id ?? databaseId,
    databaseName: input.database_name ?? databaseName,
    agentId: input.agent_id,
    clientKind: input.client_kind ?? "",
    afsVersion: input.afs_version ?? "",
    hostname: input.hostname ?? "",
    operatingSystem: input.os ?? "",
    localPath: input.local_path ?? "",
    label: input.label,
    readonly: input.readonly ?? false,
    state: input.state,
    startedAt: input.started_at,
    lastSeenAt: input.last_seen_at,
    leaseExpiresAt: input.lease_expires_at,
  };
}

function mapAdminOverview(input: HTTPAdminOverview): AFSAdminOverview {
  return {
    userCount: input.user_count,
    databaseCount: input.database_count,
    workspaceCount: input.workspace_count,
    agentCount: input.agent_count,
    activeAgentCount: input.active_agent_count,
    staleAgentCount: input.stale_agent_count,
    unavailableDatabaseCount: input.unavailable_database_count,
    totalBytes: input.total_bytes,
    fileCount: input.file_count,
  };
}

function mapAdminUser(input: HTTPAdminUser): AFSAdminUser {
  return {
    subject: input.subject,
    label: input.label,
    databaseCount: input.database_count,
    workspaceCount: input.workspace_count,
    mcpTokenCount: input.mcp_token_count,
    agentSessionCount: input.agent_session_count,
    lastSeenAt: input.last_seen_at,
    sources: input.sources ?? [],
  };
}

function mapWorkspaceSummary(input: HTTPWorkspaceSummary): AFSWorkspaceSummary {
  return {
    id: input.id,
    name: input.name,
    cloudAccount: input.cloud_account,
    databaseId: input.database_id,
    databaseName: input.database_name,
    ownerSubject: input.owner_subject,
    ownerLabel: input.owner_label,
    databaseManagementType: input.database_management_type,
    databaseCanEdit: input.database_can_edit ?? true,
    databaseCanDelete: input.database_can_delete ?? true,
    redisKey: input.redis_key,
    fileCount: input.file_count,
    folderCount: input.folder_count,
    totalBytes: input.total_bytes,
    checkpointCount: input.checkpoint_count,
    lastCheckpointAt: input.last_checkpoint_at,
    updatedAt: input.updated_at,
    region: input.region,
    source: input.source,
    templateSlug: input.template_slug,
  };
}

function mapWorkspaceDetail(input: HTTPWorkspaceDetail): AFSWorkspaceDetail {
  return {
    id: input.id,
    name: input.name,
    description: input.description ?? "",
    cloudAccount: input.cloud_account,
    databaseId: input.database_id,
    databaseName: input.database_name,
    ownerSubject: input.owner_subject,
    ownerLabel: input.owner_label,
    databaseManagementType: input.database_management_type,
    databaseCanEdit: input.database_can_edit ?? true,
    databaseCanDelete: input.database_can_delete ?? true,
    redisKey: input.redis_key,
    region: input.region,
    source: input.source,
    templateSlug: input.template_slug,
    createdAt: input.created_at,
    updatedAt: input.updated_at,
    draftState: input.draft_state,
    headSavepointId: input.head_checkpoint_id,
    tags: input.tags ?? [],
    fileCount: input.file_count,
    folderCount: input.folder_count,
    totalBytes: input.total_bytes,
    checkpointCount: input.checkpoint_count,
    files: [],
    savepoints: input.checkpoints.map(mapCheckpoint),
    activity: input.activity.map((item) =>
      mapActivity(item, {
        databaseId: input.database_id,
        databaseName: input.database_name,
      }),
    ),
    agents: [],
    capabilities: mapCapabilities(input.capabilities),
  };
}

function mapTreeResponse(input: HTTPTreeResponse): AFSTreeResponse {
  return {
    workspaceId: input.workspace_id,
    view: input.view,
    path: input.path,
    items: input.items.map((item) => ({
      path: item.path,
      name: item.name,
      kind: item.kind,
      size: item.size,
      modifiedAt: item.modified_at,
      target: item.target,
    })),
  };
}

function mapFileContent(input: HTTPFileContent): AFSFileContent {
  return {
    workspaceId: input.workspace_id,
    view: input.view,
    path: input.path,
    kind: input.kind,
    revision: input.revision,
    language: input.language,
    encoding: input.encoding,
    contentType: input.content_type,
    size: input.size,
    modifiedAt: input.modified_at,
    binary: input.binary,
    content: input.content,
    target: input.target,
  };
}

function mapWorkspaceDiff(input: HTTPWorkspaceDiffResponse): AFSWorkspaceDiffResponse {
  return {
    workspaceId: input.workspace_id,
    workspaceName: input.workspace_name,
    base: {
      view: input.base.view,
      checkpointId: input.base.checkpoint_id,
      manifestHash: input.base.manifest_hash,
      fileCount: input.base.file_count,
      folderCount: input.base.folder_count,
      totalBytes: input.base.total_bytes,
    },
    head: {
      view: input.head.view,
      checkpointId: input.head.checkpoint_id,
      manifestHash: input.head.manifest_hash,
      fileCount: input.head.file_count,
      folderCount: input.head.folder_count,
      totalBytes: input.head.total_bytes,
    },
    summary: {
      total: input.summary.total,
      created: input.summary.created,
      updated: input.summary.updated,
      deleted: input.summary.deleted,
      renamed: input.summary.renamed,
      metadataChanged: input.summary.metadata_changed,
      bytesAdded: input.summary.bytes_added,
      bytesRemoved: input.summary.bytes_removed,
    },
    entries: input.entries.map((entry) => ({
      op: entry.op,
      path: entry.path,
      previousPath: entry.previous_path,
      kind: entry.kind,
      previousKind: entry.previous_kind,
      sizeBytes: entry.size_bytes,
      previousSizeBytes: entry.previous_size_bytes,
      deltaBytes: entry.delta_bytes,
      textDiff: entry.text_diff == null
        ? undefined
        : {
            language: entry.text_diff.language,
            previousExists: entry.text_diff.previous_exists,
            nextExists: entry.text_diff.next_exists,
            hunks: entry.text_diff.hunks?.map((hunk) => ({
              oldStart: hunk.old_start,
              oldLines: hunk.old_lines,
              newStart: hunk.new_start,
              newLines: hunk.new_lines,
              lines: hunk.lines.map((line) => ({
                kind: line.kind,
                oldLine: line.old_line,
                newLine: line.new_line,
                text: line.text,
              })),
            })),
          },
    })),
  };
}

function mapWorkspaceVersioningPolicy(input: HTTPWorkspaceVersioningPolicy): AFSWorkspaceVersioningPolicy {
  return {
    mode: input.mode ?? "off",
    includeGlobs: input.include_globs ?? [],
    excludeGlobs: input.exclude_globs ?? [],
    maxVersionsPerFile: input.max_versions_per_file ?? 0,
    maxAgeDays: input.max_age_days ?? 0,
    maxTotalBytes: input.max_total_bytes ?? 0,
    largeFileCutoffBytes: input.large_file_cutoff_bytes ?? 0,
  };
}

function mapFileVersion(input: HTTPFileVersion) {
  return {
    versionId: input.version_id,
    fileId: input.file_id,
    ordinal: input.ordinal,
    path: input.path,
    prevPath: input.prev_path,
    op: input.op,
    kind: input.kind,
    blobId: input.blob_id,
    contentHash: input.content_hash,
    prevHash: input.prev_hash,
    sizeBytes: input.size_bytes,
    deltaBytes: input.delta_bytes,
    mode: input.mode,
    target: input.target,
    source: input.source,
    sessionId: input.session_id,
    agentId: input.agent_id,
    user: input.user,
    checkpointIds: input.checkpoint_ids ?? [],
    createdAt: input.created_at,
  };
}

function mapFileHistoryResponse(input: HTTPFileHistoryResponse): AFSFileHistoryResponse {
  return {
    workspaceId: input.workspace_id,
    path: input.path,
    order: input.order,
    lineages: input.lineages.map((lineage) => ({
      fileId: lineage.file_id,
      state: lineage.state,
      currentPath: lineage.current_path,
      versions: lineage.versions.map(mapFileVersion),
    })),
    nextCursor: input.next_cursor,
  };
}

function mapFileVersionContent(input: HTTPFileVersionContent): AFSFileVersionContent {
  return {
    workspaceId: input.workspace_id,
    fileId: input.file_id,
    versionId: input.version_id,
    ordinal: input.ordinal,
    path: input.path,
    kind: input.kind,
    source: input.source,
    content: input.content,
    target: input.target,
    binary: input.binary,
    encoding: input.encoding,
    contentType: input.content_type,
    language: input.language,
    size: input.size,
    createdAt: input.created_at,
  };
}

function mapFileVersionSelector(input: DiffFileVersionsInput["from"]): HTTPFileVersionSelector {
  if ("ref" in input) {
    return { ref: input.ref };
  }
  if ("versionId" in input) {
    return { version_id: input.versionId };
  }
  return { file_id: input.fileId, ordinal: input.ordinal };
}

function mapFileVersionDiff(input: HTTPFileVersionDiff): AFSFileVersionDiff {
  return {
    path: input.path,
    from: input.from,
    to: input.to,
    binary: input.binary,
    diff: input.diff,
  };
}

function mapFileVersionRestoreResponse(input: HTTPFileVersionRestoreResponse): AFSFileVersionRestoreResponse {
  return {
    workspaceId: input.workspace_id,
    path: input.path,
    fileId: input.file_id ?? "",
    versionId: input.version_id ?? "",
    restoredFromVersionId: input.restored_from_version_id ?? "",
    restoredFromFileId: input.restored_from_file_id ?? "",
    restoredFromOrdinal: input.restored_from_ordinal ?? 0,
  };
}

function mapFileVersionUndeleteResponse(input: HTTPFileVersionUndeleteResponse): AFSFileVersionUndeleteResponse {
  return {
    workspaceId: input.workspace_id,
    path: input.path,
    fileId: input.file_id ?? "",
    versionId: input.version_id ?? "",
    undeletedFromVersionId: input.undeleted_from_version_id ?? "",
    undeletedFromFileId: input.undeleted_from_file_id ?? "",
    undeletedFromOrdinal: input.undeleted_from_ordinal ?? 0,
  };
}

function mapAccount(input: HTTPAccount): AFSAccount {
  return {
    subject: input.subject,
    provider: input.provider,
    canDeleteIdentity: input.can_delete_identity,
    canResetData: input.can_reset_data,
    ownedDatabaseCount: input.owned_database_count,
    ownedWorkspaceCount: input.owned_workspace_count,
    deletedDatabaseCount: input.deleted_database_count,
    deletedWorkspaceCount: input.deleted_workspace_count,
    identityDeleted: input.identity_deleted,
  };
}

const httpAFSClient: AFSClient = {
  mode: "http",

  async listDatabases() {
    const response = await requestJSON<AFSDatabaseListResponse & { items: HTTPDatabase[] }>("/databases");
    return response.items.map(mapDatabase);
  },

  async reconcileCatalog() {
    await requestJSON<void>("/catalog/reconcile", {
      method: "POST",
    });
  },

  async saveDatabase(input: SaveDatabaseInput) {
    return mapDatabase(
      await requestJSON<HTTPDatabase>(input.id ? `/databases/${input.id}` : "/databases", {
        method: input.id ? "PUT" : "POST",
        body: JSON.stringify({
          name: input.name,
          description: input.description,
          redis_addr: input.redisAddr,
          redis_username: input.redisUsername,
          redis_password: input.redisPassword,
          redis_db: input.redisDB,
          redis_tls: input.redisTLS,
        }),
      }),
    );
  },

  async setDefaultDatabase(databaseId: string) {
    return mapDatabase(
      await requestJSON<HTTPDatabase>(`/databases/${databaseId}/default`, {
        method: "POST",
      }),
    );
  },

  async deleteDatabase(databaseId: string) {
    await requestJSON<void>(`/databases/${databaseId}`, {
      method: "DELETE",
    });
  },

  async listWorkspaceSummaries(databaseId = "") {
    const response = await requestJSON<{
      items: HTTPWorkspaceSummary[];
    }>(databaseId === "" ? "/workspaces" : `/databases/${databaseId}/workspaces`);
    return response.items.map(mapWorkspaceSummary);
  },

  async getWorkspace(databaseId = "", workspaceId: string) {
    try {
      const basePath = workspaceBasePath(databaseId, workspaceId);
      const [detailResult, sessionsResult] = await Promise.allSettled([
        requestJSON<HTTPWorkspaceDetail>(basePath),
        requestJSON<HTTPWorkspaceSessionList>(`${basePath}/sessions`),
      ]);
      if (detailResult.status !== "fulfilled") {
        throw detailResult.reason;
      }
      const detail = detailResult.value;
      const sessions = sessionsResult.status === "fulfilled" ? sessionsResult.value : { items: [] };
      return {
        ...mapWorkspaceDetail(detail),
        agents: sessions.items.map((item) =>
          mapAgentSession(item, workspaceId, detail.name, detail.database_id, detail.database_name),
        ),
      };
    } catch (error) {
      if (error instanceof HTTPError && error.status === 404) {
        return null;
      }
      throw error;
    }
  },

  async listAgents(databaseId = "") {
    const response = await requestJSON<HTTPWorkspaceSessionList>(
      databaseId === "" ? "/agents" : `/databases/${databaseId}/agents`,
    );

    return response.items
      .map((item) =>
        mapAgentSession(
          item,
          item.workspace_id ?? item.workspace,
          item.workspace_name ?? item.workspace,
          item.database_id ?? databaseId,
          item.database_name,
        ),
      )
      .sort((left, right) => right.lastSeenAt.localeCompare(left.lastSeenAt));
  },

  async createWorkspace(input: CreateWorkspaceInput) {
    return mapWorkspaceDetail(
      await requestJSON<HTTPWorkspaceDetail>(input.databaseId?.trim() ? `/databases/${input.databaseId}/workspaces` : "/workspaces", {
        method: "POST",
        body: JSON.stringify({
          name: input.name,
          description: input.description,
          database_id: input.databaseId,
          database_name: input.databaseName,
          cloud_account: input.cloudAccount,
          region: input.region,
          source: {
            kind: input.source,
          },
          template_slug: input.templateSlug,
        }),
      }),
    );
  },

  async deleteWorkspace(databaseId: string, workspaceId: string) {
    await requestJSON<void>(workspaceBasePath(databaseId, workspaceId), {
      method: "DELETE",
    });
  },

  async updateWorkspace(input: UpdateWorkspaceInput) {
    return mapWorkspaceDetail(
      await requestJSON<HTTPWorkspaceDetail>(workspaceBasePath(input.databaseId, input.workspaceId), {
        method: "PUT",
        body: JSON.stringify({
          name: input.name,
          description: input.description,
          database_name: input.databaseName,
          cloud_account: input.cloudAccount,
          region: input.region,
        }),
      }),
    );
  },

  async updateWorkspaceFile() {
    throw new Error("Working-copy editing is not available in the hosted HTTP control plane yet.");
  },

  async getWorkspaceVersioningPolicy(input: GetWorkspaceVersioningPolicyInput) {
    return mapWorkspaceVersioningPolicy(
      await requestJSON<HTTPWorkspaceVersioningPolicy>(`${workspaceBasePath(input.databaseId, input.workspaceId)}/versioning`),
    );
  },

  async updateWorkspaceVersioningPolicy(input: UpdateWorkspaceVersioningPolicyInput) {
    return mapWorkspaceVersioningPolicy(
      await requestJSON<HTTPWorkspaceVersioningPolicy>(`${workspaceBasePath(input.databaseId, input.workspaceId)}/versioning`, {
        method: "PUT",
        body: JSON.stringify({
          mode: input.policy.mode,
          include_globs: input.policy.includeGlobs,
          exclude_globs: input.policy.excludeGlobs,
          max_versions_per_file: input.policy.maxVersionsPerFile,
          max_age_days: input.policy.maxAgeDays,
          max_total_bytes: input.policy.maxTotalBytes,
          large_file_cutoff_bytes: input.policy.largeFileCutoffBytes,
        }),
      }),
    );
  },

  async getFileHistory(input: GetFileHistoryInput) {
    const params = new URLSearchParams();
    params.set("path", input.path);
    if (input.direction) {
      params.set("direction", input.direction);
    }
    if (input.limit != null && input.limit > 0) {
      params.set("limit", String(input.limit));
    }
    if (input.cursor) {
      params.set("cursor", input.cursor);
    }
    return mapFileHistoryResponse(
      await requestJSON<HTTPFileHistoryResponse>(`${workspaceBasePath(input.databaseId, input.workspaceId)}/files/history?${params.toString()}`),
    );
  },

  async getFileVersionContent(input: GetFileVersionContentInput) {
    const params = new URLSearchParams();
    params.set("path", input.path);
    if ("versionId" in input) {
      params.set("version_id", input.versionId);
    } else {
      params.set("file_id", input.fileId);
      params.set("ordinal", String(input.ordinal));
    }
    try {
      return mapFileVersionContent(
        await requestJSON<HTTPFileVersionContent>(`${workspaceBasePath(input.databaseId, input.workspaceId)}/files/version-content?${params.toString()}`),
      );
    } catch (error) {
      if (error instanceof HTTPError && error.status === 404) {
        return null;
      }
      throw error;
    }
  },

  async diffFileVersions(input: DiffFileVersionsInput) {
    return mapFileVersionDiff(
      await requestJSON<HTTPFileVersionDiff>(`${workspaceBasePath(input.databaseId, input.workspaceId)}/files/diff`, {
        method: "POST",
        body: JSON.stringify({
          path: input.path,
          from: mapFileVersionSelector(input.from),
          to: input.to ? mapFileVersionSelector(input.to) : { ref: "head" },
        }),
      }),
    );
  },

  async restoreFileVersion(input: RestoreFileVersionInput) {
    const body: Record<string, unknown> = { path: input.path };
    if ("versionId" in input) {
      body.version_id = input.versionId;
    } else {
      body.file_id = input.fileId;
      body.ordinal = input.ordinal;
    }
    return mapFileVersionRestoreResponse(
      await requestJSON<HTTPFileVersionRestoreResponse>(`${workspaceBasePath(input.databaseId, input.workspaceId)}:restore-version`, {
        method: "POST",
        body: JSON.stringify(body),
      }),
    );
  },

  async undeleteFileVersion(input: UndeleteFileVersionInput) {
    const body: Record<string, unknown> = { path: input.path };
    if ("versionId" in input && input.versionId) {
      body.version_id = input.versionId;
    } else if ("fileId" in input && input.fileId) {
      body.file_id = input.fileId;
      body.ordinal = input.ordinal;
    }
    return mapFileVersionUndeleteResponse(
      await requestJSON<HTTPFileVersionUndeleteResponse>(`${workspaceBasePath(input.databaseId, input.workspaceId)}:undelete`, {
        method: "POST",
        body: JSON.stringify(body),
      }),
    );
  },

  async createSavepoint() {
    throw new Error("Checkpoint creation requires a connected working copy and is not available in the hosted HTTP control plane yet.");
  },

  async restoreSavepoint(input: RestoreSavepointInput) {
    await requestJSON<void>(`${workspaceBasePath(input.databaseId, input.workspaceId)}:restore`, {
      method: "POST",
      body: JSON.stringify({
        checkpoint_id: input.savepointId,
      }),
    });

    return httpAFSClient.getWorkspace(input.databaseId ?? "", input.workspaceId);
  },

  async listActivity(databaseId = "", limit = 50) {
    const response = await httpAFSClient.listActivityPage({ databaseId, limit });
    return response.items;
  },

  async listActivityPage(input: ListActivityInput) {
    const params = new URLSearchParams();
    if (input.limit != null && input.limit > 0) {
      params.set("limit", String(input.limit));
    }
    if (input.until) {
      params.set("until", input.until);
    }
    const query = params.toString();
    const databaseId = input.databaseId?.trim() ?? "";
    const workspaceId = input.workspaceId?.trim() ?? "";
    if (workspaceId !== "") {
      const base = `${workspaceBasePath(input.databaseId, workspaceId)}/activity`;
      const response = await requestJSON<HTTPActivityList>(query ? `${base}?${query}` : base);
      return mapActivityList(response, {
        databaseId: input.databaseId,
      });
    }
    if (databaseId !== "") {
      const database = (await httpAFSClient.listDatabases()).find((item) => item.id === databaseId);
      const response = await requestJSON<HTTPActivityList>(
        query ? `/databases/${databaseId}/activity?${query}` : `/databases/${databaseId}/activity`,
      );
      return mapActivityList(response, {
        databaseId,
        databaseName: database?.name,
      });
    }

    const response = await requestJSON<HTTPActivityList>(query ? `/activity?${query}` : "/activity");
    return mapActivityList(response);
  },

  async listEvents(input: ListEventsInput): Promise<AFSEventListResponse> {
    const params = eventSearchParams(input);
    const query = params.toString();
    const workspaceId = input.workspaceId?.trim() ?? "";
    if (workspaceId !== "") {
      const base = `${workspaceBasePath(input.databaseId, workspaceId)}/events`;
      const response = await requestJSON<HTTPEventList>(query ? `${base}?${query}` : base);
      return mapEventList(response, {
        workspaceId,
        databaseId: input.databaseId,
      });
    }

    const databaseId = input.databaseId?.trim() ?? "";
    if (databaseId !== "") {
      const database = (await httpAFSClient.listDatabases()).find((item) => item.id === databaseId);
      const base = `/databases/${databaseId}/events`;
      const response = await requestJSON<HTTPEventList>(query ? `${base}?${query}` : base);
      return mapEventList(response, {
        databaseId,
        databaseName: database?.name,
      });
    }

    const response = await requestJSON<HTTPEventList>(query ? `/events?${query}` : "/events");
    return mapEventList(response);
  },

  async listChangelog(input: ListChangelogInput): Promise<AFSChangelogResponse> {
    const workspaceId = input.workspaceId?.trim() ?? "";
    if (workspaceId === "") {
      const params = changelogSearchParams(input);
      const query = params.toString();
      const databaseId = input.databaseId?.trim() ?? "";
      const base = databaseId === "" ? "/changes" : `/databases/${databaseId}/changes`;
      const response = await requestJSON<HTTPChangelogResponse>(query ? `${base}?${query}` : base);
      return {
        entries: response.entries.map((entry) => mapChangelogEntry(entry)),
        nextCursor: response.next_cursor,
      };
    }

    const params = changelogSearchParams(input);
    const query = params.toString();
    const base = `${workspaceBasePath(input.databaseId, workspaceId)}/changes`;
    const response = await requestJSON<HTTPChangelogResponse>(
      query ? `${base}?${query}` : base,
    );
    return {
      entries: response.entries.map((entry) =>
        mapChangelogEntry(entry, {
          workspaceId,
          databaseId: input.databaseId,
        }),
      ),
      nextCursor: response.next_cursor,
    };
  },

  async getWorkspaceTree(input: GetWorkspaceTreeInput) {
    return mapTreeResponse(
      await requestJSON<HTTPTreeResponse>(
        `${workspaceBasePath(input.databaseId, input.workspaceId)}/tree?view=${encodeURIComponent(input.view)}&path=${encodeURIComponent(input.path)}&depth=${input.depth ?? 1}`,
      ),
    );
  },

  async getWorkspaceFileContent(input: GetWorkspaceFileContentInput) {
    try {
      return mapFileContent(
        await requestJSON<HTTPFileContent>(
          `${workspaceBasePath(input.databaseId, input.workspaceId)}/files/content?view=${encodeURIComponent(input.view)}&path=${encodeURIComponent(input.path)}`,
        ),
      );
    } catch (error) {
      if (error instanceof HTTPError && error.status === 404) {
        return null;
      }
      throw error;
    }
  },

  async getWorkspaceDiff(input: GetWorkspaceDiffInput) {
    const params = new URLSearchParams({
      base: input.base,
      head: input.head,
    });
    return mapWorkspaceDiff(
      await requestJSON<HTTPWorkspaceDiffResponse>(
        `${workspaceBasePath(input.databaseId, input.workspaceId)}/diff?${params.toString()}`,
      ),
    );
  },

  async quickstart(input: QuickstartInput) {
    const response = await requestJSON<{
      database_id: string;
      workspace_id: string;
      workspace: HTTPWorkspaceDetail;
    }>("/quickstart", {
      method: "POST",
      body: JSON.stringify({
        redis_addr: input.redisAddr,
        redis_password: input.redisPassword,
        redis_username: input.redisUsername,
        redis_db: input.redisDB,
        redis_tls: input.redisTLS,
      }),
    });
    return {
      databaseId: response.database_id,
      workspaceId: response.workspace_id,
      workspace: mapWorkspaceDetail(response.workspace),
    } as QuickstartResponse;
  },

  async createOnboardingToken(databaseId: string | undefined, workspaceId: string) {
    const response = await requestJSON<HTTPOnboardingTokenResponse>(
      `${workspaceBasePath(databaseId, workspaceId)}/onboarding-token`,
      { method: "POST" },
    );
    return {
      token: response.token,
      databaseId: response.database_id,
      workspaceId: response.workspace_id,
      workspaceName: response.workspace_name,
      expiresAt: response.expires_at,
    };
  },

  async listAllMCPAccessTokens() {
    const response = await requestJSON<{ items: HTTPMCPToken[] }>("/mcp-tokens");
    return response.items.map(mapHTTPMCPToken);
  },

  async listControlPlaneTokens() {
    const response = await requestJSON<{ items: HTTPMCPToken[] }>("/mcp-tokens?scope=control-plane");
    return response.items.map(mapHTTPMCPToken);
  },

  async createControlPlaneToken(input: CreateControlPlaneTokenInput) {
    const response = await requestJSON<HTTPMCPToken>("/mcp-tokens", {
      method: "POST",
      body: JSON.stringify({
        name: input.name,
        expires_at: input.expiresAt,
      }),
    });
    return mapHTTPMCPToken(response);
  },

  async revokeControlPlaneToken(tokenId: string) {
    await requestJSON<void>(`/mcp-tokens/${encodeURIComponent(tokenId)}`, {
      method: "DELETE",
    });
  },

  async listMCPAccessTokens(databaseId: string | undefined, workspaceId: string) {
    const response = await requestJSON<{ items: HTTPMCPToken[] }>(
      `${workspaceBasePath(databaseId, workspaceId)}/mcp-tokens`,
    );
    return response.items.map(mapHTTPMCPToken);
  },

  async createMCPAccessToken(input: CreateMCPTokenInput) {
    const response = await requestJSON<HTTPMCPToken>(
      `${workspaceBasePath(input.databaseId, input.workspaceId)}/mcp-tokens`,
      {
        method: "POST",
        body: JSON.stringify({
          name: input.name,
          profile: input.profile,
          expires_at: input.expiresAt,
          template_slug: input.templateSlug,
        }),
      },
    );
    return mapHTTPMCPToken(response, { profileFallback: input.profile });
  },

  async revokeMCPAccessToken(databaseId: string | undefined, workspaceId: string, tokenId: string) {
    await requestJSON<void>(`${workspaceBasePath(databaseId, workspaceId)}/mcp-tokens/${encodeURIComponent(tokenId)}`, {
      method: "DELETE",
    });
  },

  async importLocal(input: ImportLocalInput) {
    const response = await requestJSON<{
      workspace_id: string;
      workspace: HTTPWorkspaceDetail;
      file_count: number;
      dir_count: number;
      total_bytes: number;
    }>(input.databaseId?.trim() ? `/databases/${input.databaseId}/workspaces:import-local` : "/workspaces:import-local", {
      method: "POST",
      body: JSON.stringify({
        database_id: input.databaseId,
        name: input.name,
        path: input.path,
        description: input.description,
      }),
    });
    return {
      workspaceId: response.workspace_id,
      workspace: mapWorkspaceDetail(response.workspace),
      fileCount: response.file_count,
      dirCount: response.dir_count,
      totalBytes: response.total_bytes,
    } as ImportLocalResponse;
  },

  async getAuthConfig() {
    const response = await requestJSON<HTTPAuthConfig>("/auth/config");
    return {
      mode: response.mode,
      enabled: response.enabled,
      provider: response.provider,
      signInRequired: response.sign_in_required,
      authenticated: response.authenticated,
      productMode: response.product_mode === "cloud" ? "cloud" : "self-hosted",
      clerkPublishableKey: response.clerk_publishable_key,
      user: response.user == null ? undefined : {
        subject: response.user.subject,
        name: response.user.name,
        email: response.user.email,
        groups: response.user.groups ?? [],
      },
    } as AFSAuthConfig;
  },

  async getServerVersion() {
    const response = await requestJSON<{ version: string; commit?: string; build_date?: string }>("/version");
    return {
      version: response.version,
      commit: response.commit,
      buildDate: response.build_date,
    } satisfies AFSServerVersion;
  },

  async getAccount() {
    return mapAccount(await requestJSON<HTTPAccount>("/account"));
  },

  async resetAccountData() {
    return mapAccount(await requestJSON<HTTPAccount>("/account/developer/reset", {
      method: "POST",
    }));
  },

  async deleteAccount() {
    return mapAccount(await requestJSON<HTTPAccount>("/account", {
      method: "DELETE",
    }));
  },

  async getAdminOverview() {
    return mapAdminOverview(await requestJSON<HTTPAdminOverview>("/admin/overview"));
  },

  async listAdminUsers() {
    const response = await requestJSON<{ items: HTTPAdminUser[] }>("/admin/users");
    return response.items.map(mapAdminUser);
  },

  async listAdminDatabases() {
    const response = await requestJSON<{ items: HTTPDatabase[] }>("/admin/databases");
    return response.items.map(mapDatabase);
  },

  async listAdminWorkspaceSummaries() {
    const response = await requestJSON<{ items: HTTPWorkspaceSummary[] }>("/admin/workspaces");
    return response.items.map(mapWorkspaceSummary);
  },

  async listAdminAgents() {
    const response = await requestJSON<HTTPWorkspaceSessionList>("/admin/agents");
    return response.items.map((item) =>
      mapAgentSession(
        item,
        item.workspace_id ?? item.workspace,
        item.workspace_name ?? item.workspace,
        item.database_id,
        item.database_name,
      ),
    );
  },

  resetDemo() {
    return demoAFSClient.resetDemo();
  },
};

export const afsApi = resolveAFSClient();

export function getAFSClientMode() {
  return afsApi.mode;
}

/** Returns the control plane base URL (e.g. "http://localhost:8091"). */
export function getControlPlaneURL() {
  if (HTTP_BASE_URL) {
    return HTTP_BASE_URL;
  }
  // Cloud / same-origin deploy: API is mounted at this site's root.
  if (typeof window !== "undefined") {
    return window.location.origin;
  }
  return "";
}

export function getDemoAFSClientForTesting() {
  return demoAFSClient;
}

export function formatBytes(value: number) {
  if (value >= 1024 * 1024 * 1024) {
    return `${(value / (1024 * 1024 * 1024)).toFixed(1)} GB`;
  }

  if (value >= 1024 * 1024) {
    return `${(value / (1024 * 1024)).toFixed(1)} MB`;
  }

  return `${Math.max(1, Math.round(value / 1024))} KB`;
}
