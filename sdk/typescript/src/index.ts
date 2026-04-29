import { spawn } from "node:child_process";
import { constants as fsConstants } from "node:fs";
import {
  access,
  lstat,
  mkdir,
  mkdtemp,
  readFile as nodeReadFile,
  readdir,
  rm,
  symlink,
  writeFile as nodeWriteFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import nodePath from "node:path";
import posixPath from "node:path/posix";

const DEFAULT_BASE_URL = "https://afs.cloud";

export type FetchLike = (input: string | URL, init?: RequestInit) => Promise<Response>;

export type AFSOptions = {
  apiKey?: string;
  baseUrl?: string;
  fetch?: FetchLike;
  timeoutMs?: number;
  headers?: Record<string, string>;
};

export type RepoRef = {
  name: string;
};

export type MountMode = "ro" | "rw" | "rw-checkpoint";

export type MountInput = {
  repos: RepoRef[];
  mode?: MountMode;
  tokenName?: string;
};

export type CreateRepoInput = {
  name: string;
  description?: string;
  templateSlug?: string;
};

export type ForkRepoInput = {
  source: string;
  name: string;
};

export type CheckpointInput = {
  repo: string;
  checkpoint?: string;
};

export type RestoreCheckpointInput = {
  repo: string;
  checkpoint: string;
};

export type BashExecOptions = {
  cwd?: string;
  env?: Record<string, string | undefined>;
  timeoutMs?: number;
};

export type BashResult = {
  stdout: string;
  stderr: string;
  exitCode: number | null;
  signal: NodeJS.Signals | null;
  command: string;
  mappedCommand: string;
};

export type MCPToolResult<T> = {
  content?: Array<{ type: string; text: string }>;
  structuredContent?: T;
  isError?: boolean;
};

export type Repo = {
  id?: string;
  name: string;
  description?: string;
  database_id?: string;
  database_name?: string;
  template_slug?: string;
  [key: string]: unknown;
};

export type Checkpoint = {
  id: string;
  name: string;
  created_at?: string;
  file_count?: number;
  folder_count?: number;
  total_bytes?: number;
  is_head?: boolean;
  [key: string]: unknown;
};

export type MCPTokenIssueResponse = {
  url?: string;
  token: string;
  server_name?: string;
  workspace: string;
  scope?: string;
  profile?: string;
  expires_at?: string;
};

export type FileListItem = {
  path: string;
  name: string;
  kind: "file" | "dir" | "symlink" | string;
  size?: number;
  modified_at?: string;
  target?: string;
};

export type FileReadResponse = {
  workspace?: string;
  path: string;
  kind: "file" | "dir" | "symlink" | string;
  content?: string;
  size?: number;
  binary?: boolean;
  target?: string;
};

export class AFSError extends Error {
  readonly status?: number;
  readonly code?: number;
  readonly payload?: unknown;

  constructor(message: string, options: { status?: number; code?: number; payload?: unknown } = {}) {
    super(message);
    this.name = "AFSError";
    this.status = options.status;
    this.code = options.code;
    this.payload = options.payload;
  }
}

export class AFS {
  readonly repo: RepoClient;
  readonly repos: RepoClient;
  readonly checkpoint: CheckpointClient;
  readonly checkpoints: CheckpointClient;
  readonly fs: FSClient;

  private readonly controlPlane: MCPHttpClient;

  constructor(options: AFSOptions = {}) {
    this.controlPlane = new MCPHttpClient(options);
    this.repo = new RepoClient(this.controlPlane);
    this.repos = this.repo;
    this.checkpoint = new CheckpointClient(this.controlPlane);
    this.checkpoints = this.checkpoint;
    this.fs = new FSClient(this.controlPlane);
  }

  async callTool<T = unknown>(name: string, args: Record<string, unknown> = {}): Promise<T> {
    return this.controlPlane.callTool<T>(name, args);
  }
}

export class RepoClient {
  constructor(private readonly mcp: MCPHttpClient) {}

  async create(input: CreateRepoInput): Promise<Repo> {
    return this.mcp.callTool<Repo>("workspace_create", {
      name: input.name,
      description: input.description,
      template_slug: input.templateSlug,
    });
  }

  async list(): Promise<Repo[]> {
    const response = await this.mcp.callTool<{ items?: Repo[] } | Repo[]>("workspace_list");
    return Array.isArray(response) ? response : response.items ?? [];
  }

  async get(repo: string | RepoRef): Promise<Repo> {
    return this.mcp.callTool<Repo>("workspace_get", {
      workspace: typeof repo === "string" ? repo : repo.name,
    });
  }

  async fork(input: ForkRepoInput): Promise<{ source: string; workspace: string; created: boolean }> {
    return this.mcp.callTool("workspace_fork", {
      source: input.source,
      name: input.name,
    });
  }

  async delete(repo: string | RepoRef): Promise<{ workspace: string; deleted: boolean }> {
    return this.mcp.callTool("workspace_delete", {
      workspace: typeof repo === "string" ? repo : repo.name,
    });
  }
}

export class CheckpointClient {
  constructor(private readonly mcp: MCPHttpClient) {}

  async list(repo: string | RepoRef): Promise<Checkpoint[]> {
    const response = await this.mcp.callTool<{ checkpoints?: Checkpoint[] }>("checkpoint_list", {
      workspace: typeof repo === "string" ? repo : repo.name,
    });
    return response.checkpoints ?? [];
  }

  async create(input: CheckpointInput): Promise<{ workspace: string; checkpoint: string; created: boolean }> {
    return this.mcp.callTool("checkpoint_create", {
      workspace: input.repo,
      checkpoint: input.checkpoint,
    });
  }

  async restore(input: RestoreCheckpointInput): Promise<{ workspace: string; checkpoint: string; restored: boolean }> {
    return this.mcp.callTool("checkpoint_restore", {
      workspace: input.repo,
      checkpoint: input.checkpoint,
    });
  }
}

export class FSClient {
  constructor(private readonly controlPlane: MCPHttpClient) {}

  async mount(input: MountInput): Promise<MountedFS> {
    if (!input.repos.length) {
      throw new AFSError("fs.mount requires at least one repo");
    }
    const profile = profileForMode(input.mode ?? "rw");
    const mounted: MountedRepo[] = [];
    for (const repo of input.repos) {
      const issued = await this.controlPlane.callTool<MCPTokenIssueResponse>("mcp_token_issue", {
        workspace: repo.name,
        name: input.tokenName ?? `redis-afs-sdk ${repo.name}`,
        profile,
      });
      if (!issued.token) {
        throw new AFSError(`mcp_token_issue did not return a token for ${repo.name}`, { payload: issued });
      }
      mounted.push({
        name: repo.name,
        token: issued.token,
        client: new MCPHttpClient({
          apiKey: issued.token,
          baseUrl: issued.url ?? this.controlPlane.endpoint,
          fetch: this.controlPlane.fetchImpl,
          timeoutMs: this.controlPlane.timeoutMs,
        }),
      });
    }
    return new MountedFS(mounted, { mode: input.mode ?? "rw" });
  }
}

type MountedRepo = {
  name: string;
  token: string;
  client: MCPHttpClient;
};

type ResolvedMountPath = {
  repo: MountedRepo;
  remotePath: string;
};

export class MountedFS {
  private readonly reposByName = new Map<string, MountedRepo>();
  private localRootPath?: string;

  constructor(
    private readonly repos: MountedRepo[],
    readonly options: { mode: MountMode },
  ) {
    for (const repo of repos) {
      if (this.reposByName.has(repo.name)) {
        throw new AFSError(`repo ${repo.name} is mounted more than once`);
      }
      this.reposByName.set(repo.name, repo);
    }
  }

  get repoNames(): string[] {
    return this.repos.map((repo) => repo.name);
  }

  get localRoot(): string | undefined {
    return this.localRootPath;
  }

  async readFile(path: string): Promise<string> {
    const resolved = this.resolvePath(path);
    const response = await resolved.repo.client.callTool<FileReadResponse>("file_read", {
      path: resolved.remotePath,
    });
    if (response.binary) {
      throw new AFSError(`file ${resolved.remotePath} is binary and cannot be returned as text`);
    }
    if (response.kind === "dir") {
      throw new AFSError(`path ${resolved.remotePath} is a directory`);
    }
    return response.content ?? "";
  }

  async writeFile(path: string, content: string | Uint8Array): Promise<void> {
    const resolved = this.resolvePath(path);
    const text = typeof content === "string" ? content : Buffer.from(content).toString("utf8");
    await resolved.repo.client.callTool("file_write", {
      path: resolved.remotePath,
      content: text,
    });
    if (this.localRootPath) {
      const localPath = this.localPathFor(resolved.repo.name, resolved.remotePath);
      await mkdir(nodePath.dirname(localPath), { recursive: true });
      await nodeWriteFile(localPath, text, "utf8");
    }
  }

  async listFiles(path = "/", depth = 1): Promise<FileListItem[]> {
    const resolved = this.resolvePath(path);
    const response = await resolved.repo.client.callTool<{ entries?: FileListItem[] }>("file_list", {
      path: resolved.remotePath,
      depth,
    });
    return response.entries ?? [];
  }

  async glob(pattern: string, options: { path?: string; kind?: "file" | "dir" | "symlink" | "any"; limit?: number } = {}) {
    const resolved = this.resolvePath(options.path ?? "/");
    return resolved.repo.client.callTool("file_glob", {
      path: resolved.remotePath,
      pattern,
      kind: options.kind,
      limit: options.limit,
    });
  }

  async grep(pattern: string, options: Record<string, unknown> = {}) {
    const resolved = this.resolvePath(String(options.path ?? "/"));
    return resolved.repo.client.callTool("file_grep", {
      ...options,
      path: resolved.remotePath,
      pattern,
    });
  }

  async checkpoint(name?: string): Promise<{ workspace: string; checkpoint: string; created: boolean }[]> {
    const out = [];
    for (const repo of this.repos) {
      out.push(
        await repo.client.callTool<{ workspace: string; checkpoint: string; created: boolean }>("checkpoint_create", {
          checkpoint: name,
        }),
      );
    }
    return out;
  }

  bash(): BashRunner {
    return new BashRunner(this);
  }

  async syncFromRemote(): Promise<string> {
    const root = await this.ensureLocalRoot();
    for (const repo of this.repos) {
      const repoRoot = nodePath.join(root, repo.name);
      await rm(repoRoot, { recursive: true, force: true });
      await mkdir(repoRoot, { recursive: true });
      await this.copyRemoteDirectory(repo, "/", repoRoot);
    }
    return root;
  }

  async syncToRemote(): Promise<void> {
    if (!this.localRootPath) {
      return;
    }
    for (const repo of this.repos) {
      const repoRoot = nodePath.join(this.localRootPath, repo.name);
      if (!(await exists(repoRoot))) {
        continue;
      }
      await this.copyLocalDirectory(repo, repoRoot, "/");
    }
  }

  async close(): Promise<void> {
    if (!this.localRootPath) {
      return;
    }
    await rm(this.localRootPath, { recursive: true, force: true });
    this.localRootPath = undefined;
  }

  mapAbsoluteRepoPaths(command: string): string {
    if (!this.localRootPath) {
      return command;
    }
    let out = command;
    const names = this.repoNames.sort((a, b) => b.length - a.length);
    for (const name of names) {
      const remotePrefix = `/${name}`;
      const localPrefix = nodePath.join(this.localRootPath, name).replaceAll("\\", "/");
      out = out.replace(new RegExp(`${escapeRegExp(remotePrefix)}(?=/|\\s|$)`, "g"), localPrefix);
    }
    return out;
  }

  private resolvePath(rawPath: string): ResolvedMountPath {
    const normalized = normalizeRemotePath(rawPath);
    const names = this.repoNames.sort((a, b) => b.length - a.length);
    for (const name of names) {
      const prefix = `/${name}`;
      if (normalized === prefix) {
        return { repo: this.reposByName.get(name)!, remotePath: "/" };
      }
      if (normalized.startsWith(`${prefix}/`)) {
        return {
          repo: this.reposByName.get(name)!,
          remotePath: normalized.slice(prefix.length) || "/",
        };
      }
    }
    if (this.repos.length === 1) {
      return { repo: this.repos[0]!, remotePath: normalized };
    }
    throw new AFSError(`path ${rawPath} must start with one of: ${names.map((name) => `/${name}`).join(", ")}`);
  }

  private async ensureLocalRoot(): Promise<string> {
    if (!this.localRootPath) {
      this.localRootPath = await mkdtemp(nodePath.join(tmpdir(), "afs-fs-"));
    }
    return this.localRootPath;
  }

  private localPathFor(repoName: string, remotePath: string): string {
    if (!this.localRootPath) {
      throw new AFSError("mount has not been materialized locally yet");
    }
    const relative = normalizeRemotePath(remotePath).replace(/^\/+/, "");
    return nodePath.join(this.localRootPath, repoName, relative);
  }

  private async copyRemoteDirectory(repo: MountedRepo, remotePath: string, localPath: string): Promise<void> {
    const response = await repo.client.callTool<{ entries?: FileListItem[] }>("file_list", {
      path: remotePath,
      depth: 1,
    });
    for (const entry of response.entries ?? []) {
      const target = nodePath.join(localPath, entry.name);
      if (entry.kind === "dir") {
        await mkdir(target, { recursive: true });
        await this.copyRemoteDirectory(repo, entry.path, target);
      } else if (entry.kind === "symlink" && entry.target) {
        await symlink(entry.target, target).catch(async () => undefined);
      } else if (entry.kind === "file") {
        const file = await repo.client.callTool<FileReadResponse>("file_read", { path: entry.path });
        if (!file.binary) {
          await mkdir(nodePath.dirname(target), { recursive: true });
          await nodeWriteFile(target, file.content ?? "", "utf8");
        }
      }
    }
  }

  private async copyLocalDirectory(repo: MountedRepo, localDirectory: string, remoteDirectory: string): Promise<void> {
    const entries = await readdir(localDirectory, { withFileTypes: true });
    for (const entry of entries) {
      const localPath = nodePath.join(localDirectory, entry.name);
      const remotePath = posixPath.join(remoteDirectory, entry.name);
      if (entry.isDirectory()) {
        await this.copyLocalDirectory(repo, localPath, remotePath);
      } else if (entry.isFile()) {
        const content = await nodeReadFile(localPath, "utf8");
        await repo.client.callTool("file_write", {
          path: normalizeRemotePath(remotePath),
          content,
        });
      }
    }
  }
}

export class BashRunner {
  constructor(private readonly fs: MountedFS) {}

  async exec(command: string, options: BashExecOptions = {}): Promise<BashResult> {
    const root = await this.fs.syncFromRemote();
    const mappedCommand = this.fs.mapAbsoluteRepoPaths(command);
    const result = await runShell(mappedCommand, {
      cwd: options.cwd ? nodePath.resolve(root, options.cwd) : root,
      env: options.env,
      timeoutMs: options.timeoutMs,
    });
    await this.fs.syncToRemote();
    return {
      ...result,
      command,
      mappedCommand,
    };
  }
}

export class MCPHttpClient {
  readonly endpoint: string;
  readonly timeoutMs: number;
  readonly fetchImpl: FetchLike;

  private readonly apiKey: string;
  private readonly headers: Record<string, string>;
  private nextId = 1;

  constructor(options: AFSOptions = {}) {
    this.apiKey = options.apiKey ?? readEnv("AFS_API_KEY") ?? "";
    if (!this.apiKey) {
      throw new AFSError("AFS apiKey is required");
    }
    const baseUrl = options.baseUrl ?? readEnv("AFS_API_BASE_URL") ?? DEFAULT_BASE_URL;
    this.endpoint = normalizeMCPEndpoint(baseUrl);
    this.timeoutMs = options.timeoutMs ?? 30_000;
    this.fetchImpl = options.fetch ?? globalThis.fetch?.bind(globalThis);
    if (!this.fetchImpl) {
      throw new AFSError("global fetch is unavailable; provide a fetch implementation");
    }
    this.headers = options.headers ?? {};
  }

  async callTool<T = unknown>(name: string, args: Record<string, unknown> = {}): Promise<T> {
    const response = await this.request<MCPToolResult<T>>("tools/call", {
      name,
      arguments: stripUndefined(args),
    });
    if (response.isError) {
      throw new AFSError(response.content?.map((item) => item.text).join("\n") || `MCP tool ${name} failed`, {
        payload: response,
      });
    }
    return (response.structuredContent ?? (response as T)) as T;
  }

  async request<T = unknown>(method: string, params: Record<string, unknown> = {}): Promise<T> {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), this.timeoutMs);
    try {
      const httpResponse = await this.fetchImpl(this.endpoint, {
        method: "POST",
        headers: {
          "content-type": "application/json",
          authorization: `Bearer ${this.apiKey}`,
          ...this.headers,
        },
        body: JSON.stringify({
          jsonrpc: "2.0",
          id: this.nextId++,
          method,
          params,
        }),
        signal: controller.signal,
      });
      const text = await httpResponse.text();
      if (!httpResponse.ok) {
        throw new AFSError(`MCP request failed with HTTP ${httpResponse.status}: ${text}`, {
          status: httpResponse.status,
          payload: text,
        });
      }
      const payload = text ? (JSON.parse(text) as { result?: T; error?: { code: number; message: string } }) : {};
      if (payload.error) {
        throw new AFSError(payload.error.message, { code: payload.error.code, payload });
      }
      return payload.result as T;
    } catch (error) {
      if (error instanceof AFSError) {
        throw error;
      }
      if (error instanceof Error && error.name === "AbortError") {
        throw new AFSError(`MCP request timed out after ${this.timeoutMs}ms`);
      }
      throw error;
    } finally {
      clearTimeout(timeout);
    }
  }
}

