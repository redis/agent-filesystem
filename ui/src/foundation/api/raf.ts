import { cloneInitialRAFState } from "../mocks/raf";
import type {
  CreateSavepointInput,
  CreateWorkspaceInput,
  RAFActivityEvent,
  RAFClientMode,
  RAFFile,
  RAFSavepoint,
  RAFState,
  RAFWorkspace,
  RAFWorkspaceDetail,
  RAFWorkspaceListResponse,
  RAFWorkspaceSummary,
  RestoreSavepointInput,
  UpdateWorkspaceFileInput,
} from "../types/raf";

const STORAGE_KEY = "afs-ui-demo-state-v1";
const DEMO_DELAY_MS = 120;
const HTTP_BASE_URL = import.meta.env.VITE_RAF_API_BASE_URL?.replace(/\/+$/, "") ?? "";

type RAFClient = {
  mode: RAFClientMode;
  listWorkspaceSummaries: () => Promise<RAFWorkspaceSummary[]>;
  listWorkspaces: () => Promise<RAFWorkspaceDetail[]>;
  getWorkspace: (workspaceId: string) => Promise<RAFWorkspaceDetail | null>;
  createWorkspace: (input: CreateWorkspaceInput) => Promise<RAFWorkspaceDetail>;
  deleteWorkspace: (workspaceId: string) => Promise<void>;
  updateWorkspaceFile: (input: UpdateWorkspaceFileInput) => Promise<RAFWorkspaceDetail | null>;
  createSavepoint: (input: CreateSavepointInput) => Promise<RAFWorkspaceDetail | null>;
  restoreSavepoint: (input: RestoreSavepointInput) => Promise<RAFWorkspaceDetail | null>;
  resetDemo: () => RAFState;
};

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

function bytesCount(files: RAFFile[]) {
  return files.reduce(
    (sum, file) => sum + new TextEncoder().encode(file.content).length,
    0,
  );
}

function bytesLabel(files: RAFFile[]) {
  const totalBytes = bytesCount(files);

  if (totalBytes > 1024 * 1024) {
    return `${(totalBytes / (1024 * 1024)).toFixed(1)} MB`;
  }

  return `${Math.max(1, Math.round(totalBytes / 1024))} KB`;
}

function folderCount(files: RAFFile[]) {
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

function lastCheckpointAt(workspace: RAFWorkspace) {
  const values = workspace.savepoints.map((savepoint) => savepoint.createdAt);
  return values.sort((left, right) => right.localeCompare(left))[0] ?? workspace.updatedAt;
}

function createActivity(
  title: string,
  detail: string,
  actor: string,
  kind: string,
  scope: string,
): RAFActivityEvent {
  return {
    id: makeId("evt"),
    actor,
    createdAt: nowISO(),
    detail,
    kind,
    scope,
    title,
  };
}

function createSavepointRecord(
  name: string,
  note: string,
  author: string,
  files: RAFFile[],
): RAFSavepoint {
  return {
    id: makeId("sp"),
    name,
    author,
    createdAt: nowISO(),
    note,
    fileCount: files.length,
    sizeLabel: bytesLabel(files),
    filesSnapshot: clone(files),
  };
}

function sourceLabel(source: RAFWorkspace["source"]) {
  if (source === "git-import") return "Git import";
  if (source === "cloud-import") return "Redis Cloud import";
  return "Blank workspace";
}

function workspaceToSummary(workspace: RAFWorkspace): RAFWorkspaceSummary {
  return {
    id: workspace.id,
    name: workspace.name,
    databaseId: workspace.databaseId,
    databaseName: workspace.databaseName,
    redisKey: workspace.redisKey,
    status: workspace.status,
    fileCount: workspace.files.length,
    folderCount: folderCount(workspace.files),
    totalBytes: bytesCount(workspace.files),
    checkpointCount: workspace.savepoints.length,
    draftState: workspace.draftState,
    lastCheckpointAt: lastCheckpointAt(workspace),
    updatedAt: workspace.updatedAt,
    region: workspace.region,
    source: workspace.source,
  };
}

function loadState(): RAFState {
  const raw = window.localStorage.getItem(STORAGE_KEY);

  if (raw == null) {
    const seeded = cloneInitialRAFState();
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(seeded));
    return seeded;
  }

  try {
    return JSON.parse(raw) as RAFState;
  } catch {
    const reset = cloneInitialRAFState();
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(reset));
    return reset;
  }
}

function saveState(state: RAFState) {
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
}

function updateState(mutator: (draft: RAFState) => void) {
  const state = loadState();
  const draft = clone(state);
  mutator(draft);
  saveState(draft);
  return draft;
}

function requireWorkspace(state: RAFState, workspaceId: string) {
  const workspace = state.workspaces.find((item) => item.id === workspaceId);
  if (workspace == null) {
    throw new Error(`Workspace ${workspaceId} was not found.`);
  }
  return workspace;
}

function requireSavepoint(workspace: RAFWorkspace, savepointId: string) {
  const savepoint = workspace.savepoints.find((item) => item.id === savepointId);
  if (savepoint == null) {
    throw new Error(`Savepoint ${savepointId} was not found.`);
  }
  return savepoint;
}

