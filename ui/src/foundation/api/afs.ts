import { cloneInitialAFSState } from "../mocks/afs";
import type {
  AFSDatabase,
  AFSDatabaseListResponse,
  AFSAgentSession,
  CreateSavepointInput,
  CreateWorkspaceInput,
  GetWorkspaceFileContentInput,
  GetWorkspaceTreeInput,
  AFSActivityEvent,
  AFSClientMode,
  AFSFile,
  AFSFileContent,
  AFSSavepoint,
  AFSState,
  AFSTreeItem,
  AFSTreeResponse,
  AFSWorkspace,
  AFSWorkspaceCapabilities,
  AFSWorkspaceDetail,
  AFSWorkspaceSource,
  AFSWorkspaceSummary,
  AFSWorkspaceView,
  RestoreSavepointInput,
  SaveDatabaseInput,
  UpdateWorkspaceInput,
  UpdateWorkspaceFileInput,
  QuickstartInput,
  QuickstartResponse,
  ImportLocalInput,
  ImportLocalResponse,
} from "../types/afs";

const STORAGE_KEY = "afs-ui-demo-state-v1";
const DATABASE_STORAGE_KEY = "afs-ui-demo-databases-v1";
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
  createSavepoint: (input: CreateSavepointInput) => Promise<AFSWorkspaceDetail | null>;
  restoreSavepoint: (input: RestoreSavepointInput) => Promise<AFSWorkspaceDetail | null>;
  listActivity: (databaseId?: string, limit?: number) => Promise<AFSActivityEvent[]>;
  getWorkspaceTree: (input: GetWorkspaceTreeInput) => Promise<AFSTreeResponse>;
  getWorkspaceFileContent: (input: GetWorkspaceFileContentInput) => Promise<AFSFileContent | null>;
  quickstart: (input: QuickstartInput) => Promise<QuickstartResponse>;
  importLocal: (input: ImportLocalInput) => Promise<ImportLocalResponse>;
  resetDemo: () => AFSState;
};

type HTTPDatabase = {
  id: string;
  name: string;
  description?: string;
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
};

type HTTPWorkspaceSummary = {
  id: string;
  name: string;
  cloud_account: string;
  database_id: string;
  database_name: string;
  redis_key: string;
  file_count: number;
  folder_count: number;
  total_bytes: number;
  checkpoint_count: number;
  last_checkpoint_at: string;
  updated_at: string;
  region: string;
  source: AFSWorkspaceSource;
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
  redis_key: string;
  region: string;
  source: AFSWorkspaceSource;
  created_at: string;
  updated_at: string;
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
};

type HTTPWorkspaceSessionInfo = {
  session_id: string;
  workspace: string;
  workspace_id?: string;
  workspace_name?: string;
  database_id?: string;
  database_name?: string;
  client_kind?: string;
  afs_version?: string;
  hostname?: string;
  os?: string;
  local_path?: string;
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
  normalized.activity = (workspace.activity ?? []).map((event) => ({
    ...event,
    workspaceId: event.workspaceId ?? workspace.id,
    workspaceName: event.workspaceName ?? workspace.name,
    databaseId: event.databaseId ?? workspace.databaseId,
    databaseName: event.databaseName ?? workspace.databaseName,
  }));
  normalized.agents = (workspace.agents ?? []).map((agent) => ({
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
): AFSActivityEvent {
  return {
    id: makeId("evt"),
    actor,
    createdAt: nowISO(),
    detail,
    kind,
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
      workspaceCount: existing?.workspaceCount ?? database.workspaceCount ?? 0,
      activeSessionCount: existing?.activeSessionCount ?? database.activeSessionCount ?? 0,
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

function activityForState(state: AFSState, limit: number) {
  return allActivityForState(state).slice(0, limit);
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
      workspace.description = input.description.trim();
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

  async quickstart() {
    throw new Error("Quickstart is not available in demo mode.");
  },

  async importLocal() {
    throw new Error("Import is not available in demo mode.");
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
    scope: input.scope,
    title: input.title,
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
    clientKind: input.client_kind ?? "",
    afsVersion: input.afs_version ?? "",
    hostname: input.hostname ?? "",
    operatingSystem: input.os ?? "",
    localPath: input.local_path ?? "",
    readonly: input.readonly ?? false,
    state: input.state,
    startedAt: input.started_at,
    lastSeenAt: input.last_seen_at,
    leaseExpiresAt: input.lease_expires_at,
  };
}

function mapWorkspaceSummary(input: HTTPWorkspaceSummary): AFSWorkspaceSummary {
  return {
    id: input.id,
    name: input.name,
    cloudAccount: input.cloud_account,
    databaseId: input.database_id,
    databaseName: input.database_name,
    redisKey: input.redis_key,
    fileCount: input.file_count,
    folderCount: input.folder_count,
    totalBytes: input.total_bytes,
    checkpointCount: input.checkpoint_count,
    lastCheckpointAt: input.last_checkpoint_at,
    updatedAt: input.updated_at,
    region: input.region,
    source: input.source,
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
    redisKey: input.redis_key,
    region: input.region,
    source: input.source,
    createdAt: input.created_at,
    updatedAt: input.updated_at,
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
    if (databaseId !== "") {
      const database = (await httpAFSClient.listDatabases()).find((item) => item.id === databaseId);
      const response = await requestJSON<HTTPActivityList>(`/databases/${databaseId}/activity?limit=${limit}`);
      return response.items.map((item) =>
        mapActivity(item, {
          databaseId,
          databaseName: database?.name,
        }),
      );
    }

    const response = await requestJSON<HTTPActivityList>(`/activity?limit=${limit}`);
    return response.items.map((item) => mapActivity(item));
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
  return HTTP_BASE_URL;
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
