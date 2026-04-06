import { cloneInitialAFSState } from "../mocks/afs";
import type {
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
  UpdateWorkspaceFileInput,
} from "../types/afs";

const STORAGE_KEY = "afs-ui-demo-state-v1";
const DEMO_DELAY_MS = 120;
const HTTP_BASE_URL = import.meta.env.VITE_AFS_API_BASE_URL?.replace(/\/+$/, "") ?? "";

type AFSClient = {
  mode: AFSClientMode;
  listWorkspaceSummaries: () => Promise<AFSWorkspaceSummary[]>;
  getWorkspace: (workspaceId: string) => Promise<AFSWorkspaceDetail | null>;
  createWorkspace: (input: CreateWorkspaceInput) => Promise<AFSWorkspaceDetail>;
  deleteWorkspace: (workspaceId: string) => Promise<void>;
  updateWorkspaceFile: (input: UpdateWorkspaceFileInput) => Promise<AFSWorkspaceDetail | null>;
  createSavepoint: (input: CreateSavepointInput) => Promise<AFSWorkspaceDetail | null>;
  restoreSavepoint: (input: RestoreSavepointInput) => Promise<AFSWorkspaceDetail | null>;
  listActivity: (limit?: number) => Promise<AFSActivityEvent[]>;
  getWorkspaceTree: (input: GetWorkspaceTreeInput) => Promise<AFSTreeResponse>;
  getWorkspaceFileContent: (input: GetWorkspaceFileContentInput) => Promise<AFSFileContent | null>;
  resetDemo: () => AFSState;
};