function touchWorkspace(workspace: RAFWorkspace) {
  workspace.updatedAt = nowISO();
}

function sortWorkspaces(items: RAFWorkspace[]) {
  return [...items].sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));
}

const demoRAFClient: RAFClient = {
  mode: "demo",

  async listWorkspaceSummaries() {
    await wait();
    const state = loadState();
    return sortWorkspaces(state.workspaces).map(workspaceToSummary);
  },

  async listWorkspaces() {
    await wait();
    const state = loadState();
    return sortWorkspaces(clone(state.workspaces));
  },

  async getWorkspace(workspaceId: string) {
    await wait();
    const state = loadState();
    const workspace = state.workspaces.find((item) => item.id === workspaceId);
    return workspace == null ? null : clone(workspace);
  },

  async createWorkspace(input: CreateWorkspaceInput) {
    await wait();
    const state = updateState((draft) => {
      const id = slugify(input.name);
      const createdAt = nowISO();
      const baseFiles: RAFFile[] = [
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

      draft.workspaces.unshift({
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
          ),
        ],
      });
    });

    return clone(state.workspaces[0]);
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
      const file = workspace.files.find((item) => item.path === input.path);
      if (file == null) {
        workspace.files.unshift({
          path: input.path,
          language: input.path.endsWith(".md") ? "markdown" : "text",
          modifiedAt,
          content: input.content,
        });
      } else {
        file.content = input.content;
        file.modifiedAt = modifiedAt;
      }
      workspace.draftState = "dirty";
      touchWorkspace(workspace);
      workspace.activity.unshift(
        createActivity(
          `Edited ${input.path}`,
          "Updated from the Web UI editor.",
          "webui",
          "file.updated",
          "file",
        ),
      );
    });

    return clone(requireWorkspace(state, input.workspaceId));
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
      workspace.activity.unshift(
        createActivity(
          `Created savepoint ${savepoint.name}`,
          "Checkpoint captured from the Web UI.",
          "webui",
          "savepoint.created",
          "savepoint",
        ),
      );
    });

    return clone(requireWorkspace(state, input.workspaceId));
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
        ),
      );
    });

    return clone(requireWorkspace(state, input.workspaceId));
  },

  resetDemo() {
    const seeded = cloneInitialRAFState();
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

  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || `Request failed with status ${response.status}`);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

function hasHTTPBackend() {
  return HTTP_BASE_URL !== "";
}

const httpRAFClient: RAFClient = {
  mode: "http",

  async listWorkspaceSummaries() {
    const response = await requestJSON<RAFWorkspaceListResponse>("/workspaces");
    return response.items;
  },

  async listWorkspaces() {
    const summaries = await httpRAFClient.listWorkspaceSummaries();
    const results = await Promise.all(
      summaries.map((summary) => httpRAFClient.getWorkspace(summary.id)),
    );

    return results.filter((workspace): workspace is RAFWorkspaceDetail => workspace != null);
  },

  async getWorkspace(workspaceId: string) {
    try {
      return await requestJSON<RAFWorkspaceDetail>(`/workspaces/${workspaceId}`);
    } catch (error) {
      if (error instanceof Error && error.message.includes("404")) {
        return null;
      }
      throw error;
    }
  },

  async createWorkspace(input: CreateWorkspaceInput) {
    return requestJSON<RAFWorkspaceDetail>("/workspaces", {
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
    });
  },

  async deleteWorkspace(workspaceId: string) {
    await requestJSON<void>(`/workspaces/${workspaceId}`, {
      method: "DELETE",
    });
  },

  async updateWorkspaceFile(input: UpdateWorkspaceFileInput) {
    await requestJSON(`/workspaces/${input.workspaceId}/files/content`, {
      method: "PUT",
      body: JSON.stringify({
        path: input.path,
        content: input.content,
        expected_revision: input.expectedRevision,
      }),
    });

    return httpRAFClient.getWorkspace(input.workspaceId);
  },

  async createSavepoint(input: CreateSavepointInput) {
    await requestJSON(`/workspaces/${input.workspaceId}/checkpoints`, {
      method: "POST",
      body: JSON.stringify({
        name: input.name,
        description: input.note,
      }),
    });

    return httpRAFClient.getWorkspace(input.workspaceId);
  },

  async restoreSavepoint(input: RestoreSavepointInput) {
    await requestJSON(`/workspaces/${input.workspaceId}:restore`, {
      method: "POST",
      body: JSON.stringify({
        checkpoint_id: input.savepointId,
      }),
    });

    return httpRAFClient.getWorkspace(input.workspaceId);
  },

  resetDemo() {
    return demoRAFClient.resetDemo();
  },
};

export const rafApi = hasHTTPBackend() ? httpRAFClient : demoRAFClient;

export function getRAFClientMode() {
  return rafApi.mode;
}

export function formatBytes(value: number) {
  if (value >= 1024 * 1024 * 1024) {
    return `${(value / (1024 * 1024 * 1024)).toFixed(1)} GB`;
  }

  if (value >= 1024 * 1024) {
    return `${(value / (1024 * 1024)).toFixed(1)} MB`;
  }

  if (value >= 1024) {
    return `${Math.round(value / 1024)} KB`;
  }

  return `${value} B`;
}
