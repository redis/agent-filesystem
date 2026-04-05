import { cloneInitialRAFState } from "../mocks/raf";
import type {
  CreateSavepointInput,
  CreateSessionInput,
  CreateWorkspaceInput,
  RAFActivityEvent,
  RAFClientMode,
  RAFFile,
  RAFSavepoint,
  RAFSession,
  RAFState,
  RAFWorkspace,
  RAFWorkspaceDetail,
  RAFWorkspaceListResponse,
  RAFWorkspaceSummary,
  RollbackSessionInput,
  UpdateSessionFileInput,
} from "../types/raf";

const STORAGE_KEY = "raf-ui-demo-state-v1";
const DEMO_DELAY_MS = 120;
const HTTP_BASE_URL = import.meta.env.VITE_RAF_API_BASE_URL?.replace(/\/+$/, "") ?? "";

type RAFClient = {
  mode: RAFClientMode;
  listWorkspaceSummaries: () => Promise<RAFWorkspaceSummary[]>;
  listWorkspaces: () => Promise<RAFWorkspaceDetail[]>;
  getWorkspace: (workspaceId: string) => Promise<RAFWorkspaceDetail | null>;
  createWorkspace: (input: CreateWorkspaceInput) => Promise<RAFWorkspaceDetail>;
  deleteWorkspace: (workspaceId: string) => Promise<void>;
  createSession: (input: CreateSessionInput) => Promise<RAFSession>;
  deleteSession: (workspaceId: string, sessionId: string) => Promise<void>;
  updateSessionFile: (input: UpdateSessionFileInput) => Promise<RAFWorkspaceDetail | null>;
  createSavepoint: (input: CreateSavepointInput) => Promise<RAFWorkspaceDetail | null>;
  rollbackSession: (input: RollbackSessionInput) => Promise<RAFWorkspaceDetail | null>;
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
  const values = workspace.sessions.flatMap((session) =>
    session.savepoints.map((savepoint) => savepoint.createdAt),
  );

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
  const canonicalSession =
    workspace.sessions.find((session) => session.id === workspace.defaultSessionId) ??
    workspace.sessions[0];
  const files = canonicalSession.files;

  return {
    id: workspace.id,
    name: workspace.name,
    databaseId: workspace.databaseId,
    databaseName: workspace.databaseName,
    redisKey: workspace.redisKey,
    status: workspace.status,
    fileCount: files.length,
    folderCount: folderCount(files),
    totalBytes: bytesCount(files),
    sessionCount: workspace.sessions.length,
    forkCount: workspace.sessions.filter((session) => session.kind === "branch").length,
    checkpointCount: workspace.sessions.reduce(
      (sum, session) => sum + session.savepoints.length,
      0,
    ),
    dirtySessionCount: workspace.sessions.filter((session) => session.status === "dirty").length,
    defaultSessionId: workspace.defaultSessionId,
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

function requireSession(workspace: RAFWorkspace, sessionId: string) {
  const session = workspace.sessions.find((item) => item.id === sessionId);
  if (session == null) {
    throw new Error(`Session ${sessionId} was not found.`);
  }
  return session;
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

This workspace was created from the RAF Web UI.

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
      const mainSession: RAFSession = {
        id: makeId("session"),
        name: "main",
        description: "Default session for the workspace.",
        author: "webui",
        createdAt,
        updatedAt: createdAt,
        lastRunAt: createdAt,
        status: "clean",
        kind: "main",
        headSavepointId: initialSavepoint.id,
        files: baseFiles,
        savepoints: [initialSavepoint],
      };

      draft.workspaces.unshift({
        id,
        name: input.name.trim(),
        description: input.description.trim(),
        cloudAccount: input.cloudAccount.trim(),
        databaseId: `db-${id}`,
        databaseName: input.databaseName.trim(),
        redisKey: `raf:${id}`,
        region: input.region.trim(),
        mountedPath: `~/.raf/workspaces/${id}`,
        status: input.source === "blank" ? "healthy" : "syncing",
        source: input.source,
        createdAt,
        updatedAt: createdAt,
        defaultSessionId: mainSession.id,
        tags: [input.region.trim(), sourceLabel(input.source)],
        sessions: [mainSession],
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

  async createSession(input: CreateSessionInput) {
    await wait();
    const state = updateState((draft) => {
      const workspace = requireWorkspace(draft, input.workspaceId);
      const sourceSession =
        input.baseSessionId != null
          ? requireSession(workspace, input.baseSessionId)
          : requireSession(workspace, workspace.defaultSessionId);
      const files = clone(sourceSession.files);
      const createdAt = nowISO();
      const initialSavepoint = createSavepointRecord(
        input.mode === "imported" ? "imported" : "forked",
        input.mode === "imported"
          ? "Session imported into RAF from an external source."
          : `Session branched from ${sourceSession.name}.`,
        "webui",
        files,
      );
      const session: RAFSession = {
        id: makeId("session"),
        name: input.name.trim(),
        description: input.description.trim(),
        author: "webui",
        createdAt,
        updatedAt: createdAt,
        lastRunAt: createdAt,
        status: "clean",
        kind: input.mode,
        headSavepointId: initialSavepoint.id,
        files,
        savepoints: [initialSavepoint],
      };
      workspace.sessions.unshift(session);
      touchWorkspace(workspace);
      workspace.activity.unshift(
        createActivity(
          input.mode === "imported"
            ? `Imported session ${input.name.trim()}`
            : `Created session ${input.name.trim()}`,
          `Session seeded from ${sourceSession.name}.`,
          "webui",
          input.mode === "imported" ? "session.imported" : "session.created",
          "session",
        ),
      );
    });

    const workspace = requireWorkspace(state, input.workspaceId);
    return clone(workspace.sessions[0]);
  },

  async deleteSession(workspaceId: string, sessionId: string) {
    await wait();
    updateState((draft) => {
      const workspace = requireWorkspace(draft, workspaceId);
      const session = requireSession(workspace, sessionId);
      workspace.sessions = workspace.sessions.filter((item) => item.id !== sessionId);
      if (workspace.defaultSessionId === sessionId) {
        workspace.defaultSessionId = workspace.sessions[0]?.id ?? "";
      }
      touchWorkspace(workspace);
      workspace.activity.unshift(
        createActivity(
          `Deleted session ${session.name}`,
          "Session removed from the workspace catalog.",
          "webui",
          "session.deleted",
          "session",
        ),
      );
    });
  },

  async updateSessionFile(input: UpdateSessionFileInput) {
    await wait();
    const state = updateState((draft) => {
      const workspace = requireWorkspace(draft, input.workspaceId);
      const session = requireSession(workspace, input.sessionId);
      const modifiedAt = nowISO();
      const file = session.files.find((item) => item.path === input.path);
      if (file == null) {
        session.files.unshift({
          path: input.path,
          language: input.path.endsWith(".md") ? "markdown" : "text",
          modifiedAt,
          content: input.content,
        });
      } else {
        file.content = input.content;
        file.modifiedAt = modifiedAt;
      }
      session.updatedAt = modifiedAt;
      session.status = "dirty";
      touchWorkspace(workspace);
      workspace.activity.unshift(
        createActivity(
          `Edited ${input.path}`,
          `Updated in session ${session.name}.`,
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
      const session = requireSession(workspace, input.sessionId);
      const savepoint = createSavepointRecord(
        input.name.trim(),
        input.note.trim(),
        "webui",
        session.files,
      );
      session.savepoints.unshift(savepoint);
      session.headSavepointId = savepoint.id;
      session.status = "clean";
      session.updatedAt = savepoint.createdAt;
      touchWorkspace(workspace);
      workspace.activity.unshift(
        createActivity(
          `Created savepoint ${savepoint.name}`,
          `Session ${session.name} checkpointed from the Web UI.`,
          "webui",
          "savepoint.created",
          "savepoint",
        ),
      );
    });

    return clone(requireWorkspace(state, input.workspaceId));
  },

  async rollbackSession(input: RollbackSessionInput) {
    await wait();
    const state = updateState((draft) => {
      const workspace = requireWorkspace(draft, input.workspaceId);
      const session = requireSession(workspace, input.sessionId);
      const savepoint = session.savepoints.find((item) => item.id === input.savepointId);
      if (savepoint == null) {
        throw new Error("Savepoint was not found.");
      }
      session.files = clone(savepoint.filesSnapshot);
      session.headSavepointId = savepoint.id;
      session.status = "clean";
      session.updatedAt = nowISO();
      touchWorkspace(workspace);
      workspace.activity.unshift(
        createActivity(
          `Rolled back ${session.name}`,
          `Restored session files to ${savepoint.name}.`,
          "webui",
          "session.rollback",
          "session",
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

  async createSession(input: CreateSessionInput) {
    return requestJSON<RAFSession>(`/workspaces/${input.workspaceId}/sessions`, {
      method: "POST",
      body: JSON.stringify({
        name: input.name,
        description: input.description,
        mode: input.mode === "branch" ? "fork" : input.mode,
        source_session_id: input.baseSessionId,
      }),
    });
  },

  async deleteSession(workspaceId: string, sessionId: string) {
    await requestJSON<void>(`/workspaces/${workspaceId}/sessions/${sessionId}`, {
      method: "DELETE",
    });
  },

  async updateSessionFile(input: UpdateSessionFileInput) {
    await requestJSON(`/workspaces/${input.workspaceId}/files/content`, {
      method: "PUT",
      body: JSON.stringify({
        session_id: input.sessionId,
        path: input.path,
        content: input.content,
        expected_revision: input.expectedRevision,
      }),
    });

    return httpRAFClient.getWorkspace(input.workspaceId);
  },

  async createSavepoint(input: CreateSavepointInput) {
    await requestJSON(
      `/workspaces/${input.workspaceId}/sessions/${input.sessionId}/checkpoints`,
      {
        method: "POST",
        body: JSON.stringify({
          name: input.name,
          description: input.note,
        }),
      },
    );

    return httpRAFClient.getWorkspace(input.workspaceId);
  },

  async rollbackSession(input: RollbackSessionInput) {
    await requestJSON(
      `/workspaces/${input.workspaceId}/sessions/${input.sessionId}:rollback`,
      {
        method: "POST",
        body: JSON.stringify({
          checkpoint_id: input.savepointId,
        }),
      },
    );

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
