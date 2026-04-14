import type {
  AFSActivityEvent,
  AFSAgentSession,
  AFSFile,
  AFSSavepoint,
  AFSWorkspaceCapabilities,
  AFSState,
  AFSWorkspace,
} from "../types/afs";

function bytesCount(files: AFSFile[]) {
  return files.reduce(
    (sum, file) => sum + new TextEncoder().encode(file.content).length,
    0,
  );
}

function bytesLabel(files: AFSFile[]) {
  const totalBytes = bytesCount(files);

  if (totalBytes > 1024 * 1024) {
    return `${(totalBytes / (1024 * 1024)).toFixed(1)} MB`;
  }

  return `${Math.max(1, Math.round(totalBytes / 1024))} KB`;
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

function buildSavepoint(
  id: string,
  name: string,
  createdAt: string,
  author: string,
  note: string,
  files: AFSFile[],
): AFSSavepoint {
  return {
    id,
    name,
    author,
    createdAt,
    note,
    fileCount: files.length,
    folderCount: folderCount(files),
    totalBytes: bytesCount(files),
    sizeLabel: bytesLabel(files),
    filesSnapshot: files.map((file) => ({ ...file })),
  };
}

function buildActivity(
  id: string,
  createdAt: string,
  actor: string,
  kind: string,
  scope: string,
  title: string,
  detail: string,
): AFSActivityEvent {
  return {
    id,
    actor,
    createdAt,
    detail,
    kind,
    scope,
    title,
  };
}

function buildAgent(
  sessionId: string,
  workspaceId: string,
  workspaceName: string,
  clientKind: string,
  hostname: string,
  operatingSystem: string,
  localPath: string,
  state: string,
  startedAt: string,
  lastSeenAt: string,
  leaseExpiresAt: string,
  afsVersion = "dev",
): AFSAgentSession {
  return {
    sessionId,
    workspaceId,
    workspaceName,
    clientKind,
    afsVersion,
    hostname,
    operatingSystem,
    localPath,
    readonly: false,
    state,
    startedAt,
    lastSeenAt,
    leaseExpiresAt,
  };
}

const paymentsCurrentFiles: AFSFile[] = [
  {
    path: "README.md",
    language: "markdown",
    modifiedAt: "2026-04-03T10:40:00Z",
    content: `# Payments Workspace

This workspace is validating browser editing and checkpoint restore behavior.

- active checkpoint: before-refactor
- editor draft: dirty
- next milestone: cloud control-plane hookup`,
  },
  {
    path: "src/app.tsx",
    language: "typescript",
    modifiedAt: "2026-04-03T10:44:00Z",
    content: `export function App() {
  return {
    workspace: "payments-portal",
    mode: "editor-pass",
    checkpoint: "before-refactor"
  };
}`,
  },
  {
    path: "src/routes/editor.tsx",
    language: "typescript",
    modifiedAt: "2026-04-03T10:48:00Z",
    content: `export const editorRoute = {
  id: "payments-editor",
  state: "dirty"
};`,
  },
];

const paymentsBaselineFiles: AFSFile[] = [
  {
    path: "README.md",
    language: "markdown",
    modifiedAt: "2026-04-03T09:05:00Z",
    content: `# Payments Workspace

This workspace tracks the checkout hardening effort for Redis Cloud hosted agents.

- active checkpoint: baseline-ui
- next milestone: browser editing`,
  },
  {
    path: "src/app.tsx",
    language: "typescript",
    modifiedAt: "2026-04-03T09:12:00Z",
    content: `export function App() {
  return {
    workspace: "payments-portal",
    mode: "baseline"
  };
}`,
  },
];

const memoryFiles: AFSFile[] = [
  {
    path: "memories/customer-a.md",
    language: "markdown",
    modifiedAt: "2026-04-02T16:08:00Z",
    content: `# Customer A

- prefers weekly digests
- rollout window after 18:00 UTC`,
  },
  {
    path: "memories/customer-b.md",
    language: "markdown",
    modifiedAt: "2026-04-02T16:11:00Z",
    content: `# Customer B

- import review requested
- editor permissions pending`,
  },
];

const sandboxFiles: AFSFile[] = [
  {
    path: "sandbox.yaml",
    language: "yaml",
    modifiedAt: "2026-04-01T12:00:00Z",
    content: `runtime: docker
workspace: support-sandbox
allow:
  - shell
  - editor
  - checkpoints`,
  },
  {
    path: "scripts/bootstrap.sh",
    language: "shell",
    modifiedAt: "2026-04-01T12:03:00Z",
    content: `#!/usr/bin/env bash
set -euo pipefail
echo "Preparing support sandbox"`,
  },
];

const paymentsSavepoints = [
  buildSavepoint(
    "sp-payments-before-refactor",
    "before-refactor",
    "2026-04-03T10:36:00Z",
    "maya",
    "Captured a recovery point before the editor refactor.",
    paymentsBaselineFiles,
  ),
  buildSavepoint(
    "sp-payments-baseline-ui",
    "baseline-ui",
    "2026-04-03T09:22:00Z",
    "rafa",
    "Prepared the workspace for Redis Cloud Web UI review.",
    paymentsBaselineFiles,
  ),
];

const memorySavepoints = [
  buildSavepoint(
    "sp-memory-initial",
    "initial",
    "2026-04-02T16:15:00Z",
    "anika",
    "Imported customer memory workspace.",
    memoryFiles,
  ),
];

const sandboxSavepoints = [
  buildSavepoint(
    "sp-sandbox-initial",
    "initial",
    "2026-04-01T12:05:00Z",
    "sam",
    "Created support sandbox template.",
    sandboxFiles,
  ),
];

const paymentsWorkspace: AFSWorkspace = {
  id: "payments-portal",
  name: "payments-portal",
  description: "Checkout debugging workspace mirrored into AFS for browser editing and checkpoint recovery.",
  cloudAccount: "Redis Cloud / Customer Success",
  databaseId: "db-payments-portal",
  databaseName: "payments-portal-us-east-1",
  redisKey: "afs:payments-portal",
  region: "us-east-1",
  mountedPath: "~/.afs/workspaces/payments-portal",
  source: "git-import",
  createdAt: "2026-04-02T18:15:00Z",
  updatedAt: "2026-04-03T10:48:00Z",
  headSavepointId: "sp-payments-before-refactor",
  tags: ["production", "frontend", "cloud"],
  fileCount: paymentsCurrentFiles.length,
  folderCount: folderCount(paymentsCurrentFiles),
  totalBytes: bytesCount(paymentsCurrentFiles),
  checkpointCount: paymentsSavepoints.length,
  files: paymentsCurrentFiles,
  savepoints: paymentsSavepoints,
  capabilities: demoCapabilities(),
  activity: [
    buildActivity(
      "evt-payments-1",
      "2026-04-03T10:48:00Z",
      "maya",
      "file.updated",
      "file",
      "Edited src/routes/editor.tsx",
      "Saved draft changes inside the Web UI editor.",
    ),
    buildActivity(
      "evt-payments-2",
      "2026-04-03T10:36:00Z",
      "maya",
      "savepoint.created",
      "savepoint",
      "Created before-refactor",
      "Captured a recovery point before the refactor.",
    ),
  ],
  agents: [
    buildAgent(
      "sess-payments-sync-1",
      "payments-portal",
      "payments-portal",
      "sync",
      "maya-mbp",
      "darwin",
      "/Users/maya/workspaces/payments-portal",
      "active",
      "2026-04-03T10:18:00Z",
      "2026-04-03T10:48:00Z",
      "2026-04-03T10:49:00Z",
    ),
    buildAgent(
      "sess-payments-mcp-1",
      "payments-portal",
      "payments-portal",
      "mcp",
      "support-gateway-01",
      "linux",
      "/srv/agents/payments-portal",
      "idle",
      "2026-04-03T09:58:00Z",
      "2026-04-03T10:44:00Z",
      "2026-04-03T10:45:00Z",
    ),
  ],
};

const memoryWorkspace: AFSWorkspace = {
  id: "customer-memory",
  name: "customer-memory",
  description: "Shared memory workspace for support and solution engineering follow-ups.",
  cloudAccount: "Redis Cloud / Solutions",
  databaseId: "db-customer-memory",
  databaseName: "customer-memory-eu-west-1",
  redisKey: "afs:customer-memory",
  region: "eu-west-1",
  mountedPath: "~/.afs/workspaces/customer-memory",
  source: "cloud-import",
  createdAt: "2026-04-02T16:15:00Z",
  updatedAt: "2026-04-02T16:15:00Z",
  headSavepointId: "sp-memory-initial",
  tags: ["memory", "shared"],
  fileCount: memoryFiles.length,
  folderCount: folderCount(memoryFiles),
  totalBytes: bytesCount(memoryFiles),
  checkpointCount: memorySavepoints.length,
  files: memoryFiles,
  savepoints: memorySavepoints,
  capabilities: demoCapabilities(),
  activity: [
    buildActivity(
      "evt-memory-1",
      "2026-04-02T16:15:00Z",
      "anika",
      "workspace.imported",
      "workspace",
      "Imported customer-memory",
      "Workspace imported from a managed Redis Cloud source.",
    ),
  ],
  agents: [
    buildAgent(
      "sess-memory-sync-1",
      "customer-memory",
      "customer-memory",
      "sync",
      "anika-linux",
      "linux",
      "/home/anika/customer-memory",
      "active",
      "2026-04-02T16:05:00Z",
      "2026-04-02T16:15:00Z",
      "2026-04-02T16:16:00Z",
    ),
  ],
};

const sandboxWorkspace: AFSWorkspace = {
  id: "support-sandbox",
  name: "support-sandbox",
  description: "Blank workspace for reproducing customer issues with isolated checkpoints.",
  cloudAccount: "Redis Cloud / Support",
  databaseId: "db-support-sandbox",
  databaseName: "support-sandbox-us-central-1",
  redisKey: "afs:support-sandbox",
  region: "us-central-1",
  mountedPath: "~/.afs/workspaces/support-sandbox",
  source: "blank",
  createdAt: "2026-04-01T12:05:00Z",
  updatedAt: "2026-04-01T12:05:00Z",
  headSavepointId: "sp-sandbox-initial",
  tags: ["sandbox", "ops"],
  fileCount: sandboxFiles.length,
  folderCount: folderCount(sandboxFiles),
  totalBytes: bytesCount(sandboxFiles),
  checkpointCount: sandboxSavepoints.length,
  files: sandboxFiles,
  savepoints: sandboxSavepoints,
  capabilities: demoCapabilities(),
  activity: [
    buildActivity(
      "evt-sandbox-1",
      "2026-04-01T12:05:00Z",
      "sam",
      "workspace.created",
      "workspace",
      "Created support-sandbox",
      "Blank workspace provisioned for support workflows.",
    ),
  ],
  agents: [],
};

export const initialAFSState: AFSState = {
  workspaces: [paymentsWorkspace, memoryWorkspace, sandboxWorkspace],
};

export function cloneInitialAFSState() {
  return JSON.parse(JSON.stringify(initialAFSState)) as AFSState;
}