type HTTPWorkspaceSummary = {
  id: string;
  name: string;
  database_id: string;
  database_name: string;
  redis_key: string;
  status: AFSWorkspaceSummary["status"];
  file_count: number;
  folder_count: number;
  total_bytes: number;
  checkpoint_count: number;
  draft_state: AFSWorkspaceSummary["draftState"];
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
  status: AFSWorkspaceSummary["status"];
  source: AFSWorkspaceSource;
  created_at: string;
  updated_at: string;
  draft_state: AFSWorkspaceSummary["draftState"];
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
  normalized.activity = workspace.activity.map((event) => ({
    ...event,
    workspaceId: event.workspaceId ?? workspace.id,
    workspaceName: event.workspaceName ?? workspace.name,
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
    databaseId: normalized.databaseId,
    databaseName: normalized.databaseName,
    redisKey: normalized.redisKey,
    status: normalized.status,
    fileCount: normalized.fileCount,
    folderCount: normalized.folderCount,
    totalBytes: normalized.totalBytes,
    checkpointCount: normalized.checkpointCount,
    draftState: normalized.draftState,
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

function activityForState(state: AFSState, limit: number) {
  return state.workspaces
    .flatMap((workspace) =>
      normalizeWorkspace(workspace).activity.map((event) => ({
        ...event,
        workspaceId: event.workspaceId ?? workspace.id,
        workspaceName: event.workspaceName ?? workspace.name,
      })),
    )
    .sort((left, right) => right.createdAt.localeCompare(left.createdAt))
    .slice(0, limit);
}

const demoAFSClient: AFSClient = {
  mode: "demo",

  async listWorkspaceSummaries() {
    await wait();
    const state = loadState();
    return sortWorkspaces(state.workspaces.map(normalizeWorkspace)).map(workspaceToSummary);
  },

  async getWorkspace(workspaceId: string) {
    await wait();
    const state = loadState();
    const workspace = state.workspaces.find((item) => item.id === workspaceId);
    return workspace == null ? null : normalizeWorkspace(workspace);
  },

  async createWorkspace(input: CreateWorkspaceInput) {
    await wait();
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
        cloudAccount: input.cloudAccount.trim(),
        databaseId: `db-${id}`,
        databaseName: input.databaseName.trim(),
        redisKey: `afs:${id}`,
        region: input.region.trim(),
        mountedPath: `~/.afs/workspaces/${id}`,
        status: input.source === "blank" ? "healthy" : "syncing",
        source: input.source,
        createdAt,
        updatedAt: createdAt,
        draftState: "clean",
        headSavepointId: initialSavepoint.id,
        tags: [input.region.trim(), sourceLabel(input.source)],
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

  async deleteWorkspace(workspaceId: string) {
    await wait();
    updateState((draft) => {
      draft.workspaces = draft.workspaces.filter((workspace) => workspace.id !== workspaceId);
    });
  },

  async updateWorkspaceFile(input: UpdateWorkspaceFileInput) {
    await wait();
    const state = updateState((draft) => {
      const workspace = requireWorkspace(draft, input.workspaceId);
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

  async createSavepoint(input: CreateSavepointInput) {
    await wait();
    const state = updateState((draft) => {
      const workspace = requireWorkspace(draft, input.workspaceId);
      const savepoint = createSavepointRecord(
        input.name.trim(),
        input.note.trim(),
        "webui",
        workspace.files,
      );
      workspace.savepoints.unshift(savepoint);
      workspace.headSavepointId = savepoint.id;
      workspace.draftState = "clean";
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
      const savepoint = requireSavepoint(workspace, input.savepointId);
      workspace.files = clone(savepoint.filesSnapshot);
      workspace.headSavepointId = savepoint.id;
      workspace.draftState = "clean";
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

  async listActivity(limit = 50) {
    await wait();
    const state = loadState();
    return activityForState(state, limit);
  },

  async getWorkspaceTree(input: GetWorkspaceTreeInput) {
    await wait();
    const state = loadState();
    const workspace = normalizeWorkspace(requireWorkspace(state, input.workspaceId));
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

  resetDemo() {
    const seeded = cloneInitialAFSState();
    saveState(seeded);
    return seeded;
  },
};

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${HTTP_BASE_URL}/v1${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
  });
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

  return JSON.parse(rawBody) as T;
}

function hasHTTPBackend() {
  return HTTP_BASE_URL !== "";
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

function mapActivity(input: HTTPActivity): AFSActivityEvent {
  return {
    id: input.id,
    workspaceId: input.workspace_id,
    workspaceName: input.workspace_name,
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

function mapWorkspaceSummary(input: HTTPWorkspaceSummary): AFSWorkspaceSummary {
  return {
    id: input.id,
    name: input.name,
    databaseId: input.database_id,
    databaseName: input.database_name,
    redisKey: input.redis_key,
    status: input.status,
    fileCount: input.file_count,
    folderCount: input.folder_count,
    totalBytes: input.total_bytes,
    checkpointCount: input.checkpoint_count,
    draftState: input.draft_state,
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
    status: input.status,
    source: input.source,
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
    activity: input.activity.map(mapActivity),
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

  async listWorkspaceSummaries() {
    const response = await requestJSON<{
      items: HTTPWorkspaceSummary[];
    }>("/workspaces");
    return response.items.map(mapWorkspaceSummary);
  },

  async getWorkspace(workspaceId: string) {
    try {
      return mapWorkspaceDetail(await requestJSON<HTTPWorkspaceDetail>(`/workspaces/${workspaceId}`));
    } catch (error) {
      if (error instanceof HTTPError && error.status === 404) {
        return null;
      }
      throw error;
    }
  },

  async createWorkspace(input: CreateWorkspaceInput) {
    return mapWorkspaceDetail(
      await requestJSON<HTTPWorkspaceDetail>("/workspaces", {
        method: "POST",
        body: JSON.stringify({
          name: input.name,
          description: input.description,
          database_id: slugify(input.databaseName),
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

  async deleteWorkspace(workspaceId: string) {
    await requestJSON<void>(`/workspaces/${workspaceId}`, {
      method: "DELETE",
    });
  },

  async updateWorkspaceFile() {
    throw new Error("Working-copy editing is not available in the hosted HTTP control plane yet.");
  },

  async createSavepoint() {
    throw new Error("Checkpoint creation requires a connected working copy and is not available in the hosted HTTP control plane yet.");
  },

  async restoreSavepoint(input: RestoreSavepointInput) {
    await requestJSON<void>(`/workspaces/${input.workspaceId}:restore`, {
      method: "POST",
      body: JSON.stringify({
        checkpoint_id: input.savepointId,
      }),
    });

    return httpAFSClient.getWorkspace(input.workspaceId);
  },

  async listActivity(limit = 50) {
    const response = await requestJSON<HTTPActivityList>(`/activity?limit=${limit}`);
    return response.items.map(mapActivity);
  },

  async getWorkspaceTree(input: GetWorkspaceTreeInput) {
    return mapTreeResponse(
      await requestJSON<HTTPTreeResponse>(
        `/workspaces/${input.workspaceId}/tree?view=${encodeURIComponent(input.view)}&path=${encodeURIComponent(input.path)}&depth=${input.depth ?? 1}`,
      ),
    );
  },

  async getWorkspaceFileContent(input: GetWorkspaceFileContentInput) {
    try {
      return mapFileContent(
        await requestJSON<HTTPFileContent>(
          `/workspaces/${input.workspaceId}/files/content?view=${encodeURIComponent(input.view)}&path=${encodeURIComponent(input.path)}`,
        ),
      );
    } catch (error) {
      if (error instanceof HTTPError && error.status === 404) {
        return null;
      }
      throw error;
    }
  },

  resetDemo() {
    return demoAFSClient.resetDemo();
  },
};

export const afsApi = hasHTTPBackend() ? httpAFSClient : demoAFSClient;

export function getAFSClientMode() {
  return afsApi.mode;
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