function profileForMode(mode: MountMode): string {
  switch (mode) {
    case "ro":
      return "workspace-ro";
    case "rw-checkpoint":
      return "workspace-rw-checkpoint";
    case "rw":
      return "workspace-rw";
  }
}

function normalizeMCPEndpoint(baseUrl: string): string {
  const trimmed = baseUrl.trim().replace(/\/+$/, "");
  if (!trimmed) {
    throw new AFSError("baseUrl is required");
  }
  return trimmed.endsWith("/mcp") ? trimmed : `${trimmed}/mcp`;
}

function normalizeRemotePath(input: string): string {
  const raw = input.trim();
  if (!raw) {
    return "/";
  }
  const parts = raw.split("/").filter(Boolean);
  if (parts.includes("..")) {
    throw new AFSError(`path ${input} must not contain '..'`);
  }
  const normalized = posixPath.normalize(raw.startsWith("/") ? raw : `/${raw}`);
  return normalized === "." ? "/" : normalized;
}

function stripUndefined(input: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(Object.entries(input).filter(([, value]) => value !== undefined));
}

function readEnv(name: string): string | undefined {
  if (typeof process === "undefined") {
    return undefined;
  }
  return process.env[name];
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

async function exists(path: string): Promise<boolean> {
  try {
    await access(path, fsConstants.F_OK);
    return true;
  } catch {
    return false;
  }
}

async function runShell(
  command: string,
  options: { cwd: string; env?: Record<string, string | undefined>; timeoutMs?: number },
): Promise<Omit<BashResult, "command" | "mappedCommand">> {
  return new Promise((resolve, reject) => {
    const child = spawn("/bin/bash", ["-lc", command], {
      cwd: options.cwd,
      env: { ...process.env, ...options.env },
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    let timeout: NodeJS.Timeout | undefined;
    if (options.timeoutMs) {
      timeout = setTimeout(() => child.kill("SIGTERM"), options.timeoutMs);
    }
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.on("error", reject);
    child.on("close", (exitCode, signal) => {
      if (timeout) {
        clearTimeout(timeout);
      }
      resolve({ stdout, stderr, exitCode, signal });
    });
  });
}

export const _testing = {
  normalizeMCPEndpoint,
  normalizeRemotePath,
};
